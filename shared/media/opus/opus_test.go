package opus

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func testPage(sequence uint32, flags byte, segments []byte, data []byte) []byte {
	page := make([]byte, 27+len(segments)+len(data))
	copy(page, "OggS")
	page[5] = flags
	binary.LittleEndian.PutUint32(page[14:18], 42)
	binary.LittleEndian.PutUint32(page[18:22], sequence)
	page[26] = byte(len(segments))
	copy(page[27:], segments)
	copy(page[27+len(segments):], data)
	var crc uint32
	for _, b := range page {
		crc = crc<<8 ^ oggCRC[byte(crc>>24)^b]
	}
	binary.LittleEndian.PutUint32(page[22:26], crc)
	return page
}

func testHeaders() []byte {
	head := make([]byte, 19)
	copy(head, "OpusHead")
	head[8], head[9] = 1, 2
	tags := make([]byte, 16)
	copy(tags, "OpusTags")
	return append(testPage(0, 2, []byte{19}, head), testPage(1, 0, []byte{16}, tags)...)
}

func TestOggContinuedPacket(t *testing.T) {
	packet := bytes.Repeat([]byte{0x78}, 300)
	input := testHeaders()
	input = append(input, testPage(2, 0, []byte{255}, packet[:255])...)
	input = append(input, testPage(3, 5, []byte{45, 2}, append(append([]byte{}, packet[255:]...), 0xf8, 0xff))...)
	r := &oggReader{r: bytes.NewReader(input)}
	got, err := r.next()
	if err != nil || !bytes.Equal(got, packet) {
		t.Fatalf("continued packet = %x, %v", got, err)
	}
	got, err = r.next()
	if err != nil || !bytes.Equal(got, []byte{0xf8, 0xff}) {
		t.Fatalf("following packet = %x, %v", got, err)
	}
	if _, err := r.next(); err != io.EOF {
		t.Fatalf("EOS = %v", err)
	}
}

func TestOggContinuedTagsAndExactLacing(t *testing.T) {
	head := make([]byte, 19)
	copy(head, "OpusHead")
	head[8], head[9] = 1, 2
	tags := make([]byte, 300)
	copy(tags, "OpusTags")
	binary.LittleEndian.PutUint32(tags[8:12], 284) // Vendor consumes the rest, except the comment count.
	packet := bytes.Repeat([]byte{0x78}, 255)
	input := testPage(0, 2, []byte{19}, head)
	input = append(input, testPage(1, 0, []byte{255}, tags[:255])...)
	input = append(input, testPage(2, 1, []byte{45}, tags[255:])...)
	input = append(input, testPage(3, 4, []byte{255, 0}, packet)...)
	r := &oggReader{r: bytes.NewReader(input)}
	got, err := r.next()
	if err != nil || !bytes.Equal(got, packet) {
		t.Fatalf("packet following continued tags = %x, %v", got, err)
	}
	if _, err := r.next(); err != io.EOF {
		t.Fatalf("EOS = %v", err)
	}
}

func TestOggRejectsMalformedStreams(t *testing.T) {
	valid := append(testHeaders(), testPage(2, 4, []byte{2}, []byte{0xf8, 0xff})...)
	badCRC := append([]byte{}, valid...)
	badCRC[len(badCRC)-1] ^= 1
	cases := map[string][]byte{
		"checksum":             badCRC,
		"missing EOS":          append(testHeaders(), testPage(2, 0, []byte{2}, []byte{0xf8, 0xff})...),
		"missing continuation": append(testHeaders(), testPage(2, 5, []byte{2}, []byte{0xf8, 0xff})...),
		"unfinished packet":    append(testHeaders(), testPage(2, 4, []byte{255}, bytes.Repeat([]byte{1}, 255))...),
		"truncated data":       valid[:len(valid)-1],
		"sequence gap":         append(testHeaders(), testPage(4, 4, []byte{2}, []byte{0xf8, 0xff})...),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			r := &oggReader{r: bytes.NewReader(input)}
			for range 3 {
				_, err := r.next()
				if err != nil {
					if err == io.EOF {
						t.Fatal("malformed stream returned clean EOF")
					}
					return
				}
			}
			t.Fatal("malformed stream accepted")
		})
	}
}

func TestDCAFrameBoundsAndTruncation(t *testing.T) {
	for _, input := range [][]byte{
		{0, 0}, {0xff, 0xff}, {2, 0, 1},
		append([]byte("DCA1"), 0xff, 0xff, 0xff, 0x7f),
	} {
		if _, err := NewDecoder(bytes.NewReader(input)).OpusFrame(context.Background()); err == nil || err == io.EOF {
			t.Fatalf("invalid input %x returned %v", input, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewDecoder(bytes.NewReader(nil)).OpusFrame(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read = %v", err)
	}
}

func TestDCAOldMetadataAndRawFrames(t *testing.T) {
	metadata := []byte(`{"opus":{"channels":2,"sample_rate":48000}}`)
	old := append([]byte("DCA1"), 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(old[4:8], uint32(len(metadata)))
	old = append(old, metadata...)
	raw := []byte{2, 0, 0xf8, 0xff}
	for _, input := range [][]byte{raw, append(old, raw...)} {
		r := NewDecoder(bytes.NewReader(input))
		got, err := r.OpusFrame(context.Background())
		if err != nil || !bytes.Equal(got, raw[2:]) {
			t.Fatalf("frame = %x, %v", got, err)
		}
		if _, err := r.OpusFrame(context.Background()); err != io.EOF {
			t.Fatalf("frame EOF = %v", err)
		}
	}
}
