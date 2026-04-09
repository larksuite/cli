// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDocs_WhiteboardUpdateWorkflow tests the whiteboard-update functionality.
// Note: whiteboard-update reads DSL from stdin, which is not supported by the
// current test harness. This test is skipped unless stdin support is added.
func TestDocs_WhiteboardUpdateWorkflow(t *testing.T) {
	t.Skip("whiteboard-update requires stdin support in test harness")

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	docTitle := "lark-cli-e2e-whiteboard-" + suffix

	var docToken string
	var whiteboardToken string

	t.Run("create-document-with-whiteboard", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+create",
				"--title", docTitle,
				"--markdown", `<whiteboard type="blank"></whiteboard>`,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		docToken = gjson.Get(result.Stdout, "data.doc_id").String()
		require.NotEmpty(t, docToken, "stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			// best-effort cleanup
		})
	})

	t.Run("fetch-to-get-whiteboard-token", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before fetch")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"docs", "+fetch"},
			Params: map[string]any{
				"doc": docToken,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		whiteboardToken = gjson.Get(result.Stdout, "data.blocks.0.block_id").String()
	})

	t.Run("whiteboard-update", func(t *testing.T) {
		require.NotEmpty(t, whiteboardToken, "whiteboard token should be available")
		// DSL is read from stdin, not supported by current harness
		// This test would use:
		// result, err := clie2e.RunCmd(ctx, clie2e.Request{
		// 	Args: []string{"docs", "+whiteboard-update"},
		// 	Params: map[string]any{
		// 		"whiteboard-token": whiteboardToken,
		// 	},
		// })
	})
}