// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imageconfig

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"testing"
)

// A far IFD offset must reach the source as a random read, without buffering
// the intervening bytes. Keep it small enough to safely test a regression.
func TestDecodeTIFFFarOffsetUsesRandomAccess(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		t.Run(order.String(), func(t *testing.T) {
			const offset = 1 << 20
			data := make([]byte, 8)
			order.PutUint16(data[:2], 0x4949)
			if order == binary.BigEndian {
				copy(data[:2], "MM")
			}
			order.PutUint16(data[2:4], 42)
			order.PutUint32(data[4:8], offset)
			wantErr := errors.New("source offset beyond EOF")
			r := &offsetReader{Reader: bytes.NewReader(data), offset: offset, err: wantErr}
			_, format, err := Decode(r)
			if format != "tiff" || !errors.Is(err, wantErr) || !r.called {
				t.Fatalf("format=%q err=%v random read=%v; want TIFF source error", format, err, r.called)
			}
		})
	}
}

type offsetReader struct {
	*bytes.Reader
	offset int64
	err    error
	called bool
}

func (r *offsetReader) ReadAt(p []byte, off int64) (int, error) {
	if off == r.offset {
		r.called = true
		return 0, r.err
	}
	return r.Reader.ReadAt(p, off)
}

func TestDecodePreservesConfigAndSource(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 5))
	src.Set(0, 0, color.NRGBA{R: 100, A: 255})
	encoders := map[string]func(io.Writer) error{
		"png":  func(w io.Writer) error { return png.Encode(w, src) },
		"jpeg": func(w io.Writer) error { return jpeg.Encode(w, src, nil) },
		"gif":  func(w io.Writer) error { return gif.Encode(w, src, nil) },
	}
	for name, encode := range encoders {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			if err := encode(&b); err != nil {
				t.Fatal(err)
			}
			before := append([]byte(nil), b.Bytes()...)
			r := bytes.NewReader(b.Bytes())
			got, format, err := Decode(r)
			if err != nil || got.Width != 4 || got.Height != 5 || format != name {
				t.Fatalf("config=%+v format=%q err=%v", got, format, err)
			}
			remaining, err := io.ReadAll(r)
			if err != nil || !bytes.Equal(remaining, before) {
				t.Fatal("source content or position changed")
			}
		})
	}
}

func TestDecodeTruncatedInput(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("II"), {'I', 'I', 42, 0}, {'M', 'M', 0, 42}} {
		if _, _, err := Decode(bytes.NewReader(data)); err == nil {
			t.Fatalf("accepted truncated input %x", data)
		}
	}
}

func TestDecodeBigEndianTIFF(t *testing.T) {
	// Two scalar LONG entries: width and height. The TIFF defaults describe
	// a one-bit grayscale image, so no pixel payload is needed for metadata.
	data := make([]byte, 34)
	copy(data, "MM\x00\x2a")
	binary.BigEndian.PutUint32(data[4:8], 8)
	binary.BigEndian.PutUint16(data[8:10], 2)
	for i, value := range []uint32{4, 5} {
		entry := data[10+i*12 : 22+i*12]
		binary.BigEndian.PutUint16(entry[0:2], uint16(256+i))
		binary.BigEndian.PutUint16(entry[2:4], 4)
		binary.BigEndian.PutUint32(entry[4:8], 1)
		binary.BigEndian.PutUint32(entry[8:12], value)
	}
	cfg, format, err := Decode(bytes.NewReader(data))
	if err != nil || format != "tiff" || cfg.Width != 4 || cfg.Height != 5 {
		t.Fatalf("config=%+v format=%q err=%v", cfg, format, err)
	}
}
