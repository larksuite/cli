// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"github.com/larksuite/cli/cmd"
	"github.com/larksuite/cli/internal/affordance"
)

// embeddedContentFS bundles the agent-readable content that must ship in lockstep
// with the binary: each skill's docs (SKILL.md + references/, plus whiteboard's
// routes/ and scenes/) and the per-domain affordance guidance (affordance/*.md).
// Machine-resource skill dirs (assets/, scripts/) are excluded. It's a whitelist —
// a new content type is omitted until added to the embed list. The embed must live
// in this root package because go:embed cannot reach up out of a package's dir.
//
//go:embed skills/*/SKILL.md skills/*/references skills/*/routes skills/*/scenes affordance/*.md
var embeddedContentFS embed.FS

// init wires the embedded content into the CLI. It compiles into `go build .` but
// not the single-file preview build (`go build ./main.go`), so that build stays
// self-contained (shipping no embedded content). Assembly failures warn on stderr
// rather than panicking — embedded content is nice-to-have, not load-bearing.
func init() {
	if sub, err := fs.Sub(embeddedContentFS, "skills"); err != nil {
		fmt.Fprintln(os.Stderr, "warning: skills embed assembly failed, skills commands disabled:", err)
	} else {
		cmd.SetEmbeddedSkillContent(sub)
	}
	if sub, err := fs.Sub(embeddedContentFS, "affordance"); err != nil {
		fmt.Fprintln(os.Stderr, "warning: affordance embed assembly failed, command guidance disabled:", err)
	} else {
		affordance.SetSource(sub)
	}
}
