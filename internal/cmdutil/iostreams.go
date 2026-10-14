// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"io"
	"os"

	"golang.org/x/term"
)

// IOStreams provides the standard input/output/error streams.
// Commands should use these instead of os.Stdin/Stdout/Stderr
// to enable testing and output capture.
type IOStreams struct {
	In         io.Reader
	Out        io.Writer
	ErrOut     io.Writer
	IsTerminal bool
	// OutIsTerminal reports whether Out is an interactive terminal. Mirrors
	// IsTerminal; computed once in NewIOStreams and assignable directly in tests.
	OutIsTerminal bool
	// StderrIsTerminal reports whether ErrOut is an interactive terminal.
	// Advisory warnings written to stderr (e.g. the proxy notice) gate on this
	// so they stay out of non-interactive output (pipes, CI, agent runs).
	// Computed once in NewIOStreams, mirroring IsTerminal; tests assign it
	// directly like cmd/config/bind_test.go does for IsTerminal.
	StderrIsTerminal bool
}

// NewIOStreams builds an IOStreams from arbitrary readers/writers.
// IsTerminal / OutIsTerminal / StderrIsTerminal are each derived from the
// underlying *os.File of in / out / errOut respectively; non-file
// readers/writers (bytes.Buffer, strings.Reader, …) yield false.
func NewIOStreams(in io.Reader, out, errOut io.Writer) *IOStreams {
	fileIsTerminal := func(v any) bool {
		if f, ok := v.(*os.File); ok {
			return term.IsTerminal(int(f.Fd()))
		}
		return false
	}
	return &IOStreams{
		In:               in,
		Out:              out,
		ErrOut:           errOut,
		IsTerminal:       fileIsTerminal(in),
		OutIsTerminal:    fileIsTerminal(out),
		StderrIsTerminal: fileIsTerminal(errOut),
	}
}

// SystemIO creates an IOStreams wired to the process's standard file descriptors.
//
//nolint:forbidigo // entry point for real stdio
func SystemIO() *IOStreams {
	return NewIOStreams(os.Stdin, os.Stdout, os.Stderr)
}

// normalizeStreams returns a fresh IOStreams with any nil field filled from
// SystemIO(). Callers constructing a partial struct like &IOStreams{Out: buf}
// get a usable result without nil writers leaking into RoundTripper warnings,
// Cobra I/O, or credential-provider error paths.
func normalizeStreams(s *IOStreams) *IOStreams {
	if s == nil {
		return SystemIO()
	}
	out := *s
	if out.In == nil || out.Out == nil || out.ErrOut == nil {
		sys := SystemIO()
		if out.In == nil {
			out.In = sys.In
		}
		if out.Out == nil {
			out.Out = sys.Out
		}
		if out.ErrOut == nil {
			out.ErrOut = sys.ErrOut
		}
	}
	return &out
}
