package discordgo

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func compressedGatewayFixture(t *testing.T, raw []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	z := zlib.NewWriter(&out)
	if _, err := z.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func configuredGatewayFixture(t *testing.T, server *httptest.Server, frame, decoded int64, fatal func(error)) *Session {
	t.Helper()
	s, err := New("Bot host-only")
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	s.Dialer = &websocket.Dialer{TLSClientConfig: &tls.Config{RootCAs: roots}}
	s.gateway = "wss" + strings.TrimPrefix(server.URL, "https") + "/"
	if err := s.ConfigureGatewayIngress(frame, decoded, fatal); err != nil {
		t.Fatal(err)
	}
	return s
}

func gatewayFixtureHandshake(conn *websocket.Conn) error {
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"op":10,"d":{"heartbeat_interval":60000}}`)); err != nil {
		return err
	}
	if _, _, err := conn.ReadMessage(); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, []byte(`{"op":0,"s":1,"t":"READY","d":{"v":10,"session_id":"private","user":{"id":"bot"},"guilds":[]}}`))
}

func TestGatewayIngressBoundedDecompressionAndStrictEnvelope(t *testing.T) {
	s, err := New("Bot host-only")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ConfigureGatewayIngress(4096, 160, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"MESSAGE_CREATE", "UNKNOWN_HOST_ONLY"} {
		raw := []byte(`{"op":0,"s":1,"t":"` + name + `","d":{"content":"` + strings.Repeat("private-secret", 100) + `"}}`)
		compressed := compressedGatewayFixture(t, raw)
		if len(compressed) >= 160 {
			t.Fatal("fixture did not exercise decompression expansion")
		}
		for _, frame := range []struct {
			kind int
			data []byte
		}{{websocket.TextMessage, raw}, {websocket.BinaryMessage, compressed}} {
			if _, err := s.decodeGatewayEvent(frame.kind, frame.data); !errors.Is(err, ErrGatewayIngress) {
				t.Fatalf("unbounded %s input: %v", name, err)
			}
		}
	}
	valid := []byte(`{"op":0,"s":2,"t":"UNKNOWN_HOST_ONLY","d":{"n":9007199254740993,"zero":0}}`)
	if err := s.ConfigureGatewayIngress(4096, int64(len(valid)), nil); err != nil {
		t.Fatal(err)
	}
	for _, frame := range []struct {
		kind int
		data []byte
	}{{websocket.TextMessage, valid}, {websocket.BinaryMessage, compressedGatewayFixture(t, valid)}} {
		e, err := s.decodeGatewayEvent(frame.kind, frame.data)
		if err != nil || string(e.RawData) != `{"n":9007199254740993,"zero":0}` {
			t.Fatalf("exact bounded JSON changed: %v", err)
		}
	}
	if err := s.ConfigureGatewayIngress(4096, 4096, nil); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{
		append(bytes.Clone(valid), []byte(` {}`)...),
		[]byte(`{"op":0,"s":1,"t":"MESSAGE_CREATE","d":`),
		[]byte(`{"s":1,"t":"UNKNOWN_HOST_ONLY","d":{}}`),
		[]byte(`{"op":0,"s":1,"d":{}}`),
		[]byte(`null`),
		{'{', '"', 'o', 'p', '"', ':', '0', ',', '"', 'd', '"', ':', '"', 0xff, '"', '}'},
	} {
		for _, frame := range []struct {
			kind int
			data []byte
		}{{websocket.TextMessage, raw}, {websocket.BinaryMessage, compressedGatewayFixture(t, raw)}} {
			if _, err := s.decodeGatewayEvent(frame.kind, frame.data); !errors.Is(err, ErrGatewayIngress) {
				t.Fatalf("malformed outer JSON accepted: %v", err)
			}
		}
	}
	compressed := compressedGatewayFixture(t, valid)
	for _, corrupt := range [][]byte{compressed[:len(compressed)-1], append(bytes.Clone(compressed), []byte("private-secret")...)} {
		if _, err := s.decodeGatewayEvent(websocket.BinaryMessage, corrupt); !errors.Is(err, ErrGatewayIngress) {
			t.Fatalf("invalid zlib frame accepted: %v", err)
		}
	}
}

func TestGatewayIngressWireLimitAppliesBeforeHello(t *testing.T) {
	peerClosed := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		defer close(peerClosed)
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"op":10,"d":{"heartbeat_interval":60000,"extra":"`+strings.Repeat("secret", 100)+`"}}`))
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()
	fatal := make(chan error, 1)
	s := configuredGatewayFixture(t, server, 128, 4096, func(err error) { fatal <- err })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.OpenContext(ctx); !errors.Is(err, ErrGatewayIngress) {
		t.Fatalf("oversized Hello did not fail: %v", err)
	}
	select {
	case err := <-fatal:
		if err != ErrGatewayIngress {
			t.Fatal("fatal diagnostic leaked source error")
		}
	case <-ctx.Done():
		t.Fatal("missing fatal notification")
	}
	select {
	case <-peerClosed:
	case <-ctx.Done():
		t.Fatal("oversized Hello did not close real socket")
	}
}

