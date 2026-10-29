// Discordgo - Discord bindings for Go
// Available at https://github.com/bwmarrin/discordgo

// Copyright 2015-2016 Bruce Marriner <bruce@sqls.net>.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file contains low level functions for interacting with the Discord
// data websocket interface.

package discordgo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// ErrWSAlreadyOpen is thrown when you attempt to open
// a websocket that already is open.
var ErrWSAlreadyOpen = errors.New("web socket already opened")

// ErrWSNotFound is thrown when you attempt to use a websocket
// that doesn't exist
var ErrWSNotFound = errors.New("no websocket connection exists")

// ErrWSShardBounds is thrown when you try to use a shard ID that is
// more than the total shard count
var ErrWSShardBounds = errors.New("ShardID must be less than ShardCount")

type resumePacket struct {
	Op   int `json:"op"`
	Data struct {
		Token     string `json:"token"`
		SessionID string `json:"session_id"`
		Sequence  int64  `json:"seq"`
	} `json:"d"`
}

// Open creates a websocket connection to Discord.
// See: https://discord.com/developers/docs/topics/gateway#connecting
func (s *Session) Open() error {
	return s.OpenContext(context.Background())
}

// OpenContext establishes one main Gateway connection. Cancellation applies to
// discovery, dialing and the complete Hello/Identify/Ready handshake, not to the
// lifetime of a successfully opened session. No mutation is automatically resent.
func (s *Session) OpenContext(ctx context.Context) (err error) {
	if err = s.wsLifecycle.lockContext(ctx); err != nil {
		return err
	}
	defer s.wsLifecycle.Unlock()
	s.RLock()
	alreadyOpen := s.wsConn != nil
	frameMax, faulted := s.gatewayFrameMaxBytes, s.gatewayFaulted
	gateway, sessionID, resumeURL := s.gateway, s.sessionID, s.resumeGatewayURL
	s.RUnlock()
	if alreadyOpen {
		return ErrWSAlreadyOpen
	}
	if faulted {
		return ErrGatewayIngress
	}
	if frameMax <= 0 {
		return errors.New("Gateway ingress limits are not configured")
	}
	sequence := atomic.LoadInt64(s.sequence)
	if sequence != 0 && sessionID != "" && resumeURL != "" {
		gateway = resumeURL
	} else if gateway == "" {
		gateway, err = s.gatewayContext(ctx)
		if err != nil {
			return err
		}
		s.Lock()
		s.gateway = gateway
		s.Unlock()
	}
	target, err := gatewayURL(gateway)
	if err != nil {
		return err
	}
	header := http.Header{}
	header.Add("accept-encoding", "zlib")
	conn, response, err := s.Dialer.DialContext(ctx, target, header)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		s.Lock()
		s.gateway = ""
		s.Unlock()
		return err
	}
	conn.SetReadLimit(frameMax)
	conn.SetCloseHandler(func(int, string) error { return nil })
	// Gorilla's default ping handler writes a pong; route that writer through
	// the same gate as every explicit main Gateway write.
	conn.SetPingHandler(func(data string) error {
		pingCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		return s.gatewayWriteFrame(pingCtx, conn, websocket.PongMessage, []byte(data), nil)
	})
	finish := cancelGatewayIO(ctx, conn)
	defer func() {
		finish()
		if err != nil && !errors.Is(err, ErrGatewayIngress) {
			err = gatewayContextError(ctx, err)
		}
		if err != nil {
			if errors.Is(err, ErrGatewayIngress) {
				s.failGatewayIngress(conn)
			}
			conn.Close()
			s.Lock()
			if s.wsConn == conn {
				s.wsConn = nil
			}
			s.Unlock()
		}
	}()
	deadline, _ := ctx.Deadline()
	if err = conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	mt, message, err := conn.ReadMessage()
	if err != nil {
		return gatewayReadError(err)
	}
	hello, err := s.decodeGatewayEvent(mt, message)
	if err != nil {
		return err
	}
	if hello.Operation != 10 {
		return ErrGatewayIngress
	}
	var h helloOp
	if err = json.Unmarshal(hello.RawData, &h); err != nil {
		return ErrGatewayIngress
	}
	if h.HeartbeatInterval <= 0 || h.HeartbeatInterval > time.Duration(1<<63-1)/time.Millisecond {
		return ErrGatewayIngress
	}
	s.Lock()
	s.LastHeartbeatAck = time.Now().UTC()
	// A basic state/current user is also required by standalone voice users.
	if s.State == nil {
		s.State = NewState()
		s.State.TrackChannels = false
		s.State.TrackEmojis = false
		s.State.TrackMembers = false
		s.State.TrackRoles = false
		s.State.TrackVoice = false
	}
	s.Unlock()
	if sessionID == "" && sequence == 0 {
		err = s.identifyContext(ctx, conn)
	} else {
		p := resumePacket{}
		p.Op = 6
		p.Data.Token = s.Token
		p.Data.SessionID = sessionID
		p.Data.Sequence = sequence
		err = s.gatewayWrite(ctx, conn, p)
	}
	if err != nil {
		return err
	}
	// Publish only after Identify/Resume has been sent. No session lock is
	// held across socket I/O or the ordered host observer.
	s.Lock()
	s.wsConn = conn
	s.Unlock()
	for {
		mt, message, err = conn.ReadMessage()
		if err != nil {
			return gatewayReadError(err)
		}
		event, decodeErr := s.decodeGatewayEvent(mt, message)
		if decodeErr != nil {
			return decodeErr
		}
		switch event.Operation {
		case 0:
			if _, err = s.onGatewayEvent(event); err != nil {
				return err
			}
		case 1:
			if err = s.gatewayWrite(ctx, conn, heartbeatOp{1, atomic.LoadInt64(s.sequence)}); err != nil {
				return err
			}
		case 11:
			s.Lock()
			s.LastHeartbeatAck = time.Now().UTC()
			s.Unlock()
		default:
			// Invalid sessions/reconnect requests during this attempt are
			// failures, not permission to resend Identify behind the caller.
			return errors.New("Gateway handshake interrupted")
		}
		if event.Operation == 0 && (event.Type == "READY" || event.Type == "RESUMED") {
			break
		}
	}
	// Join any racing cancellation before removing handshake deadlines.
	finish()
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = conn.SetReadDeadline(time.Time{}); err != nil {
		return err
	}
	s.Lock()
	if s.VoiceConnections == nil {
		s.VoiceConnections = make(map[string]*VoiceConnection)
	}
	listening := make(chan interface{})
	s.listening = listening
	s.Unlock()
	s.handleEvent(connectEventType, &Connect{})
	go s.heartbeat(conn, listening, h.HeartbeatInterval)
	go s.listen(conn, listening)
	return nil
}

