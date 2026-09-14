// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imageconfig

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func tiffFixture(order binary.ByteOrder, typ uint16) []byte {
	b := make([]byte, 38)
	copy(b, "II\x2a\x00")
	if order == binary.BigEndian {
		copy(b, "MM\x00\x2a")
	}
	order.PutUint32(b[4:], 8)
	order.PutUint16(b[8:], 2)
	for i, v := range []uint32{4, 5} {
		p := b[10+i*12:]
		order.PutUint16(p, uint16(256+i))
		order.PutUint16(p[2:], typ)
		order.PutUint32(p[4:], 1)
		if typ == 3 {
			order.PutUint16(p[8:], uint16(v))
		} else {
			order.PutUint32(p[8:], v)
		}
	}
	return b
}

func bmpFixture(size uint32, bits uint16, topDown bool) []byte {
	palette := 0
	if bits <= 8 {
		palette = 4 * (1 << bits)
	}
	b := make([]byte, 14+int(size)+palette)
	copy(b, "BM")
	o := binary.LittleEndian
	o.PutUint32(b[10:], uint32(len(b)))
	o.PutUint32(b[14:], size)
	o.PutUint32(b[18:], 4)
	o.PutUint32(b[22:], 5)
	if topDown {
		o.PutUint32(b[22:], 0xfffffffb)
	}
	o.PutUint16(b[26:], 1)
	o.PutUint16(b[28:], bits)
	return b
}

func webpFixture(kind string) []byte {
	n := 10
	if kind == "VP8L" {
		n = 5
	}
	b := make([]byte, 20+n+(n&1))
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WEBP")
	copy(b[12:], kind)
	binary.LittleEndian.PutUint32(b[16:], uint32(n))
	p := b[20:]
	switch kind {
	case "VP8 ":
		copy(p[3:], "\x9d\x01\x2a")
		binary.LittleEndian.PutUint16(p[6:], 4)
		binary.LittleEndian.PutUint16(p[8:], 5)
	case "VP8L":
		p[0] = 0x2f
		binary.LittleEndian.PutUint32(p[1:], 3|(4<<14))
	case "VP8X":
		p[4] = 3
		p[7] = 4
	}
	return b
}

func TestMetadataDimensions(t *testing.T) {
	fixtures := map[string][]byte{
		"tiff_le_short": tiffFixture(binary.LittleEndian, 3), "tiff_be_short": tiffFixture(binary.BigEndian, 3),
		"tiff_le_long": tiffFixture(binary.LittleEndian, 4), "tiff_be_long": tiffFixture(binary.BigEndian, 4),
		"bmp_info": bmpFixture(40, 24, false), "bmp_v4": bmpFixture(108, 32, true), "bmp_v5": bmpFixture(124, 8, false),
		"webp_lossy": webpFixture("VP8 "), "webp_lossless": webpFixture("VP8L"), "webp_extended": webpFixture("VP8X"),
	}
	for name, b := range fixtures {
		t.Run(name, func(t *testing.T) {
			r := bytes.NewReader(b)
			_, _ = r.Seek(1, io.SeekStart)
			cfg, format, err := Decode(r)
			if err != nil || cfg.Width != 4 || cfg.Height != 5 || format != name[:len(format)] || format == "" {
				t.Fatalf("%+v %q %v", cfg, format, err)
			}
			if pos, _ := r.Seek(0, io.SeekCurrent); pos != 1 {
				t.Fatal("changed source position")
			}
			// Metadata must itself be complete, even when no pixels are needed.
			required := len(b)
			if format == "tiff" {
				required -= 4
			}
			if name == "webp_lossless" {
				required--
			}
			for n := 0; n < required; n++ {
				if _, _, err := Decode(bytes.NewReader(b[:n])); err == nil {
					t.Fatalf("accepted prefix of %d bytes", n)
				}
			}
		})
	}
}

