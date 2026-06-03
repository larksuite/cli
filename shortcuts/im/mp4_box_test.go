// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"encoding/binary"
	"testing"
)

// build64BitBox builds an ISO-BMFF box using the 64-bit "largesize" form: the
// 32-bit size field is set to 1 and an 8-byte largesize follows the 4-byte box
// type. largesize is the total box length including the 16-byte header.
func build64BitBox(boxType string, largesize uint64, payload []byte) []byte {
	box := make([]byte, 16+len(payload))
	binary.BigEndian.PutUint32(box[0:4], 1) // size == 1 → 64-bit largesize follows
	copy(box[4:8], boxType)
	binary.BigEndian.PutUint64(box[8:16], largesize)
	copy(box[16:], payload)
	return box
}

// build32BitBox builds an ISO-BMFF box using the ordinary 32-bit size form.
func build32BitBox(boxType string, payload []byte) []byte {
	box := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(box[0:4], uint32(len(box)))
	copy(box[4:8], boxType)
	copy(box[8:], payload)
	return box
}

// TestMP4BoxLargeSizeOverflowNoPanic guards the 64-bit box-size branch against
// CWE-190 integer overflow. A largesize whose high bit is set converts to a
// negative offset; without a bounds guard that offset indexes the input slice
// out of range and panics, crashing the CLI on a crafted/corrupt MP4 (the
// in-memory walkers run on URL-sourced media that the caller does not control).
// The walkers' contract is best-effort: malformed input must return 0, not panic.
func TestMP4BoxLargeSizeOverflowNoPanic(t *testing.T) {
	// A single top-level box in the 64-bit form with largesize = 2^64-1.
	data := build64BitBox("ftyp", 0xFFFFFFFFFFFFFFFF, nil)

	if got := readMp4DurationBytes(data); got != 0 {
		t.Errorf("readMp4DurationBytes(overflow largesize) = %d, want 0", got)
	}
	if got := parseMp4Duration(data); got != 0 {
		t.Errorf("parseMp4Duration(overflow largesize) = %d, want 0", got)
	}
	if start, end := findMP4Box(data, 0, len(data), "ftyp"); start != -1 || end != -1 {
		t.Errorf("findMP4Box(overflow largesize) = (%d, %d), want (-1, -1)", start, end)
	}
}

// TestMP4Box64BitSizeAtNonZeroOffset locks in correct handling of a 64-bit box
// that does not start at offset 0. boxEnd must be offset+largesize (as the
// 32-bit branch already does with offset+size); dropping the offset truncates
// the box and the duration is silently lost.
func TestMP4Box64BitSizeAtNonZeroOffset(t *testing.T) {
	mvhd := buildMvhdBox(0, 1000, 5000) // timescale=1000, duration=5000 → 5000ms
	// moov carried as a 64-bit box: largesize = 16-byte header + mvhd payload.
	moov := build64BitBox("moov", uint64(16+len(mvhd)), mvhd)
	// Precede moov with a 32-bit ftyp box so it sits at a non-zero offset —
	// that is where the missing "offset +" surfaces.
	data := append(build32BitBox("ftyp", []byte("isom")), moov...)

	if got := readMp4DurationBytes(data); got != 5000 {
		t.Errorf("readMp4DurationBytes(64-bit moov at offset>0) = %d, want 5000", got)
	}
}

// TestFindMP4Box64BitSizeAtNonZeroOffset is the findMP4Box-level analogue: a
// 64-bit box preceding the target must advance the cursor by offset+largesize
// so the following box is located at the right position.
func TestFindMP4Box64BitSizeAtNonZeroOffset(t *testing.T) {
	free := build64BitBox("free", 24, make([]byte, 8)) // 16-byte header + 8 bytes
	target := build32BitBox("mvhd", []byte("payload!"))
	data := append(free, target...)

	start, end := findMP4Box(data, 0, len(data), "mvhd")
	if start < 0 {
		t.Fatalf("findMP4Box did not find mvhd after a 64-bit box (start=%d)", start)
	}
	if got := string(data[start:end]); got != "payload!" {
		t.Errorf("findMP4Box returned %q, want %q", got, "payload!")
	}
}