// listen polls the websocket connection for events, it will stop when the
// listening channel is closed, or an error occurs.
func (s *Session) listen(wsConn *websocket.Conn, listening <-chan interface{}) {

	s.log(LogInformational, "called")

	for {

		messageType, message, err := wsConn.ReadMessage()

		if err != nil {
			if errors.Is(err, websocket.ErrReadLimit) {
				s.failGatewayIngress(wsConn)
				return
			}

			// Detect if we have been closed manually. If a Close() has already
			// happened, the websocket we are listening on will be different to
			// the current session.
			s.RLock()
			sameConnection := s.wsConn == wsConn
			s.RUnlock()

			if sameConnection {

				s.log(LogWarning, "Gateway connection read failed")
				// There has been an error reading, close the websocket so that
				// OnDisconnect event is emitted.
				err := s.Close()
				if err != nil {
					s.log(LogWarning, "Gateway connection close failed")
				}

				s.log(LogInformational, "calling reconnect() now")
				s.reconnect()
			}

			return
		}

		select {

		case <-listening:
			return

		default:
			event, eventErr := s.onEvent(messageType, message)
			if eventErr != nil {
				if errors.Is(eventErr, ErrGatewayIngress) {
					s.failGatewayIngress(wsConn)
					return
				}
				// A recognized malformed typed payload was already delivered
				// selectively to the host observer, not a healthy zero event.
				if event != nil && event.Operation == 0 && event.Struct != nil {
					continue
				}
				s.Close()
				s.reconnect()
				return
			}

		}
	}
}

type heartbeatOp struct {
	Op   int   `json:"op"`
	Data int64 `json:"d"`
}

type helloOp struct {
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
}

// FailedHeartbeatAcks is the Number of heartbeat intervals to wait until forcing a connection restart.
const FailedHeartbeatAcks time.Duration = 5 * time.Millisecond

// HeartbeatLatency returns the latency between heartbeat acknowledgement and heartbeat send.
func (s *Session) HeartbeatLatency() time.Duration {
	s.RLock()
	defer s.RUnlock()
	return s.LastHeartbeatAck.Sub(s.LastHeartbeatSent)
}

