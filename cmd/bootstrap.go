// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"io"

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
	if skipProjectProfileLookup(args, fs.Args()) {
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

func skipProjectProfileLookup(rawArgs, positionals []string) bool {
	for _, arg := range rawArgs {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	if len(positionals) == 0 {
		return false
	}
	switch positionals[0] {
	case "completion", "__complete", "__completeNoDesc":
		return true
	case "profile":
		return len(positionals) < 2 || positionals[1] != "current"
	case "config":
		return len(positionals) >= 2 && (positionals[1] == "bind" || positionals[1] == "init" || positionals[1] == "remove")
	default:
		return false
	}
}
