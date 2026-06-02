// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAppsGitCredentialInitDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"apps", "+git-credential-init",
			"--app-id", "app_xxx",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	assert.Equal(t, "GET", gjson.Get(result.Stdout, "api.0.method").String())
	assert.Equal(t, "/open-apis/spark/v1/apps/app_xxx/git_info", gjson.Get(result.Stdout, "api.0.url").String())
	assert.Equal(t, "app_xxx", gjson.Get(result.Stdout, "api.0.params.app_id").String())
	assert.False(t, gjson.Get(result.Stdout, "api.0.body").Exists())
}
