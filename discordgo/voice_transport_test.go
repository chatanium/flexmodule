//go:build voice && cgo

package discordgo

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// A public MLS ExternalSender encoded per the pinned libdave MLSpp source:
// SEC1 P256 generator public key and BasicCredential identity 00 01 01 00.
// This is not a private signing key or a replacement MLS implementation.
const voiceExternalSenderFixture = "4041046b17d1f2e12c4247f8bce6e563a440f277037d812deb33a0f4a13945d898c2964fe342e2fe1a7f9b8ee7eb4a7c0f9e162bce33576b315ececbb6406837bf51f500010400010100"

type voiceWireFixture struct {
	kind int
	data []byte
}

func nativePendingTransportFixture(t *testing.T) (*nativeVoice, <-chan voiceWireFixture) {
	t.Helper()
	return nativePendingTransportFixtureOrder(t, true)
}
func nativePendingTransportFixtureOrder(t *testing.T, externalFirst bool) (*nativeVoice, <-chan voiceWireFixture) {
	t.Helper()
	wires := make(chan voiceWireFixture, 32)
	serverConn := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConn <- c
		for {
			kind, b, err := c.ReadMessage()
			if err != nil {
				return
			}
			select {
			case wires <- voiceWireFixture{kind, b}:
			default:
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	peer := <-serverConn
	t.Cleanup(func() { peer.Close() })
	session, _ := New("Bot fixture")
	v := &VoiceConnection{session: session, config: voiceFixtureConfig(), GuildID: "1", ChannelID: "1", UserID: "2", credentials: make(chan struct{}, 1)}
	engine, err := newVoiceEngine(v)
	if err != nil {
		t.Fatal(err)
	}
	n := engine.(*nativeVoice)
	v.engine = n
	n.ws = ws
	close(n.setupDone)
	if err = n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(`{"op":8,"d":{"heartbeat_interval":1000}}`)}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		n.stop(ErrVoiceClosed)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := n.wait(ctx); err != nil {
			t.Error(err)
		}
	})
	external, err := hex.DecodeString(voiceExternalSenderFixture)
	if err != nil {
		t.Fatal(err)
	}
	if externalFirst {
		binaryFrame := append([]byte{0xff, 0xff, 25}, external...)
		if err = n.handleVoiceMessage(voiceMessage{websocket.BinaryMessage, binaryFrame}); err != nil {
			t.Fatal(err)
		}
	}
	n.mode = voiceXChaChaMode
	n.ssrc = 42
	var key [32]int
	description, _ := json.Marshal(struct {
		Mode    string  `json:"mode"`
		Key     [32]int `json:"secret_key"`
		Version int     `json:"dave_protocol_version"`
	}{voiceXChaChaMode, key, 1})
	description = append(append([]byte(`{"op":4,"seq":0,"d":`), description...), '}')
	if err = n.handleVoiceMessage(voiceMessage{websocket.TextMessage, description}); err != nil {
		t.Fatal(err)
	}
	first := nextVoiceWire(t, wires)
	if first.kind != websocket.BinaryMessage || len(first.data) < 2 || first.data[0] != 26 {
		t.Fatalf("client KeyPackage framing: %x", first.data)
	}
	if n.ready() {
		t.Fatal("transport-ready before DAVE transition")
	}
	return n, wires
}
func nativeTransportFixture(t *testing.T) (*nativeVoice, <-chan voiceWireFixture) {
	t.Helper()
	n, wires := nativePendingTransportFixture(t)
	// Initial SessionDescription and its package are deliberately insufficient:
	// this pending-self path requires authenticated24(epoch=1), then a fresh26.
	if err := n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(`{"op":24,"seq":1,"d":{"epoch":1,"protocol_version":1}}`)}); err != nil {
		t.Fatal(err)
	}
	reset := nextVoiceWire(t, wires)
	if reset.kind != websocket.BinaryMessage || len(reset.data) < 2 || reset.data[0] != 26 {
		t.Fatalf("reset key-package framing: %x", reset.data)
	}
	if err := n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(`{"op":21,"seq":2,"d":{"transition_id":0,"protocol_version":1}}`)}); err != nil {
		t.Fatal(err)
	}
	if !n.ready() {
		t.Fatal("authorized epoch-one transition0 did not activate sender")
	}
	return n, wires
}
func nextVoiceWire(t *testing.T, wires <-chan voiceWireFixture) voiceWireFixture {
	t.Helper()
	select {
	case w := <-wires:
		return w
	case <-time.After(time.Second):
		t.Fatal("missing bounded voice wire message")
		return voiceWireFixture{}
	}
}
func nativeUDPFixture(t *testing.T, n *nativeVoice) *net.UDPConn {
	t.Helper()
	peer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { peer.Close() })
	conn, err := net.DialUDP("udp", nil, peer.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	n.udp = conn
	return peer
}
func TestVoiceNativeInitialAndSoleMemberReset(t *testing.T) {
	n, wires := nativeTransportFixture(t)
	if n.seqAck != 2 {
		t.Fatalf("latest text sequence=%d", n.seqAck)
	}
	old := n.changed()
	if err := n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(`{"op":24,"seq":3,"d":{"epoch":1,"protocol_version":1}}`)}); err != nil {
		t.Fatal(err)
	}
	packageWire := nextVoiceWire(t, wires)
	if packageWire.kind != websocket.BinaryMessage || packageWire.data[0] != 26 {
		t.Fatalf("fresh key package: %x", packageWire.data)
	}
	if !n.ready() {
		t.Fatal("reset discarded active sender before execute")
	}
	if err := n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(`{"op":21,"seq":4,"d":{"transition_id":0,"protocol_version":1}}`)}); err != nil {
		t.Fatal(err)
	}
	if !n.ready() || !n.nextRekeyDeadline().IsZero() {
		t.Fatal("sole member immediate transition did not finish")
	}
	select {
	case <-old:
		t.Fatal("ordinary rekey spuriously lost valid sender readiness")
	default:
	}
	// No Ready23/Execute22 round trip is required for immediate zero.
	select {
	case wire := <-wires:
		t.Fatalf("unexpected transition0 response: %s", wire.data)
	default:
	}
}
func TestVoiceNativeV8ParticipantsAndInvalidRecovery(t *testing.T) {
	n, wires := nativeTransportFixture(t)
	for _, data := range []string{`{"op":11,"seq":4,"d":{"user_ids":["3"]}}`, `{"op":5,"seq":5,"d":{"user_id":"3","ssrc":99,"speaking":1}}`} {
		if err := n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(data)}); err != nil {
			t.Fatal(err)
		}
	}
	if n.senders[99] != "3" {
		t.Fatal("recognized SSRC mapping missing")
	}
	if err := n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(`{"op":13,"seq":6,"d":{"user_id":"3"}}`)}); err != nil {
		t.Fatal(err)
	}
	if _, ok := n.senders[99]; ok {
		t.Fatal("disconnected sender retained")
	}
	// No staged proposals/current established MLS state: pinned libdave ignores
	// this commit before parsing it. Ignored must not produce31 or a replacement26.
	if err := n.handleVoiceMessage(voiceMessage{websocket.BinaryMessage, []byte{0, 7, 29, 0, 9, 0xff}}); err != nil {
		t.Fatal(err)
	}
	select {
	case wire := <-wires:
		t.Fatalf("ignored commit caused recovery: %s", wire.data)
	default:
	}
	// An empty, valid TLS proposal vector creates stateWithProposals in the
	// official parser without emitting a synthetic commit. Now ff truly fails.
	if err := n.handleVoiceMessage(voiceMessage{websocket.BinaryMessage, []byte{0, 8, 27, 0, 0}}); err != nil {
		t.Fatal(err)
	}
	if err := n.handleVoiceMessage(voiceMessage{websocket.BinaryMessage, []byte{0, 9, 29, 0, 9, 0xff}}); err != nil {
		t.Fatal(err)
	}
	invalid := nextVoiceWire(t, wires)
	fresh := nextVoiceWire(t, wires)
	if invalid.kind != websocket.TextMessage || string(invalid.data) != `{"op":31,"d":{"transition_id":9}}` {
		t.Fatalf("invalid recovery wire: %s", invalid.data)
	}
	if fresh.kind != websocket.BinaryMessage || fresh.data[0] != 26 || n.seqAck != 9 {
		t.Fatal("invalid recovery did not refresh key package/binary seq_ack")
	}
	if n.resetPackageSent || n.resetAuthorizationGeneration != 0 {
		t.Fatal("invalid-commit recovery26 alone authorized pending-self activation")
	}
	for _, data := range []string{`{"op":21,"d":{"protocol_version":1}}`, `{"op":22,"d":{}}`, `{"op":24,"d":{"transition_id":2,"protocol_version":1}}`, `{"op":25,"d":{}}`} {
		if n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(data)}) == nil {
			t.Fatalf("invalid control accepted: %s", data)
		}
	}
}
func TestVoiceNativeOneReceiptAndPacing(t *testing.T) {
	n, wires := nativeTransportFixture(t)
	peer := nativeUDPFixture(t, n)
	n.wg.Add(1)
	go n.senderLoop()
	frame := []byte{0xf8, 0xff, 0xfe}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sub, err := n.submit(ctx, frame); !errors.Is(err, context.Canceled) || sub != nil {
		t.Fatalf("canceled before handoff: %v %v", sub, err)
	}
	first, err := n.submit(context.Background(), frame)
	if err != nil {
		t.Fatal(err)
	}
	one := <-first.Done()
	if one.Err != nil {
		t.Fatal(one.Err)
	}
	n.udpWriter <- struct{}{} // Hold the second real write to make admission deterministic.
	secondCtx, secondCancel := context.WithCancel(context.Background())
	second, err := n.submit(secondCtx, frame)
	if err != nil {
		t.Fatal(err)
	}
	secondCancel()
	if sub, err := n.submit(context.Background(), frame); err != ErrVoiceInFlight || sub != nil {
		t.Fatalf("second outstanding handoff accepted: %v %v", sub, err)
	}
	<-n.udpWriter
	two := <-second.Done()
	if two.Err != nil {
		t.Fatal("post-handoff cancellation retracted packet", two.Err)
	}
	if two.WrittenAt.Sub(one.WrittenAt) < 20*time.Millisecond {
		t.Fatal("sender emitted a catch-up burst")
	}
	speaking := nextVoiceWire(t, wires)
	if speaking.kind != websocket.TextMessage || !bytes.Contains(speaking.data, []byte(`"speaking":1`)) {
		t.Fatalf("first UDP lacked speaking: %s", speaking.data)
	}
	packet := make([]byte, 2048)
	if err := peer.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		size, _, err := peer.ReadFromUDP(packet)
		if err != nil {
			t.Fatal(err)
		}
		plain, _, _, _, err := openVoiceRTP(make([]byte, 2048), n.aead, packet[:size])
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(plain, frame) || len(plain) < 2 || !bytes.Equal(plain[len(plain)-2:], []byte{0xfa, 0xfa}) {
			t.Fatal("UDP bypassed real DAVE frame encryption")
		}
	}
	if err := n.shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestVoiceNativeReceiveOnlyRejectsUnrecognizedPlaintext(t *testing.T) {
	n, _ := nativeTransportFixture(t)
	peer := nativeUDPFixture(t, n)
	n.wg.Add(1)
	go n.receiverLoop()
	packet, err := sealVoiceRTP(make([]byte, 0, 2048), n.aead, 123, 1, 960, 1, []byte{0xf8, 0xff, 0xfe})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = peer.WriteToUDP(packet, n.udp.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if p, err := n.receive(ctx); p != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unknown/plaintext sender leaked: %v %v", p, err)
	}
	if !n.ready() {
		t.Fatal("receive-only operation unexpectedly needs a producer")
	}
}
func TestVoiceNativeExternalMoveFailsGeneration(t *testing.T) {
	n, _ := nativeTransportFixture(t)
	n.v.session.VoiceConnections = map[string]*VoiceConnection{"1": n.v}
	err := n.v.session.acceptVoiceState(&VoiceStateUpdate{VoiceState: &VoiceState{GuildID: "1", ChannelID: "3", UserID: "2", SessionID: "new"}})
	if err == nil || n.err() == nil || n.ready() {
		t.Fatal("external move did not fail generation")
	}
	select {
	case <-n.done():
	case <-time.After(time.Second):
		t.Fatal("failed generation cleanup did not join")
	}
}
func TestVoiceJoinRequiresBothMainGatewayEvents(t *testing.T) {
	for _, onlyServer := range []bool{false, true} {
		t.Run(map[bool]string{false: "state-only", true: "server-only"}[onlyServer], func(t *testing.T) {
			wires := make(chan voiceWireFixture, 8)
			serverConn := make(chan *websocket.Conn, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				serverConn <- c
				for {
					kind, b, err := c.ReadMessage()
					if err != nil {
						return
					}
					select {
					case wires <- voiceWireFixture{kind, b}:
					default:
						return
					}
				}
			}))
			defer srv.Close()
			ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer ws.Close()
			peer := <-serverConn
			defer peer.Close()
			s, _ := New("Bot fixture")
			s.State.User = &User{ID: "2"}
			s.wsConn = ws
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				_, err := s.ChannelVoiceJoinContext(ctx, "1", "1", true, false, voiceFixtureConfig())
				result <- err
			}()
			join := nextVoiceWire(t, wires)
			if !bytes.Contains(join.data, []byte(`"channel_id":"1"`)) {
				t.Fatalf("missing real main Gateway join: %s", join.data)
			}
			if onlyServer {
				err = s.VoiceGatewayDispatch(&Event{Struct: &VoiceServerUpdate{GuildID: "1", Token: "fixture", Endpoint: "invalid-endpoint/"}})
			} else {
				err = s.VoiceGatewayDispatch(&Event{Struct: &VoiceStateUpdate{VoiceState: &VoiceState{GuildID: "1", ChannelID: "1", UserID: "2", SessionID: "fixture"}}})
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-result:
				t.Fatalf("join continued with only one main Gateway event: %v", err)
			case <-time.After(20 * time.Millisecond):
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("canceled join: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("join cancellation did not close real transport/native owners")
			}
			leave := nextVoiceWire(t, wires)
			if !bytes.Contains(leave.data, []byte(`"channel_id":null`)) {
				t.Fatalf("failed join did not leave: %s", leave.data)
			}
			s.RLock()
			retained := len(s.VoiceConnections)
			s.RUnlock()
			if retained != 0 {
				t.Fatal("failed join retained guild reservation")
			}
		})
	}
}
func TestVoiceNativeHeartbeatAcknowledgesBinarySequence(t *testing.T) {
	n, wires := nativeTransportFixture(t)
	// A pending-only context validly ignores this in-flight commit. Unlike
	// replacing ExternalSender, that does not recreate an activated key domain.
	if err := n.handleVoiceMessage(voiceMessage{websocket.BinaryMessage, []byte{0xff, 0xff, 29, 0, 9, 0xff}}); err != nil {
		t.Fatal(err)
	}
	n.cryptoMu.Lock()
	err := n.sendHeartbeat()
	n.cryptoMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	wire := nextVoiceWire(t, wires)
	var heartbeat struct {
		Op int                `json:"op"`
		D  voiceHeartbeatData `json:"d"`
	}
	if err = json.Unmarshal(wire.data, &heartbeat); err != nil {
		t.Fatal(err)
	}
	if heartbeat.Op != 3 || heartbeat.D.SeqAck != 65535 || heartbeat.D.T == 0 {
		t.Fatalf("v8 binary seq_ack missing: %s", wire.data)
	}
	ack, _ := json.Marshal(struct {
		Op int `json:"op"`
		D  struct {
			T int64 `json:"t"`
		} `json:"d"`
	}{Op: 6, D: struct {
		T int64 `json:"t"`
	}{heartbeat.D.T}})
	if err = n.handleVoiceMessage(voiceMessage{websocket.TextMessage, bytes.Clone(ack)}); err != nil {
		t.Fatal(err)
	}
	n.heartbeatMu.Lock()
	pending := n.heartbeatPending
	n.heartbeatMu.Unlock()
	if pending {
		t.Fatal("heartbeat ACK did not clear outstanding nonce")
	}
	if n.handleVoiceMessage(voiceMessage{websocket.TextMessage, ack}) == nil {
		t.Fatal("duplicate ACK accepted without an outstanding heartbeat")
	}
}
func TestVoiceNativeConnectionLossNeverResumes(t *testing.T) {
	n, wires := nativeTransportFixture(t)
	n.wg.Add(1)
	go n.readWebSocket()
	if err := n.ws.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-n.done():
	case <-time.After(time.Second):
		t.Fatal("real socket loss did not fail generation")
	}
	if n.ready() {
		t.Fatal("lost connection remained ready")
	}
	select {
	case wire := <-wires:
		t.Fatalf("connection loss unexpectedly wrote/replayed control: %s", wire.data)
	default:
	}
}
func TestVoiceNativeRekeyDeadlineRetainsOldSenderThenFails(t *testing.T) {
	n, wires := nativeTransportFixture(t)
	n.v.config.RekeyTimeout = 20 * time.Millisecond
	n.wg.Add(1)
	go n.controlLoop()
	n.messages <- voiceMessage{websocket.TextMessage, []byte(`{"op":24,"seq":3,"d":{"epoch":1,"protocol_version":1}}`)}
	wire := nextVoiceWire(t, wires)
	if wire.kind != websocket.BinaryMessage || wire.data[0] != 26 {
		t.Fatal("missing reset key package")
	}
	select {
	case <-n.done():
	case <-time.After(time.Second):
		t.Fatal("unfinished rekey was not bounded")
	}
	if !errors.Is(n.err(), ErrVoiceDAVETimeout) || !errors.Is(n.err(), context.DeadlineExceeded) {
		t.Fatalf("rekey deadline classification: %v", n.err())
	}
	if n.ready() {
		t.Fatal("failed rekey retained admission readiness")
	}
}
func TestVoiceNativeDoneJoinsAndLateDisconnectReleasesOnlyOwnGeneration(t *testing.T) {
	n, _ := nativeTransportFixture(t)
	n.v.session.VoiceConnections = map[string]*VoiceConnection{"1": n.v}
	// Keep one transport worker outstanding across the caller's cleanup deadline.
	// Native destruction and Done must wait for that worker, not merely cancel it.
	n.wg.Add(1)
	released := false
	defer func() {
		if !released {
			n.wg.Done()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	err := n.v.DisconnectContext(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded disconnect: %v", err)
	}
	if n.err() == nil {
		t.Fatal("failure was not published immediately")
	}
	select {
	case <-n.done():
		t.Fatal("Done closed before worker/native destruction joined")
	default:
	}
	n.v.session.RLock()
	retained := n.v.session.VoiceConnections["1"] == n.v
	n.v.session.RUnlock()
	if !retained {
		t.Fatal("joining cleanup lost its guild reservation")
	}
	released = true
	n.wg.Done()
	select {
	case <-n.done():
	case <-time.After(time.Second):
		t.Fatal("late native cleanup did not join")
	}
	n.v.session.RLock()
	old := n.v.session.VoiceConnections["1"]
	n.v.session.RUnlock()
	if old != nil {
		t.Fatal("expired main-leave context permanently retained a dead SDK generation")
	}
	replacement, _ := nativeTransportFixture(t)
	replacement.v.session = n.v.session
	n.v.session.Lock()
	n.v.session.VoiceConnections["1"] = replacement.v
	n.v.session.Unlock()
	if err := n.v.DisconnectContext(context.Background()); err != nil {
		t.Fatalf("stale cleanup attempted another main leave: %v", err)
	}
	n.v.session.RLock()
	current := n.v.session.VoiceConnections["1"]
	n.v.session.RUnlock()
	if current != replacement.v || !replacement.ready() {
		t.Fatal("stale disconnect affected the replacement generation")
	}
}
func TestVoiceInitialDescriptionCannotAuthorizePendingSelf(t *testing.T) {
	n, wires := nativePendingTransportFixture(t)
	if err := n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(`{"op":21,"seq":1,"d":{"transition_id":0,"protocol_version":1}}`)}); err == nil {
		t.Fatal("initial SessionDescription and generated/sent26 authorized pending-self activation without24")
	}
	if n.ready() {
		t.Fatal("unauthorized pending-self activation published readiness")
	}
	select {
	case wire := <-wires:
		t.Fatalf("unauthorized activation wrote control: %s", wire.data)
	default:
	}
}

