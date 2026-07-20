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

func TestMinutesApplyPermission_DryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"minutes", "+apply-permission",
			"--minute-token", "obcnexampleminute",
			"--perm", "view",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "POST"), "dry-run should contain POST method, got: %s", output)
	assert.True(t, strings.Contains(output, "/open-apis/minutes/v1/minutes/obcnexampleminute/permissions/apply"), "dry-run should contain API path, got: %s", output)
	assert.True(t, strings.Contains(output, `"perm": "view"`) || strings.Contains(output, `"perm":"view"`), "dry-run should contain perm body, got: %s", output)
}

func TestMinutesApplyPermission_InvalidPerm(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"minutes", "+apply-permission",
			"--minute-token", "obcnexampleminute",
			"--perm", "full_access",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	assert.True(t, strings.Contains(result.Stderr, "--perm"), "stderr should name --perm, got: %s", result.Stderr)
	assert.True(t, strings.Contains(result.Stderr, "view") && strings.Contains(result.Stderr, "edit"), "stderr should list allowed values, got: %s", result.Stderr)
}
