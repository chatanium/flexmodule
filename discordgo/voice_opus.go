package discordgo

import "errors"

var ErrInvalidOpusPacket = errors.New("invalid OPUS_48KHZ_20MS packet")

// ValidateOpusPacket validates RFC 6716 section 3 packet framing without decoding
// audio. Only a complete 960-sample packet at 48 kHz is accepted. Frame sizes,
// VBR lengths, code-3 padding and total duration are checked, not caller metadata.
func ValidateOpusPacket(p []byte, maxBytes int) error {
	return validateOpusPacket(p, maxBytes, 960)
}
func validateOpusPacket(p []byte, maxBytes, expectedSamples int) error {
	bad := ErrInvalidOpusPacket
	if maxBytes <= 0 || len(p) == 0 || len(p) > maxBytes {
		return bad
	}
	config := int(p[0] >> 3)
	var samples int
	switch {
	case config >= 16:
		samples = 120 << (config & 3)
	case config >= 12:
		samples = 480 << (config & 1)
	default:
		samples = []int{480, 960, 1920, 2880}[config&3]
	}
	code := p[0] & 3
	count := 1
	pos := 1
	end := len(p)
	vbr := false
	switch code {
	case 1:
		count = 2
	case 2:
		count = 2
		vbr = true
	case 3:
		if pos == end {
			return bad
		}
		flags := p[pos]
		pos++
		count = int(flags & 63)
		vbr = flags&128 != 0
		if count == 0 || count > 48 {
			return bad
		}
		if flags&64 != 0 {
			for {
				if pos >= end {
					return bad
				}
				n := int(p[pos])
				pos++
				if n == 255 {
					end -= 254
				} else {
					end -= n
				}
				if end < pos {
					return bad
				}
				if n != 255 {
					break
				}
			}
		}
	}
	if count*samples > 5760 || (expectedSamples > 0 && count*samples != expectedSamples) {
		return bad
	}
	if !vbr {
		n := end - pos
		if n < 0 || n%count != 0 || n/count > 1275 {
			return bad
		}
		return nil
	}
	remaining := end - pos
	for range count - 1 {
		if pos >= end {
			return bad
		}
		n := int(p[pos])
		pos++
		remaining--
		if n >= 252 {
			if pos >= end {
				return bad
			}
			n += 4 * int(p[pos])
			pos++
			remaining--
		}
		if n > 1275 {
			return bad
		}
		remaining -= n
		if remaining < 0 {
			return bad
		}
	}
	if remaining > 1275 {
		return bad
	}
	return nil
}