func TestVoiceResetPackageWriteFailureCannotAuthorizePendingSelf(t *testing.T) {
	n, wires := nativePendingTransportFixture(t)
	if err := n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(`{"op":24,"seq":1,"d":{"epoch":1,"protocol_version":1}}`)}); err != nil {
		t.Fatal(err)
	}
	nextVoiceWire(t, wires)
	if !n.resetPackageSent || n.resetAuthorizationGeneration != n.daveGeneration {
		t.Fatal("first reset did not establish its generation-bound authorization")
	}
	if err := n.ws.Close(); err != nil {
		t.Fatal(err)
	}
	if err := n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(`{"op":24,"seq":2,"d":{"epoch":1,"protocol_version":1}}`)}); err == nil {
		t.Fatal("closed websocket accepted fresh26 write")
	}
	if n.resetPackageSent || n.resetAuthorizationGeneration != 0 {
		t.Fatal("superseding failed reset retained the previous generation's authorization")
	}
	// The native key package was generated, but generation alone is insufficient:
	// transport never called AuthorizePendingReset after the failed actual write.
	if err := n.dave.PrepareTransition(n.ctx, 0, 1); err == nil {
		t.Fatal("unsent reset key package authorized native pending-self activation")
	}
}

func TestVoiceIgnoredCommitPreservesUnfinishedResetDeadline(t *testing.T) {
	n, wires := nativeTransportFixture(t)
	n.v.config.RekeyTimeout = 20 * time.Millisecond
	if err := n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(`{"op":24,"seq":3,"d":{"epoch":1,"protocol_version":1}}`)}); err != nil {
		t.Fatal(err)
	}
	wire := nextVoiceWire(t, wires)
	if wire.kind != websocket.BinaryMessage || wire.data[0] != 26 {
		t.Fatal("reset did not write its fresh key package")
	}
	resetDeadline := n.resetDeadline
	if err := n.handleVoiceMessage(voiceMessage{websocket.BinaryMessage, []byte{0, 4, 29, 0, 9, 0xff}}); err != nil {
		t.Fatal(err)
	}
	if resetDeadline.IsZero() || !n.resetDeadline.Equal(resetDeadline) {
		t.Fatal("ignored in-flight commit completed or extended an unfinished reset")
	}
	if len(n.transitions) != 0 || !n.ready() {
		t.Fatal("ignored commit retained a temporary transition or discarded the old sender")
	}
	// Run the real deadline owner after the deterministic reset/ignored sequence.
	// A valid participant no-op wakes its existing-deadline calculation.
	n.wg.Add(1)
	go n.controlLoop()
	n.messages <- voiceMessage{websocket.TextMessage, []byte(`{"op":11,"seq":5,"d":{"user_ids":["2"]}}`)}
	select {
	case <-n.done():
	case <-time.After(time.Second):
		t.Fatal("ignored commit canceled the unfinished reset timeout")
	}
	if !errors.Is(n.err(), ErrVoiceDAVETimeout) {
		t.Fatalf("unfinished reset deadline: %v", n.err())
	}
}

