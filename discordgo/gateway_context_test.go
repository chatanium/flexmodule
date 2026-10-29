package discordgo

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestGatewayContextAdmissionCancellation(t *testing.T) {
	s, err := New("Bot host-only")
	if err != nil {
		t.Fatal(err)
	}
	s.wsMutex.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.UpdateStatusComplexContext(ctx, UpdateStatusData{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("writer admission: %v", err)
	}
	s.wsMutex.Unlock()
	s.wsLifecycle.Lock()
	if err := s.OpenContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("open admission: %v", err)
	}
	s.wsLifecycle.Unlock()
	if err := s.UpdateStatusComplexContext(context.Background(), UpdateStatusData{}); !errors.Is(err, ErrWSNotFound) {
		t.Fatalf("missing socket: %v", err)
	}
}

func TestGatewayOpenContextAbortsHandshake(t *testing.T) {
	exited := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer close(exited)
		defer conn.Close()
		// Deliberately withhold Hello. Cancellation must close actual I/O.
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	s, err := New("Bot host-only")
	if err != nil {
		t.Fatal(err)
	}
	s.Dialer = &websocket.Dialer{TLSClientConfig: &tls.Config{RootCAs: roots}}
	s.gateway = "wss" + strings.TrimPrefix(server.URL, "https") + "/"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.OpenContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("handshake cancellation: %v", err)
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("canceled handshake left server socket alive")
	}
	if s.wsConn != nil {
		t.Fatal("failed handshake published a connection")
	}
}

func TestGatewayURLNormalizationAndSecurity(t *testing.T) {
	target, err := gatewayURL("wss://gateway.discord.gg/?v=9&encoding=etf")
	if err != nil || target != "wss://gateway.discord.gg/?encoding=json&v=10" {
		t.Fatalf("Gateway version: %q %v", target, err)
	}
	for _, insecure := range []string{"ws://gateway.discord.gg/", "https://gateway.discord.gg/", "wss://user:secret@gateway.discord.gg/"} {
		if _, err := gatewayURL(insecure); err == nil {
			t.Fatal("accepted insecure Gateway URL")
		}
	}
}

func TestGatewayObserverRawOrderingAndDetachedBefore(t *testing.T) {
	s, err := New("Bot host-only")
	if err != nil {
		t.Fatal(err)
	}
	s.SyncEvents = true
	s.State.MaxMessageCount = 5
	if err := s.State.GuildAdd(&Guild{ID: "g"}); err != nil {
		t.Fatal(err)
	}
	if err := s.State.ChannelAdd(&Channel{ID: "c", GuildID: "g", Members: []*ThreadMember{{ID: "thread-member"}}}); err != nil {
		t.Fatal(err)
	}
	old := &Message{ID: "m", ChannelID: "c", Content: "old", Author: &User{ID: "u", Username: "original"}, Components: []MessageComponent{&Button{Label: "old button"}}}
	if err := s.State.MessageAdd(old); err != nil {
		t.Fatal(err)
	}
	raw := `{"content":"","id":"m","channel_id":"c","attachments":[],"flags":0}`
	observed := 0
	var before *Message
	s.SetDispatchObserver(func(session *Session, event *Event, decodeErr error) {
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if event.Type != "MESSAGE_UPDATE" || string(event.RawData) != raw {
			t.Fatalf("raw payload changed: %s %s", event.Type, event.RawData)
		}
		// These locks must be available during host projection/admission.
		session.State.RLock()
		session.State.RUnlock()
		session.RLock()
		session.RUnlock()
		before = event.Struct.(*MessageUpdate).BeforeUpdate
		observed++
	})
	s.AddHandler(func(_ *Session, _ *MessageUpdate) {
		if observed != 1 {
			t.Fatal("ordinary handler preceded host observer")
		}
	})
	if _, err := s.onEvent(websocket.TextMessage, []byte(`{"op":0,"s":1,"t":"MESSAGE_UPDATE","d":`+raw+`}`)); err != nil {
		t.Fatal(err)
	}
	if observed != 1 || before == nil || before.Content != "old" {
		t.Fatalf("before snapshot: %#v", before)
	}
	old.Author.Username = "changed"
	old.Components[0].(*Button).Label = "changed"
	if before.Author.Username != "original" || before.Components[0].(*Button).Label != "old button" {
		t.Fatal("before snapshot aliases mutable cache")
	}
	// A channel snapshot includes SDK-only cached collections, not JSON loss.
	channelBefore := s.cachedBefore(&ChannelUpdate{Channel: &Channel{ID: "c"}}).(*Channel)
	if len(channelBefore.Messages) != 1 || len(channelBefore.Members) != 1 {
		t.Fatal("SDK-only snapshot fields were discarded")
	}
	old.Content = "changed again"
	if channelBefore.Messages[0].Content == "changed again" {
		t.Fatal("nested channel snapshot aliases message cache")
	}
	s.SetDispatchObserver(nil)
}

