//go:build voice && cgo

package discordgo

import (
	"context"
	"crypto/cipher"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo/internal/dave"
	"github.com/gorilla/websocket"
)

func VoiceAvailable() bool { return dave.MaxSupportedProtocolVersion() > 0 }

type opusHandoff struct {
	ctx   context.Context
	opus  []byte
	reply chan handoffResult
}
type handoffResult struct {
	submission *OpusSubmission
	err        error
}
type voiceMessage struct {
	kind int
	data []byte
}

type nativeVoice struct {
	v                            *VoiceConnection
	ctx                          context.Context
	cancel                       context.CancelFunc
	mu                           sync.Mutex
	failure                      error
	closing                      bool
	readyFlag                    bool
	transportReady               bool
	nativeReadyFlag              bool
	change                       chan struct{}
	cleaned                      chan struct{}
	setupDone                    chan struct{}
	cleanupErr                   error
	ws                           *websocket.Conn
	udp                          *net.UDPConn
	writer                       chan struct{}
	udpWriter                    chan struct{}
	wg                           sync.WaitGroup
	cryptoMu                     gatewayMutex
	dave                         *dave.Context
	aead                         cipher.AEAD
	mode                         string
	ssrc                         uint32
	nonce                        uint64
	rtpSequence                  uint16
	rtpTimestamp                 uint32
	seqAck                       int
	version                      uint16
	daveGeneration               uint64
	resetAuthorizationGeneration uint64
	resetAuthorizationVersion    uint16
	resetPackageSent             bool
	participants                 map[string]struct{}
	senders                      map[uint32]string
	external                     []byte
	resetDeadline                time.Time
	proposalDeadline             time.Time
	transitions                  map[uint16]time.Time
	messages                     chan voiceMessage
	handoffs                     chan opusHandoff
	outstanding                  chan struct{}
	packets                      chan *Packet
	closeGate                    chan struct{}
	queueMu                      sync.Mutex
	queueBytes                   int
	heartbeatMu                  sync.Mutex
	heartbeatInterval            time.Duration
	heartbeatSent                time.Time
	heartbeatNonce               int64
	heartbeatPending             bool
	heartbeatWake                chan struct{}
}

