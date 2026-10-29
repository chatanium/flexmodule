// Package opus provides module-side ffmpeg encoding and bounded Opus frame
// reading. It has no Discord SDK or native Go library dependency.
package opus

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"time"
)

// Encode writes raw DCA frames (a little-endian int16 length followed by the
// Opus packet). Every packet is stereo, 48kHz, 20ms; bitrate is in kb/s.
// Cancellation terminates and reaps ffmpeg. dst must not block indefinitely;
// callers writing files must also close their file after Encode returns.
func Encode(ctx context.Context, input string, dst io.Writer, bitrate int) error {
	if bitrate < 8 || bitrate > 128 {
		return fmt.Errorf("Opus bitrate must be between 8 and 128 kb/s")
	}
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg executable is required in the module media image PATH (with libopus enabled): %w", err)
	}
	parentContext := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, path,
		"-nostdin", "-hide_banner", "-loglevel", "error", "-i", input,
		"-map", "0:a:0", "-vn", "-sn", "-dn", "-c:a", "libopus",
		"-ar", "48000", "-ac", "2", "-b:a", strconv.Itoa(bitrate*1000),
		"-application", "audio", "-frame_duration", "20", "-vbr", "on",
		"-compression_level", "10", "-packet_loss", "1", "-threads", "1",
		"-f", "ogg", "pipe:1")
	cmd.WaitDelay = 2 * time.Second
	var diagnostic diagnosticBuffer
	cmd.Stderr = &diagnostic
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open ffmpeg output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		stdout.Close()
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	reader := &oggReader{r: stdout}
	var encodeErr error
	var header [2]byte
	frames := 0
	for {
		packet, err := reader.next()
		if err != nil {
			if err != io.EOF {
				encodeErr = err
			}
			break
		}
		if err := ctx.Err(); err != nil {
			encodeErr = err
			break
		}
		binary.LittleEndian.PutUint16(header[:], uint16(len(packet)))
		if err := writeFull(dst, header[:]); err != nil {
			encodeErr = err
			break
		}
		if err := writeFull(dst, packet); err != nil {
			encodeErr = err
			break
		}
		frames++
	}
	if encodeErr == nil && frames == 0 {
		encodeErr = fmt.Errorf("ffmpeg emitted no Opus audio frames")
	}
	if encodeErr != nil {
		cancel()
	}
	stdout.Close()
	waitErr := cmd.Wait()
	if err := parentContext.Err(); err != nil {
		encodeErr = errors.Join(encodeErr, err)
	}
	if encodeErr != nil || waitErr != nil {
		return fmt.Errorf("ffmpeg Opus encoding failed: %w; stderr: %s", errors.Join(encodeErr, waitErr), diagnostic.data[:diagnostic.n])
	}
	return nil
}

func writeFull(w io.Writer, data []byte) error {
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}
	return err
}

// Keep stderr bounded while continuing to drain it, so the child cannot block
// when emitting diagnostics. Only accessed by the caller after Cmd.Wait.
type diagnosticBuffer struct {
	data [8192]byte
	n    int
}

func (b *diagnosticBuffer) Write(p []byte) (int, error) {
	n := len(p)
	b.n += copy(b.data[b.n:], p)
	return n, nil
}