func TestGatewayObserverMissingCacheAndDecodeFailure(t *testing.T) {
	s, err := New("Bot host-only")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	s.SetDispatchObserver(func(_ *Session, event *Event, decodeErr error) {
		count++
		switch count {
		case 1:
			if decodeErr != nil || event.Struct.(*MessageUpdate).BeforeUpdate != nil {
				t.Fatal("manufactured missing before snapshot")
			}
		case 2:
			if decodeErr == nil {
				t.Fatal("decode error hidden from host")
			}
		case 3:
			if event.Type != "UNKNOWN_HOST_ONLY" || event.Struct != nil {
				t.Fatal("unknown event manufactured a typed payload")
			}
		}
	})
	if _, err := s.onEvent(websocket.TextMessage, []byte(`{"op":0,"s":1,"t":"MESSAGE_UPDATE","d":{"id":"missing","channel_id":"missing"}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.onEvent(websocket.TextMessage, []byte(`{"op":0,"s":2,"t":"MESSAGE_UPDATE","d":{"flags":"invalid"}}`)); err == nil {
		t.Fatal("expected decode failure")
	}
	if _, err := s.onEvent(websocket.TextMessage, []byte(`{"op":0,"s":3,"t":"UNKNOWN_HOST_ONLY","d":{"n":9007199254740993}}`)); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("observer count: %d", count)
	}
}

func TestGatewayContextHandshakeAndTypedWrites(t *testing.T) {
	type packet struct {
		Op   int                        `json:"op"`
		Data map[string]json.RawMessage `json:"d"`
	}
	writes := make(chan packet, 4)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "10" || r.URL.Query().Get("encoding") != "json" {
			t.Error("wrong Gateway wire version")
			return
		}
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"op":10,"d":{"heartbeat_interval":60000}}`)); err != nil {
			return
		}
		var identify packet
		if err := conn.ReadJSON(&identify); err != nil {
			return
		}
		if identify.Op != 2 {
			t.Error("missing Identify")
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"op":0,"s":1,"t":"READY","d":{"v":10,"session_id":"private","user":{"id":"bot"},"guilds":[]}}`)); err != nil {
			return
		}
		for {
			var incoming struct {
				Op   int             `json:"op"`
				Data json.RawMessage `json:"d"`
			}
			if err := conn.ReadJSON(&incoming); err != nil {
				return
			}
			if incoming.Op == 1 {
				if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"op":11,"d":null}`)); err != nil {
					return
				}
				continue
			}
			p := packet{Op: incoming.Op}
			if err := json.Unmarshal(incoming.Data, &p.Data); err != nil {
				return
			}
			writes <- p
		}
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	s, err := New("Bot host-only")
	if err != nil {
		t.Fatal(err)
	}
	s.ShouldReconnectOnError = false
	s.Dialer = &websocket.Dialer{TLSClientConfig: &tls.Config{RootCAs: roots}}
	s.gateway = "wss" + strings.TrimPrefix(server.URL, "https") + "/"
	openCtx, cancelOpen := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelOpen()
	if err := s.OpenContext(openCtx); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cancelOpen() // The successful open context is not the connection lifetime.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.UpdateStatusComplexContext(ctx, UpdateStatusData{Status: "online"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestGuildMembersContext(ctx, "g", "", 0, "private-nonce", true); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestGuildMembersListContext(ctx, "g", nil, 0, "private-nonce-2", false); err != nil {
		t.Fatal(err)
	}
	if err := s.ChannelVoiceJoinManualContext(ctx, "g", "", false, true); err != nil {
		t.Fatal(err)
	}
	for index := range 4 {
		var p packet
		select {
		case p = <-writes:
		case <-ctx.Done():
			t.Fatal("missing typed Gateway write")
		}
		switch index {
		case 0:
			if p.Op != 3 || string(p.Data["activities"]) != "[]" || string(p.Data["status"]) != `"online"` {
				t.Fatal("status wire fields changed")
			}
		case 1:
			if p.Op != 8 || string(p.Data["guild_id"]) != `"g"` || string(p.Data["query"]) != `""` || string(p.Data["limit"]) != "0" || string(p.Data["nonce"]) != `"private-nonce"` || string(p.Data["presences"]) != "true" || p.Data["user_ids"] != nil {
				t.Fatal("query member selector changed")
			}
		case 2:
			if p.Op != 8 || string(p.Data["guild_id"]) != `"g"` || string(p.Data["user_ids"]) != "[]" || p.Data["query"] != nil {
				t.Fatal("user-ID member selector changed")
			}
		case 3:
			if p.Op != 4 || string(p.Data["guild_id"]) != `"g"` || string(p.Data["channel_id"]) != "null" || string(p.Data["self_mute"]) != "false" || string(p.Data["self_deaf"]) != "true" {
				t.Fatal("typed voice disconnect changed")
			}
		}
	}
}
