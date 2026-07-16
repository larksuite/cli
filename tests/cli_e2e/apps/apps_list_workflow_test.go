// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestAppsListWorkflowAsUser exercises `apps +list` against the live service.
// +list is the only apps shortcut that is read-only AND requires no pre-existing
// app_id fixture, so it is the sole command in the domain that can be live-tested
// without leaking tenant state (apps has no +delete endpoint). All assertions are
// tenant-data-independent: the envelope/array shape is checked unconditionally,
// field-level contracts only when items are present. An empty app list is valid.
func TestAppsListWorkflowAsUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	// assertListEnvelope checks the shared contract for every +list invocation:
	// exit 0, ok:true envelope, and data.items is a JSON array. It returns the
	// items result for scenario-specific follow-up assertions. Failure messages
	// reference field paths only (not the full data envelope) so real tenant app
	// names are not written into test logs.
	assertListEnvelope := func(t *testing.T, result *clie2e.Result) gjson.Result {
		t.Helper()
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		items := gjson.Get(result.Stdout, "data.items")
		require.True(t, items.IsArray(), "data.items should be a JSON array")
		return items
	}

	t.Run("default list", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+list"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		items := assertListEnvelope(t, result)

		// Field-level contract only when the tenant has at least one app: every
		// item carries app_id + name, and the shortcut strips icon_url/created_at
		// (apps_list.go projects them away before output). The loop is a no-op on
		// an empty tenant, keeping the test non-flaky.
		for _, item := range items.Array() {
			assert.NotEmpty(t, item.Get("app_id").String(), "each item should have app_id")
			assert.NotEmpty(t, item.Get("name").String(), "each item should have name")
			assert.False(t, item.Get("icon_url").Exists(), "icon_url should be stripped from list output")
			assert.False(t, item.Get("created_at").Exists(), "created_at should be stripped from list output")
		}
	})

	t.Run("page size honored", func(t *testing.T) {
		// Baseline uncapped list first: the page-size=1 cap is only a meaningful
		// assertion when the tenant actually has >= 2 apps. On a near-empty tenant
		// the cap is vacuously satisfied, so we skip the comparison rather than
		// claim coverage we don't have.
		baseline, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+list"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		baselineItems := assertListEnvelope(t, baseline)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+list", "--page-size", "1"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		items := assertListEnvelope(t, result)

		if len(baselineItems.Array()) >= 2 {
			assert.LessOrEqual(t, len(items.Array()), 1, "page-size 1 should cap items at 1 when the tenant has multiple apps")
		}
	})

	t.Run("keyword no match", func(t *testing.T) {
		// A high-entropy keyword that cannot match any real app name; proves the
		// keyword filter is accepted and returns a well-formed (typically empty)
		// list rather than erroring.
		keyword := "lark-cli-e2e-nomatch-" + clie2e.GenerateSuffix()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+list", "--keyword", keyword},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		assertListEnvelope(t, result)
	})

	t.Run("ownership filter mine", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+list", "--ownership", "mine"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		assertListEnvelope(t, result)
	})

	t.Run("app-type filter html", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+list", "--app-type", "html"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		items := assertListEnvelope(t, result)
		// When the server echoes app_type on items, the html filter must return
		// exactly html apps. Conditional so the test does not assume the field is
		// always present in the list response.
		for _, item := range items.Array() {
			if at := item.Get("app_type"); at.Exists() {
				assert.Equal(t, "html", at.String(), "html filter should only return html apps when app_type is present")
			}
		}
	})
}
