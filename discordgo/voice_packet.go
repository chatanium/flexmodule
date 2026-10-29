package discordgo

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"math"

	"golang.org/x/crypto/chacha20poly1305"
)

const voiceAESMode = "aead_aes256_gcm_rtpsize"
const voiceXChaChaMode = "aead_xchacha20_poly1305_rtpsize"
const voiceFrameIntervalSamples = 960

func selectVoiceMode(modes []string) (string, error) {
	for _, m := range modes {
		if m == voiceAESMode {
			return m, nil
		}
	}
	for _, m := range modes {
		if m == voiceXChaChaMode {
			return m, nil
		}
	}
	return "", ErrVoiceUnavailable
}
func voiceAEAD(mode string, key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("invalid voice transport key length")
	}
	switch mode {
	case voiceAESMode:
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		return cipher.NewGCM(block)
	case voiceXChaChaMode:
		return chacha20poly1305.NewX(key)
	default:
		return nil, ErrVoiceUnavailable
	}
}

// RTP-size AEAD authenticates the fixed header, CSRCs and extension preamble.
// Extension data itself is encrypted (RFC 3711 section 3.1), not additional data.
func voiceRTPHeader(p []byte) (aadLen, extensionBytes int, err error) {
	if len(p) < 12 || p[0]>>6 != 2 || p[1]&127 != 120 {
		return 0, 0, errors.New("invalid voice RTP header")
	}
	aadLen = 12 + 4*int(p[0]&15)
	if aadLen > len(p) {
		return 0, 0, errors.New("truncated RTP CSRC")
	}
	if p[0]&16 != 0 {
		if aadLen+4 > len(p) {
			return 0, 0, errors.New("truncated RTP extension")
		}
		extensionBytes = 4 * int(binary.BigEndian.Uint16(p[aadLen+2:aadLen+4]))
		aadLen += 4
	}
	return aadLen, extensionBytes, nil
}
func sealVoiceRTP(dst []byte, aead cipher.AEAD, ssrc uint32, seq uint16, timestamp uint32, counter uint64, frame []byte) ([]byte, error) {
	if counter > math.MaxUint32 {
		return nil, errors.New("voice transport nonce exhausted")
	}
	needed := 12 + len(frame) + aead.Overhead() + 4
	if cap(dst) < needed {
		return nil, errors.New("voice RTP output exceeds bound")
	}
	dst = dst[:12]
	dst[0] = 128
	dst[1] = 120
	binary.BigEndian.PutUint16(dst[2:4], seq)
	binary.BigEndian.PutUint32(dst[4:8], timestamp)
	binary.BigEndian.PutUint32(dst[8:12], ssrc)
	var nonce [24]byte
	binary.BigEndian.PutUint32(nonce[:4], uint32(counter))
	dst = aead.Seal(dst, nonce[:aead.NonceSize()], frame, dst[:12])
	dst = append(dst, nonce[:4]...)
	return dst, nil
}
func openVoiceRTP(dst []byte, aead cipher.AEAD, packet []byte) (frame []byte, ssrc uint32, seq uint16, timestamp uint32, err error) {
	header, ext, err := voiceRTPHeader(packet)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	if len(packet) < header+aead.Overhead()+4 {
		return nil, 0, 0, 0, errors.New("truncated voice ciphertext")
	}
	cipherEnd := len(packet) - 4
	plainLen := cipherEnd - header - aead.Overhead()
	if cap(dst) < plainLen {
		return nil, 0, 0, 0, errors.New("voice decrypt output exceeds bound")
	}
	var nonce [24]byte
	copy(nonce[:4], packet[cipherEnd:])
	plain, err := aead.Open(dst[:0], nonce[:aead.NonceSize()], packet[header:cipherEnd], packet[:header])
	if err != nil {
		return nil, 0, 0, 0, errors.New("voice transport authentication failed")
	}
	if ext > len(plain) {
		return nil, 0, 0, 0, errors.New("truncated authenticated RTP extension")
	}
	plain = plain[ext:]
	if packet[0]&32 != 0 {
		if len(plain) == 0 {
			return nil, 0, 0, 0, errors.New("missing RTP padding")
		}
		padding := int(plain[len(plain)-1])
		if padding == 0 || padding > len(plain) {
			return nil, 0, 0, 0, errors.New("invalid RTP padding")
		}
		plain = plain[:len(plain)-padding]
	}
	if len(plain) == 0 {
		return nil, 0, 0, 0, errors.New("empty voice frame")
	}
	return plain, binary.BigEndian.Uint32(packet[8:12]), binary.BigEndian.Uint16(packet[2:4]), binary.BigEndian.Uint32(packet[4:8]), nil
}