func newVoiceEngine(v *VoiceConnection) (voiceEngine, error) {
	c := v.config
	native, err := dave.New(dave.Config{MaxParticipants: c.MaxParticipants, MaxDecryptors: c.MaxDecryptors, MaxControlBytes: c.MaxWebSocketBytes, MaxProposalCount: c.MaxProposals, MaxProposalBytes: c.MaxProposalBytes, MaxPreparedTransitions: c.MaxTransitions, MaxTransitionBytes: c.MaxTransitionBytes, MaxFrameBytes: c.MaxOpusBytes, MaxEncryptedFrameBytes: c.MaxEncryptedFrameBytes})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	n := &nativeVoice{v: v, ctx: ctx, cancel: cancel, dave: native, change: make(chan struct{}), cleaned: make(chan struct{}), setupDone: make(chan struct{}), writer: make(chan struct{}, 1), seqAck: -1, participants: map[string]struct{}{v.UserID: {}}, senders: make(map[uint32]string), transitions: make(map[uint16]time.Time), messages: make(chan voiceMessage, c.MaxControlMessages), handoffs: make(chan opusHandoff), outstanding: make(chan struct{}, 1), packets: make(chan *Packet, c.ReceiveQueuePackets), heartbeatWake: make(chan struct{}, 1)}
	n.udpWriter = make(chan struct{}, 1)
	n.outstanding <- struct{}{}
	n.closeGate = make(chan struct{}, 1)
	n.closeGate <- struct{}{}
	go n.cleanup()
	return n, nil
}
func (n *nativeVoice) done() <-chan struct{} { return n.cleaned }
func (n *nativeVoice) err() error            { n.mu.Lock(); defer n.mu.Unlock(); return n.failure }
func (n *nativeVoice) ready() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.readyFlag && n.failure == nil && !n.closing
}
func (n *nativeVoice) changed() <-chan struct{} { n.mu.Lock(); defer n.mu.Unlock(); return n.change }
func (n *nativeVoice) setReady(ready, native bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.nativeReadyFlag = native
	if n.readyFlag != ready {
		n.readyFlag = ready
		close(n.change)
		n.change = make(chan struct{})
	}
}
func (n *nativeVoice) stop(err error) {
	if err == nil {
		err = ErrVoiceClosed
	}
	n.mu.Lock()
	if n.failure != nil {
		n.mu.Unlock()
		return
	}
	n.failure = err
	n.readyFlag = false
	close(n.change)
	n.change = make(chan struct{})
	ws, udp := n.ws, n.udp
	n.mu.Unlock()
	if ws != nil {
		_ = ws.Close()
	}
	if udp != nil {
		_ = udp.Close()
	}
	n.cancel()
}
func (n *nativeVoice) cleanup() {
	<-n.ctx.Done()
	<-n.setupDone
	n.wg.Wait()
	n.cryptoMu.Lock()
	n.cleanupErr = n.dave.Close()
	n.clearPendingResetAuthorization()
	n.aead = nil
	clear(n.external)
	n.external = nil
	clear(n.participants)
	clear(n.senders)
	n.cryptoMu.Unlock()
	n.mu.Lock()
	n.ws = nil
	n.udp = nil
	n.transportReady = false
	n.nativeReadyFlag = false
	n.mu.Unlock()
	for draining := true; draining; {
		select {
		case message := <-n.messages:
			clear(message.data)
		default:
			draining = false
		}
	}
	n.queueMu.Lock()
	for draining := true; draining; {
		select {
		case packet := <-n.packets:
			n.queueBytes -= len(packet.Opus)
			clear(packet.Opus)
		default:
			draining = false
		}
	}
	n.queueMu.Unlock()
	n.v.Lock()
	n.v.token = ""
	n.v.sessionID = ""
	n.v.endpoint = ""
	n.v.Unlock()
	n.v.finishLocalCleanup()
	close(n.cleaned)
}
func (n *nativeVoice) wait(ctx context.Context) error {
	select {
	case <-n.cleaned:
		return n.cleanupErr
	default:
	}
	select {
	case <-n.cleaned:
		return n.cleanupErr
	case <-ctx.Done():
		select {
		case <-n.cleaned:
			return n.cleanupErr
		default:
			return ctx.Err()
		}
	}
}
func (n *nativeVoice) run(ctx context.Context) (err error) {
	defer close(n.setupDone)
	defer func() {
		if err != nil {
			n.stop(err)
		}
	}()
	if err = n.v.session.ChannelVoiceJoinManualContext(ctx, n.v.GuildID, n.v.ChannelID, n.v.mute, n.v.deaf); err != nil {
		return err
	}
	for {
		n.v.Lock()
		sid, token, endpoint := n.v.sessionID, n.v.token, n.v.endpoint
		if sid != "" && token != "" && endpoint != "" {
			n.v.started = true
			n.v.Unlock()
			break
		}
		n.v.Unlock()
		select {
		case <-n.v.credentials:
		case <-ctx.Done():
			return ctx.Err()
		case <-n.ctx.Done():
			return n.err()
		}
	}
	n.v.RLock()
	endpoint, sid, token := n.v.endpoint, n.v.sessionID, n.v.token
	n.v.RUnlock()
	u, err := url.Parse("wss://" + endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.Contains(endpoint, "@") {
		return errors.New("invalid voice endpoint")
	}
	u.Path = "/"
	u.RawQuery = "v=8"
	dialCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(n.ctx, cancel)
	dialer := *n.v.session.Dialer
	dialer.HandshakeTimeout = n.v.config.HandshakeTimeout
	dialer.ReadBufferSize = n.v.config.MaxWebSocketBytes
	dialer.WriteBufferSize = n.v.config.MaxWebSocketBytes
	dialer.EnableCompression = false
	ws, response, dialErr := dialer.DialContext(dialCtx, u.String(), nil)
	stop()
	cancel()
	if dialErr != nil {
		if response != nil {
			if response.Body != nil {
				response.Body.Close()
			}
			if response.StatusCode == 403 {
				return ErrVoicePermissionDenied
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if n.ctx.Err() != nil {
			return n.err()
		}
		return errors.New("voice websocket connection failed")
	}
	n.mu.Lock()
	if n.failure != nil {
		n.mu.Unlock()
		ws.Close()
		return n.err()
	}
	n.ws = ws
	n.mu.Unlock()
	ws.SetReadLimit(int64(n.v.config.MaxWebSocketBytes))
	ws.SetPingHandler(func(data string) error { return n.write(n.ctx, websocket.PongMessage, []byte(data)) })
	ws.SetCloseHandler(func(int, string) error { return nil })
	identify := struct {
		ServerID  string `json:"server_id"`
		UserID    string `json:"user_id"`
		SessionID string `json:"session_id"`
		Token     string `json:"token"`
		MaxDAVE   uint16 `json:"max_dave_protocol_version"`
	}{n.v.GuildID, n.v.UserID, sid, token, dave.MaxSupportedProtocolVersion()}
	if err = n.writeJSON(ctx, 0, identify); err != nil {
		return err
	}
	n.wg.Add(4)
	go n.readWebSocket()
	go n.controlLoop()
	go n.heartbeatLoop()
	go n.senderLoop()
	err = n.v.WaitReady(ctx)
	if errors.Is(err, context.DeadlineExceeded) {
		n.mu.Lock()
		daveMissing := n.transportReady && !n.nativeReadyFlag
		n.mu.Unlock()
		if daveMissing {
			return ErrVoiceDAVETimeout
		}
	}
	return err
}
func (n *nativeVoice) write(ctx context.Context, kind int, data []byte) error {
	ctx, cancel := context.WithTimeout(ctx, n.v.config.WriteTimeout)
	defer cancel()
	if len(data) > n.v.config.MaxWebSocketBytes {
		return errors.New("voice control exceeds bound")
	}
	select {
	case n.writer <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	case <-n.ctx.Done():
		return n.err()
	}
	defer func() { <-n.writer }()
	if err := ctx.Err(); err != nil {
		return err
	}
	n.mu.Lock()
	ws := n.ws
	n.mu.Unlock()
	if ws == nil {
		return ErrVoiceClosed
	}
	deadline, _ := ctx.Deadline()
	if err := ws.SetWriteDeadline(deadline); err != nil {
		return err
	}
	callbackDone := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = ws.Close(); close(callbackDone) })
	err := ws.WriteMessage(kind, data)
	if !stop() {
		<-callbackDone
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return errors.New("voice websocket write failed")
	}
	return nil
}
func (n *nativeVoice) writeJSON(ctx context.Context, op int, value any) error {
	b, err := json.Marshal(struct {
		Op int `json:"op"`
		D  any `json:"d"`
	}{op, value})
	if err != nil {
		return err
	}
	defer clear(b)
	return n.write(ctx, websocket.TextMessage, b)
}
func (n *nativeVoice) writeBinary(ctx context.Context, op byte, value []byte) error {
	if len(value) >= n.v.config.MaxWebSocketBytes {
		return errors.New("voice binary exceeds bound")
	}
	b := make([]byte, len(value)+1)
	b[0] = op
	copy(b[1:], value)
	defer clear(b)
	return n.write(ctx, websocket.BinaryMessage, b)
}
func (n *nativeVoice) readWebSocket() {
	defer n.wg.Done()
	for {
		kind, b, err := n.ws.ReadMessage()
		if err != nil {
			var closed *websocket.CloseError
			if errors.As(err, &closed) && (closed.Code == 4012 || closed.Code == 4016 || closed.Code == 4017) {
				n.stop(ErrVoiceUnavailable)
			} else {
				n.stop(errors.New("voice websocket lost"))
			}
			return
		}
		if kind != websocket.TextMessage && kind != websocket.BinaryMessage {
			clear(b)
			continue
		}
		select {
		case n.messages <- voiceMessage{kind, b}:
		case <-n.ctx.Done():
			clear(b)
			return
		default:
			clear(b)
			n.stop(errors.New("voice control mailbox exhausted"))
			return
		}
	}
}
func (n *nativeVoice) handleVoiceMessage(message voiceMessage) error {
	defer clear(message.data)
	if err := n.cryptoMu.lockContext(n.ctx); err != nil {
		return err
	}
	defer n.cryptoMu.Unlock()
	if err := n.err(); err != nil {
		n.clearPendingResetAuthorization()
		return err
	}
	err := n.control(message)
	if err != nil {
		n.clearPendingResetAuthorization()
	}
	n.heartbeatMu.Lock()
	helloReceived := n.heartbeatInterval > 0
	n.heartbeatMu.Unlock()
	nativeReady := n.dave.Ready()
	n.setReady(helloReceived && n.aead != nil && nativeReady, nativeReady)
	return err
}
func (n *nativeVoice) controlLoop() {
	defer n.wg.Done()
	timer := time.NewTimer(n.v.config.RekeyTimeout)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()
	for {
		select {
		case <-n.ctx.Done():
			return
		case message := <-n.messages:
			if err := n.handleVoiceMessage(message); err != nil {
				n.stop(err)
				return
			}
		case <-timer.C:
		}
		n.cryptoMu.Lock()
		deadline := n.nextRekeyDeadline()
		n.cryptoMu.Unlock()
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		if !deadline.IsZero() {
			if !time.Now().Before(deadline) {
				n.stop(ErrVoiceDAVETimeout)
				return
			}
			timer.Reset(time.Until(deadline))
		}
	}
}
func (n *nativeVoice) submit(ctx context.Context, p []byte) (*OpusSubmission, error) {
	if err := ValidateOpusPacket(p, n.v.config.MaxOpusBytes); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !n.ready() {
		if err := n.err(); err != nil {
			return nil, err
		}
		return nil, ErrVoiceNotReady
	}
	select {
	case <-n.outstanding:
	default:
		return nil, ErrVoiceInFlight
	}
	release := true
	defer func() {
		if release {
			n.outstanding <- struct{}{}
		}
	}()
	submitCtx, cancel := context.WithTimeout(ctx, n.v.config.SubmitTimeout)
	defer cancel()
	request := opusHandoff{ctx: submitCtx, opus: p, reply: make(chan handoffResult, 1)}
	select {
	case n.handoffs <- request:
	case <-submitCtx.Done():
		return nil, submitCtx.Err()
	case <-n.ctx.Done():
		return nil, n.err()
	}
	// Once the sender receives the request, it alone decides whether ownership was
	// transferred. Always await that decision, even if the submit context cancels.
	result := <-request.reply
	if result.err != nil {
		return nil, result.err
	}
	release = false
	return result.submission, nil
}
func (n *nativeVoice) receive(ctx context.Context) (*Packet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if n.ctx.Err() != nil {
		return nil, n.err()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-n.ctx.Done():
		return nil, n.err()
	case packet := <-n.packets:
		n.queueMu.Lock()
		n.queueBytes -= len(packet.Opus)
		n.queueMu.Unlock()
		if n.ctx.Err() != nil {
			return nil, n.err()
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return packet, nil
	}
}
func (n *nativeVoice) shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, n.v.config.CloseTimeout)
	defer cancel()
	select {
	case <-n.closeGate:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { n.closeGate <- struct{}{} }()
	n.mu.Lock()
	if n.failure != nil {
		n.mu.Unlock()
		return n.wait(ctx)
	}
	n.closing = true
	n.readyFlag = false
	close(n.change)
	n.change = make(chan struct{})
	ws := n.ws
	n.mu.Unlock()
	// Admission is closed. Wait for the sole non-retractable handoff to complete
	// before clearing speaking, then close all real sockets and native owners.
	select {
	case <-n.outstanding:
		defer func() { n.outstanding <- struct{}{} }()
	case <-n.ctx.Done():
		return n.wait(ctx)
	case <-ctx.Done():
		n.stop(ctx.Err())
		return ctx.Err()
	}
	var speakingErr error
	if ws != nil {
		speakingErr = n.speaking(ctx, false)
	}
	if ws != nil && speakingErr == nil {
		speakingErr = n.write(ctx, websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}
	if speakingErr != nil {
		n.stop(speakingErr)
	} else {
		n.stop(ErrVoiceClosed)
	}
	return errors.Join(speakingErr, n.wait(ctx))
}