func TestGatewayIngressFatalEstablishedFramesCloseSocket(t *testing.T) {
	large := []byte(`{"op":0,"s":2,"t":"UNKNOWN_HOST_ONLY","d":{"secret":"` + strings.Repeat("private", 200) + `"}}`)
	for _, test := range []struct {
		name           string
		kind           int
		data           []byte
		frame, decoded int64
	}{
		{"wire-size", websocket.TextMessage, large, 256, 4096},
		{"expanded-size", websocket.BinaryMessage, compressedGatewayFixture(t, large), 4096, 256},
		{"malformed-known-outer", websocket.TextMessage, []byte(`{"op":0,"s":2,"t":"MESSAGE_CREATE","d":`), 4096, 4096},
		{"malformed-unknown-outer", websocket.TextMessage, []byte(`{"op":0,"s":2,"t":"UNKNOWN_HOST_ONLY","d":`), 4096, 4096},
		{"trailing-json", websocket.TextMessage, []byte(`{"op":0,"s":2,"t":"UNKNOWN_HOST_ONLY","d":{}} {}`), 4096, 4096},
		{"bad-zlib", websocket.BinaryMessage, []byte("private-secret-not-zlib"), 4096, 4096},
	} {
		t.Run(test.name, func(t *testing.T) {
			send := make(chan struct{})
			peerClosed := make(chan struct{})
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				defer close(peerClosed)
				if gatewayFixtureHandshake(conn) != nil {
					return
				}
				<-send
				_ = conn.WriteMessage(test.kind, test.data)
				for {
					if _, _, err := conn.ReadMessage(); err != nil {
						return
					}
				}
			}))
			defer server.Close()
			fatal := make(chan error, 1)
			s := configuredGatewayFixture(t, server, test.frame, test.decoded, func(err error) { fatal <- err })
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := s.OpenContext(ctx); err != nil {
				close(send)
				t.Fatal(err)
			}
			defer s.Close()
			close(send)
			select {
			case err := <-fatal:
				if err != ErrGatewayIngress {
					t.Fatal("unsafe fatal error")
				}
			case <-ctx.Done():
				t.Fatal("ingress error was ignored")
			}
			select {
			case <-peerClosed:
			case <-ctx.Done():
				t.Fatal("failed ingress left socket alive")
			}
			if err := s.OpenContext(ctx); !errors.Is(err, ErrGatewayIngress) {
				t.Fatalf("faulted session reopened/retried: %v", err)
			}
		})
	}
}

