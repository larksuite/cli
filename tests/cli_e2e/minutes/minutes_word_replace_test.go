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

func TestMinutesWordReplace_DryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"minutes", "+word-replace",
			"--minute-token", "obcnexampleminute",
			"--replace-words", `[{"source_word":"foo","target_word":"bar"}]`,
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "PUT"), "dry-run should contain PUT method, got: %s", output)
	assert.True(t, strings.Contains(output, "/open-apis/minutes/v1/minutes/obcnexampleminute/transcript/word"), "dry-run should contain API path, got: %s", output)
	assert.True(t, strings.Contains(output, "foo"), "dry-run should contain source_word, got: %s", output)
	assert.True(t, strings.Contains(output, "bar"), "dry-run should contain target_word, got: %s", output)
}