func TestVoiceEmptyRevokeRetiresOnlyProposalDeadlineBookkeeping(t *testing.T) {
	for _, resetPending := range []bool{false, true} {
		t.Run(map[bool]string{false: "healthy", true: "reset-pending"}[resetPending], func(t *testing.T) {
			n, wires := nativeTransportFixture(t)
			if resetPending {
				if err := n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(`{"op":24,"seq":3,"d":{"epoch":1,"protocol_version":1}}`)}); err != nil {
					t.Fatal(err)
				}
				nextVoiceWire(t, wires)
			}
			resetDeadline := n.resetDeadline
			// Unit-state setup for the transport's proposal negotiation deadline.
			// This does not pretend to create a signed native Add proposal: the
			// actual queue here is empty and receives a valid empty revoke list.
			// Real Add→last-revoke cryptographic coverage requires the separate
			// official external-sender test harness, not fabricated MLS bytes.
			n.proposalDeadline = time.Now().Add(n.v.config.RekeyTimeout)
			if err := n.handleVoiceMessage(voiceMessage{websocket.BinaryMessage, []byte{0, 4, 27, 1, 0}}); err != nil {
				t.Fatal(err)
			}
			if !n.proposalDeadline.IsZero() || !n.resetDeadline.Equal(resetDeadline) {
				t.Fatal("successful empty revoke did not retire only proposal negotiation")
			}
			select {
			case wire := <-wires:
				t.Fatalf("empty proposal queue produced a synthetic commit: %x", wire.data)
			default:
			}
			if !resetPending && !n.nextRekeyDeadline().IsZero() {
				t.Fatal("healthy empty-queue negotiation retained a rekey deadline")
			}
		})
	}
}

