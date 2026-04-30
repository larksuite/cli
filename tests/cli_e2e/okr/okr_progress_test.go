// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Upload Image Dry-run E2E tests ---

// TestOKR_UploadImageDryRun validates +upload-image dry-run output contains the correct method and API path.
func TestOKR_UploadImageDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+upload-image",
			"--file", "./test.png",
			"--target-id", "123456789",
			"--target-type", "objective",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v1/images/upload"), "dry-run should contain API path, got: %s", output)
	assert.True(t, strings.Contains(output, "POST"), "dry-run should contain POST method, got: %s", output)
}

// TestOKR_UploadImageDryRun_KeyResult validates +upload-image dry-run with key_result target type.
func TestOKR_UploadImageDryRun_KeyResult(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+upload-image",
			"--file", "./test.jpg",
			"--target-id", "987654321",
			"--target-type", "key_result",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v1/images/upload"), "dry-run should contain API path, got: %s", output)
	assert.True(t, strings.Contains(output, "key_result"), "dry-run should contain target type, got: %s", output)
}

// --- Progress Create Dry-run E2E tests ---

// TestOKR_ProgressCreateDryRun validates +progress-create dry-run output contains the correct method and API path.
func TestOKR_ProgressCreateDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+progress-create",
			"--content", `{"blocks":[{"type":"text","text":"test progress"}]}`,
			"--target-id", "123456789",
			"--target-type", "objective",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v1/progress_records/"), "dry-run should contain API path, got: %s", output)
	assert.True(t, strings.Contains(output, "POST"), "dry-run should contain POST method, got: %s", output)
	assert.True(t, strings.Contains(output, "123456789"), "dry-run should contain target-id, got: %s", output)
}

// TestOKR_ProgressCreateDryRun_WithProgress validates +progress-create dry-run with progress rate.
func TestOKR_ProgressCreateDryRun_WithProgress(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+progress-create",
			"--content", `{"blocks":[{"type":"text","text":"test progress"}]}`,
			"--target-id", "123456789",
			"--target-type", "key_result",
			"--progress-percent", "75",
			"--progress-status", "normal",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v1/progress_records/"), "dry-run should contain API path, got: %s", output)
}

// --- Progress Get Dry-run E2E tests ---

// TestOKR_ProgressGetDryRun validates +progress-get dry-run output contains the correct method and API path.
func TestOKR_ProgressGetDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+progress-get",
			"--progress-id", "123456789",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v1/progress_records/123456789"), "dry-run should contain API path with progress-id, got: %s", output)
	assert.True(t, strings.Contains(output, "GET"), "dry-run should contain GET method, got: %s", output)
}

// TestOKR_ProgressGetDryRun_WithUserIDType validates +progress-get dry-run with user-id-type flag.
func TestOKR_ProgressGetDryRun_WithUserIDType(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+progress-get",
			"--progress-id", "987654321",
			"--user-id-type", "union_id",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v1/progress_records/987654321"), "dry-run should contain API path, got: %s", output)
}

// --- Progress Update Dry-run E2E tests ---

// TestOKR_ProgressUpdateDryRun validates +progress-update dry-run output contains the correct method and API path.
func TestOKR_ProgressUpdateDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+progress-update",
			"--progress-id", "123456789",
			"--content", `{"blocks":[{"type":"text","text":"updated progress"}]}`,
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v1/progress_records/123456789"), "dry-run should contain API path with progress-id, got: %s", output)
	assert.True(t, strings.Contains(output, "PUT"), "dry-run should contain PUT method, got: %s", output)
}

// TestOKR_ProgressUpdateDryRun_WithProgress validates +progress-update dry-run with progress rate.
func TestOKR_ProgressUpdateDryRun_WithProgress(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+progress-update",
			"--progress-id", "123456789",
			"--content", `{"blocks":[{"type":"text","text":"updated progress"}]}`,
			"--progress-percent", "100",
			"--progress-status", "done",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v1/progress_records/123456789"), "dry-run should contain API path, got: %s", output)
}

// --- Progress Delete Dry-run E2E tests ---

// TestOKR_ProgressDeleteDryRun validates +progress-delete dry-run output contains the correct method and API path.
func TestOKR_ProgressDeleteDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+progress-delete",
			"--progress-id", "123456789",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v1/progress_records/123456789"), "dry-run should contain API path with progress-id, got: %s", output)
	assert.True(t, strings.Contains(output, "DELETE"), "dry-run should contain DELETE method, got: %s", output)
}

// --- Progress List Dry-run E2E tests ---

// TestOKR_ProgressListDryRun_Objective validates +progress-list dry-run for objective.
func TestOKR_ProgressListDryRun_Objective(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+progress-list",
			"--target-id", "123456789",
			"--target-type", "objective",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v2/objectives/123456789/progresses"), "dry-run should contain objective API path, got: %s", output)
	assert.True(t, strings.Contains(output, "GET"), "dry-run should contain GET method, got: %s", output)
}

// TestOKR_ProgressListDryRun_KeyResult validates +progress-list dry-run for key_result.
func TestOKR_ProgressListDryRun_KeyResult(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+progress-list",
			"--target-id", "987654321",
			"--target-type", "key_result",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v2/key_results/987654321/progresses"), "dry-run should contain key_result API path, got: %s", output)
	assert.True(t, strings.Contains(output, "GET"), "dry-run should contain GET method, got: %s", output)
}
