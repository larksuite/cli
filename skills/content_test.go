// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skills

import (
	"io/fs"
	"testing"
)

func TestDefaultFSContainsSkillAndReference(t *testing.T) {
	for _, path := range []string{"lark-doc/SKILL.md", "lark-doc/references/lark-doc-fetch.md"} {
		if _, err := fs.ReadFile(DefaultFS(), path); err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
	}
}
