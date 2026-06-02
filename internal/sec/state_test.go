// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadState_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	in := &State{
		Version:     "1.2.3",
		BuildID:     "build-42",
		InstalledAt: time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
		BinaryPath:  "/tmp/lark-sec-cli",
	}
	if err := SaveState(path, in); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got == nil {
		t.Fatal("LoadState returned nil")
	}
	if got.Version != in.Version || got.BuildID != in.BuildID || got.BinaryPath != in.BinaryPath {
		t.Errorf("roundtrip mismatch: got=%+v want=%+v", got, in)
	}
	if !got.InstalledAt.Equal(in.InstalledAt) {
		t.Errorf("InstalledAt mismatch: got=%v want=%v", got.InstalledAt, in.InstalledAt)
	}
}

func TestLoadState_AbsentFile(t *testing.T) {
	got, err := LoadState(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil state for missing file, got %+v", got)
	}
}
