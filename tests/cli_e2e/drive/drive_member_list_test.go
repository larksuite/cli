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

func TestDrive_MemberListDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	tests := []struct {
		name         string
		args         []string
		wantURL      string
		wantType     string
		wantFields   string
		wantPermType string
	}{
		{
			name: "bare folder token",
			args: []string{
				"drive", "+member-list",
				"--token", "fldE2E001",
				"--type", "folder",
				"--dry-run",
			},
			wantURL:  "/open-apis/drive/v1/permissions/fldE2E001/members",
			wantType: "folder",
		},
		{
			name: "folder URL infers folder type",
			args: []string{
				"drive", "+member-list",
				"--token", "https://example.feishu.cn/drive/folder/fldE2E002?from=share",
				"--dry-run",
			},
			wantURL:  "/open-apis/drive/v1/permissions/fldE2E002/members",
			wantType: "folder",
		},
		{
			name: "fields star is passed only when explicit",
			args: []string{
				"drive", "+member-list",
				"--token", "doxE2E003",
				"--type", "docx",
				"--fields", "*",
				"--dry-run",
			},
			wantURL:    "/open-apis/drive/v1/permissions/doxE2E003/members",
			wantType:   "docx",
			wantFields: "*",
		},
		{
			name: "wiki perm type",
			args: []string{
				"drive", "+member-list",
				"--token", "wikE2E004",
				"--type", "wiki",
				"--fields", "name,type",
				"--perm-type", "single_page",
				"--dry-run",
			},
			wantURL:      "/open-apis/drive/v1/permissions/wikE2E004/members",
			wantType:     "wiki",
			wantFields:   "name,type",
			wantPermType: "single_page",
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
			if tt.wantFields == "" {
				if gjson.Get(out, "data.api.0.params.fields").Exists() {
					t.Fatalf("params.fields should be omitted\nstdout:\n%s", out)
				}
			} else if got := gjson.Get(out, "data.api.0.params.fields").String(); got != tt.wantFields {
				t.Fatalf("params.fields = %q, want %q\nstdout:\n%s", got, tt.wantFields, out)
			}
			if tt.wantPermType == "" {
				if gjson.Get(out, "data.api.0.params.perm_type").Exists() {
					t.Fatalf("params.perm_type should be omitted\nstdout:\n%s", out)
				}
			} else if got := gjson.Get(out, "data.api.0.params.perm_type").String(); got != tt.wantPermType {
				t.Fatalf("params.perm_type = %q, want %q\nstdout:\n%s", got, tt.wantPermType, out)
			}
		})
	}
}

func TestDrive_MemberListWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	folderName := "lark-cli-e2e-drive-member-list-" + clie2e.GenerateSuffix()
	folderToken := createDriveFolderOrSkipPermission(t, parentT, ctx, folderName)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+member-list",
			"--token", folderToken,
			"--type", "folder",
			"--format", "json",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 {
		combinedOutput := strings.ToLower(result.Stdout + "\n" + result.Stderr)
		if strings.Contains(combinedOutput, "docs:permission.member:retrieve") ||
			strings.Contains(combinedOutput, "app scope not enabled") ||
			strings.Contains(combinedOutput, "missing required scope") ||
			strings.Contains(combinedOutput, "missing_scope") ||
			strings.Contains(combinedOutput, "99991672") ||
			strings.Contains(combinedOutput, "1063002") ||
			strings.Contains(combinedOutput, "1063004") ||
			strings.Contains(combinedOutput, "permission denied") ||
			strings.Contains(combinedOutput, "no share permission") {
			t.Skipf("skip drive member list workflow due to missing bot scope or folder permission: %s", strings.TrimSpace(result.Stdout+"\n"+result.Stderr))
		}
		if strings.Contains(combinedOutput, "99992402") &&
			strings.Contains(combinedOutput, "field validation failed") {
			t.Skipf("skip drive member list workflow because this environment does not yet accept type=folder on the member list API: %s", strings.TrimSpace(result.Stdout+"\n"+result.Stderr))
		}
		t.Fatalf("drive member list workflow failed: exit=%d\nstdout:\n%s\nstderr:\n%s", result.ExitCode, result.Stdout, result.Stderr)
	}
	result.AssertStdoutStatus(t, true)

	if items := gjson.Get(result.Stdout, "data.items"); !items.Exists() || !items.IsArray() {
		t.Fatalf("data.items must be present as an array\nstdout:\n%s", result.Stdout)
	}
}
