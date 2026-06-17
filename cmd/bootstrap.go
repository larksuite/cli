// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/pflag"
)

// BootstrapInvocationContext extracts global invocation options before
// the real command tree is built, so provider-backed config resolution sees
// the correct profile from the start.
func BootstrapInvocationContext(args []string) (cmdutil.InvocationContext, error) {
	var globals GlobalOptions

	fs := pflag.NewFlagSet("bootstrap", pflag.ContinueOnError)
	fs.ParseErrorsAllowlist.UnknownFlags = true
	fs.SetInterspersed(true)
	fs.SetOutput(io.Discard)
	RegisterGlobalFlags(fs, &globals)

	if err := fs.Parse(args); err != nil && !errors.Is(err, pflag.ErrHelp) {
		return cmdutil.InvocationContext{}, err
	}
	if globals.Profile != "" {
		return cmdutil.InvocationContext{
			Profile:       globals.Profile,
			ProfileSource: core.ProfileSourceCLI,
		}, nil
	}
	if skipProjectProfileLookup(args) {
		return cmdutil.InvocationContext{ProfileSource: core.ProfileSourceGlobal}, nil
	}
	project, err := core.ResolveProjectProfile()
	if err != nil {
		return cmdutil.InvocationContext{}, err
	}
	if project != nil {
		return cmdutil.InvocationContext{
			Profile:           project.Profile,
			ProfileSource:     core.ProfileSourceProject,
			ProfileConfigPath: project.Path,
		}, nil
	}
	return cmdutil.InvocationContext{ProfileSource: core.ProfileSourceGlobal}, nil
}

func skipProjectProfileLookup(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "completion" || arg == "__complete" || arg == "__completeNoDesc" {
			return true
		}
	}
	parts := firstPositionalArgs(args, 2)
	if len(parts) == 0 {
		return false
	}
	switch parts[0] {
	case "profile":
		return len(parts) < 2 || parts[1] != "current"
	case "config":
		return len(parts) >= 2 && (parts[1] == "bind" || parts[1] == "init" || parts[1] == "remove")
	default:
		return false
	}
}

func firstPositionalArgs(args []string, limit int) []string {
	var out []string
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "--" {
			break
		}
		if arg == "--profile" {
			skipNext = true
			continue
		}
		if strings.HasPrefix(arg, "--profile=") || strings.HasPrefix(arg, "-") {
			continue
		}
		out = append(out, arg)
		if len(out) >= limit {
			return out
		}
	}
	return out
}
