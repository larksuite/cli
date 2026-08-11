// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package affordance exposes the repository's default embedded command guidance.
package affordance

import (
	"embed"
	"io/fs"
)

//go:embed *.md
var content embed.FS

// DefaultFS returns the immutable default affordance tree rooted at domain files.
func DefaultFS() fs.FS { return content }
