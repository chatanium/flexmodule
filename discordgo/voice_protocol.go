package discordgo

import (
	"encoding/binary"
	"encoding/json"
	"errors"
)

type voiceEnvelope struct {
	Op  int             `json:"op"`
	D   json.RawMessage `json:"d"`
	Seq *uint16         `json:"seq,omitempty"`
}
type voiceHeartbeatData struct {
	T      int64 `json:"t"`
	SeqAck int   `json:"seq_ack"`
}
type voiceResumeData struct {
	ServerID  string `json:"server_id"`
	SessionID string `json:"session_id"`
	Token     string `json:"token"`
	SeqAck    int    `json:"seq_ack"`
}
type voiceTransition struct {
	ID uint16 `json:"transition_id"`
}
type voicePrepareTransition struct {
	ID      uint16 `json:"transition_id"`
	Version uint16 `json:"protocol_version"`
}
type voicePrepareEpoch struct {
	Epoch   uint64 `json:"epoch"`
	Version uint16 `json:"protocol_version"`
}

var voiceRequiredFields = map[int][]string{2: {"ssrc", "ip", "port", "modes"}, 4: {"mode", "secret_key", "dave_protocol_version"}, 5: {"user_id", "ssrc", "speaking"}, 8: {"heartbeat_interval"}, 6: {"t"}, 11: {"user_ids"}, 13: {"user_id"}, 21: {"protocol_version", "transition_id"}, 22: {"transition_id"}, 24: {"protocol_version", "epoch"}}

// A Resume wire encoder is retained for v8 protocol fixtures only. Production
// connections deliberately fail on loss and never invoke Resume or replay.
func voiceResumeWire(serverID, sessionID, token string, seqAck int) ([]byte, error) {
	return json.Marshal(struct {
		Op int             `json:"op"`
		D  voiceResumeData `json:"d"`
	}{7, voiceResumeData{serverID, sessionID, token, seqAck}})
}
func parseVoiceBinary(p []byte, max int) (sequence uint16, opcode byte, payload []byte, err error) {
	if len(p) < 3 || len(p) > max {
		return 0, 0, nil, errors.New("invalid voice binary framing")
	}
	return binary.BigEndian.Uint16(p[:2]), p[2], p[3:], nil
}
func requireVoiceFields(data json.RawMessage, names ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return errors.New("invalid voice opcode object")
	}
	for _, name := range names {
		value, ok := fields[name]
		if !ok || len(value) == 0 || string(value) == "null" {
			return errors.New("missing required voice opcode field")
		}
	}
	return nil
}