func TestRejectMalformedMetadata(t *testing.T) {
	cases := []struct {
		name   string
		b      []byte
		mutate func([]byte)
	}{
		{"tiff_count", tiffFixture(binary.LittleEndian, 4), func(b []byte) { binary.LittleEndian.PutUint32(b[14:], 0xffffffff) }},
		{"tiff_duplicate", tiffFixture(binary.LittleEndian, 4), func(b []byte) { binary.LittleEndian.PutUint16(b[22:], 256) }},
		{"tiff_zero", tiffFixture(binary.LittleEndian, 4), func(b []byte) { binary.LittleEndian.PutUint32(b[18:], 0) }},
		{"tiff_type", tiffFixture(binary.LittleEndian, 4), func(b []byte) { binary.LittleEndian.PutUint16(b[12:], 7) }},
		{"tiff_offset", tiffFixture(binary.LittleEndian, 4), func(b []byte) { binary.LittleEndian.PutUint32(b[4:], 0) }},
		{"bmp_header", bmpFixture(40, 24, false), func(b []byte) { binary.LittleEndian.PutUint32(b[14:], 0xffffffff) }},
		{"bmp_planes", bmpFixture(40, 24, false), func(b []byte) { binary.LittleEndian.PutUint16(b[26:], 2) }},
		{"bmp_negative_width", bmpFixture(40, 24, false), func(b []byte) { binary.LittleEndian.PutUint32(b[18:], 0xffffffff) }},
		{"bmp_palette", bmpFixture(40, 8, false), func(b []byte) { binary.LittleEndian.PutUint32(b[46:], 257) }},
		{"webp_riff", webpFixture("VP8L"), func(b []byte) { copy(b[8:], "WAVE") }},
		{"webp_chunk", webpFixture("VP8L"), func(b []byte) { binary.LittleEndian.PutUint32(b[16:], 0xffffffff) }},
		{"webp_version", webpFixture("VP8L"), func(b []byte) { b[24] |= 0x20 }},
		{"webp_signature", webpFixture("VP8 "), func(b []byte) { b[23] = 0 }},
		{"webp_vp8x_size", webpFixture("VP8X"), func(b []byte) { binary.LittleEndian.PutUint32(b[16:], 9) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mutate(tc.b)
			if _, _, err := Decode(bytes.NewReader(tc.b)); err == nil {
				t.Fatal("accepted malformed metadata")
			}
		})
	}
}

// Fail before any oversized read; the fixture declares an enormous unrelated
// metadata payload, which must never be read or allocated.
func TestTIFFIgnoresUnrelatedPayload(t *testing.T) {
	b := tiffFixture(binary.LittleEndian, 4)
	b = append(b[:34], make([]byte, 16)...)
	binary.LittleEndian.PutUint16(b[8:], 3)
	binary.LittleEndian.PutUint16(b[34:], 34675)
	binary.LittleEndian.PutUint16(b[36:], 7)
	binary.LittleEndian.PutUint32(b[38:], 0xffffffff)
	binary.LittleEndian.PutUint32(b[42:], 0xffffffff)
	r := &boundedMetadataReader{Reader: bytes.NewReader(b)}
	cfg, _, err := Decode(r)
	if err != nil || cfg.Width != 4 || cfg.Height != 5 {
		t.Fatalf("%+v %v", cfg, err)
	}
	if r.calls > 6 {
		t.Fatalf("unexpected reads: %d", r.calls)
	}
}

type boundedMetadataReader struct {
	*bytes.Reader
	calls int
}

func (r *boundedMetadataReader) ReadAt(p []byte, off int64) (int, error) {
	r.calls++
	if len(p) > 1024 || r.calls > 70000 {
		return 0, errors.New("metadata read budget exceeded")
	}
	return r.Reader.ReadAt(p, off)
}

func TestWebPSkipsUnknownChunkByOffset(t *testing.T) {
	const offset = 1 << 20
	original := webpFixture("VP8L")
	prefix := make([]byte, 20)
	copy(prefix, original[:12])
	binary.LittleEndian.PutUint32(prefix[4:], offset+uint32(len(original))-20)
	copy(prefix[12:], "JUNK")
	binary.LittleEndian.PutUint32(prefix[16:], offset-20)
	sentinel := errors.New("distant chunk")
	r := &offsetReader{Reader: bytes.NewReader(prefix), offset: offset, err: sentinel}
	_, _, err := Decode(r)
	if !errors.Is(err, sentinel) || !r.called {
		t.Fatalf("random access=%v err=%v", r.called, err)
	}
}

