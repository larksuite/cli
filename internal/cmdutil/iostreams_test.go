// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"bytes"
	"os"
	"testing"
)

func TestNewIOStreamsTerminalFlagsNonFile(t *testing.T) {
	s := NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	if s.IsTerminal || s.OutIsTerminal || s.StderrIsTerminal {
		t.Errorf("non-file streams must not be terminals: in=%v out=%v err=%v",
			s.IsTerminal, s.OutIsTerminal, s.StderrIsTerminal)
	}
}

func TestNewIOStreamsTerminalFlagsPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	s := NewIOStreams(r, w, w)
	if s.OutIsTerminal || s.StderrIsTerminal {
		t.Errorf("os.Pipe must not be a terminal: out=%v err=%v", s.OutIsTerminal, s.StderrIsTerminal)
	}
}

func TestStdoutIsTerminal(t *testing.T) {
	// Buffer-backed output (tests, captured output) is never a terminal.
	if (&IOStreams{Out: &bytes.Buffer{}}).StdoutIsTerminal() {
		t.Error("bytes.Buffer Out should not be a terminal")
	}
	// An os.Pipe write end is an *os.File but not a terminal — mirrors `cmd | jq`,
	// the case the stdin-based IsTerminal would get wrong.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if (&IOStreams{Out: w}).StdoutIsTerminal() {
		t.Error("os.Pipe Out should not be a terminal")
	}
}
