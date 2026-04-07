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

func TestMail_TriagePermissionConstraint_Bot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"mail", "+triage", "--max", "1", "--format", "json"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode, "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)

	payload := mailJSONPayload(t, result)
	assert.Equal(t, "permission", gjson.Get(payload, "error.type").String())
	assert.Equal(t, "bot", gjson.Get(payload, "identity").String())
	assert.Contains(t, gjson.Get(payload, "error.message").String(), "mail:user_mailbox.message:readonly")
}
