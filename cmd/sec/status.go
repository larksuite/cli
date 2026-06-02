// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	intsec "github.com/larksuite/cli/internal/sec"
)

// StatusOptions holds inputs for `lark-cli sec status`.
type StatusOptions struct {
	Factory *cmdutil.Factory
}

// NewCmdSecStatus shows install + runtime state. Implementation strategy:
//
//  1. Read lark-cli's local install record (state.json) — works even when the
//     daemon's not installed, and gives the user a version/buildId/path
//     fingerprint regardless of whether the service is up.
//  2. If the install exists, shell out to `lark-sec-cli status` for the
//     live daemon view (service registration, pid liveness, proxy probe,
//     sec_config.json contents). The daemon's own status command does a
//     thorough check; we just pass it through.
func NewCmdSecStatus(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{Factory: f}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show lark-sec-cli install and runtime state",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return runStatus(cmd, opts)
		},
	}
	return cmd
}

func runStatus(cmd *cobra.Command, opts *StatusOptions) error {
	errOut := opts.Factory.IOStreams.ErrOut
	trace := verboseOut(cmd, errOut)

	tracef(trace, "sec status", "constructing installer (lazy credentials)")
	_, paths, err := installer(opts.Factory)
	if err != nil {
		return output.Errorf(output.ExitInternal, "internal", "%v", err)
	}
	out := opts.Factory.IOStreams.Out
	tracef(trace, "sec status", "loading state from %s", paths.StateFile())
	state, err := intsec.LoadState(paths.StateFile())
	if err != nil {
		return output.Errorf(output.ExitInternal, "internal", "load sec state: %v", err)
	}
	if state == nil {
		fmt.Fprintln(out, "lark-sec-cli: not installed")
		fmt.Fprintln(out, "  run: lark-cli sec run")
		return nil
	}
	fmt.Fprintf(out, "lark-sec-cli %s\n", state.Version)
	fmt.Fprintf(out, "  binary: %s\n", state.BinaryPath)

	// Daemon-side detail via `lark-sec-cli status`. The daemon's status
	// command already covers service registration + pid + proxy reachability
	// + bridge file — better than re-implementing those here.
	tracef(trace, "sec status", "shelling out to %s status", state.BinaryPath)
	c := exec.CommandContext(cmd.Context(), state.BinaryPath, "status")
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	runErr := c.Run()
	tracef(trace, "sec status", "daemon status exit=%v stdout=%d bytes stderr=%d bytes", runErr, stdout.Len(), stderr.Len())
	fmt.Fprintln(out, "  --- lark-sec-cli status ---")
	if stdout.Len() > 0 {
		fmt.Fprint(out, indent(stdout.String(), "  "))
	}
	if stderr.Len() > 0 {
		fmt.Fprint(out, indent(stderr.String(), "  "))
	}
	// `lark-sec-cli status` exits 1 when not running — that's diagnostic
	// data, not a failure of OUR command. Surface it for the user but don't
	// propagate the non-zero exit upward.
	_ = runErr
	return nil
}

// indent prefixes every line of s with prefix. Cheap pass-through formatter
// used to make the embedded `lark-sec-cli status` output read as a sub-block
// under our own header.
func indent(s, prefix string) string {
	if s == "" {
		return s
	}
	var buf bytes.Buffer
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			buf.WriteString(prefix)
			buf.WriteString(s[start : i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		buf.WriteString(prefix)
		buf.WriteString(s[start:])
	}
	return buf.String()
}
