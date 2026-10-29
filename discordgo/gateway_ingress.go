package discordgo

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

// Ordinary standalone sessions created by New accept at most 8 MiB per wire
// message and 32 MiB after decompression. Hosts must explicitly configure their
// operator-selected bounds; these defaults are not backend configuration.
const defaultGatewayFrameMaxBytes int64 = 8 << 20
const defaultGatewayDecodedMaxBytes int64 = 32 << 20

// ErrGatewayIngress is a fatal frame/decompression/envelope failure. It carries
// no payload, token, URL, parser diagnostic or attacker-controlled text.
var ErrGatewayIngress = errors.New("invalid or oversized Gateway ingress")

// ConfigureGatewayIngress installs finite wire/decompressed byte bounds and a
// host-only fatal callback before opening. It rejects an open or opening session
// rather than waiting for its handshake. onFatal receives only ErrGatewayIngress
// after the actual socket has been closed; invalid ingress never auto-reconnects.
// Callbacks must be bounded and must not reconfigure, detach or open/close the
// session recursively. SetDispatchObserver(nil) disables and joins both callback
// kinds, so a host should cancel its lifetime before detaching during shutdown.
func (s *Session) ConfigureGatewayIngress(frameMaxBytes, decodedMaxBytes int64, onFatal func(error)) error {
	maxInt := int64(^uint(0) >> 1)
	if frameMaxBytes <= 0 || decodedMaxBytes <= 0 || frameMaxBytes >= maxInt || decodedMaxBytes >= maxInt {
		return errors.New("Gateway ingress limits must be positive finite byte counts")
	}
	if !s.wsLifecycle.tryLock() {
		return ErrWSAlreadyOpen
	}
	defer s.wsLifecycle.Unlock()
	s.Lock()
	if s.wsConn != nil {
		s.Unlock()
		return ErrWSAlreadyOpen
	}
	s.gatewayFrameMaxBytes = frameMaxBytes
	s.gatewayDecodedMaxBytes = decodedMaxBytes
	s.gatewayFaulted = false
	s.Unlock()
	s.dispatchMu.Lock()
	s.gatewayFatal = onFatal
	s.gatewayCallbacksDetached = false
	s.dispatchMu.Unlock()
	return nil
}

func (s *Session) decodeGatewayEvent(messageType int, message []byte) (*Event, error) {
	s.RLock()
	frameMax, decodedMax := s.gatewayFrameMaxBytes, s.gatewayDecodedMaxBytes
	s.RUnlock()
	if frameMax <= 0 || decodedMax <= 0 || int64(len(message)) > frameMax {
		return nil, ErrGatewayIngress
	}
	var raw []byte
	switch messageType {
	case websocket.TextMessage:
		if int64(len(message)) > decodedMax {
			return nil, ErrGatewayIngress
		}
		raw = message
	case websocket.BinaryMessage:
		compressed := bytes.NewReader(message)
		z, err := zlib.NewReader(compressed)
		if err != nil {
			return nil, ErrGatewayIngress
		}
		// Enforce the decompressed bound before constructing JSON strings,
		// RawMessage values, event structs or any typed domain allocations.
		raw, err = io.ReadAll(io.LimitReader(z, decodedMax+1))
		closeErr := z.Close()
		if err != nil || closeErr != nil || int64(len(raw)) > decodedMax || compressed.Len() != 0 {
			return nil, ErrGatewayIngress
		}
	default:
		return nil, ErrGatewayIngress
	}
	if !utf8.Valid(raw) {
		return nil, ErrGatewayIngress
	}
	var envelope struct {
		Operation *int            `json:"op"`
		Sequence  *int64          `json:"s"`
		Type      *string         `json:"t"`
		Data      json.RawMessage `json:"d"`
	}
	// Unmarshal requires one complete JSON value and rejects trailing values or
	// non-whitespace bytes, unlike a single Decoder.Decode call.
	if json.Unmarshal(raw, &envelope) != nil || envelope.Operation == nil || len(envelope.Data) == 0 {
		return nil, ErrGatewayIngress
	}
	e := &Event{Operation: *envelope.Operation, RawData: envelope.Data}
	if envelope.Sequence != nil {
		e.Sequence = *envelope.Sequence
	}
	if envelope.Type != nil {
		e.Type = *envelope.Type
	}
	if e.Operation == 0 && (envelope.Sequence == nil || e.Type == "") {
		return nil, ErrGatewayIngress
	}
	return e, nil
}

func gatewayReadError(err error) error {
	if errors.Is(err, websocket.ErrReadLimit) {
		return ErrGatewayIngress
	}
	return err
}

// failGatewayIngress revokes this connection before closing it, so a heartbeat
// woken by the close cannot mistake this failure for permission to reconnect.
func (s *Session) failGatewayIngress(conn *websocket.Conn) {
	s.Lock()
	alreadyFailed := s.gatewayFaulted
	s.gatewayFaulted = true
	if s.wsConn == conn {
		s.wsConn = nil
		s.DataReady = false
		if s.listening != nil {
			close(s.listening)
			s.listening = nil
		}
	}
	voices := make([]*VoiceConnection, 0, len(s.VoiceConnections))
	for _, voice := range s.VoiceConnections {
		voices = append(voices, voice)
	}
	s.Unlock()
	_ = conn.Close()
	for _, voice := range voices {
		voice.engine.stop(ErrGatewayIngress)
	}
	if alreadyFailed {
		return
	}
	s.dispatchSerial.Lock()
	defer s.dispatchSerial.Unlock()
	s.dispatchMu.RLock()
	defer s.dispatchMu.RUnlock()
	if !s.gatewayCallbacksDetached && s.gatewayFatal != nil {
		s.gatewayFatal(ErrGatewayIngress)
	}
}
