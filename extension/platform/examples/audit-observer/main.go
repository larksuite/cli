// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Command audit-observer is a runnable fork of lark-cli that logs
// every dispatched command to stderr. Demonstrates the simplest
// possible plugin: one After observer matching All commands.
//
// Build & run:
//
//	cd extension/platform/examples/audit-observer
//	go build -o audit-cli .
//	./audit-cli config plugins show     # see "audit" in the list
//	./audit-cli api GET /open-apis/...  # observer logs to stderr
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/larksuite/cli/cmd"
	"github.com/larksuite/cli/extension/platform"
)

func init() {
	platform.Register(
		platform.NewPlugin("audit", "0.1.0").
			Observer(platform.After, "log", platform.All(),
				func(ctx context.Context, inv platform.Invocation) {
					path := inv.Cmd().Path()
					if err := inv.Err(); err != nil {
						fmt.Fprintf(os.Stderr, "[audit] %s FAILED: %v\n", path, err)
					} else {
						log.Printf("[audit] %s ok", path)
					}
				}).
			FailOpen().
			MustBuild())
}

func main() {
	os.Exit(cmd.Execute())
}
