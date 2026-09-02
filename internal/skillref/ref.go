// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package skillref models structured references into the binary-embedded skill
// tree. It deliberately does not render prose: callers decide which complete
// help/recovery fragment to keep once a reference resolves.
package skillref

import (
	"fmt"
	"io/fs"
	"strings"
)

// Ref is one exact canonical or runtime skill reference. Path is relative to
// the named skill; an empty Path denotes the skill's SKILL.md.
type Ref struct {
	Skill string
	Path  string
}

// Parse parses the "name[/relative/path]" form accepted by `skills read`.
func Parse(raw string) (Ref, error) {
	if raw == "" {
		return Ref{}, fmt.Errorf("skill reference is empty")
	}
	skill, path, _ := strings.Cut(raw, "/")
	if !ValidSkillName(skill) {
		return Ref{}, fmt.Errorf("%q has invalid skill name %q", raw, skill)
	}
	if path != "" && (!fs.ValidPath(path) || path == "." || strings.Contains(path, `\`)) {
		return Ref{}, fmt.Errorf("%q has invalid relative path %q", raw, path)
	}
	if path == "" && strings.HasSuffix(raw, "/") {
		return Ref{}, fmt.Errorf("%q has an empty relative path", raw)
	}
	return Ref{Skill: skill, Path: path}, nil
}

// ValidSkillName reports whether name can identify a top-level skill
// directory. Keep this rule aligned with the skill-tree manifest validator.
func ValidSkillName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}

// String returns the canonical "name[/relative/path]" form.
func (r Ref) String() string {
	if r.Path == "" {
		return r.Skill
	}
	return r.Skill + "/" + r.Path
}

// StatPath returns the path whose existence makes the reference readable.
func (r Ref) StatPath() string {
	if r.Path == "" {
		return r.Skill + "/SKILL.md"
	}
	return r.String()
}
