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

// SystemIO creates an IOStreams wired to the process's standard file descriptors.
func SystemIO() *IOStreams {
	return &IOStreams{
		In:         os.Stdin,                            //nolint:forbidigo // entry point for real stdio
		Out:        os.Stdout,                           //nolint:forbidigo // entry point for real stdio
		ErrOut:     os.Stderr,                           //nolint:forbidigo // entry point for real stdio
		IsTerminal: term.IsTerminal(int(os.Stdin.Fd())), //nolint:forbidigo // need Fd() for terminal check
	}
}
