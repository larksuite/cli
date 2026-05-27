// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMinutesUpdate_DryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"minutes", "+update",
			"--minute-token", "obcnexampleminute",
			"--topic", "新的妙记标题",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "PATCH"), "dry-run should contain PATCH method, got: %s", output)
	assert.True(t, strings.Contains(output, "/open-apis/minutes/v1/minutes/obcnexampleminute"), "dry-run should contain API path, got: %s", output)
	assert.True(t, strings.Contains(output, "新的妙记标题"), "dry-run should contain topic, got: %s", output)
}
