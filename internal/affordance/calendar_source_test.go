// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"os"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/meta"
	"github.com/stretchr/testify/require"
)

func TestCalendarSubscriptionAffordance(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	for _, command := range []string{"+subscribe", "+unsubscribe"} {
		raw, ok := For(apicatalog.Catalog{}, "calendar", command)
		require.True(t, ok, command)
		guidance, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
		require.True(t, ok, command)
		// The domain skill is the only route; subscription guidance lives in
		// the SKILL.md Shortcuts table, not a dedicated reference.
		require.Contains(t, guidance.Skills, "lark-calendar")
		require.Empty(t, guidance.Tips)
	}
}
