// Discordgo - Discord bindings for Go
// Copyright 2015-2016 Bruce Marriner. BSD-style license; see LICENSE.
package discordgo

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrVoiceUnavailable = errors.New("VOICE_TRANSPORT_UNSUPPORTED: build with voice and cgo and link pinned libdave")
var ErrVoiceClosed = errors.New("voice connection closed")
var ErrVoiceNotReady = errors.New("DAVE_NOT_READY")
var ErrVoiceInFlight = errors.New("voice submission already outstanding")

var ErrVoiceChannelChanged = errors.New("voice channel or server generation changed")
var ErrVoicePermissionDenied = errors.New("Discord voice handshake permission denied")
var ErrVoiceDAVETimeout = fmt.Errorf("DAVE_NOT_READY: %w", context.DeadlineExceeded)

// VoiceConfig deliberately has no defaults. All resource limits and deadlines
// must be supplied by the host; ordinary Gateway use does not require it.
type VoiceConfig struct {
	MaxWebSocketBytes      int           `json:"max_websocket_bytes"`
	MaxControlMessages     int           `json:"max_control_messages"`
	MaxParticipants        int           `json:"max_participants"`
	MaxDecryptors          int           `json:"max_decryptors"`
	MaxProposalBytes       int           `json:"max_proposal_bytes"`
	MaxProposals           int           `json:"max_proposals"`
	MaxTransitions         int           `json:"max_transitions"`
	MaxTransitionBytes     int           `json:"max_transition_bytes"`
	MaxOpusBytes           int           `json:"max_opus_bytes"`
	MaxEncryptedFrameBytes int           `json:"max_encrypted_frame_bytes"`
	MaxUDPPacketBytes      int           `json:"max_udp_packet_bytes"`
	ReceiveQueuePackets    int           `json:"receive_queue_packets"`
	ReceiveQueueBytes      int           `json:"receive_queue_bytes"`
	HandshakeTimeout       time.Duration `json:"handshake_timeout"`
	RekeyTimeout           time.Duration `json:"rekey_timeout"`
	WriteTimeout           time.Duration `json:"write_timeout"`
	UDPDiscoveryTimeout    time.Duration `json:"udp_discovery_timeout"`
	UDPKeepAliveInterval   time.Duration `json:"udp_keepalive_interval"`
	HeartbeatTimeout       time.Duration `json:"heartbeat_timeout"`
	SubmitTimeout          time.Duration `json:"submit_timeout"`
	ReceiptTimeout         time.Duration `json:"receipt_timeout"`
	CloseTimeout           time.Duration `json:"close_timeout"`
}

func (c VoiceConfig) Validate() error {
	for _, n := range []int{c.MaxWebSocketBytes, c.MaxControlMessages, c.MaxParticipants, c.MaxDecryptors, c.MaxProposalBytes, c.MaxProposals, c.MaxTransitions, c.MaxTransitionBytes, c.MaxOpusBytes, c.MaxEncryptedFrameBytes, c.MaxUDPPacketBytes, c.ReceiveQueuePackets, c.ReceiveQueueBytes} {
		if n <= 0 {
			return errors.New("voice resource bounds must be explicit positive integers")
		}
	}
	for _, d := range []time.Duration{c.HandshakeTimeout, c.RekeyTimeout, c.WriteTimeout, c.UDPDiscoveryTimeout, c.UDPKeepAliveInterval, c.HeartbeatTimeout, c.SubmitTimeout, c.ReceiptTimeout, c.CloseTimeout} {
		if d <= 0 {
			return errors.New("voice deadlines must be explicit positive durations")
		}
	}
	if c.MaxUDPPacketBytes > 65507 || c.MaxUDPPacketBytes < 74 || c.MaxEncryptedFrameBytes > c.MaxUDPPacketBytes-32 || c.MaxOpusBytes > c.MaxEncryptedFrameBytes || c.ReceiveQueueBytes < c.MaxOpusBytes || c.MaxTransitionBytes < c.MaxProposalBytes || c.MaxWebSocketBytes < 3 || c.MaxProposalBytes > c.MaxWebSocketBytes-3 {
		return errors.New("inconsistent voice resource bounds")
	}
	return nil
}

type OpusWriteResult struct {
	Err       error
	WrittenAt time.Time
}
type OpusSubmission struct{ done chan OpusWriteResult }

func (s *OpusSubmission) Done() <-chan OpusWriteResult { return s.done }

// Packet owns immutable encoded Opus. ReceiveOpus returns no PCM or key material.
type Packet struct {
	SSRC       uint32
	Sequence   uint16
	Timestamp  uint32
	Opus       []byte
	ReceivedAt time.Time
}

type voiceEngine interface {
	run(context.Context) error
	submit(context.Context, []byte) (*OpusSubmission, error)
	receive(context.Context) (*Packet, error)
	stop(error)
	wait(context.Context) error
	shutdown(context.Context) error
	ready() bool
	changed() <-chan struct{}
	done() <-chan struct{}
	err() error
}

