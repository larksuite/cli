// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"embed"
	"io/fs"

	"github.com/larksuite/cli/cmd/skill"
)

// skillsEmbedFS embeds the entire skills/ tree at build time so the CLI can
// serve skill content that is guaranteed to match the binary version.
//
// We use `//go:embed skills` (not `all:skills`) deliberately: the default form
// excludes files/dirs whose names begin with "." or "_" at any depth, which
// keeps editor/OS junk (e.g. .DS_Store) out of the binary. The trade-off is
// that any future skills/ file intentionally named with a "." or "_" prefix
// would be silently omitted from `skill list` / `skill read` / `skill reference`.
//
//go:embed skills
var skillsEmbedFS embed.FS

func init() {
	// Strip the "skills/" prefix so paths are "lark-calendar/SKILL.md".
	sub, err := fs.Sub(skillsEmbedFS, "skills")
	if err != nil {
		// Unreachable for the literal, valid path "skills"; fs.Sub only errors
		// on a malformed sub-path. If a refactor ever breaks the embed root,
		// fail loud at startup rather than ship a binary whose every `skill`
		// command returns an opaque "not embedded in this build" error.
		panic("skills_embed: fs.Sub(\"skills\") failed: " + err.Error())
	}
	skill.ContentFS = sub
}
