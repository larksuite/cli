// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}

// TestDocs_SearchWorkflow tests the search functionality.
// Note: +search requires user identity and login, which may not be available in CI.
func TestDocs_SearchWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	searchQuery := "lark-cli-e2e-docs-" + suffix

	t.Run("search", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+search",
				"--as", "user",
				"--query", searchQuery,
			},
		})
		// Skip if user login is not available
		if result.ExitCode != 0 && containsString(result.Stderr, "not logged in") {
			t.Skip("user login required for +search")
		}
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		// Search returns a list, verify structure
		hasItems := gjson.Get(result.Stdout, "data.items").Exists()
		assert.True(t, hasItems, "should have items field in search result, stdout:\n%s", result.Stdout)
	})
}