// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDrive_PermissionGetSettingDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	tests := []struct {
		name     string
		args     []string
		wantURL  string
		wantType string
	}{
		{
			name: "bare folder token",
			args: []string{
				"drive", "+permission-get-setting",
				"--token", "fldE2E001",
				"--type", "folder",
				"--dry-run",
			},
			wantURL:  "/open-apis/drive/v2/permissions/fldE2E001/public",
			wantType: "folder",
		},
		{
			name: "folder URL",
			args: []string{
				"drive", "+permission-get-setting",
				"--token", "https://example.feishu.cn/drive/folder/fldE2E001?from=share",
				"--dry-run",
			},
			wantURL:  "/open-apis/drive/v2/permissions/fldE2E001/public",
			wantType: "folder",
		},
		{
			name: "docx URL",
			args: []string{
				"drive", "+permission-get-setting",
				"--token", "https://example.feishu.cn/docx/doxE2E001",
				"--dry-run",
			},
			wantURL:  "/open-apis/drive/v2/permissions/doxE2E001/public",
			wantType: "docx",
		},
		{
			name: "apps page URL infers apps type",
			args: []string{
				"drive", "+permission-get-setting",
				"--token", "https://example.feishu.cn/page/appMetaE2E/?from=share",
				"--dry-run",
			},
			wantURL:  "/open-apis/drive/v2/permissions/appMetaE2E/public",
			wantType: "apps",
		},
		{
			name: "bare token with explicit apps type",
			args: []string{
				"drive", "+permission-get-setting",
				"--token", "appBareMetaE2E",
				"--type", "apps",
				"--dry-run",
			},
			wantURL:  "/open-apis/drive/v2/permissions/appBareMetaE2E/public",
			wantType: "apps",
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			if got := gjson.Get(out, "data.api.0.method").String(); got != "GET" {
				t.Fatalf("method = %q, want GET\nstdout:\n%s", got, out)
			}
			if got := gjson.Get(out, "data.api.0.url").String(); got != tt.wantURL {
				t.Fatalf("url = %q, want %q\nstdout:\n%s", got, tt.wantURL, out)
			}
			if got := gjson.Get(out, "data.api.0.params.type").String(); got != tt.wantType {
				t.Fatalf("params.type = %q, want %q\nstdout:\n%s", got, tt.wantType, out)
			}
			if gjson.Get(out, "data.folder_token").Exists() {
				t.Fatalf("folder_token exists in dry-run output, want omitted\nstdout:\n%s", out)
			}
		})
	}
}

func TestDrive_PermissionGetSettingWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	folderToken := CreateDriveFolder(
		t,
		t,
		ctx,
		"lark-cli-e2e-drive-permission-get-setting-"+clie2e.GenerateSuffix(),
		"bot",
		"",
	)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+permission-get-setting",
			"--token", folderToken,
			"--type", "folder",
			"--format", "json",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 {
		combinedOutput := strings.ToLower(result.Stdout + "\n" + result.Stderr)
		if strings.Contains(combinedOutput, "docs:permission.setting:read") ||
			strings.Contains(combinedOutput, "app scope not enabled") ||
			strings.Contains(combinedOutput, "missing required scope") ||
			strings.Contains(combinedOutput, "99991672") {
			t.Skipf("skip drive permission setting workflow due to missing bot scope docs:permission.setting:read: %s", strings.TrimSpace(result.Stdout+"\n"+result.Stderr))
		}
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	if !gjson.Get(result.Stdout, "data.permission_public").Exists() {
		t.Fatalf("permission_public missing in output\nstdout:\n%s", result.Stdout)
	}
}
