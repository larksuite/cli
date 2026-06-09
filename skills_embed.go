// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"github.com/larksuite/cli/cmd"
)

// skillsEmbedFS embeds each skill's agent-readable content (SKILL.md +
// references/, plus lark-whiteboard's routes/ and scenes/) so the CLI serves
// content matching the binary version; machine-resource dirs (assets/, scripts/)
// are excluded, saving ~3.3 MB. It's a whitelist — a new subdirectory type is
// silently omitted until added here.
//
//go:embed skills/*/SKILL.md skills/*/references skills/*/routes skills/*/scenes
var skillsEmbedFS embed.FS

// init wires the embedded tree in as the default skill content. It compiles into
// `go build .` but not the single-file preview build (`go build ./main.go`), so
// main.go stays self-contained and that build still compiles (shipping no
// embedded skills). Assembly failure warns on stderr rather than panicking.
func init() {
	sub, err := fs.Sub(skillsEmbedFS, "skills")
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: skills embed assembly failed, skills commands disabled:", err)
		return
	}
	cmd.SetEmbeddedSkillContent(sub)
}
