// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddedBaseViewGuideReachable(t *testing.T) {
	const guide = "lark-base-dashboard-view.md"
	for _, route := range []struct{ path, link string }{
		{"skills/lark-base/SKILL.md", "](references/lark-base-dashboard.md)"},
		{"skills/lark-base/references/lark-base-dashboard.md", "](" + guide + ")"},
		{"skills/lark-base/references/lark-base-dashboard-block-config.md", "](" + guide + ")"},
		{"affordance/base.md", "`lark-base/references/" + guide + "`"},
	} {
		content, err := embeddedContentFS.ReadFile(route.path)
		require.NoError(t, err)
		require.Contains(t, string(content), route.link, route.path)
	}
	_, err := fs.Stat(embeddedContentFS, "skills/lark-base/references/"+guide)
	require.NoError(t, err)
}
