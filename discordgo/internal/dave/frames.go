//go:build voice && cgo

package dave

/*
#include "bridge.h"
*/
import "C"

import (
	"context"
	"runtime"
	"time"
)

func (c *Context) maxCiphertextSize(frameBytes int) (int, error) {
	if frameBytes <= 0 || frameBytes > c.cfg.MaxFrameBytes {
		return 0, ErrBounds
	}
	n := C.daveEncryptorGetMaxCiphertextByteSize(c.encryptor, C.DAVE_MEDIA_TYPE_AUDIO, C.size_t(frameBytes))
	if n < C.size_t(frameBytes) || n > C.size_t(c.cfg.MaxEncryptedFrameBytes) {
		return 0, ErrBounds
	}
	return int(n), nil
}

// MaxCiphertextSize can size a reusable caller buffer before media is ready.
func (c *Context) MaxCiphertextSize(frameBytes int) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, ErrClosed
	}
	return c.maxCiphertextSize(frameBytes)
}

func (c *Context) maxPlaintextSize(senderID string, frameBytes int) (int, error) {
	if frameBytes <= 0 || frameBytes > c.cfg.MaxEncryptedFrameBytes {
		return 0, ErrBounds
	}
	if _, known := c.participants[senderID]; !known {
		return 0, ErrUnknownSender
	}
	c.expireReceivers(time.Now())
	r := c.receivers[senderID]
	if r == nil {
		return 0, ErrNotReady
	}
	n := C.daveDecryptorGetMaxPlaintextByteSize(r.handle, C.DAVE_MEDIA_TYPE_AUDIO, C.size_t(frameBytes))
	// Pinned libdave returns encryptedFrameSize as its upper bound. The scratch
	// buffer may therefore exceed MaxFrameBytes, but authenticated Opus output
	// is independently checked against MaxFrameBytes before release.
	if n == 0 || n > C.size_t(c.cfg.MaxEncryptedFrameBytes) {
		return 0, ErrBounds
	}
	return int(n), nil
}

func (c *Context) MaxPlaintextSize(senderID string, frameBytes int) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, ErrClosed
	}
	return c.maxPlaintextSize(senderID, frameBytes)
}

func (c *Context) EncryptOpus(ctx context.Context, ssrc uint32, opus, dst []byte) (int, error) {
	if err := c.enter(ctx); err != nil {
		return 0, err
	}
	defer c.mu.Unlock()
	if !c.active || !bool(C.daveEncryptorHasKeyRatchet(c.encryptor)) ||
		bool(C.daveEncryptorIsPassthroughMode(c.encryptor)) {
		return 0, ErrNotReady
	}
	n, err := c.maxCiphertextSize(len(opus))
	if err != nil {
		return 0, err
	}
	if len(dst) < n {
		return 0, ErrBuffer
	}
	if c.ssrcBound && c.ssrc != ssrc {
		return 0, ErrInput
	}
	if !c.ssrcBound {
		C.daveEncryptorAssignSsrcToCodec(c.encryptor, C.uint32_t(ssrc), C.DAVE_CODEC_OPUS)
		c.ssrc, c.ssrcBound = ssrc, true
	}
	var written C.size_t
	result := C.daveEncryptorEncrypt(c.encryptor, C.DAVE_MEDIA_TYPE_AUDIO, C.uint32_t(ssrc),
		pointer(opus), C.size_t(len(opus)), pointer(dst), C.size_t(n), &written)
	runtime.KeepAlive(opus)
	runtime.KeepAlive(dst)
	if result != C.DAVE_ENCRYPTOR_RESULT_CODE_SUCCESS || written == 0 || written > C.size_t(n) {
		clear(dst[:n])
		return 0, ErrNative
	}
	return int(written), nil
}

func (c *Context) DecryptOpus(ctx context.Context, senderID string, encrypted, dst []byte) (int, error) {
	if err := c.enter(ctx); err != nil {
		return 0, err
	}
	defer c.mu.Unlock()
	n, err := c.maxPlaintextSize(senderID, len(encrypted))
	if err != nil {
		return 0, err
	}
	if len(dst) < n {
		return 0, ErrBuffer
	}
	// libdave has an intentional SFU synthesized-silence exception. This strict
	// authenticated-Opus API never publishes an unauthenticated frame, including
	// that exception. The transport may discard it, not turn it into media.
	if len(encrypted) == 3 && encrypted[0] == 0xf8 && encrypted[1] == 0xff && encrypted[2] == 0xfe {
		return 0, ErrAuthentication
	}
	r := c.receivers[senderID]
	var written C.size_t
	result := C.daveDecryptorDecrypt(r.handle, C.DAVE_MEDIA_TYPE_AUDIO,
		pointer(encrypted), C.size_t(len(encrypted)), pointer(dst), C.size_t(n), &written)
	runtime.KeepAlive(encrypted)
	runtime.KeepAlive(dst)
	if result != C.DAVE_DECRYPTOR_RESULT_CODE_SUCCESS || written == 0 ||
		written > C.size_t(n) || written > C.size_t(c.cfg.MaxFrameBytes) {
		clear(dst[:n])
		return 0, ErrAuthentication
	}
	return int(written), nil
}
