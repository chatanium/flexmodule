//go:build voice && cgo

package discordgo

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func voiceHandoffGateway(t *testing.T, s *Session) <-chan voiceWireFixture {
	t.Helper()
	wires := make(chan voiceWireFixture, 8)
	serverConn := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConn <- c
		for {
			kind, data, err := c.ReadMessage()
			if err != nil {
				return
			}
			wires <- voiceWireFixture{kind, data}
		}
	}))
	t.Cleanup(srv.Close)
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	peer := <-serverConn
	t.Cleanup(func() { peer.Close(); ws.Close() })
	s.wsConn = ws
	return wires
}

func TestVoiceRetirementWaitsForOrderedGatewayLeaveBeforeReplacement(t *testing.T) {
	n, _ := nativeTransportFixture(t)
	s := n.v.session
	s.VoiceConnections = map[string]*VoiceConnection{"1": n.v}
	wires := voiceHandoffGateway(t, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- n.v.DisconnectContext(ctx) }()
	leave := nextVoiceWire(t, wires)
	if !bytes.Contains(leave.data, []byte(`"channel_id":null`)) {
		t.Fatal("retirement did not send main Gateway leave")
	}
	// End the caller's wait only after observing the leave write, but before
	// delivering its acknowledgement. No timer determines the event ordering.
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("disconnect completed before the Gateway confirmed its leave")
		}
	case <-time.After(time.Second):
		t.Fatal("disconnect did not respect its caller cancellation")
	}
	s.RLock()
	retained := s.VoiceConnections["1"] == n.v
	s.RUnlock()
	if !retained {
		t.Fatal("unacknowledged leave exposed the guild to a replacement join")
	}
	// The real ingress calls the SDK handler and then the host observer. A
	// retirement acknowledgement must be consumed by the same old generation
	// on both deliveries, not reported as an external move to the host.
	observed := false
	s.SetDispatchObserver(func(s *Session, e *Event, err error) {
		observed = true
		if err != nil || s.VoiceGatewayDispatch(e) != nil {
			t.Error("owned retirement acknowledgement failed host dispatch")
		}
		s.RLock()
		owned := s.VoiceConnections["1"] == n.v
		s.RUnlock()
		if !owned {
			t.Error("reservation released before ordered host observation finished")
		}
	})
	t.Cleanup(func() { s.SetDispatchObserver(nil) })
	_, err := s.onGatewayEvent(&Event{Operation: 0, Type: "VOICE_STATE_UPDATE", Sequence: 1, RawData: []byte(`{"guild_id":"1","channel_id":null,"user_id":"2","session_id":"old"}`)})
	if err != nil || !observed {
		t.Fatal("leave acknowledgement did not traverse ordered Gateway ingress")
	}
	if err := n.v.DisconnectContext(context.Background()); err != nil {
		t.Fatal("confirmed retirement could not finish idempotently")
	}
	select {
	case <-wires:
		t.Fatal("repeat disconnect sent another leave that could hit a replacement")
	default:
	}
	s.RLock()
	old := s.VoiceConnections["1"]
	s.RUnlock()
	if old != nil {
		t.Fatal("confirmed retirement retained its guild reservation")
	}
	replacement, _ := nativeTransportFixture(t)
	replacement.v.session = s
	s.Lock()
	s.VoiceConnections["1"] = replacement.v
	s.Unlock()
	if err := n.v.DisconnectContext(context.Background()); err != nil {
		t.Fatal("stale retirement attempted to disconnect the replacement")
	}
	if err := s.acceptVoiceState(&VoiceStateUpdate{VoiceState: &VoiceState{GuildID: "1", ChannelID: "1", UserID: "2", SessionID: "new"}}); err != nil || !replacement.ready() {
		t.Fatal("new generation could not accept its own channel/session state")
	}
}

func TestVoiceUnsolicitedGatewayChangesStillFailClosed(t *testing.T) {
	for _, change := range []struct {
		name, channel, session string
	}{
		{"kick", "", "original"},
		{"move", "3", "original"},
		{"session", "1", "replacement"},
	} {
		t.Run(change.name, func(t *testing.T) {
			n, _ := nativeTransportFixture(t)
			n.v.sessionID = "original"
			n.v.session.VoiceConnections = map[string]*VoiceConnection{"1": n.v}
			err := n.v.session.acceptVoiceState(&VoiceStateUpdate{VoiceState: &VoiceState{GuildID: "1", ChannelID: change.channel, UserID: "2", SessionID: change.session}})
			if !errors.Is(err, ErrVoiceChannelChanged) || !errors.Is(n.err(), ErrVoiceChannelChanged) || n.ready() {
				t.Fatal("unsolicited channel change was treated as owned retirement")
			}
		})
	}
}
