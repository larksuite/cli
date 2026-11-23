// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestDriveMoveDryRunFolderDestination(t *testing.T) {
	setDriveDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"drive", "+move", "--file-token", "folderMoveSource", "--type", "folder", "--folder-token", "folderMoveTarget", "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	out := result.Stdout
	require.Equal(t, int64(2), clie2e.DryRunGet(out, "api.#").Int())
	checks := map[string]string{
		"api.0.method":            "POST",
		"api.0.url":               "/open-apis/drive/v1/files/folderMoveSource/move",
		"api.0.body.type":         "folder",
		"api.0.body.folder_token": "folderMoveTarget",
		"api.1.method":            "GET",
		"api.1.url":               "/open-apis/drive/v1/files/task_check",
		"api.1.params.task_id":    "<task_id>",
	}
	for path, want := range checks {
		require.Equal(t, want, clie2e.DryRunGet(out, path).String(), path)
	}
	require.Contains(t, clie2e.DryRunGet(out, "api.1.desc").String(), "If task_id is returned")
}