// heartbeat sends regular heartbeats to Discord so it knows the client
// is still connected.  If you do not send these heartbeats Discord will
// disconnect the websocket connection after a few seconds.
func (s *Session) heartbeat(wsConn *websocket.Conn, listening <-chan interface{}, heartbeatIntervalMsec time.Duration) {

	s.log(LogInformational, "called")

	if listening == nil || wsConn == nil {
		return
	}

	var err error
	ticker := time.NewTicker(heartbeatIntervalMsec * time.Millisecond)
	defer ticker.Stop()

	for {
		s.RLock()
		last := s.LastHeartbeatAck
		s.RUnlock()
		sequence := atomic.LoadInt64(s.sequence)
		s.log(LogDebug, "sending gateway websocket heartbeat seq %d", sequence)
		s.Lock()
		s.LastHeartbeatSent = time.Now().UTC()
		s.Unlock()
		err = s.gatewayWrite(context.Background(), wsConn, heartbeatOp{1, sequence})
		if err != nil || time.Now().UTC().Sub(last) > (heartbeatIntervalMsec*FailedHeartbeatAcks) {
			s.RLock()
			sameConnection := s.wsConn == wsConn && !s.gatewayFaulted
			s.RUnlock()
			if !sameConnection {
				return
			}
			if err != nil {
				s.log(LogError, "Gateway heartbeat write failed")
			} else {
				s.log(LogError, "haven't gotten a heartbeat ACK in %v, triggering a reconnection", time.Now().UTC().Sub(last))
			}
			s.Close()
			s.reconnect()
			return
		}
		s.Lock()
		if s.wsConn != wsConn || s.gatewayFaulted {
			s.Unlock()
			return
		}
		s.DataReady = true
		s.Unlock()

		select {
		case <-ticker.C:
			// continue loop and send heartbeat
		case <-listening:
			return
		}
	}
}

// UpdateStatusData is provided to UpdateStatusComplex()
type UpdateStatusData struct {
	IdleSince  *int        `json:"since"`
	Activities []*Activity `json:"activities"`
	AFK        bool        `json:"afk"`
	Status     string      `json:"status"`
}

type updateStatusOp struct {
	Op   int              `json:"op"`
	Data UpdateStatusData `json:"d"`
}

func newUpdateStatusData(idle int, activityType ActivityType, name, url string) *UpdateStatusData {
	usd := &UpdateStatusData{
		Status: "online",
	}

	if idle > 0 {
		usd.IdleSince = &idle
	}

	if name != "" {
		usd.Activities = []*Activity{{
			Name: name,
			Type: activityType,
			URL:  url,
		}}
	}

	return usd
}

// UpdateGameStatus is used to update the user's status.
// If idle>0 then set status to idle.
// If name!="" then set game.
// if otherwise, set status to active, and no activity.
func (s *Session) UpdateGameStatus(idle int, name string) (err error) {
	return s.UpdateStatusComplex(*newUpdateStatusData(idle, ActivityTypeGame, name, ""))
}

// UpdateWatchStatus is used to update the user's watch status.
// If idle>0 then set status to idle.
// If name!="" then set movie/stream.
// if otherwise, set status to active, and no activity.
func (s *Session) UpdateWatchStatus(idle int, name string) (err error) {
	return s.UpdateStatusComplex(*newUpdateStatusData(idle, ActivityTypeWatching, name, ""))
}

// UpdateStreamingStatus is used to update the user's streaming status.
// If idle>0 then set status to idle.
// If name!="" then set game.
// If name!="" and url!="" then set the status type to streaming with the URL set.
// if otherwise, set status to active, and no game.
func (s *Session) UpdateStreamingStatus(idle int, name string, url string) (err error) {
	gameType := ActivityTypeGame
	if url != "" {
		gameType = ActivityTypeStreaming
	}
	return s.UpdateStatusComplex(*newUpdateStatusData(idle, gameType, name, url))
}

// UpdateListeningStatus is used to set the user to "Listening to..."
// If name!="" then set to what user is listening to
// Else, set user to active and no activity.
func (s *Session) UpdateListeningStatus(name string) (err error) {
	return s.UpdateStatusComplex(*newUpdateStatusData(0, ActivityTypeListening, name, ""))
}

