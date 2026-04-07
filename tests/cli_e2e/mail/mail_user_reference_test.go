// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMail_UserOnlyReferenceOutputs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	t.Run("watch print output schema", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"mail", "+watch", "--print-output-schema"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		payload := mailJSONPayload(t, result)
		assert.True(t, gjson.Get(payload, "metadata.message.message_id").Exists(), "stdout:\n%s", result.Stdout)
		assert.True(t, gjson.Get(payload, "full.message.attachments").Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("draft edit print patch template", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"mail", "+draft-edit", "--print-patch-template"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		payload := mailJSONPayload(t, result)
		assert.Equal(t, "user", gjson.Get(payload, "identity").String())
		assert.True(t, gjson.Get(payload, "data.template.ops").Exists(), "stdout:\n%s", result.Stdout)
		assert.True(t, gjson.Get(payload, "data.supported_ops_by_group").Exists(), "stdout:\n%s", result.Stdout)
	})
}
