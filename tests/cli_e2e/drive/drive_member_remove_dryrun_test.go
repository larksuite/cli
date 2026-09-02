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

func TestDrive_MemberRemoveDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	tests := []struct {
		name             string
		args             []string
		wantURL          string
		wantResourceType string
		wantMemberType   string
		wantMemberKind   string
		wantPermType     string
	}{
		{
			name: "docx URL infers resource type",
			args: []string{
				"drive", "+member-remove",
				"--token", "https://example.feishu.cn/docx/doxcnRemove001?from=share",
				"--member-id", "ou_remove_user",
				"--member-type", "openid",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/doxcnRemove001/members/ou_remove_user",
			wantResourceType: "docx",
			wantMemberType:   "openid",
			wantMemberKind:   "user",
		},
		{
			name: "user ID is accepted",
			args: []string{
				"drive", "+member-remove",
				"--token", "doxcnRemoveUserID",
				"--type", "docx",
				"--member-id", "tenant_defined_user",
				"--member-type", "userid",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/doxcnRemoveUserID/members/tenant_defined_user",
			wantResourceType: "docx",
			wantMemberType:   "userid",
			wantMemberKind:   "user",
		},
		{
			name: "app ID is accepted",
			args: []string{
				"drive", "+member-remove",
				"--token", "doxcnRemoveAppID",
				"--type", "docx",
				"--member-id", "cli_app_123",
				"--member-type", "appid",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/doxcnRemoveAppID/members/cli_app_123",
			wantResourceType: "docx",
			wantMemberType:   "appid",
		},
		{
			name: "ordinary wiki member defaults container scope",
			args: []string{
				"drive", "+member-remove",
				"--token", "wikcnE2E002",
				"--type", "wiki",
				"--member-id", "oc_remove_chat",
				"--member-type", "openchat",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/wikcnE2E002/members/oc_remove_chat",
			wantResourceType: "wiki",
			wantMemberType:   "openchat",
			wantMemberKind:   "chat",
			wantPermType:     "container",
		},
		{
			name: "wiki single-page scope",
			args: []string{
				"drive", "+member-remove",
				"--token", "wikcnE2E003",
				"--type", "wiki",
				"--member-id", "ou_remove_user",
				"--member-type", "openid",
				"--perm-type", "single_page",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/wikcnE2E003/members/ou_remove_user",
			wantResourceType: "wiki",
			wantMemberType:   "openid",
			wantMemberKind:   "user",
			wantPermType:     "single_page",
		},
		{
			name: "wiki-space member uses explicit kind without perm type",
			args: []string{
				"drive", "+member-remove",
				"--token", "wikcnE2E004",
				"--type", "wiki",
				"--member-id", "space_remove_member",
				"--member-type", "wikispaceid",
				"--member-kind", "wiki_space_editor",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/wikcnE2E004/members/space_remove_member",
			wantResourceType: "wiki",
			wantMemberType:   "wikispaceid",
			wantMemberKind:   "wiki_space_editor",
		},
		{
			name: "apps page URL infers resource type",
			args: []string{
				"drive", "+member-remove",
				"--token", "https://example.feishu.cn/page/appRemovePageTok?from=share",
				"--member-id", "ou_remove_user",
				"--member-type", "openid",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/appRemovePageTok/members/ou_remove_user",
			wantResourceType: "apps",
			wantMemberType:   "openid",
			wantMemberKind:   "user",
		},
		{
			name: "apps bare token with explicit type",
			args: []string{
				"drive", "+member-remove",
				"--token", "appRemoveTok",
				"--type", "apps",
				"--member-id", "ou_remove_user",
				"--member-type", "openid",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/appRemoveTok/members/ou_remove_user",
			wantResourceType: "apps",
			wantMemberType:   "openid",
			wantMemberKind:   "user",
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: tt.args, DefaultAs: "user"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			require.Equal(t, "DELETE", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
			require.Equal(t, tt.wantURL, clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
			require.Equal(t, tt.wantResourceType, clie2e.DryRunGet(out, "api.0.params.type").String(), "stdout:\n%s", out)
			require.Equal(t, tt.wantMemberType, clie2e.DryRunGet(out, "api.0.params.member_type").String(), "stdout:\n%s", out)
			require.Equal(t, tt.wantMemberKind, clie2e.DryRunGet(out, "api.0.body.type").String(), "stdout:\n%s", out)

			permType := clie2e.DryRunGet(out, "api.0.body.perm_type")
			if tt.wantPermType == "" {
				require.False(t, permType.Exists(), "perm_type should be omitted\nstdout:\n%s", out)
			} else {
				require.Equal(t, tt.wantPermType, permType.String(), "stdout:\n%s", out)
			}
		})
	}
}

func TestDrive_MemberRemoveDryRunRejectsInvalidInputs(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	tests := []struct {
		name      string
		args      []string
		defaultAs string
		wantErr   string
	}{
		{
			name:    "slash in normalized token is rejected",
			args:    []string{"drive", "+member-remove", "--token", "token/with/slash", "--type", "docx", "--member-id", "ou_user", "--member-type", "openid", "--dry-run"},
			wantErr: "--token must resolve to a single resource token and cannot contain '/'",
		},
		{
			name:    "slash in member ID is rejected",
			args:    []string{"drive", "+member-remove", "--token", "doxcnRemove", "--type", "docx", "--member-id", "userid/with/slash", "--member-type", "userid", "--dry-run"},
			wantErr: "--member-id must be a single collaborator ID and cannot contain '/'",
		},
		{
			name:    "blank member ID is rejected",
			args:    []string{"drive", "+member-remove", "--token", "doxcnRemove", "--type", "docx", "--member-id", "   ", "--member-type", "openid", "--dry-run"},
			wantErr: "--member-id is required and cannot be blank",
		},
		{
			name:    "bare token requires type",
			args:    []string{"drive", "+member-remove", "--token", "doxcnRemove", "--member-id", "ou_user", "--member-type", "openid", "--dry-run"},
			wantErr: "--type is required when --token is a bare token",
		},
		{
			name:    "multiple members are rejected",
			args:    []string{"drive", "+member-remove", "--token", "doxcnRemove", "--type", "docx", "--member-id", "ou_a,ou_b", "--member-type", "openid", "--dry-run"},
			wantErr: "exactly one collaborator ID",
		},
		{
			name:    "wiki-space ID requires member kind",
			args:    []string{"drive", "+member-remove", "--token", "wikcnRemove", "--type", "wiki", "--member-id", "space_member", "--member-type", "wikispaceid", "--dry-run"},
			wantErr: "--member-kind is required",
		},
		{
			name:    "wiki-space ID is rejected outside wiki",
			args:    []string{"drive", "+member-remove", "--token", "doxcnRemove", "--type", "docx", "--member-id", "space_member", "--member-type", "wikispaceid", "--member-kind", "wiki_space_member", "--dry-run"},
			wantErr: "only applies when resource type is wiki",
		},
		{
			name:    "non-wiki rejects perm type",
			args:    []string{"drive", "+member-remove", "--token", "doxcnRemove", "--type", "docx", "--member-id", "ou_user", "--member-type", "openid", "--perm-type", "single_page", "--dry-run"},
			wantErr: "only applies when resource type is wiki",
		},
		{
			name:    "apps rejects perm type",
			args:    []string{"drive", "+member-remove", "--token", "appRemoveBadTok", "--type", "apps", "--member-id", "ou_user", "--member-type", "openid", "--perm-type", "single_page", "--dry-run"},
			wantErr: `resource type is wiki; got \"apps\"`,
		},
		{
			name:      "bot rejects department member",
			args:      []string{"drive", "+member-remove", "--token", "doxcnRemove", "--type", "docx", "--member-id", "od_department", "--member-type", "opendepartmentid", "--dry-run"},
			defaultAs: "bot",
			wantErr:   "requires --as user",
		},
		{
			name:    "url with extra path segment is rejected",
			args:    []string{"drive", "+member-remove", "--token", "https://example.feishu.cn/docx/doxReal/other", "--member-id", "ou_user", "--member-type", "openid", "--dry-run"},
			wantErr: "unsupported URL path",
		},
		{
			name:    "url with encoded separator is rejected",
			args:    []string{"drive", "+member-remove", "--token", "https://example.feishu.cn/docx/doxReal%2Fother", "--member-id", "ou_user", "--member-type", "openid", "--dry-run"},
			wantErr: "unsupported URL path",
		},
		{
			// Double-encoded separator: PathUnescape decodes one level to the
			// single literal token "doxReal%2Fother" (still holding a '%', no
			// real '/'), so it survives the URL segment check but is then
			// rejected by validate.ResourceName's unsafe-character rule before any
			// request is shaped. This pins that the '%' can never reach the DELETE
			// path, so the token cannot silently address a different resource.
			name:    "url with double-encoded separator is rejected",
			args:    []string{"drive", "+member-remove", "--token", "https://example.feishu.cn/docx/doxReal%252Fother", "--member-id", "ou_user", "--member-type", "openid", "--dry-run"},
			wantErr: "--token contains invalid characters",
		},
		{
			name:    "type conflicting with url path is rejected",
			args:    []string{"drive", "+member-remove", "--token", "https://example.feishu.cn/docx/doxReal", "--type", "wiki", "--member-id", "ou_user", "--member-type", "openid", "--dry-run"},
			wantErr: "conflicts with URL path type",
		},
		{
			name:    "bare dot-dot token is rejected",
			args:    []string{"drive", "+member-remove", "--token", "..", "--type", "docx", "--member-id", "ou_user", "--member-type", "openid", "--dry-run"},
			wantErr: "--token must not contain '..' path traversal",
		},
		{
			name:    "encoded dot-dot in url path is rejected",
			args:    []string{"drive", "+member-remove", "--token", "https://example.feishu.cn/docx/%2e%2e", "--member-id", "ou_user", "--member-type", "openid", "--dry-run"},
			wantErr: "--token must not contain '..' path traversal",
		},
		{
			name:    "dot-dot member ID is rejected",
			args:    []string{"drive", "+member-remove", "--token", "doxcnRemove", "--type", "docx", "--member-id", "..", "--member-type", "openid", "--dry-run"},
			wantErr: "--member-id must not contain '..' path traversal",
		},
		{
			name:    "bare single-dot token is rejected",
			args:    []string{"drive", "+member-remove", "--token", ".", "--type", "docx", "--member-id", "ou_user", "--member-type", "openid", "--dry-run"},
			wantErr: "--token must not contain '.' or '..' path segments",
		},
		{
			name:    "encoded single-dot in url path is rejected",
			args:    []string{"drive", "+member-remove", "--token", "https://example.feishu.cn/docx/%2e", "--member-id", "ou_user", "--member-type", "openid", "--dry-run"},
			wantErr: "--token must not contain '.' or '..' path segments",
		},
		{
			name:    "single-dot member ID is rejected",
			args:    []string{"drive", "+member-remove", "--token", "doxcnRemove", "--type", "docx", "--member-id", ".", "--member-type", "openid", "--dry-run"},
			wantErr: "--member-id must not contain '.' or '..' path segments",
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			defaultAs := tt.defaultAs
			if defaultAs == "" {
				defaultAs = "user"
			}

			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: tt.args, DefaultAs: defaultAs})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			require.Contains(t, result.Stdout+result.Stderr, tt.wantErr, "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
		})
	}
}

func TestDrive_MemberRemoveRequiresConfirmation(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+member-remove",
			"--token", "doxcnRemove",
			"--type", "docx",
			"--member-id", "ou_user",
			"--member-type", "openid",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 10)
	require.Contains(t, result.Stderr, "confirmation_required", "stderr:\n%s", result.Stderr)
}
