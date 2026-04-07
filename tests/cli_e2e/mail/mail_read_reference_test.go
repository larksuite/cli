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

func TestMail_ReadShortcutReferenceOutputs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	testCases := []struct {
		name string
		req  clie2e.Request
		key  string
	}{
		{
			name: "message print output schema",
			req: clie2e.Request{
				Args:      []string{"mail", "+message", "--message-id", "msg_dummy", "--print-output-schema"},
				DefaultAs: "bot",
			},
			key: "data.fields.message_id",
		},
		{
			name: "messages print output schema",
			req: clie2e.Request{
				Args:      []string{"mail", "+messages", "--message-ids", "msg1,msg2", "--print-output-schema"},
				DefaultAs: "bot",
			},
			key: "data.messages_extra_fields.total",
		},
		{
			name: "thread print output schema",
			req: clie2e.Request{
				Args:      []string{"mail", "+thread", "--thread-id", "thr_dummy", "--print-output-schema"},
				DefaultAs: "bot",
			},
			key: "data.thread_extra_fields.thread_id",
		},
		{
			name: "triage print filter schema",
			req: clie2e.Request{
				Args:      []string{"mail", "+triage", "--print-filter-schema"},
				DefaultAs: "bot",
			},
			key: "data.fields.folder.type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, tc.req)
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			result.AssertStdoutStatus(t, true)

			payload := mailJSONPayload(t, result)
			assert.True(t, gjson.Get(payload, tc.key).Exists(), "stdout:\n%s", result.Stdout)
		})
	}
}
