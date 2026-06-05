// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Command readonly-policy is a runnable fork of lark-cli that
// installs a Rule permitting only docs/* and im/* read commands.
// Any write command produces a structured command_denied envelope.
//
// Build & run:
//
//	cd extension/platform/examples/readonly-policy
//	go build -o readonly-cli .
//	./readonly-cli docs +update --doc-token X --content Y
//	# {"ok":false,"error":{"type":"command_denied", ...}}
//
//	./readonly-cli config policy show
//	# shows the active Rule with source=plugin:readonly
package main

import (
	"os"

	"github.com/larksuite/cli/cmd"
	"github.com/larksuite/cli/extension/platform"
)

func init() {
	platform.Register(
		platform.NewPlugin("readonly", "0.1.0").
			Restrict(&platform.Rule{
				Name:        "agent-readonly",
				Description: "Only read-class docs/im commands. Suitable for AI-agent sessions.",
				Allow:       []string{"docs/**", "im/**"},
				MaxRisk:     platform.RiskRead,
				// AllowUnannotated stays default false (fail-closed):
				// unannotated commands are denied, surfacing missing
				// risk_level annotations early in adoption.
			}).
			MustBuild())
	// Note: Restrict() implicitly sets Restricts=true and FailClosed.
	// No need to call FailClosed() explicitly.
}

func main() {
	os.Exit(cmd.Execute())
}