func FuzzMetadata(f *testing.F) {
	for _, b := range [][]byte{tiffFixture(binary.LittleEndian, 4), tiffFixture(binary.BigEndian, 3), bmpFixture(40, 24, false), webpFixture("VP8 "), webpFixture("VP8L"), webpFixture("VP8X")} {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) < 4 {
			return
		}
		magic := string(b[:4])
		if magic != "RIFF" && magic != "II\x2a\x00" && magic != "MM\x00\x2a" && string(b[:2]) != "BM" {
			return
		}
		_, _, _ = Decode(&boundedMetadataReader{Reader: bytes.NewReader(b)})
	})
}

func TestWebPChunkTraversalLimit(t *testing.T) {
	b := make([]byte, 12+4096*8+18)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WEBP")
	for i := 0; i < 4096; i++ {
		copy(b[12+i*8:], "JUNK")
	}
	copy(b[12+4096*8:], webpFixture("VP8L")[12:])
	r := &boundedMetadataReader{Reader: bytes.NewReader(b)}
	if _, _, err := Decode(r); !errors.Is(err, errMetadata) {
		t.Fatalf("expected traversal limit, got %v", err)
	}
	if r.calls > 4098 {
		t.Fatalf("too many reads: %d", r.calls)
	}
}

// Offset 0 is consumed by Decode's magic read, so injecting there never
// reaches readBMP or readWebP. Use the first offset each format reader
// requests on its own, and assert the reader was actually entered.
func TestMetadataPreservesReadCause(t *testing.T) {
	for _, tc := range []struct {
		format string
		b      []byte
		offset int64
	}{
		{"bmp", bmpFixture(40, 24, false), 18},
		{"webp", webpFixture("VP8L"), 12},
	} {
		t.Run(tc.format, func(t *testing.T) {
			sentinel := errors.New("source unavailable")
			r := &offsetReader{Reader: bytes.NewReader(tc.b), offset: tc.offset, err: sentinel}
			_, format, err := Decode(r)
			if !errors.Is(err, sentinel) || !r.called || format != tc.format {
				t.Fatalf("format=%q random read=%v err=%v", format, r.called, err)
			}
		})
	}
}

// A writer may omit the even-padding byte after a final odd-sized chunk, and
// may cover trailing bytes in the RIFF size. Neither hides the dimensions.
func TestWebPAcceptsUnpaddedFinalChunk(t *testing.T) {
	padded := webpFixture("VP8L")
	unpadded := append([]byte(nil), padded[:len(padded)-1]...)
	binary.LittleEndian.PutUint32(unpadded[4:], uint32(len(unpadded)-8))

	oddContainer := append(append([]byte(nil), padded...), 1, 2, 3)
	binary.LittleEndian.PutUint32(oddContainer[4:], uint32(len(oddContainer)-8))

	for name, b := range map[string][]byte{"unpadded_final_chunk": unpadded, "odd_container_size": oddContainer} {
		t.Run(name, func(t *testing.T) {
			cfg, format, err := Decode(bytes.NewReader(b))
			if err != nil || format != "webp" || cfg.Width != 4 || cfg.Height != 5 {
				t.Fatalf("config=%+v format=%q err=%v", cfg, format, err)
			}
		})
	}
}

// The container spec says of every VP8X reserved field: "MUST be 0. Readers
// MUST ignore this field." A writer that sets one still describes a readable
// canvas, and x/image reads such files -- both its config and a full decode.
func TestWebPIgnoresVP8XReservedFields(t *testing.T) {
	for name, mutate := range map[string]func([]byte){
		"rsv_high_bits":  func(b []byte) { b[20] |= 0xc0 },
		"r_low_bit":      func(b []byte) { b[20] |= 0x01 },
		"reserved_block": func(b []byte) { b[21], b[22], b[23] = 1, 2, 3 },
	} {
		t.Run(name, func(t *testing.T) {
			b := webpFixture("VP8X")
			mutate(b)
			cfg, format, err := Decode(bytes.NewReader(b))
			if err != nil || format != "webp" || cfg.Width != 4 || cfg.Height != 5 {
				t.Fatalf("config=%+v format=%q err=%v", cfg, format, err)
			}
		})
	}
}

// Relaxing the padding requirement must not let a chunk payload escape the
// container it declares.
func TestWebPRejectsChunkPayloadBeyondContainer(t *testing.T) {
	b := webpFixture("VP8L")
	binary.LittleEndian.PutUint32(b[16:], uint32(len(b)))
	if _, _, err := Decode(bytes.NewReader(b)); !errors.Is(err, errMetadata) {
		t.Fatalf("accepted chunk reaching past the container: %v", err)
	}
}
