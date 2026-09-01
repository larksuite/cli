// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestDetectImageDimensionsSupportsBMPAndTIFF(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		image  []byte
		width  int
		height int
	}{
		{name: "BMP", image: testBMPImage(2, 3), width: 2, height: 3},
		{name: "TIFF", image: testTIFFImage(4, 5), width: 4, height: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, height, err := detectImageDimensions(bytes.NewReader(tt.image))
			if err != nil {
				t.Fatalf("detectImageDimensions() error: %v", err)
			}
			if width != tt.width || height != tt.height {
				t.Fatalf("dimensions = %dx%d, want %dx%d", width, height, tt.width, tt.height)
			}
		})
	}
}

func TestDocumentImageMIMETypesAndExtensions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		contentType string
		ext         string
	}{
		{contentType: "image/bmp", ext: ".bmp"},
		{contentType: "image/gif", ext: ".gif"},
		{contentType: "image/jpeg", ext: ".jpg"},
		{contentType: "image/png", ext: ".png"},
		{contentType: "image/tiff", ext: ".tiff"},
		{contentType: "image/webp", ext: ".webp"},
	}
	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			if got := docCoverAllowedContentTypes[tt.contentType]; got != tt.ext {
				t.Fatalf("allowed extension = %q, want %q", got, tt.ext)
			}
			resolution := docMediaExtensionByContentType(tt.contentType + "; charset=binary")
			if resolution == nil || resolution.Ext != tt.ext {
				t.Fatalf("extension resolution = %#v, want %q", resolution, tt.ext)
			}
		})
	}
}

func testBMPImage(width, height int) []byte {
	rowSize := (width*3 + 3) &^ 3
	pixelBytes := rowSize * height
	data := make([]byte, 54+pixelBytes)
	copy(data[:2], "BM")
	binary.LittleEndian.PutUint32(data[2:6], uint32(len(data)))
	binary.LittleEndian.PutUint32(data[10:14], 54)
	binary.LittleEndian.PutUint32(data[14:18], 40)
	binary.LittleEndian.PutUint32(data[18:22], uint32(width))
	binary.LittleEndian.PutUint32(data[22:26], uint32(height))
	binary.LittleEndian.PutUint16(data[26:28], 1)
	binary.LittleEndian.PutUint16(data[28:30], 24)
	binary.LittleEndian.PutUint32(data[34:38], uint32(pixelBytes))
	return data
}

func testTIFFImage(width, height int) []byte {
	const entryCount = 4
	data := make([]byte, 8+2+entryCount*12)
	copy(data[:4], []byte{'I', 'I', 42, 0})
	binary.LittleEndian.PutUint32(data[4:8], 8)
	binary.LittleEndian.PutUint16(data[8:10], entryCount)

	entries := []struct {
		tag, fieldType uint16
		value          uint32
	}{
		{tag: 256, fieldType: 4, value: uint32(width)},
		{tag: 257, fieldType: 4, value: uint32(height)},
		{tag: 258, fieldType: 3, value: 8},
		{tag: 262, fieldType: 3, value: 1},
	}
	for i, entry := range entries {
		offset := 10 + i*12
		binary.LittleEndian.PutUint16(data[offset:offset+2], entry.tag)
		binary.LittleEndian.PutUint16(data[offset+2:offset+4], entry.fieldType)
		binary.LittleEndian.PutUint32(data[offset+4:offset+8], 1)
		binary.LittleEndian.PutUint32(data[offset+8:offset+12], entry.value)
	}
	return data
}
