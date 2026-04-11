// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrive_MetasBatchQueryWorkflow tests the metas batch_query resource command.
// Workflow: import a doc -> batch query metas for the doc.
func TestDrive_MetasBatchQueryWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	testContent := "# Lark CLI E2E Metas Batch Query Test\n\nDocument for testing metas resource.\nTimestamp: " + suffix

	docToken := importTestDoc(t, parentT, ctx, "metas-batch-query", testContent)
	require.NotEmpty(t, docToken)

	t.Run("batch_query - get document metadata", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "metas", "batch_query"},
			Data: map[string]any{
				"request_docs": []map[string]any{
					{"doc_token": docToken, "doc_type": "docx"},
				},
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		// Verify response has metas array
		metas := gjson.Get(result.Stdout, "data.metas")
		require.True(t, metas.IsArray(), "should have metas array, stdout:\n%s", result.Stdout)
	})
}