func TestGatewayIngressMalformedTypedPayloadRemainsSelective(t *testing.T) {
	send := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if gatewayFixtureHandshake(conn) != nil {
			return
		}
		<-send
		for _, raw := range []string{
			`{"op":0,"s":2,"t":"MESSAGE_UPDATE","d":null}`,
			`{"op":0,"s":3,"t":"MESSAGE_UPDATE","d":{"id":"m","channel_id":"c","flags":"private-secret"}}`,
			`{"op":0,"s":4,"t":"MESSAGE_UPDATE","d":{"id":"m","channel_id":"c","content":"after"}}`,
		} {
			if conn.WriteMessage(websocket.TextMessage, []byte(raw)) != nil {
				return
			}
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	fatal := make(chan error, 1)
	s := configuredGatewayFixture(t, server, 4096, 4096, func(err error) { fatal <- err })
	observed := make(chan error, 3)
	s.SetDispatchObserver(func(_ *Session, e *Event, err error) {
		if e.Type == "MESSAGE_UPDATE" {
			observed <- err
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.OpenContext(ctx); err != nil {
		close(send)
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.ConfigureGatewayIngress(1024, 1024, nil); !errors.Is(err, ErrWSAlreadyOpen) {
		t.Fatal("live ingress policy changed")
	}
	close(send)
	for index := range 3 {
		select {
		case err := <-observed:
			if (index < 2) != (err != nil) {
				t.Fatal("typed payload failure changed event selection")
			}
		case err := <-fatal:
			t.Fatalf("typed payload killed shared session: %v", err)
		case <-ctx.Done():
			t.Fatal("listener did not continue after selective failure")
		}
	}
	if err := s.UpdateStatusComplexContext(ctx, UpdateStatusData{Status: "online"}); err != nil {
		t.Fatalf("selective failure closed shared socket: %v", err)
	}
}

func TestGatewayObserverDetachJoinsInFlightCallback(t *testing.T) {
	s, err := New("Bot host-only")
	if err != nil {
		t.Fatal(err)
	}
	entered, release, delivered, detached := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	calls := 0
	s.SetDispatchObserver(func(_ *Session, _ *Event, _ error) { calls++; close(entered); <-release })
	go func() {
		_, _ = s.onEvent(websocket.TextMessage, []byte(`{"op":0,"s":1,"t":"UNKNOWN_HOST_ONLY","d":{}}`))
		close(delivered)
	}()
	<-entered
	go func() { s.SetDispatchObserver(nil); close(detached) }()
	select {
	case <-detached:
		t.Fatal("detach returned while callback still running")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-detached:
	case <-time.After(time.Second):
		t.Fatal("detach did not join released callback")
	}
	<-delivered
	if _, err := s.onEvent(websocket.TextMessage, []byte(`{"op":0,"s":2,"t":"UNKNOWN_HOST_ONLY","d":{}}`)); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("callback ran after detach completed")
	}
}

func TestGatewayIngressPartialThreadMemberDoesNotFabricateUser(t *testing.T) {
	s, err := New("Bot host-only")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.State.GuildAdd(&Guild{ID: "g"}); err != nil {
		t.Fatal(err)
	}
	if err := s.State.ChannelAdd(&Channel{ID: "t", GuildID: "g", Type: ChannelTypeGuildPublicThread}); err != nil {
		t.Fatal(err)
	}
	observed := false
	s.SetDispatchObserver(func(_ *Session, event *Event, decodeErr error) {
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		e := event.Struct.(*ThreadMembersUpdate)
		if len(e.AddedMembers) != 1 || e.AddedMembers[0].Member == nil || e.AddedMembers[0].Member.User != nil || e.AddedMembers[0].UserID != "u" {
			t.Fatal("partial member changed or acquired fabricated identity")
		}
		observed = true
	})
	raw := []byte(`{"op":0,"s":1,"t":"THREAD_MEMBERS_UPDATE","d":{"id":"t","guild_id":"g","member_count":1,"added_members":[{"id":"t","user_id":"u","member":{"roles":[]}}]}}`)
	if _, err := s.onEvent(websocket.TextMessage, raw); err != nil {
		t.Fatal(err)
	}
	if !observed {
		t.Fatal("partial source event was discarded")
	}
	thread, err := s.State.Channel("t")
	if err != nil || len(thread.Members) != 1 || thread.Members[0].UserID != "u" {
		t.Fatal("thread membership state was discarded")
	}
	if _, err := s.State.Member("g", "u"); !errors.Is(err, ErrStateNotFound) {
		t.Fatal("fabricated member cache entry")
	}
}
