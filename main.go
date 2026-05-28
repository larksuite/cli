// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// lark-cli — Feishu/Lark CLI tool (Go implementation).
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/larksuite/cli/cmd"

	_ "github.com/larksuite/cli/extension/credential/env" // activate env credential provider
)

func main() {
	// Recover from any unhandled panics to ensure stderr always receives
	// diagnostics instead of crashing silently (especially in automation/
	// CI contexts where a panic without stack trace looks like exit 1 with
	// empty stdout/stderr). See https://github.com/larksuite/cli/issues/1139
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "panic: %v\n%s\n", r, debug.Stack())
			os.Exit(1)
		}
	}()
	os.Exit(cmd.Execute())
}