func TestVoiceDeadlineScopesDoNotCompleteEachOther(t *testing.T) {
	now := time.Now()
	n := &nativeVoice{
		v:                &VoiceConnection{config: voiceFixtureConfig()},
		resetDeadline:    now.Add(time.Second),
		proposalDeadline: now.Add(2 * time.Second),
		transitions:      map[uint16]time.Time{7: now.Add(3 * time.Second)},
	}
	reset, proposal := n.resetDeadline, n.proposalDeadline
	n.finishTransition(7)
	if !n.resetDeadline.Equal(reset) || !n.proposalDeadline.Equal(proposal) {
		t.Fatal("finishing one prepared transition completed unrelated work")
	}
	n.proposalDeadline = time.Time{}
	if !n.nextRekeyDeadline().Equal(reset) {
		t.Fatal("retiring proposal-only work canceled the unfinished reset")
	}
}
func TestVoiceLateExternalSenderFinishesOnlyEpochOneAuthorization(t *testing.T) {
	n, wires := nativePendingTransportFixtureOrder(t, false)
	if err := n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(`{"op":24,"seq":1,"d":{"epoch":1,"protocol_version":1}}`)}); err != nil {
		t.Fatal(err)
	}
	wire := nextVoiceWire(t, wires)
	if wire.kind != websocket.BinaryMessage || wire.data[0] != 26 {
		t.Fatal("epoch-one reset did not write its fresh package")
	}
	if !n.resetPackageSent || n.resetAuthorizationGeneration != n.daveGeneration || n.resetAuthorizationVersion != 1 || n.ready() {
		t.Fatal("reset provenance was lost or published readiness before external sender")
	}
	external, err := hex.DecodeString(voiceExternalSenderFixture)
	if err != nil {
		t.Fatal(err)
	}
	if err = n.handleVoiceMessage(voiceMessage{websocket.BinaryMessage, append([]byte{0, 2, 25}, external...)}); err != nil {
		t.Fatal(err)
	}
	if err = n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(`{"op":21,"seq":3,"d":{"transition_id":0,"protocol_version":1}}`)}); err != nil {
		t.Fatal(err)
	}
	if !n.ready() || n.resetPackageSent || n.resetAuthorizationGeneration != 0 {
		t.Fatal("pending activation did not consume its matching epoch-one authorization")
	}
	// Do not weaken native's same-domain replacement/nonce-reuse guard.
	if err = n.handleVoiceMessage(voiceMessage{websocket.BinaryMessage, append([]byte{0, 4, 25}, external...)}); err == nil {
		t.Fatal("external sender replacement recreated an activated pending domain")
	}
}

func TestVoiceLateExternalSenderDoesNotAuthorizeInitialDescription(t *testing.T) {
	n, _ := nativePendingTransportFixtureOrder(t, false)
	external, err := hex.DecodeString(voiceExternalSenderFixture)
	if err != nil {
		t.Fatal(err)
	}
	if err = n.handleVoiceMessage(voiceMessage{websocket.BinaryMessage, append([]byte{0, 1, 25}, external...)}); err != nil {
		t.Fatal(err)
	}
	if n.resetPackageSent || n.resetAuthorizationGeneration != 0 {
		t.Fatal("late external sender invented epoch-one reset provenance")
	}
	if err = n.handleVoiceMessage(voiceMessage{websocket.TextMessage, []byte(`{"op":21,"seq":2,"d":{"transition_id":0,"protocol_version":1}}`)}); err == nil {
		t.Fatal("initial-only package plus late external sender authorized pending activation")
	}
}
