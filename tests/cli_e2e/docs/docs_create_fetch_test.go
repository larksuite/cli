// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDocs_CreateAndFetchWorkflow tests the create and fetch lifecycle.
func TestDocs_CreateAndFetchWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	docTitle := "lark-cli-e2e-docs-" + suffix
	docContent := "# Test Document\n\nThis document was created by lark-cli e2e test."

	var docToken string

	t.Run("create", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+create",
				"--title", docTitle,
				"--markdown", docContent,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		docToken = gjson.Get(result.Stdout, "data.doc_id").String()
		require.NotEmpty(t, docToken, "stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			// No docs delete command is currently available in lark-cli,
			// so created docs are intentionally left in the test account.
		})
	})

	t.Run("fetch", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before fetch")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+fetch",
				"--doc", docToken,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, docTitle, gjson.Get(result.Stdout, "data.title").String())
	})
}
