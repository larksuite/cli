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

func TestMinutesSpeakerReplace_DryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"minutes", "+speaker-replace",
			"--minute-token", "obcnexampleminute",
			"--from-user-id", "ou_old_speaker",
			"--to-user-id", "ou_new_speaker",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "PUT"), "dry-run should contain PUT method, got: %s", output)
	assert.True(t, strings.Contains(output, "/open-apis/minutes/v1/minutes/obcnexampleminute/transcript/speaker"), "dry-run should contain API path, got: %s", output)
	assert.True(t, strings.Contains(output, "ou_old_speaker"), "dry-run should contain from_user_id, got: %s", output)
	assert.True(t, strings.Contains(output, "ou_new_speaker"), "dry-run should contain to_user_id, got: %s", output)
}

func TestMinutesSpeakerReplace_DryRun_FromSpeakerID(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"minutes", "+speaker-replace",
			"--minute-token", "obcnexampleminute",
			"--from-speaker-id", "ENCRYPTED_TOKEN_ABC",
			"--to-user-id", "ou_new_speaker",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "PUT"), "dry-run should contain PUT method, got: %s", output)
	assert.True(t, strings.Contains(output, "/open-apis/minutes/v1/minutes/obcnexampleminute/transcript/speaker"), "dry-run should contain API path, got: %s", output)
	assert.True(t, strings.Contains(output, "from_speaker_id"), "dry-run should contain from_speaker_id, got: %s", output)
	assert.True(t, strings.Contains(output, "ENCRYPTED_TOKEN_ABC"), "dry-run should contain the encrypted speaker id, got: %s", output)
	assert.False(t, strings.Contains(output, "from_user_id"), "dry-run should not contain from_user_id when from-speaker-id is set, got: %s", output)
	assert.False(t, strings.Contains(output, "/transcript/speakerlist"), "+speaker-replace should never fetch speakerlist, got: %s", output)
}
