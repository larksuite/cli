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

// skillsEmbedFS embeds skill content at build time so the CLI can serve content
// guaranteed to match the binary version.
//
// The patterns whitelist the agent-readable content — each skill's SKILL.md and
// its references/ (plus lark-whiteboard's routes/ and scenes/) — and deliberately
// EXCLUDE machine-resource directories: assets/ (e.g. lark-slides' ~3.4 MB of
// .xml slide templates, which SKILL.md marks as machine resources not to be read
// in full) and scripts/. That keeps ~3.4 MB of never-read bytes out of every
// release binary (≈34.1 → 30.8 MB) while preserving everything `skills read` /
// `skills list` actually serves.
//
// Trade-offs: (1) this is a whitelist, so content placed in a NEW subdirectory
// type (not SKILL.md / references / routes / scenes) would be silently omitted —
// add a pattern here when introducing one; (2) a pattern that matches zero files
// is a build error, so removing routes/ or scenes/ fails loudly rather than
// silently. ("." / "_"-prefixed files are auto-excluded, as with the plain form.)
//
//go:embed skills/*/SKILL.md skills/*/references skills/*/routes skills/*/scenes
var skillsEmbedFS embed.FS

// init registers the embedded skills/ tree as the default skill content
// (rooted at the skill list so paths are "lark-calendar/..."). It runs in the
// standard package build (`go build .`) but NOT the single-file preview build
// (`go build ./main.go`, used by scripts/build-pkg-pr-new.sh) — matching the
// main_*sidecar.go convention of wiring optional features through init side
// effects so main.go stays self-contained and the minimal preview build still
// compiles (it then ships without embedded skills, the pre-existing behavior).
//
// On assembly failure it degrades with a stderr warning rather than panicking:
// the optional skills subsystem must not hold the CLI hostage. The branch is
// effectively unreachable (the compiler rejects a missing skills/ dir; fs.Sub
// only validates path syntax for the literal "skills"), but the trace separates
// that diagnosis from the opaque "not embedded in this build" the commands
// would otherwise report.
//
// Wrapper mains that build their own entrypoint inject content explicitly via
// cmd.Execute(cmd.WithSkillContent(...)) instead; that option overrides this
// default.
func init() {
	sub, err := fs.Sub(skillsEmbedFS, "skills")
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: skills embed assembly failed, skills commands disabled:", err)
		return
	}
	cmd.SetEmbeddedSkillContent(sub)
}
