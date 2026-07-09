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
