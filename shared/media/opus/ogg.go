package opus

import (
	"encoding/binary"
	"fmt"
	"io"
)

// oggReader handles a single logical Opus stream, including packets continued
// across pages. Page and packet storage are bounded independently of input size.
type oggReader struct {
	r        io.Reader
	page     [65307]byte
	packet   []byte
	segments []byte
	data     []byte
	index    int
	offset   int
	serial   uint32
	sequence uint32
	started  bool
	end      bool
	headers  int
}

func (r *oggReader) next() ([]byte, error) {
	for {
		if r.index == len(r.segments) {
			if r.end {
				if len(r.packet) != 0 || r.headers != 2 {
					return nil, fmt.Errorf("incomplete Ogg Opus stream")
				}
				return nil, io.EOF
			}
			if err := r.readPage(); err != nil {
				return nil, err
			}
		}
		for r.index < len(r.segments) {
			n := int(r.segments[r.index])
			r.index++
			limit := 1275 // One 20ms Opus packet, as emitted by our encoder.
			if r.headers < 2 {
				limit = 1 << 20
			}
			if len(r.packet)+n > limit {
				return nil, fmt.Errorf("Ogg Opus packet exceeds %d bytes", limit)
			}
			r.packet = append(r.packet, r.data[r.offset:r.offset+n]...)
			r.offset += n
			if n == 255 {
				continue
			}
			packet := r.packet
			r.packet = r.packet[:0]
			switch r.headers {
			case 0:
				if len(packet) != 19 || string(packet[:8]) != "OpusHead" || packet[8] != 1 || packet[9] != 2 || packet[18] != 0 {
					return nil, fmt.Errorf("expected stereo mapping-family-zero OpusHead")
				}
				r.headers++
			case 1:
				if len(packet) < 16 || string(packet[:8]) != "OpusTags" {
					return nil, fmt.Errorf("expected OpusTags")
				}
				r.headers++
			default:
				if len(packet) == 0 {
					return nil, fmt.Errorf("empty Opus packet")
				}
				return packet, nil
			}
		}
	}
}

func (r *oggReader) readPage() error {
	header := r.page[:27]
	if _, err := io.ReadFull(r.r, header); err != nil {
		if err == io.EOF {
			return io.ErrUnexpectedEOF // A successful stream must include EOS.
		}
		return err
	}
	if string(header[:4]) != "OggS" || header[4] != 0 || header[5]&^byte(7) != 0 {
		return fmt.Errorf("invalid Ogg page header")
	}
	flags := header[5]
	serial := binary.LittleEndian.Uint32(header[14:18])
	sequence := binary.LittleEndian.Uint32(header[18:22])
	if !r.started {
		if flags&2 == 0 || flags&1 != 0 || sequence != 0 {
			return fmt.Errorf("invalid first Ogg page")
		}
		r.serial = serial
		r.started = true
	} else if flags&2 != 0 || serial != r.serial || sequence != r.sequence+1 {
		return fmt.Errorf("discontinuous or multiplexed Ogg stream")
	}
	if (flags&1 != 0) != (len(r.packet) != 0) {
		return fmt.Errorf("invalid Ogg packet continuation")
	}
	r.sequence = sequence
	r.end = flags&4 != 0
	n := int(header[26])
	r.segments = r.page[27 : 27+n]
	if _, err := io.ReadFull(r.r, r.segments); err != nil {
		return unexpectedEOF(err)
	}
	size := 0
	for _, segment := range r.segments {
		size += int(segment)
	}
	r.data = r.page[27+n : 27+n+size]
	if _, err := io.ReadFull(r.r, r.data); err != nil {
		return unexpectedEOF(err)
	}
	page := r.page[:27+n+size]
	want := binary.LittleEndian.Uint32(page[22:26])
	clear(page[22:26])
	var crc uint32
	for _, b := range page {
		crc = crc<<8 ^ oggCRC[byte(crc>>24)^b]
	}
	if crc != want {
		return fmt.Errorf("Ogg page checksum mismatch")
	}
	r.index, r.offset = 0, 0
	return nil
}

var oggCRC = func() (table [256]uint32) {
	for i := range table {
		v := uint32(i) << 24
		for range 8 {
			if v&0x80000000 != 0 {
				v = v<<1 ^ 0x04c11db7
			} else {
				v <<= 1
			}
		}
		table[i] = v
	}
	return table
}()
