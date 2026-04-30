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

func TestWriterSink_PrettyFallbackWarnsOnce(t *testing.T) {
	var out, errOut bytes.Buffer
	s := &WriterSink{W: &out, Pretty: true, ErrOut: &errOut}

	if err := s.Write(json.RawMessage("not json {{{")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := s.Write(json.RawMessage("still not json")); err != nil {
		t.Fatalf("second write: %v", err)
	}

	warnings := strings.Count(errOut.String(), "WARN:")
	if warnings != 1 {
		t.Errorf("expected exactly 1 WARN line, got %d: %q", warnings, errOut.String())
	}
	if !strings.Contains(errOut.String(), "pretty") {
		t.Errorf("warning should mention pretty: %q", errOut.String())
	}

	if strings.Count(out.String(), "not json") != 2 {
		t.Errorf("expected 2 raw passthrough lines in W, got: %q", out.String())
	}
}

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

func TestWriterSink_PrettyNoErrOut(t *testing.T) {
	var out bytes.Buffer
	s := &WriterSink{W: &out, Pretty: true}

	if err := s.Write(json.RawMessage("not json")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(out.String(), "not json") {
		t.Errorf("expected raw passthrough, got: %q", out.String())
	}
}

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