// VoiceConnection is host-owned and fixed to one guild/channel generation.
// No public send/receive channels or channel-moving operation exist.
type VoiceConnection struct {
	sync.RWMutex
	LogLevel                   int
	GuildID, ChannelID, UserID string
	session                    *Session
	config                     VoiceConfig
	engine                     voiceEngine
	credentials                chan struct{}
	sessionID, token, endpoint string
	mute, deaf                 bool
	started                    bool
	disconnectGate             gatewayMutex
	disconnected               bool
	disconnectErr              error
	localClosed                bool
	releaseOnClose             bool
	leaveAck                   chan struct{}
	leaveConfirmed             bool
}

// ChannelVoiceJoinContext reserves a fresh connection. It never reuses or moves
// another channel's connection, resumes a lost transport, or retries a frame.
func (s *Session) ChannelVoiceJoinContext(ctx context.Context, guildID, channelID string, mute, deaf bool, config VoiceConfig) (*VoiceConnection, error) {
	if !VoiceAvailable() {
		return nil, ErrVoiceUnavailable
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if guildID == "" || channelID == "" {
		return nil, errors.New("guild and channel required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.RLock()
	state := s.State
	s.RUnlock()
	if state == nil {
		return nil, errors.New("voice requires main Gateway identity")
	}
	state.RLock()
	user := state.User
	userID := ""
	if user != nil {
		userID = user.ID
	}
	state.RUnlock()
	if userID == "" {
		return nil, errors.New("voice requires authenticated bot identity")
	}
	v := &VoiceConnection{session: s, config: config, GuildID: guildID, ChannelID: channelID, UserID: userID, mute: mute, deaf: deaf, credentials: make(chan struct{}, 1)}
	s.Lock()
	if s.VoiceConnections == nil {
		s.VoiceConnections = make(map[string]*VoiceConnection)
	}
	if s.VoiceConnections[guildID] != nil {
		s.Unlock()
		return nil, errors.New("voice guild already reserved")
	}
	engine, err := newVoiceEngine(v)
	if err != nil {
		s.Unlock()
		return nil, err
	}
	v.engine = engine
	s.VoiceConnections[guildID] = v
	s.Unlock()
	joinCtx, cancel := context.WithTimeout(ctx, config.HandshakeTimeout)
	defer cancel()
	err = engine.run(joinCtx)
	if err != nil {
		cleanup, c := context.WithTimeout(context.Background(), config.CloseTimeout)
		defer c()
		cleanupErr := v.DisconnectContext(cleanup)
		return nil, errors.Join(err, cleanupErr)
	}
	return v, nil
}

// VoiceGatewayDispatch consumes host-private main Gateway voice events. Hosts
// may call this from their ordered raw observer; it performs no network I/O.
func (s *Session) VoiceGatewayDispatch(e *Event) error {
	if e == nil {
		return nil
	}
	switch x := e.Struct.(type) {
	case *VoiceStateUpdate:
		return s.acceptVoiceState(x)
	case *VoiceServerUpdate:
		return s.acceptVoiceServer(x)
	}
	return nil
}
func (s *Session) acceptVoiceState(x *VoiceStateUpdate) error {
	if x == nil || x.VoiceState == nil {
		return errors.New("invalid voice state")
	}
	s.RLock()
	v := s.VoiceConnections[x.GuildID]
	s.RUnlock()
	if v == nil || x.UserID != v.UserID {
		return nil
	}
	v.Lock()
	// Only an explicitly requested retirement may consume a null-channel
	// update. Keep that generation indexed through ordered host observation.
	if x.ChannelID == "" && v.leaveAck != nil {
		first := !v.leaveConfirmed
		if first {
			v.leaveConfirmed = true
			close(v.leaveAck)
		}
		release := first && v.localClosed && v.releaseOnClose
		v.Unlock()
		if release {
			go v.removeReservation()
		}
		return nil
	}
	if x.ChannelID != v.ChannelID || (v.sessionID != "" && v.sessionID != x.SessionID) {
		v.Unlock()
		v.engine.stop(ErrVoiceChannelChanged)
		return ErrVoiceChannelChanged
	}
	v.sessionID = x.SessionID
	v.Unlock()
	v.signalCredentials()
	return nil
}
func (s *Session) acceptVoiceServer(x *VoiceServerUpdate) error {
	if x == nil {
		return errors.New("invalid voice server")
	}
	s.RLock()
	v := s.VoiceConnections[x.GuildID]
	s.RUnlock()
	if v == nil {
		return nil
	}
	v.Lock()
	if x.Endpoint == "" || (v.started && (v.endpoint != x.Endpoint || v.token != x.Token)) {
		v.Unlock()
		v.engine.stop(ErrVoiceChannelChanged)
		return ErrVoiceChannelChanged
	}
	v.endpoint = x.Endpoint
	v.token = x.Token
	v.Unlock()
	v.signalCredentials()
	return nil
}
func (v *VoiceConnection) signalCredentials() {
	select {
	case v.credentials <- struct{}{}:
	default:
	}
}
func (v *VoiceConnection) SubmitOpus(ctx context.Context, p []byte) (*OpusSubmission, error) {
	if v == nil || v.engine == nil {
		return nil, ErrVoiceUnavailable
	}
	return v.engine.submit(ctx, p)
}
func (v *VoiceConnection) ReceiveOpus(ctx context.Context) (*Packet, error) {
	if v == nil || v.engine == nil {
		return nil, ErrVoiceUnavailable
	}
	return v.engine.receive(ctx)
}
func (v *VoiceConnection) ReadyState() bool                  { return v != nil && v.engine != nil && v.engine.ready() }
func (v *VoiceConnection) ReadinessChanged() <-chan struct{} { return v.engine.changed() }
func (v *VoiceConnection) WaitReady(ctx context.Context) error {
	for {
		ch := v.ReadinessChanged()
		if err := v.Err(); err != nil {
			return err
		}
		if v.ReadyState() {
			return nil
		}
		select {
		case <-ch:
		case <-v.Done():
			return v.Err()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
func (v *VoiceConnection) SetStateContext(ctx context.Context, mute, deaf bool) error {
	if err := v.Err(); err != nil {
		return err
	}
	if err := v.session.ChannelVoiceJoinManualContext(ctx, v.GuildID, v.ChannelID, mute, deaf); err != nil {
		return err
	}
	v.Lock()
	v.mute = mute
	v.deaf = deaf
	v.Unlock()
	return nil
}
func (v *VoiceConnection) CloseContext(ctx context.Context) error { return v.engine.shutdown(ctx) }
func (v *VoiceConnection) DisconnectContext(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, v.config.CloseTimeout)
	defer cancel()
	if err := v.disconnectGate.lockContext(ctx); err != nil {
		return err
	}
	defer v.disconnectGate.Unlock()
	if v.disconnected {
		return v.disconnectErr
	}
	closeErr := v.engine.shutdown(ctx)
	v.session.RLock()
	owned := v.session.VoiceConnections[v.GuildID] == v
	v.session.RUnlock()
	var leave error
	v.Lock()
	ack := v.leaveAck
	v.Unlock()
	// A stale handle may never disconnect a later generation. Retain this
	// reservation until both local cleanup and the ordered leave event finish.
	if owned && ack == nil {
		ack = make(chan struct{})
		v.Lock()
		v.leaveAck = ack
		v.Unlock()
		leave = v.session.ChannelVoiceJoinManualContext(ctx, v.GuildID, "", true, true)
		if leave != nil {
			v.Lock()
			v.leaveAck = nil
			v.leaveConfirmed = false
			v.Unlock()
		}
	}
	v.requestDisconnectRelease()
	wait := v.engine.wait(ctx)
	var confirmation error
	if leave == nil && ack != nil {
		select {
		case <-ack:
		default:
			select {
			case <-ack:
			case <-ctx.Done():
				confirmation = ctx.Err()
			}
		}
	}
	result := errors.Join(closeErr, leave, wait, confirmation)
	if wait == nil && confirmation == nil {
		v.disconnected = true
		v.disconnectErr = result
		v.removeReservation()
	}
	return result
}
func (v *VoiceConnection) Disconnect() error {
	ctx, c := context.WithTimeout(context.Background(), v.config.CloseTimeout)
	defer c()
	return v.DisconnectContext(ctx)
}
func (v *VoiceConnection) Close() {
	ctx, c := context.WithTimeout(context.Background(), v.config.CloseTimeout)
	defer c()
	_ = v.CloseContext(ctx)
}
func (v *VoiceConnection) String() string {
	return fmt.Sprintf("VoiceConnection(guild=%s,channel=%s)", v.GuildID, v.ChannelID)
}

// Done closes only after media goroutines and native owners have been joined.
// Err and ReadinessChanged report failure immediately, before cleanup completes.
func (v *VoiceConnection) Done() <-chan struct{} { return v.engine.done() }
func (v *VoiceConnection) Err() error            { return v.engine.err() }
func (v *VoiceConnection) removeReservation() {
	// onInterface and the host observer consume the same Gateway event. Do not
	// expose a replacement between those two deliveries of the old leave.
	v.session.dispatchSerial.Lock()
	defer v.session.dispatchSerial.Unlock()
	v.RLock()
	release := v.localClosed && v.releaseOnClose && (v.leaveAck == nil || v.leaveConfirmed)
	v.RUnlock()
	if !release {
		return
	}
	v.session.Lock()
	if v.session.VoiceConnections[v.GuildID] == v {
		delete(v.session.VoiceConnections, v.GuildID)
	}
	v.session.Unlock()
}
func (v *VoiceConnection) requestDisconnectRelease() {
	v.Lock()
	v.releaseOnClose = true
	closed := v.localClosed
	v.Unlock()
	if closed {
		v.removeReservation()
	}
}
func (v *VoiceConnection) finishLocalCleanup() {
	v.Lock()
	v.localClosed = true
	release := v.releaseOnClose
	v.Unlock()
	if release {
		v.removeReservation()
	}
}
