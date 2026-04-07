// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirRecordWriterUsesPrivatePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "events")
	writer := &dirRecordWriter{dir: dir, seq: new(uint64)}

	if err := writer.WriteRecord("im.message.receive_v1", map[string]interface{}{
		"event_type": "im.message.receive_v1",
		"event_id":   "evt_123",
	}); err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat(dir) error = %v", err)
	}
	if got, want := dirInfo.Mode().Perm(), os.FileMode(0o700); got != want {
		t.Fatalf("dir perm = %#o, want %#o", got, want)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(dir) error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}

	filePath := filepath.Join(dir, entries[0].Name())
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Stat(file) error = %v", err)
	}
	if got, want := fileInfo.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("file perm = %#o, want %#o", got, want)
	}
}