// UpdateCustomStatus is used to update the user's custom status.
// If state!="" then set the custom status.
// Else, set user to active and remove the custom status.
func (s *Session) UpdateCustomStatus(state string) (err error) {
	data := UpdateStatusData{
		Status: "online",
	}

	if state != "" {
		// Discord requires a non-empty activity name, therefore we provide "Custom Status" as a placeholder.
		data.Activities = []*Activity{{
			Name:  "Custom Status",
			Type:  ActivityTypeCustom,
			State: state,
		}}
	}

	return s.UpdateStatusComplex(data)
}

// UpdateStatusComplex allows for sending the raw status update data untouched by discordgo.
func (s *Session) UpdateStatusComplex(usd UpdateStatusData) error {
	return s.UpdateStatusComplexContext(context.Background(), usd)
}

type requestGuildMembersData struct {
	GuildID   string    `json:"guild_id"`
	Query     *string   `json:"query,omitempty"`
	UserIDs   *[]string `json:"user_ids,omitempty"`
	Limit     int       `json:"limit"`
	Nonce     string    `json:"nonce,omitempty"`
	Presences bool      `json:"presences"`
}

type requestGuildMembersOp struct {
	Op   int                     `json:"op"`
	Data requestGuildMembersData `json:"d"`
}

// RequestGuildMembers requests guild members from the gateway
// The gateway responds with GuildMembersChunk events
// guildID   : Single Guild ID to request members of
// query     : String that username starts with, leave empty to return all members
// limit     : Max number of items to return, or 0 to request all members matched
// nonce     : Nonce to identify the Guild Members Chunk response
// presences : Whether to request presences of guild members
func (s *Session) RequestGuildMembers(guildID, query string, limit int, nonce string, presences bool) error {
	return s.RequestGuildMembersContext(context.Background(), guildID, query, limit, nonce, presences)
}

// RequestGuildMembersList requests guild members from the gateway
// The gateway responds with GuildMembersChunk events
// guildID   : Single Guild ID to request members of
// userIDs   : IDs of users to fetch
// limit     : Max number of items to return, or 0 to request all members matched
// nonce     : Nonce to identify the Guild Members Chunk response
// presences : Whether to request presences of guild members
func (s *Session) RequestGuildMembersList(guildID string, userIDs []string, limit int, nonce string, presences bool) error {
	return s.RequestGuildMembersListContext(context.Background(), guildID, userIDs, limit, nonce, presences)
}

// RequestGuildMembersBatch requests guild members from the gateway
// The gateway responds with GuildMembersChunk events
// guildID   : Slice of guild IDs to request members of
// query     : String that username starts with, leave empty to return all members
// limit     : Max number of items to return, or 0 to request all members matched
// nonce     : Nonce to identify the Guild Members Chunk response
// presences : Whether to request presences of guild members
//
// NOTE: this function is deprecated, please use RequestGuildMembers instead
func (s *Session) RequestGuildMembersBatch(guildIDs []string, query string, limit int, nonce string, presences bool) error {
	for _, guildID := range guildIDs {
		if err := s.RequestGuildMembers(guildID, query, limit, nonce, presences); err != nil {
			return err
		}
	}
	return nil
}

// RequestGuildMembersBatchList requests guild members from the gateway
// The gateway responds with GuildMembersChunk events
// guildID   : Slice of guild IDs to request members of
// userIDs   : IDs of users to fetch
// limit     : Max number of items to return, or 0 to request all members matched
// nonce     : Nonce to identify the Guild Members Chunk response
// presences : Whether to request presences of guild members
//
// NOTE: this function is deprecated, please use RequestGuildMembersList instead
func (s *Session) RequestGuildMembersBatchList(guildIDs []string, userIDs []string, limit int, nonce string, presences bool) error {
	for _, guildID := range guildIDs {
		if err := s.RequestGuildMembersList(guildID, userIDs, limit, nonce, presences); err != nil {
			return err
		}
	}
	return nil
}

// GatewayWriteStruct allows for sending raw gateway structs over the gateway.
func (s *Session) GatewayWriteStruct(data interface{}) error {
	return s.gatewayWrite(context.Background(), nil, data)
}

// onEvent is the "event handler" for all messages received on the
// Discord Gateway API websocket connection.
//
// If you use the AddHandler() function to register a handler for a
// specific event this function will pass the event along to that handler.
//
// If you use the AddHandler() function to register a handler for the
// "OnEvent" event then all events will be passed to that handler.

