// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestTask_CommentWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	commentContent := "lark-cli-e2e-comment-" + suffix
	taskGUID := createTask(t, parentT, ctx, clie2e.Request{
		Args: []string{"task", "+create"},
		Data: map[string]any{
			"summary":     "lark-cli-e2e-comment-task-" + suffix,
			"description": "created by tests/cli_e2e/task comment workflow",
		},
	})

	t.Run("comment", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"task", "+comment", "--task-id", taskGUID, "--content", commentContent},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		assert.NotEmpty(t, gjson.Get(result.Stdout, "data.id").String(), "stdout:\n%s", result.Stdout)
	})
}
