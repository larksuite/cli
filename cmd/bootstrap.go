// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"io"
	"os"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/envvars"
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
	// Resolve the session-level default only at the process boundary. Core
	// config and credential packages consume the immutable invocation context
	// and remain independent of ambient environment state. An explicitly empty
	// --profile= remains a flag selection and suppresses the environment value.
	if fs.Changed("profile") {
		return cmdutil.InvocationContext{Profile: globals.Profile}, nil
	}
	if profile := os.Getenv(envvars.CliProfile); profile != "" {
		return cmdutil.InvocationContext{
			Profile:                profile,
			ProfileFromEnvironment: true,
		}, nil
	}
	return cmdutil.InvocationContext{}, nil
}
