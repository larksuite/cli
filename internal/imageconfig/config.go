// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package imageconfig reads dimensions without decoding image pixels.
package imageconfig

import (
	"encoding/binary"
	"errors"
	"image"
	"io"
	"math"

	// PNG, JPEG and GIF are read through the standard library's format
	// registry. Registering them here keeps Decode self-contained: a caller
	// that imports only this package still gets every supported format, and
	// tidying an apparently unused blank import elsewhere cannot silently
	// disable dimension detection.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// Config describes dimensions only, not a color model or validated pixel data.
type Config struct{ Width, Height int }

var errMetadata = errors.New("invalid or unsupported image dimensions")

// Decode reads dimensions from the beginning of r without consuming it.
// TIFF, BMP and WebP use bounded metadata reads, never pixel decompression.
// Successfully reading dimensions does not validate the complete image.
func Decode(r io.ReaderAt) (Config, string, error) {
	var magic [4]byte
	if err := readAt(r, magic[:], 0); err != nil {
		return Config{}, "", err
	}
	switch {
	case string(magic[:]) == "II\x2a\x00" || string(magic[:]) == "MM\x00\x2a":
		cfg, err := readTIFF(r)
		return cfg, "tiff", err
	case string(magic[:2]) == "BM":
		cfg, err := readBMP(r)
		return cfg, "bmp", err
	case string(magic[:]) == "RIFF":
		cfg, err := readWebP(r)
		return cfg, "webp", err
	default:
		cfg, format, err := image.DecodeConfig(io.NewSectionReader(r, 0, math.MaxInt64))
		return Config{Width: cfg.Width, Height: cfg.Height}, format, err
	}
}

func dimensions(w, h int64) (Config, error) {
	if w <= 0 || h <= 0 || uint64(w) > uint64(^uint(0)>>1) || uint64(h) > uint64(^uint(0)>>1) {
		return Config{}, errMetadata
	}
	return Config{Width: int(w), Height: int(h)}, nil
}

func readAt(r io.ReaderAt, b []byte, offset int64) error {
	n, err := r.ReadAt(b, offset)
	if n == len(b) {
		return nil
	}
	if err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}

// Classic TIFF stores scalar SHORT/LONG dimensions in the first IFD. Skip all
// other fields without following their offsets, including EXIF and pixel data.
// The uint16 entry count bounds work to 65535 fixed-size entries, with no
// allocation proportional to a tag count, file offset or image dimensions.
func readTIFF(r io.ReaderAt) (Config, error) {
	var header [8]byte
	if err := readAt(r, header[:], 0); err != nil {
		return Config{}, err
	}
	var order binary.ByteOrder = binary.LittleEndian
	if header[0] == 'M' {
		order = binary.BigEndian
	}
	offset := int64(order.Uint32(header[4:]))
	if offset < 8 {
		return Config{}, errMetadata
	}
	var count [2]byte
	if err := readAt(r, count[:], offset); err != nil {
		return Config{}, err
	}
	var values [2]int64
	var found [2]bool
	var entry [12]byte
	for i := 0; i < int(order.Uint16(count[:])); i++ {
		if err := readAt(r, entry[:], offset+2+int64(i)*12); err != nil {
			return Config{}, err
		}
		tag := order.Uint16(entry[:2])
		if tag != 256 && tag != 257 {
			continue
		}
		idx := int(tag - 256)
		if found[idx] || order.Uint32(entry[4:8]) != 1 {
			return Config{}, errMetadata
		}
		found[idx] = true
		switch order.Uint16(entry[2:4]) {
		case 3:
			values[idx] = int64(order.Uint16(entry[8:10]))
		case 4:
			values[idx] = int64(order.Uint32(entry[8:12]))
		default:
			return Config{}, errMetadata
		}
	}
	return dimensions(values[0], values[1])
}

// BMP dimensions reside in the DIB header. Keep the supported Windows INFO,
// V4 and V5 variants. Negative height denotes top-down storage.
// https://learn.microsoft.com/en-us/windows/win32/api/wingdi/ns-wingdi-bitmapinfoheader
func readBMP(r io.ReaderAt) (Config, error) {
	var header [138]byte
	if err := readAt(r, header[:18], 0); err != nil {
		return Config{}, err
	}
	order := binary.LittleEndian
	size := order.Uint32(header[14:18])
	if size != 40 && size != 108 && size != 124 {
		return Config{}, errMetadata
	}
	if err := readAt(r, header[18:14+size], 18); err != nil {
		return Config{}, err
	}
	w, h := int64(int32(order.Uint32(header[18:22]))), int64(int32(order.Uint32(header[22:26])))
	if h < 0 {
		h = -h
	}
	if order.Uint16(header[26:28]) != 1 {
		return Config{}, errMetadata
	}
	bits := order.Uint16(header[28:30])
	switch bits {
	case 1, 2, 4, 8, 24, 32:
	default:
		return Config{}, errMetadata
	}
	compression := order.Uint32(header[30:34])
	if compression != 0 {
		if compression != 3 || size == 40 || order.Uint32(header[54:58]) != 0xff0000 || order.Uint32(header[58:62]) != 0xff00 || order.Uint32(header[62:66]) != 0xff || order.Uint32(header[66:70]) != 0xff000000 {
			return Config{}, errMetadata
		}
	}
	pixelOffset := uint64(order.Uint32(header[10:14]))
	expected := uint64(14 + size)
	if bits <= 8 {
		colors := order.Uint32(header[46:50])
		if colors == 0 {
			colors = 1 << bits
		}
		if colors > 1<<bits {
			return Config{}, errMetadata
		}
		expected += uint64(colors) * 4
		// The palette is bounded to 256 entries; check it exists, without decoding it.
		var palette [1024]byte
		if err := readAt(r, palette[:colors*4], int64(14+size)); err != nil {
			return Config{}, err
		}
	}
	if pixelOffset != expected {
		return Config{}, errMetadata
	}
	return dimensions(w, h)
}

// WebP dimensions are stored in VP8, VP8L or VP8X headers. No compressed data
// needs decoding. Limit traversal to 4096 chunks; unusually fragmented files
// require an explicit limit review instead of unbounded attacker-controlled IO.
// https://developers.google.com/speed/webp/docs/riff_container
func readWebP(r io.ReaderAt) (Config, error) {
	var header [12]byte
	if err := readAt(r, header[:], 0); err != nil {
		return Config{}, err
	}
	if string(header[8:]) != "WEBP" {
		return Config{}, errMetadata
	}
	end := int64(binary.LittleEndian.Uint32(header[4:8])) + 8
	if end < 20 || end > 1<<32-2 {
		return Config{}, errMetadata
	}
	var chunk [8]byte
	var data [10]byte
	for off, i := int64(12), 0; off+8 <= end && i < 4096; i++ {
		if err := readAt(r, chunk[:], off); err != nil {
			return Config{}, err
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:]))
		// The chunk payload must lie inside the container. Its even-padding
		// byte need not: writers that omit the pad after a final odd-sized
		// chunk still describe complete dimensions, so padding is only
		// required where it is actually consumed -- skipping to the next
		// chunk, below.
		if off+8+size > end {
			return Config{}, errMetadata
		}
		need := 0
		switch string(chunk[:4]) {
		case "VP8 ", "VP8X":
			need = 10
		case "VP8L":
			need = 5
		}
		if need != 0 {
			if size < int64(need) {
				return Config{}, errMetadata
			}
			if err := readAt(r, data[:need], off+8); err != nil {
				return Config{}, err
			}
			switch string(chunk[:4]) {
			case "VP8 ":
				if data[0]&1 != 0 || string(data[3:6]) != "\x9d\x01\x2a" {
					return Config{}, errMetadata
				}
				return dimensions(int64(binary.LittleEndian.Uint16(data[6:8])&0x3fff), int64(binary.LittleEndian.Uint16(data[8:10])&0x3fff))
			case "VP8L":
				if data[0] != 0x2f || data[4]>>5 != 0 {
					return Config{}, errMetadata
				}
				packed := binary.LittleEndian.Uint32(data[1:5])
				return dimensions(int64(packed&0x3fff)+1, int64((packed>>14)&0x3fff)+1)
			case "VP8X":
				// The spec fixes the chunk at 10 bytes, but says of every
				// reserved field -- the two high flag bits, the low flag bit
				// and the 24-bit block -- "MUST be 0. Readers MUST ignore
				// this field." Rejecting a non-zero reserved bit would refuse
				// files that decode fine everywhere else.
				if size != 10 {
					return Config{}, errMetadata
				}
				return dimensions(uint24(data[4:7])+1, uint24(data[7:10])+1)
			}
		}
		next := off + 8 + size + (size & 1)
		if next > end {
			return Config{}, errMetadata
		}
		off = next
	}
	return Config{}, errMetadata
}

func uint24(b []byte) int64 { return int64(b[0]) | int64(b[1])<<8 | int64(b[2])<<16 }