func (s *Session) onEvent(messageType int, message []byte) (*Event, error) {
	e, err := s.decodeGatewayEvent(messageType, message)
	if err != nil {
		return e, err
	}
	return s.onGatewayEvent(e)
}

func (s *Session) onGatewayEvent(e *Event) (*Event, error) {
	var err error

	// Ping request.
	// Must respond with a heartbeat packet within 5 seconds
	if e.Operation == 1 {
		s.log(LogInformational, "sending heartbeat in response to Op1")
		err = s.gatewayWrite(context.Background(), nil, heartbeatOp{1, atomic.LoadInt64(s.sequence)})
		if err != nil {
			s.log(LogError, "error sending heartbeat in response to Op1")
			return e, err
		}

		return e, nil
	}

	// Reconnect
	// Must immediately disconnect from gateway and reconnect to new gateway.
	if e.Operation == 7 {
		s.log(LogInformational, "Closing and reconnecting in response to Op7")
		s.CloseWithCode(websocket.CloseServiceRestart)
		s.reconnect()
		return e, nil
	}

	// Invalid Session
	// Must respond with a Identify packet.
	if e.Operation == 9 {
		var resumable *bool
		if json.Unmarshal(e.RawData, &resumable) != nil || resumable == nil {
			return e, ErrGatewayIngress
		}
		s.log(LogInformational, "Closing and reconnecting in response to Op9")
		s.CloseWithCode(websocket.CloseServiceRestart)

		if !*resumable {
			s.log(LogInformational, "Gateway session is not resumable, discarding its information")
			s.Lock()
			s.resumeGatewayURL = ""
			s.sessionID = ""
			s.Unlock()
			atomic.StoreInt64(s.sequence, 0)
		}

		s.reconnect()
		return e, nil
	}

	if e.Operation == 10 {
		// Op10 is handled by Open()
		return e, nil
	}

	if e.Operation == 11 {
		s.Lock()
		s.LastHeartbeatAck = time.Now().UTC()
		s.Unlock()
		s.log(LogDebug, "got heartbeat ACK")
		return e, nil
	}

	// Do not try to Dispatch a non-Dispatch Message
	if e.Operation != 0 {
		// But we probably should be doing something with them.
		// TEMP
		s.log(LogWarning, "unknown Gateway opcode %d", e.Operation)
		return e, nil
	}

	// Reconnection can overlap the old reader's final dispatch. The host
	// admission lane must remain serial across that boundary as well.
	s.dispatchSerial.Lock()
	// This read lock is only the callback lifetime barrier, not a State or
	// Session lock. Detach takes its write side and joins the active callback.
	s.dispatchMu.RLock()
	s.RLock()
	faulted := s.gatewayFaulted
	s.RUnlock()
	if faulted {
		s.dispatchMu.RUnlock()
		s.dispatchSerial.Unlock()
		return e, ErrGatewayIngress
	}
	// Store the message sequence
	atomic.StoreInt64(s.sequence, e.Sequence)

	observer := s.dispatchObserver
	if eh, ok := registeredInterfaceProviders[e.Type]; ok {
		e.Struct = eh.New()
		if !validGatewayPayloadShape(e.Type, e.RawData) {
			err = errGatewayPayload
		} else {
			err = json.Unmarshal(e.RawData, e.Struct)
			if err == nil && !validGatewayStatePayload(e.Struct) {
				err = errGatewayPayload
			}
		}
		if err == nil {
			var before interface{}
			if observer != nil {
				before = s.cachedBefore(e.Struct)
			}
			s.onInterface(e.Struct)
			if observer != nil {
				setDispatchBefore(e.Struct, before)
				observer(s, e, nil)
			}
		} else if observer != nil {
			observer(s, e, err)
		}
	} else if observer != nil {
		// Unknown events are visible only to the trusted observer; the host
		// allowlist decides whether they are supported, never raw fallback.
		observer(s, e, nil)
	}
	s.dispatchMu.RUnlock()
	s.dispatchSerial.Unlock()
	if e.Struct != nil && err == nil {
		s.handleEventHandlers(e.Type, e.Struct)
	}
	// Preserve the standalone SDK's legacy raw-event callback, separately.
	s.handleEvent(eventEventType, e)
	return e, err
}

// ------------------------------------------------------------------------------------------------
// Code related to voice connections that initiate over the data websocket
// ------------------------------------------------------------------------------------------------

