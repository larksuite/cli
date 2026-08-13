// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package cmdupdate

func runEditionUpdate(*UpdateOptions) (bool, error) { return false, nil }

func updateLongDescription() string {
	return `Update lark-cli to the latest version.

Detects the installation method automatically:
  - npm install:  runs npm install -g @larksuite/cli@<version>
  - pnpm install: runs pnpm add -g @larksuite/cli@<version>
  - manual/other: shows GitHub Releases download URL

Use --json for structured output (for AI agents and scripts).
Use --check to only check for updates without installing.`
}
