// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package skills exposes the repository's default embedded skill content.
package skills

import (
	"embed"
	"io/fs"
)

//go:embed */SKILL.md */references */routes */scenes
var content embed.FS

// DefaultFS returns the immutable default skill tree rooted at skill names.
func DefaultFS() fs.FS { return content }
