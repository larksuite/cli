// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriterSink_PrettyFallbackWarnsOnce verifies that WriterSink logs a
// single WARN to ErrOut when asked to pretty-print a non-JSON payload
// and suppresses duplicates on subsequent malformed writes. Callers rely
// on this to notice that --pretty silently degraded, while avoiding log
// spam on pathological streams.
func TestWriterSink_PrettyFallbackWarnsOnce(t *testing.T) {
	var out, errOut bytes.Buffer
	s := &WriterSink{W: &out, Pretty: true, ErrOut: &errOut}

	// Two malformed payloads back-to-back.
	if err := s.Write(json.RawMessage("not json {{{")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := s.Write(json.RawMessage("still not json")); err != nil {
		t.Fatalf("second write: %v", err)
	}

	warnings := strings.Count(errOut.String(), "WARN:")
	if warnings != 1 {
		t.Errorf("expected exactly 1 WARN line (once-semantics), got %d: %q", warnings, errOut.String())
	}
	if !strings.Contains(errOut.String(), "pretty") {
		t.Errorf("warning should mention pretty: %q", errOut.String())
	}

	// Raw passthrough should have been written both times.
	if strings.Count(out.String(), "not json") != 2 {
		t.Errorf("expected 2 raw passthrough lines in W, got: %q", out.String())
	}
}

// TestWriterSink_PrettyHappyPath verifies that valid JSON is formatted
// with 2-space indent and no warning fires.
func TestWriterSink_PrettyHappyPath(t *testing.T) {
	var out, errOut bytes.Buffer
	s := &WriterSink{W: &out, Pretty: true, ErrOut: &errOut}

	if err := s.Write(json.RawMessage(`{"k":"v"}`)); err != nil {
		t.Fatal(err)
	}
	if errOut.Len() != 0 {
		t.Errorf("expected no warning on valid JSON, got: %q", errOut.String())
	}
	if !strings.Contains(out.String(), "\n  \"k\"") {
		t.Errorf("expected indented output, got: %q", out.String())
	}
}

// TestWriterSink_PrettyNoErrOut verifies that omitting ErrOut suppresses
// the warning (no panic, silent degradation) — legacy callers that don't
// wire ErrOut still get the prior behaviour.
func TestWriterSink_PrettyNoErrOut(t *testing.T) {
	var out bytes.Buffer
	s := &WriterSink{W: &out, Pretty: true} // ErrOut left nil

	if err := s.Write(json.RawMessage("not json")); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Reached here without panic, raw output emitted — good enough.
	if !strings.Contains(out.String(), "not json") {
		t.Errorf("expected raw passthrough, got: %q", out.String())
	}
}

// TestDirSink_FilenameIncludesPID verifies filenames embed os.Getpid() so
// two processes writing to the same output dir can't collide even if
// their nanosecond clocks and sequence values happen to align.
func TestDirSink_FilenameIncludesPID(t *testing.T) {
	dir := t.TempDir()
	s := &DirSink{Dir: dir, pid: os.Getpid()}

	if err := s.Write(json.RawMessage(`{"a":1}`)); err != nil {
		t.Fatalf("write: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(entries), err)
	}
	name := entries[0].Name()
	wantPID := fmt.Sprintf("_%d_", os.Getpid())
	if !strings.Contains(name, wantPID) {
		t.Errorf("filename %q should contain PID segment %q", name, wantPID)
	}
	if filepath.Ext(name) != ".json" {
		t.Errorf("filename %q should have .json extension", name)
	}
}

// TestDirSink_FilenameFormat verifies the full "<ns>_<pid>_<seq>.json"
// shape — guards against a refactor silently dropping the seq or pid
// segment and reintroducing the collision risk.
func TestDirSink_FilenameFormat(t *testing.T) {
	dir := t.TempDir()
	s := &DirSink{Dir: dir, pid: 12345}

	for i := 0; i < 3; i++ {
		if err := s.Write(json.RawMessage(`{}`)); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 files, got %d", len(entries))
	}
	for _, e := range entries {
		name := e.Name()
		// Expect three numeric segments then .json: ns_pid_seq.json.
		trimmed := strings.TrimSuffix(name, ".json")
		parts := strings.Split(trimmed, "_")
		if len(parts) != 3 {
			t.Errorf("filename %q should split into 3 underscore parts, got %d", name, len(parts))
			continue
		}
		if parts[1] != "12345" {
			t.Errorf("filename %q should have PID=12345 as middle segment, got %q", name, parts[1])
		}
	}
}
