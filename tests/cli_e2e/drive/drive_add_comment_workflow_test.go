// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// TestDrive_AddCommentWorkflow tests the add-comment shortcut method.
// Workflow: import a docx -> add a full-document comment -> verify comment created.
func TestDrive_AddCommentWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	testContent := "# Lark CLI E2E Add Comment Test\n\nDocument for testing add-comment shortcut.\nTimestamp: " + suffix

	docToken := importTestDoc(t, parentT, ctx, "add-comment", testContent)
	require.NotEmpty(t, docToken)

	t.Run("add full-document comment", func(t *testing.T) {
		commentContent := `[{"type":"text","text":"lark-cli-e2e-drive-comment-` + suffix + `"}]`

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "+add-comment",
				"--doc",         docToken,
				"--full-comment",
				"--content",     commentContent,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})
}