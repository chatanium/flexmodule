package discordgo

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"
	"time"
)

func voiceFixtureConfig() VoiceConfig {
	return VoiceConfig{
		MaxWebSocketBytes: 65536, MaxControlMessages: 16, MaxParticipants: 16, MaxDecryptors: 16, MaxProposalBytes: 16384, MaxProposals: 16, MaxTransitions: 8, MaxTransitionBytes: 32768, MaxOpusBytes: 1275, MaxEncryptedFrameBytes: 1536, MaxUDPPacketBytes: 2048, ReceiveQueuePackets: 16, ReceiveQueueBytes: 20400,
		HandshakeTimeout: time.Second, RekeyTimeout: time.Second, WriteTimeout: time.Second, UDPDiscoveryTimeout: time.Second, UDPKeepAliveInterval: time.Second, HeartbeatTimeout: time.Second, SubmitTimeout: time.Second, ReceiptTimeout: time.Second, CloseTimeout: time.Second,
	}
}
func TestVoiceConfigRequiresEveryFiniteBound(t *testing.T) {
	good := voiceFixtureConfig()
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := []VoiceConfig{{}, good, good, good, good, good}
	cases[1].MaxControlMessages = 0
	cases[2].RekeyTimeout = 0
	cases[3].MaxUDPPacketBytes = 65508
	cases[4].ReceiveQueueBytes = 1
	cases[5].MaxProposalBytes = math.MaxInt
	for i, c := range cases {
		if c.Validate() == nil {
			t.Fatalf("invalid configuration %d accepted", i)
		}
	}
}
func TestVoiceBinarySequenceAndDirection(t *testing.T) {
	for _, op := range []byte{25, 27, 29, 30} {
		wire := []byte{0xff, 0xff, op, 0x12, 0x34, 0xaa}
		seq, got, payload, err := parseVoiceBinary(wire, 64)
		if err != nil || seq != 65535 || got != op || !bytes.Equal(payload, wire[3:]) {
			t.Fatalf("binary decode %d: seq=%d op=%d payload=%x err=%v", op, seq, got, payload, err)
		}
	}
	for _, p := range [][]byte{nil, {25}, {0, 25}} {
		if _, _, _, err := parseVoiceBinary(p, 64); err == nil {
			t.Fatalf("truncated frame accepted %x", p)
		}
	}
	if _, _, _, err := parseVoiceBinary([]byte{0, 1, 25, 0}, 3); err == nil {
		t.Fatal("oversized binary frame accepted")
	}
	wire, err := voiceResumeWire("1", "session", "credential", 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(wire) != `{"op":7,"d":{"server_id":"1","session_id":"session","token":"credential","seq_ack":0}}` {
		t.Fatalf("incorrect v8 Resume: %s", wire)
	}
	wire, err = voiceResumeWire("1", "session", "credential", 65535)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(wire, []byte(`"seq_ack":65535`)) {
		t.Fatal("last binary sequence missing from Resume")
	}
}
func TestVoiceEpochWireHasNoTransitionID(t *testing.T) {
	b, err := json.Marshal(voicePrepareEpoch{Epoch: 1, Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"epoch":1,"protocol_version":1}` {
		t.Fatalf("incorrect PrepareEpoch schema: %s", b)
	}
	b, err = json.Marshal(voiceHeartbeatData{T: 123, SeqAck: -1})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"t":123,"seq_ack":-1}` {
		t.Fatalf("incorrect v8 heartbeat: %s", b)
	}
}
func TestVoiceModeNegotiationNoLegacyFallback(t *testing.T) {
	for _, c := range []struct {
		modes []string
		want  string
	}{
		{[]string{voiceXChaChaMode, voiceAESMode}, voiceAESMode},
		{[]string{voiceXChaChaMode}, voiceXChaChaMode},
	} {
		got, err := selectVoiceMode(c.modes)
		if err != nil || got != c.want {
			t.Fatalf("mode %q %v", got, err)
		}
	}
	for _, mode := range []string{"xsalsa20_poly1305", "aead_aes256_gcm", "xsalsa20_poly1305_lite_rtpsize", ""} {
		if _, err := selectVoiceMode([]string{mode}); err == nil {
			t.Fatalf("legacy mode accepted %q", mode)
		}
		if _, err := voiceAEAD(mode, make([]byte, 32)); err == nil {
			t.Fatalf("legacy AEAD constructed %q", mode)
		}
	}
}
func TestVoiceAEADRoundTripAndAuthentication(t *testing.T) {
	for _, mode := range []string{voiceAESMode, voiceXChaChaMode} {
		t.Run(mode, func(t *testing.T) {
			aead, err := voiceAEAD(mode, bytes.Repeat([]byte{7}, 32))
			if err != nil {
				t.Fatal(err)
			}
			frame := []byte{0xaa, 0xbb, 0xcc, 0xdd}
			dst := make([]byte, 0, 2048)
			wire, err := sealVoiceRTP(dst, aead, 42, 65535, math.MaxUint32, 1234, frame)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(wire[len(wire)-4:], []byte{0, 0, 4, 0xd2}) {
				t.Fatalf("nonce suffix must equal first4 nonce bytes: %x", wire)
			}
			plain, ssrc, seq, ts, err := openVoiceRTP(make([]byte, 2048), aead, wire)
			if err != nil || !bytes.Equal(plain, frame) || ssrc != 42 || seq != 65535 || ts != math.MaxUint32 {
				t.Fatalf("RTP roundtrip: %x %d %d %d %v", plain, ssrc, seq, ts, err)
			}
			for _, index := range []int{2, 4, 8, 12, len(wire) - 1} {
				bad := append([]byte(nil), wire...)
				bad[index] ^= 1
				if _, _, _, _, err := openVoiceRTP(make([]byte, 2048), aead, bad); err == nil {
					t.Fatalf("tampering at %d authenticated", index)
				}
			}
			if _, err := sealVoiceRTP(dst, aead, 42, 0, 0, uint64(math.MaxUint32)+1, frame); err == nil {
				t.Fatal("nonce wrapped")
			}
			if _, err := sealVoiceRTP(make([]byte, 0, 12), aead, 42, 0, 0, 0, frame); err == nil {
				t.Fatal("undersized output accepted")
			}
		})
	}
}
func TestVoiceAEADAuthenticatedExtensionsAndPadding(t *testing.T) {
	for _, mode := range []string{voiceAESMode, voiceXChaChaMode} {
		t.Run(mode, func(t *testing.T) {
			aead, err := voiceAEAD(mode, make([]byte, 32))
			if err != nil {
				t.Fatal(err)
			}
			// RTP X+P+one CSRC; extension preamble remains AAD, its four-byte
			// extension body is encrypted and removed only after authentication.
			header := []byte{0xb1, 120, 0, 9, 0, 0, 0, 10, 0, 0, 0, 11, 0, 0, 0, 12, 0xbe, 0xde, 0, 1}
			payload := []byte{0x10, 0xff, 0, 0, 0xaa, 0xbb, 0, 2}
			nonce := make([]byte, aead.NonceSize())
			binary.BigEndian.PutUint32(nonce[:4], 7)
			wire := append([]byte(nil), header...)
			wire = aead.Seal(wire, nonce, payload, header)
			wire = append(wire, nonce[:4]...)
			plain, ssrc, seq, ts, err := openVoiceRTP(make([]byte, 2048), aead, wire)
			if err != nil || !bytes.Equal(plain, []byte{0xaa, 0xbb}) || ssrc != 11 || seq != 9 || ts != 10 {
				t.Fatalf("extension decode: %x %v", plain, err)
			}
			bad := append([]byte(nil), wire...)
			bad[19] = 2
			if _, _, _, _, err := openVoiceRTP(make([]byte, 2048), aead, bad); err == nil {
				t.Fatal("extension length not authenticated")
			}
			for length := range len(header) {
				if _, _, _, _, err := openVoiceRTP(make([]byte, 2048), aead, wire[:length]); err == nil {
					t.Fatalf("truncated RTP accepted %d", length)
				}
			}
		})
	}
}
func TestVoiceOpusFramingAndDuration(t *testing.T) {
	valid := [][]byte{
		{0xf8, 0xff, 0xfe},                // CELT single 20 ms frame
		{0x08, 0xaa},                      // SILK single20
		{0x91, 0xaa, 0xbb},                // CELT two10ms CBR
		{0x92, 1, 0xaa, 0xbb},             // CELT two10ms VBR
		{0x83, 8, 1, 2, 3, 4, 5, 6, 7, 8}, // eight2.5ms CBR
		{0x93, 0x42, 1, 0xaa, 0xbb, 0},    // two10ms CBR +1byte padding
	}
	for _, p := range valid {
		if err := ValidateOpusPacket(p, 1275); err != nil {
			t.Fatalf("valid packet %x: %v", p, err)
		}
	}
	invalid := [][]byte{nil, {}, {0x00, 0xaa}, {0x10, 0xaa}, {0x91, 1}, {0x92}, {0x92, 252}, {0x92, 4, 1}, {0x83}, {0x83, 0}, {0x83, 49}, {0x93, 0x42, 255}, {0x93, 0x42, 4, 1}, {0x93, 0x82, 20, 1}}
	for _, p := range invalid {
		if ValidateOpusPacket(p, 1275) == nil {
			t.Fatalf("invalid packet accepted %x", p)
		}
	}
	if ValidateOpusPacket([]byte{0xf8, 0xff, 0xfe}, 2) == nil {
		t.Fatal("packet byte bound ignored")
	}
	if err := validateOpusPacket([]byte{0x10, 0xaa}, 1275, 0); err != nil {
		t.Fatal("valid incoming40ms must not inherit producer20ms restriction", err)
	}
}
func TestVoiceUnavailableOrdinaryJoin(t *testing.T) {
	if VoiceAvailable() {
		t.Skip("ordinary-build boundary only")
	}
	s, _ := New("Bot token")
	if _, err := s.ChannelVoiceJoinContext(context.Background(), "1", "2", true, false, VoiceConfig{}); err != ErrVoiceUnavailable {
		t.Fatalf("ordinary join: %v", err)
	}
}
func TestVoiceAEADInPlaceSenderStorage(t *testing.T) {
	for _, mode := range []string{voiceAESMode, voiceXChaChaMode} {
		t.Run(mode, func(t *testing.T) {
			aead, err := voiceAEAD(mode, make([]byte, 32))
			if err != nil {
				t.Fatal(err)
			}
			storage := make([]byte, 2048)
			frame := []byte{1, 2, 3, 4}
			copy(storage[12:], frame)
			packet, err := sealVoiceRTP(storage[:0], aead, 1, 2, 3, 4, storage[12:16])
			if err != nil {
				t.Fatal(err)
			}
			plain, _, _, _, err := openVoiceRTP(make([]byte, 2048), aead, packet)
			if err != nil || !bytes.Equal(plain, frame) {
				t.Fatalf("in-place sender: %x %v", plain, err)
			}
		})
	}
}
