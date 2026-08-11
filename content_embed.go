// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	defaultaffordance "github.com/larksuite/cli/affordance"
	"github.com/larksuite/cli/cmd"
	defaultskills "github.com/larksuite/cli/skills"
)

// init wires the embedded content into the CLI. It compiles into `go build .` but
// not the single-file preview build (`go build ./main.go`), so that build stays
// self-contained (shipping no embedded content). External wrapper distributions
// can import the same default files from the skills and affordance packages.
func init() {
	cmd.SetEmbeddedSkillContent(defaultskills.DefaultFS())
	cmd.SetEmbeddedAffordanceContent(defaultaffordance.DefaultFS())
}