type voiceChannelJoinData struct {
	GuildID   *string `json:"guild_id"`
	ChannelID *string `json:"channel_id"`
	SelfMute  bool    `json:"self_mute"`
	SelfDeaf  bool    `json:"self_deaf"`
}

type voiceChannelJoinOp struct {
	Op   int                  `json:"op"`
	Data voiceChannelJoinData `json:"d"`
}

// ChannelVoiceJoinManual initiates a voice session to a voice channel, but does not complete it.
//
// This should only be used when the VoiceServerUpdate will be intercepted and used elsewhere.
//
//	gID     : Guild ID of the channel to join.
//	cID     : Channel ID of the channel to join, leave empty to disconnect.
//	mute    : If true, you will be set to muted upon joining.
//	deaf    : If true, you will be set to deafened upon joining.
func (s *Session) ChannelVoiceJoinManual(gID, cID string, mute, deaf bool) error {
	return s.ChannelVoiceJoinManualContext(context.Background(), gID, cID, mute, deaf)
}

type identifyOp struct {
	Op   int      `json:"op"`
	Data Identify `json:"d"`
}

// identify sends the identify packet to the gateway
func (s *Session) identifyContext(ctx context.Context, conn *websocket.Conn) error {
	s.log(LogDebug, "called")

	// TODO: This is a temporary block of code to help
	// maintain backwards compatibility
	if s.Compress == false {
		s.Identify.Compress = false
	}

	// TODO: This is a temporary block of code to help
	// maintain backwards compatibility
	if s.Token != "" && s.Identify.Token == "" {
		s.Identify.Token = s.Token
	}

	// TODO: Below block should be refactored so ShardID and ShardCount
	// can be deprecated and their usage moved to the Session.Identify
	// struct
	if s.ShardCount > 1 {

		if s.ShardID >= s.ShardCount {
			return ErrWSShardBounds
		}

		s.Identify.Shard = &[2]int{s.ShardID, s.ShardCount}
	}

	// Send Identify packet to Discord
	op := identifyOp{2, s.Identify}
	return s.gatewayWrite(ctx, conn, op)
}

func (s *Session) reconnect() {

	s.log(LogInformational, "called")
	// Voice generations fail on main Gateway loss. Never resume/rejoin or replay
	// their media when the general Gateway reconnects.
	s.RLock()
	for _, voice := range s.VoiceConnections {
		voice.engine.stop(errors.New("main Gateway connection lost"))
	}
	s.RUnlock()

	var err error

	if s.ShouldReconnectOnError {

		wait := time.Duration(1)

		for {
			s.log(LogInformational, "trying to reconnect to gateway")

			err = s.Open()
			if err == nil {
				s.log(LogInformational, "successfully reconnected to gateway")

				return
			}

			// Certain race conditions can call reconnect() twice. If this happens, we
			// just break out of the reconnect loop
			if err == ErrWSAlreadyOpen {
				return
			}
			if errors.Is(err, ErrGatewayIngress) {
				return
			}

			s.log(LogError, "error reconnecting to gateway, %s", err)

			<-time.After(wait * time.Second)
			wait *= 2
			if wait > 600 {
				wait = 600
			}
		}
	}
}

// Close closes the main websocket and stops all voice generations.
func (s *Session) Close() error {
	return s.CloseWithCode(websocket.CloseNormalClosure)
}

// CloseWithCode closes a websocket using the provided closeCode and stops all
// listening/heartbeat goroutines.
func (s *Session) CloseWithCode(closeCode int) error {
	s.wsLifecycle.Lock()
	s.Lock()
	s.DataReady = false
	if s.listening != nil {
		close(s.listening)
		s.listening = nil
	}
	conn := s.wsConn
	s.wsConn = nil
	voices := make([]*VoiceConnection, 0, len(s.VoiceConnections))
	for _, voice := range s.VoiceConnections {
		voices = append(voices, voice)
	}
	s.Unlock()
	for _, voice := range voices {
		voice.engine.stop(ErrVoiceClosed)
	}
	var err error
	if conn != nil {
		// Bound both gate acquisition and the close frame. A stuck writer is
		// interrupted by Close even if it used a background context.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err = s.gatewayWriteFrame(ctx, conn, websocket.CloseMessage, websocket.FormatCloseMessage(closeCode, ""), nil)
		cancel()
		closeErr := conn.Close()
		if err == nil {
			err = closeErr
		}
	}
	s.wsLifecycle.Unlock()
	s.handleEvent(disconnectEventType, &Disconnect{})
	return err
}
