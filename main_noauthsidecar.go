// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !authsidecar

// This file is the fail-closed guard for builds that do NOT include the
// `authsidecar` tag. The sidecar credential-isolation feature is only
// compiled in under that tag; deploying the plain build into an environment
// that expects sidecar isolation would silently fall back to direct env
// credential use — exactly the failure mode the feature is meant to prevent.
//
// When LARKSUITE_CLI_AUTH_PROXY is set, we refuse to run rather than ignore
// the variable. The operator either rebuilt without realizing (wrong
// artifact) or the sandbox inherited the var by accident; both cases want
// a loud startup error, not a mysterious token leak on the first API call.

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/larksuite/cli/internal/envvars"
)

func init() {
	if code := checkNoAuthsidecarBuild(os.Getenv, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

// checkNoAuthsidecarBuild returns a non-zero exit code (and writes a
// human-readable reason to stderr) when the environment asks for sidecar
// isolation that this binary cannot provide. Factored out from init() so
// tests can exercise the decision without actually calling os.Exit.
func checkNoAuthsidecarBuild(getenv func(string) string, stderr io.Writer) int {
	v := getenv(envvars.CliAuthProxy)
	if v == "" {
		return 0
	}
	fmt.Fprintf(stderr,
		"ERROR: %s is set, but this lark-cli binary was built WITHOUT the "+
			"'authsidecar' build tag.\n"+
			"The sidecar credential-isolation feature is compiled out — "+
			"running would bypass isolation and\n"+
			"send any real credentials present in the environment directly "+
			"to the Lark API.\n\n"+
			"To fix, either:\n"+
			"  - rebuild the CLI with: go build -tags authsidecar\n"+
			"  - or unset %s if sidecar isolation is not required\n",
		envvars.CliAuthProxy, envvars.CliAuthProxy)
	return 2
}
