// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrive_MemberAddDryRun locks in the request shape the shortcut emits
// under --dry-run: the real CLI binary is invoked end-to-end (so flag parsing,
// validation, and dry-run rendering all execute), and the printed request is
// inspected to confirm
//   - HTTP method, URL template, and token path segment,
//   - type query parameter (auto-inferred from a URL input, explicit for a
//     bare token input),
//   - explicit --type overriding URL inference,
//   - member_type / member kind / perm / perm_type body fields,
//   - single-member vs batch endpoint selection.
//
// Fake credentials are sufficient because --dry-run short-circuits before any
// network call.
func TestDrive_MemberAddDryRun(t *testing.T) {
	// Isolate from any local CLI state: the subprocess inherits the parent
	// test environment, and without an explicit config dir it could read a
	// developer's real credentials/profile instead of the fake ones below.
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	tests := []struct {
		name                 string
		args                 []string
		wantURL              string
		wantResourceType     string
		wantNeedNotification string
		wantMemberID         string
		wantMemberType       string
		wantPerm             string
		wantMemberKind       string
		wantPermType         string
		wantBatch            bool
	}{
		{
			name: "URL input auto-infers docx type",
			args: []string{
				"drive", "+member-add",
				"--token", "https://example.feishu.cn/docx/doxcnE2E001?from=share",
				"--member-id", "ou_e2e_user",
				"--member-type", "openid",
				"--perm", "view",
				"--need-notification=false",
				"--dry-run",
			},
			wantURL:              "/open-apis/drive/v1/permissions/doxcnE2E001/members",
			wantResourceType:     "docx",
			wantNeedNotification: "false",
			wantMemberID:         "ou_e2e_user",
			wantMemberType:       "openid",
			wantPerm:             "view",
			wantMemberKind:       "user",
		},
		{
			name: "URL input auto-infers mindnote type from mindnotes path",
			args: []string{
				"drive", "+member-add",
				"--token", "https://example.feishu.cn/mindnotes/mndE2E011?from=share",
				"--member-id", "ou_e2e_user",
				"--member-type", "openid",
				"--perm", "view",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/mndE2E011/members",
			wantResourceType: "mindnote",
			wantMemberID:     "ou_e2e_user",
			wantMemberType:   "openid",
			wantPerm:         "view",
			wantMemberKind:   "user",
		},
		{
			name: "bare token with explicit wiki type defaults perm_type container",
			args: []string{
				"drive", "+member-add",
				"--token", "wikcnE2E002",
				"--type", "wiki",
				"--member-id", "ou_e2e_admin",
				"--member-type", "openid",
				"--perm", "full_access",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/wikcnE2E002/members",
			wantResourceType: "wiki",
			wantMemberID:     "ou_e2e_admin",
			wantMemberType:   "openid",
			wantPerm:         "full_access",
			wantMemberKind:   "user",
			wantPermType:     "container",
		},
		{
			// Explicit --type must override URL inference: the /docx/ marker
			// would infer type=docx, but the caller asked to grant permission
			// against a wiki node. The URL token itself is still used as the
			// path token.
			name: "explicit --type overrides URL inference",
			args: []string{
				"drive", "+member-add",
				"--token", "https://example.feishu.cn/docx/doxcnE2E009",
				"--type", "wiki",
				"--member-id", "ou_e2e_override",
				"--member-type", "openid",
				"--perm", "view",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/doxcnE2E009/members",
			wantResourceType: "wiki",
			wantMemberID:     "ou_e2e_override",
			wantMemberType:   "openid",
			wantPerm:         "view",
			wantMemberKind:   "user",
			wantPermType:     "container",
		},
		{
			name: "bare token with explicit folder type",
			args: []string{
				"drive", "+member-add",
				"--token", "fldE2E010",
				"--type", "folder",
				"--member-id", "ou_e2e_folder",
				"--member-type", "openid",
				"--perm", "view",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/fldE2E010/members",
			wantResourceType: "folder",
			wantMemberID:     "ou_e2e_folder",
			wantMemberType:   "openid",
			wantPerm:         "view",
			wantMemberKind:   "user",
		},
		{
			name: "email member-type",
			args: []string{
				"drive", "+member-add",
				"--token", "shtcnE2E003",
				"--type", "sheet",
				"--member-id", "user@example.com",
				"--member-type", "email",
				"--perm", "edit",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/shtcnE2E003/members",
			wantResourceType: "sheet",
			wantMemberID:     "user@example.com",
			wantMemberType:   "email",
			wantPerm:         "edit",
			wantMemberKind:   "user",
		},
		{
			name: "unionid member-type",
			args: []string{
				"drive", "+member-add",
				"--token", "doxcnE2E006",
				"--type", "docx",
				"--member-id", "on_e2e_union",
				"--member-type", "unionid",
				"--perm", "view",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/doxcnE2E006/members",
			wantResourceType: "docx",
			wantMemberID:     "on_e2e_union",
			wantMemberType:   "unionid",
			wantPerm:         "view",
			wantMemberKind:   "user",
		},
		{
			name: "explicit-only groupid member-type",
			args: []string{
				"drive", "+member-add",
				"--token", "doxcnE2E007",
				"--type", "docx",
				"--member-id", "group_e2e",
				"--member-type", "groupid",
				"--perm", "view",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/doxcnE2E007/members",
			wantResourceType: "docx",
			wantMemberID:     "group_e2e",
			wantMemberType:   "groupid",
			wantPerm:         "view",
			wantMemberKind:   "group",
		},
		{
			name: "batch members use batch_create endpoint",
			args: []string{
				"drive", "+member-add",
				"--token", "bascnE2E004",
				"--type", "bitable",
				"--member-id", "ou_a,ou_b",
				"--member-type", "openid",
				"--perm", "view",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/bascnE2E004/members/batch_create",
			wantResourceType: "bitable",
			wantMemberID:     "ou_a",
			wantMemberType:   "openid",
			wantPerm:         "view",
			wantMemberKind:   "user",
			wantBatch:        true,
		},
		{
			name: "explicit groupid member-type",
			args: []string{
				"drive", "+member-add",
				"--token", "doxcnE2E005",
				"--type", "docx",
				"--member-id", "grp_abc",
				"--member-type", "groupid",
				"--perm", "edit",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/doxcnE2E005/members",
			wantResourceType: "docx",
			wantMemberID:     "grp_abc",
			wantMemberType:   "groupid",
			wantPerm:         "edit",
			wantMemberKind:   "group",
		},
		{
			name: "appid member-type is passed through",
			args: []string{
				"drive", "+member-add",
				"--token", "doxcnE2E008",
				"--type", "docx",
				"--member-id", "cli_app_123",
				"--member-type", "appid",
				"--perm", "view",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/doxcnE2E008/members",
			wantResourceType: "docx",
			wantMemberID:     "cli_app_123",
			wantMemberType:   "appid",
			wantPerm:         "view",
		},
		{
			name: "wikispaceid member-type requires wiki-space body type",
			args: []string{
				"drive", "+member-add",
				"--token", "doxcnE2E012",
				"--type", "docx",
				"--member-id", "spc_e2e_wiki",
				"--member-type", "wikispaceid",
				"--member-kind", "wiki_space_editor",
				"--perm", "view",
				"--dry-run",
			},
			wantURL:          "/open-apis/drive/v1/permissions/doxcnE2E012/members",
			wantResourceType: "docx",
			wantMemberID:     "spc_e2e_wiki",
			wantMemberType:   "wikispaceid",
			wantPerm:         "view",
			wantMemberKind:   "wiki_space_editor",
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			if got := gjson.Get(out, "api.0.method").String(); got != "POST" {
				t.Fatalf("method = %q, want POST\nstdout:\n%s", got, out)
			}
			if got := gjson.Get(out, "api.0.url").String(); got != tt.wantURL {
				t.Fatalf("url = %q, want %q\nstdout:\n%s", got, tt.wantURL, out)
			}
			if got := gjson.Get(out, "api.0.params.type").String(); got != tt.wantResourceType {
				t.Fatalf("params.type = %q, want %q\nstdout:\n%s", got, tt.wantResourceType, out)
			}
			notification := gjson.Get(out, "api.0.params.need_notification")
			if tt.wantNeedNotification == "" {
				if notification.Exists() {
					t.Fatalf("need_notification should be omitted\nstdout:\n%s", out)
				}
			} else if got := notification.String(); got != tt.wantNeedNotification {
				t.Fatalf("need_notification = %q, want %q\nstdout:\n%s", got, tt.wantNeedNotification, out)
			}
			bodyPath := "api.0.body"
			if tt.wantBatch {
				bodyPath = "api.0.body.members.0"
				if count := len(gjson.Get(out, "api.0.body.members").Array()); count != 2 {
					t.Fatalf("body.members count = %d, want 2\nstdout:\n%s", count, out)
				}
			}
			if got := gjson.Get(out, bodyPath+".member_id").String(); got != tt.wantMemberID {
				t.Fatalf("body.member_id = %q, want %q\nstdout:\n%s", got, tt.wantMemberID, out)
			}
			if got := gjson.Get(out, bodyPath+".member_type").String(); got != tt.wantMemberType {
				t.Fatalf("body.member_type = %q, want %q\nstdout:\n%s", got, tt.wantMemberType, out)
			}
			if got := gjson.Get(out, bodyPath+".perm").String(); got != tt.wantPerm {
				t.Fatalf("body.perm = %q, want %q\nstdout:\n%s", got, tt.wantPerm, out)
			}
			if got := gjson.Get(out, bodyPath+".type").String(); got != tt.wantMemberKind {
				t.Fatalf("body.type = %q, want %q\nstdout:\n%s", got, tt.wantMemberKind, out)
			}
			permType := gjson.Get(out, bodyPath+".perm_type")
			if tt.wantPermType == "" {
				if permType.Exists() {
					t.Fatalf("perm_type should be omitted\nstdout:\n%s", out)
				}
			} else if got := permType.String(); got != tt.wantPermType {
				t.Fatalf("body.perm_type = %q, want %q\nstdout:\n%s", got, tt.wantPermType, out)
			}
		})
	}
}

func TestDrive_MemberAddDryRunRejectsInvalidInputs(t *testing.T) {
	// Isolate from any local CLI state: the subprocess inherits the parent
	// test environment, and without an explicit config dir it could read a
	// developer's real credentials/profile instead of the fake ones below.
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
			name: "any host accepted",
			args: []string{
				"drive", "+member-add",
				"--token", "https://google.com/docx/doxcnE2E001",
				"--member-id", "ou_e2e_user",
				"--member-type", "openid",
				"--dry-run",
			},
		},
		{
			name: "unsupported URL path",
			args: []string{
				"drive", "+member-add",
				"--token", "https://example.feishu.cn/calendar/calE2E001",
				"--member-id", "ou_e2e_user",
				"--member-type", "openid",
				"--dry-run",
			},
			wantErr: "unsupported URL path",
		},
		{
			name: "bare token requires explicit type",
			args: []string{
				"drive", "+member-add",
				"--token", "unknownE2E001",
				"--member-id", "ou_e2e_user",
				"--member-type", "openid",
				"--dry-run",
			},
			wantErr: "--type is required when --token is a bare token",
		},
		{
			name: "duplicate member IDs are rejected",
			args: []string{
				"drive", "+member-add",
				"--token", "doxcnE2E001",
				"--type", "docx",
				"--member-id", "ou_a,ou_b,ou_a",
				"--member-type", "openid",
				"--dry-run",
			},
			wantErr: "duplicate collaborator ID",
		},
		{
			name: "invalid explicit type is rejected",
			args: []string{
				"drive", "+member-add",
				"--token", "mincnE2E001",
				"--type", "invalidtype",
				"--member-id", "ou_e2e_user",
				"--member-type", "openid",
				"--dry-run",
			},
			wantErr: "--type must be one of: docx, doc, sheet, bitable, file, folder, wiki, mindnote, slides, minutes",
		},
		{
			name: "member-id prefix conflicts with explicit member-type",
			args: []string{
				"drive", "+member-add",
				"--token", "doxcnE2E001",
				"--type", "docx",
				"--member-id", "ou_e2e_user,oc_e2e_chat",
				"--member-type", "openid",
				"--dry-run",
			},
			wantErr: "implies --member-type openchat",
		},
		{
			name: "explicit member-type conflicts with member-id prefix",
			args: []string{
				"drive", "+member-add",
				"--token", "doxcnE2E001",
				"--type", "docx",
				"--member-id", "oc_e2e_chat",
				"--member-type", "openid",
				"--dry-run",
			},
			wantErr: "implies --member-type openchat",
		},
		{
			name: "department collaborator requires user identity",
			args: []string{
				"drive", "+member-add",
				"--token", "doxcnE2E001",
				"--type", "docx",
				"--member-id", "od_e2e_dept",
				"--member-type", "opendepartmentid",
				"--dry-run",
			},
			defaultAs: "bot",
			wantErr:   "--member-type=opendepartmentid requires --as user",
		},
		{
			name: "wikispaceid requires member-kind",
			args: []string{
				"drive", "+member-add",
				"--token", "doxcnE2E001",
				"--type", "docx",
				"--member-id", "spc_e2e_wiki",
				"--member-type", "wikispaceid",
				"--dry-run",
			},
			wantErr: "--member-kind is required when --member-type=wikispaceid",
		},
		{
			name: "member-kind only applies to wikispaceid",
			args: []string{
				"drive", "+member-add",
				"--token", "doxcnE2E001",
				"--type", "docx",
				"--member-id", "ou_e2e_user",
				"--member-type", "openid",
				"--member-kind", "wiki_space_member",
				"--dry-run",
			},
			wantErr: "--member-kind only applies when --member-type=wikispaceid",
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

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: defaultAs,
			})
			require.NoError(t, err)

			if tt.wantErr == "" {
				result.AssertExitCode(t, 0)
				return
			}

			result.AssertExitCode(t, 2)
			output := result.Stdout + result.Stderr
			require.Contains(t, output, tt.wantErr, "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
		})
	}
}
