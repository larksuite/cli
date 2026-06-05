// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBase_AttachmentDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "report.txt"), []byte("hello"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(workDir, "downloads"), 0o700))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("upload", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+record-upload-attachment",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--record-id", "rec_x",
				"--field-id", "fld_att",
				"--file", "report.txt",
				"--dry-run",
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		out := result.Stdout
		require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_att", gjson.Get(out, "api.0.url").String(), out)
		require.Equal(t, "/open-apis/drive/v1/medias/upload_all", gjson.Get(out, "api.1.url").String(), out)
		require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/append_attachments", gjson.Get(out, "api.2.url").String(), out)
		require.Equal(t, "<uploaded_file_token>", gjson.Get(out, "api.2.body.attachments.rec_x.fld_att.0.file_token").String(), out)
	})

	t.Run("download", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+record-download-attachment",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--record-id", "rec_x",
				"--file-token", "box_a",
				"--output", "downloads",
				"--dry-run",
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		out := result.Stdout
		require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/get_attachments", gjson.Get(out, "api.0.url").String(), out)
		require.Equal(t, "/open-apis/drive/v1/medias/%3Cfile_token%3E/download", gjson.Get(out, "api.1.url").String(), out)
		require.Equal(t, "<extra_info_if_present>", gjson.Get(out, "api.1.params.extra").String(), out)
	})

	t.Run("download all", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+record-download-attachment",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--record-id", "rec_x",
				"--output", "downloads",
				"--dry-run",
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		out := result.Stdout
		require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/get_attachments", gjson.Get(out, "api.0.url").String(), out)
		require.Equal(t, "/open-apis/drive/v1/medias/%3Cfile_token%3E/download", gjson.Get(out, "api.1.url").String(), out)
	})

	t.Run("remove", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+record-remove-attachment",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--record-id", "rec_x",
				"--field-id", "fld_att",
				"--file-token", "box_a",
				"--dry-run",
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		out := result.Stdout
		require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/remove_attachments", gjson.Get(out, "api.0.url").String(), out)
		require.Equal(t, "box_a", gjson.Get(out, "api.0.body.attachments.rec_x.fld_att.0.file_token").String(), out)
	})
}

func setBaseDryRunConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "base_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "base_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
