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
}

// NewIOStreams builds an IOStreams from arbitrary readers/writers.
// IsTerminal is derived from in's underlying *os.File, if any; non-file
// readers (bytes.Buffer, strings.Reader, …) yield IsTerminal=false.
func NewIOStreams(in io.Reader, out, errOut io.Writer) *IOStreams {
	isTerminal := false
	if f, ok := in.(*os.File); ok {
		isTerminal = term.IsTerminal(int(f.Fd()))
	}
	return &IOStreams{In: in, Out: out, ErrOut: errOut, IsTerminal: isTerminal}
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
