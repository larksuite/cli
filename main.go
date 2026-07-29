// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// lark-cli — Feishu/Lark CLI tool (Go implementation).
package main

import (
	"fmt"
	"os"

	"github.com/larksuite/cli/cmd"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/runtimedelegate"

	_ "github.com/larksuite/cli/extension/credential/env" // activate env credential provider
)

func main() {
	if runtimedelegate.IsCapabilityRequest(os.Args[1:]) {
		fmt.Fprintln(os.Stdout, runtimedelegate.Capabilities(build.Version))
		return
	}
	if code, handled, err := runtimedelegate.Dispatch(os.Args, build.Version); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error: runtime delegate:", err)
		}
		os.Exit(code)
	}
	os.Exit(cmd.Execute())
}
