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
)

func TestMail_BotIdentityConstraints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	testCases := []struct {
		name string
		args []string
	}{
		{
			name: "watch",
			args: []string{"mail", "+watch", "--print-output-schema"},
		},
		{
			name: "reply",
			args: []string{"mail", "+reply", "--message-id", "msg_001", "--body", "hello"},
		},
		{
			name: "reply-all",
			args: []string{"mail", "+reply-all", "--message-id", "msg_001", "--body", "hello"},
		},
		{
			name: "send",
			args: []string{"mail", "+send", "--subject", "hello", "--to", "alice@example.com", "--body", "body"},
		},
		{
			name: "draft-create",
			args: []string{"mail", "+draft-create", "--subject", "hello", "--body", "body"},
		},
		{
			name: "draft-edit",
			args: []string{"mail", "+draft-edit", "--print-patch-template"},
		},
		{
			name: "forward",
			args: []string{"mail", "+forward", "--message-id", "msg_001", "--to", "alice@example.com"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tc.args,
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			assert.Equal(t, 1, result.ExitCode, "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			assert.Contains(t, result.Stderr, "--as bot is not supported", "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			assert.Contains(t, result.Stderr, "only supports: user", "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
		})
	}
}
