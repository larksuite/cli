// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAppsRoleManagementDryRun_RequestShapes(t *testing.T) {
	setAppsRoleDryRunEnv(t)

	tests := []struct {
		name        string
		args        []string
		wantMethod  string
		wantURL     string
		assertShape func(t *testing.T, stdout string)
	}{
		{
			name:       "RoleList_NameFilterShowsFirstAutomaticScanRequest",
			args:       []string{"apps", "+role-list", "--app-id", "app_role_e2e", "--name", "admin", "--page-size", "20", "--page-token", "40", "--dry-run"},
			wantMethod: "GET",
			wantURL:    "/open-apis/spark/v1/apps/app_role_e2e/roles",
			assertShape: func(t *testing.T, stdout string) {
				assert.Equal(t, "admin", gjson.Get(stdout, "api.0.params.name").String(), "stdout:\n%s", stdout)
				assert.Equal(t, int64(100), gjson.Get(stdout, "api.0.params.limit").Int(), "name scan uses maximum backend page size; stdout:\n%s", stdout)
				assert.Equal(t, int64(0), gjson.Get(stdout, "api.0.params.offset").Int(), "name scan starts from the first backend page; stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.params.page_size").Exists(), "CLI page-size must be mapped to backend limit; stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.params.page_token").Exists(), "CLI page-token must be mapped to backend offset; stdout:\n%s", stdout)
			},
		},
		{
			name:       "RoleList_DefaultsPageSizeTo20",
			args:       []string{"apps", "+role-list", "--app-id", "app_role_e2e", "--dry-run"},
			wantMethod: "GET",
			wantURL:    "/open-apis/spark/v1/apps/app_role_e2e/roles",
			assertShape: func(t *testing.T, stdout string) {
				assert.Equal(t, int64(20), gjson.Get(stdout, "api.0.params.limit").Int(), "stdout:\n%s", stdout)
				assert.Equal(t, int64(0), gjson.Get(stdout, "api.0.params.offset").Int(), "stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.params.page_size").Exists(), "CLI page-size must be mapped to backend limit; stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.params.page_token").Exists(), "CLI page-token must be mapped to backend offset; stdout:\n%s", stdout)
			},
		},
		{
			name:       "RoleGet_UsesRolePathSegment",
			args:       []string{"apps", "+role-get", "--app-id", "app_role_e2e", "--role-id", "role_admin", "--dry-run"},
			wantMethod: "GET",
			wantURL:    "/open-apis/spark/v1/apps/app_role_e2e/roles/role_admin",
			assertShape: func(t *testing.T, stdout string) {
				assert.False(t, gjson.Get(stdout, "api.0.params").Exists(), "role-get should not send query params; stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.body").Exists(), "role-get should not send body; stdout:\n%s", stdout)
			},
		},
		{
			name:       "RoleCreate_WithExplicitRoleID",
			args:       []string{"apps", "+role-create", "--app-id", "app_role_e2e", "--role-id", "role_analyst", "--name", "Data Analyst", "--description", "Can inspect BI reports", "--dry-run"},
			wantMethod: "POST",
			wantURL:    "/open-apis/spark/v1/apps/app_role_e2e/roles",
			assertShape: func(t *testing.T, stdout string) {
				assert.Equal(t, "role_analyst", gjson.Get(stdout, "api.0.body.role_id").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "Data Analyst", gjson.Get(stdout, "api.0.body.name").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "Can inspect BI reports", gjson.Get(stdout, "api.0.body.description").String(), "stdout:\n%s", stdout)
			},
		},
		{
			name:       "RoleCreate_OmitsRoleIDWhenServerGeneratesIt",
			args:       []string{"apps", "+role-create", "--app-id", "app_role_e2e", "--name", "Generated Role", "--dry-run"},
			wantMethod: "POST",
			wantURL:    "/open-apis/spark/v1/apps/app_role_e2e/roles",
			assertShape: func(t *testing.T, stdout string) {
				assert.Equal(t, "Generated Role", gjson.Get(stdout, "api.0.body.name").String(), "stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.body.role_id").Exists(), "role_id should be omitted unless explicitly provided; stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.body.description").Exists(), "description should be omitted unless explicitly provided; stdout:\n%s", stdout)
			},
		},
		{
			name:       "RoleCreate_PreservesExplicitEmptyDescription",
			args:       []string{"apps", "+role-create", "--app-id", "app_role_e2e", "--name", "Empty Description", "--description", "", "--dry-run"},
			wantMethod: "POST",
			wantURL:    "/open-apis/spark/v1/apps/app_role_e2e/roles",
			assertShape: func(t *testing.T, stdout string) {
				assert.Equal(t, "Empty Description", gjson.Get(stdout, "api.0.body.name").String(), "stdout:\n%s", stdout)
				assert.True(t, gjson.Get(stdout, "api.0.body.description").Exists(), "description should be present when explicitly set empty; stdout:\n%s", stdout)
				assert.Equal(t, "", gjson.Get(stdout, "api.0.body.description").String(), "stdout:\n%s", stdout)
			},
		},
		{
			name:       "RoleUpdate_SendsOnlyExplicitFields",
			args:       []string{"apps", "+role-update", "--app-id", "app_role_e2e", "--role-id", "role_analyst", "--description", "Updated description", "--dry-run"},
			wantMethod: "PATCH",
			wantURL:    "/open-apis/spark/v1/apps/app_role_e2e/roles/role_analyst",
			assertShape: func(t *testing.T, stdout string) {
				assert.Equal(t, "Updated description", gjson.Get(stdout, "api.0.body.description").String(), "stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.body.name").Exists(), "name should be omitted when not explicitly provided; stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.body.role_id").Exists(), "role_id is immutable and must not be sent in update body; stdout:\n%s", stdout)
			},
		},
		{
			name:       "RoleUpdate_PreservesExplicitEmptyDescription",
			args:       []string{"apps", "+role-update", "--app-id", "app_role_e2e", "--role-id", "role_analyst", "--description", "", "--dry-run"},
			wantMethod: "PATCH",
			wantURL:    "/open-apis/spark/v1/apps/app_role_e2e/roles/role_analyst",
			assertShape: func(t *testing.T, stdout string) {
				assert.True(t, gjson.Get(stdout, "api.0.body.description").Exists(), "description should be present when explicitly set empty; stdout:\n%s", stdout)
				assert.Equal(t, "", gjson.Get(stdout, "api.0.body.description").String(), "stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.body.name").Exists(), "name should be omitted when not explicitly provided; stdout:\n%s", stdout)
			},
		},
		{
			name:       "RoleDelete_RequiresHighRiskConfirmationAndUsesDelete",
			args:       []string{"apps", "+role-delete", "--app-id", "app_role_e2e", "--role-id", "role_analyst", "--yes", "--dry-run"},
			wantMethod: "DELETE",
			wantURL:    "/open-apis/spark/v1/apps/app_role_e2e/roles/role_analyst",
			assertShape: func(t *testing.T, stdout string) {
				assert.False(t, gjson.Get(stdout, "api.0.body").Exists(), "role-delete should not send a request body; stdout:\n%s", stdout)
			},
		},
		{
			name:       "RoleMemberList_UsesMemberTypeOnly",
			args:       []string{"apps", "+role-member-list", "--app-id", "app_role_e2e", "--role-id", "role_analyst", "--member-type", "user", "--dry-run"},
			wantMethod: "GET",
			wantURL:    "/open-apis/spark/v1/apps/app_role_e2e/roles/role_analyst/member_list",
			assertShape: func(t *testing.T, stdout string) {
				assert.Equal(t, "user", gjson.Get(stdout, "api.0.params.member_type").String(), "stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.params.limit").Exists(), "member-list no longer sends pagination; stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.params.offset").Exists(), "member-list no longer sends pagination; stdout:\n%s", stdout)
			},
		},
		{
			name:       "RoleMemberAdd_MapsUsersDepartmentsChatsToBackendTypes",
			args:       []string{"apps", "+role-member-add", "--app-id", "app_role_e2e", "--role-id", "role_analyst", "--users", "ou_a,ou_b", "--departments", "od-a", "--chats", "oc_a", "--dry-run"},
			wantMethod: "POST",
			wantURL:    "/open-apis/spark/v1/apps/app_role_e2e/roles/role_analyst/member_add",
			assertShape: func(t *testing.T, stdout string) {
				assert.Equal(t, "ou_a", gjson.Get(stdout, "api.0.body.users.0").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "ou_b", gjson.Get(stdout, "api.0.body.users.1").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "od-a", gjson.Get(stdout, "api.0.body.departments.0").String(), "stdout:\n%s", stdout)
				assert.Equal(t, "oc_a", gjson.Get(stdout, "api.0.body.chats.0").String(), "stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.body.members").Exists(), "API contract uses users/departments/chats arrays; stdout:\n%s", stdout)
			},
		},
		{
			name:       "RoleMemberRemove_UsesGroupedBody",
			args:       []string{"apps", "+role-member-remove", "--app-id", "app_role_e2e", "--role-id", "role_analyst", "--users", "ou_a", "--yes", "--dry-run"},
			wantMethod: "POST",
			wantURL:    "/open-apis/spark/v1/apps/app_role_e2e/roles/role_analyst/member_remove",
			assertShape: func(t *testing.T, stdout string) {
				assert.Equal(t, "ou_a", gjson.Get(stdout, "api.0.body.users.0").String(), "stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.body.members").Exists(), "API contract uses users/departments/chats arrays; stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.body.all").Exists(), "all should be omitted when removing explicit members; stdout:\n%s", stdout)
			},
		},
		{
			name:       "RoleMemberRemoveAll_SendsOnlyAllTrue",
			args:       []string{"apps", "+role-member-remove", "--app-id", "app_role_e2e", "--role-id", "role_analyst", "--all", "--yes", "--dry-run"},
			wantMethod: "POST",
			wantURL:    "/open-apis/spark/v1/apps/app_role_e2e/roles/role_analyst/member_remove",
			assertShape: func(t *testing.T, stdout string) {
				assert.True(t, gjson.Get(stdout, "api.0.body.all").Bool(), "stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.body.members").Exists(), "--all body must not include explicit members; stdout:\n%s", stdout)
			},
		},
		{
			name:       "RoleMatchList_UsesFullQueryEndpointWithoutRoleID",
			args:       []string{"apps", "+role-match-list", "--app-id", "app_role_e2e", "--user-id", "ou_target", "--dry-run"},
			wantMethod: "POST",
			wantURL:    "/open-apis/spark/v1/apps/app_role_e2e/user_role_list",
			assertShape: func(t *testing.T, stdout string) {
				assert.Equal(t, "ou_target", gjson.Get(stdout, "api.0.body.target_user_id").String(), "stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.body.role_id").Exists(), "role-match-list must not require role_id; stdout:\n%s", stdout)
				assert.False(t, gjson.Get(stdout, "api.0.params").Exists(), "role-match-list must use body instead of query params; stdout:\n%s", stdout)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tc.args,
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			dryRunData := gjson.Get(result.Stdout, "data").Raw
			require.NotEmpty(t, dryRunData, "dry-run output must expose request data under data; stdout:\n%s", result.Stdout)
			assert.Equal(t, tc.wantMethod, gjson.Get(dryRunData, "api.0.method").String(), "stdout:\n%s", result.Stdout)
			assert.Equal(t, tc.wantURL, gjson.Get(dryRunData, "api.0.url").String(), "stdout:\n%s", result.Stdout)
			tc.assertShape(t, dryRunData)
		})
	}
}

func TestAppsRoleManagementValidation(t *testing.T) {
	setAppsRoleDryRunEnv(t)

	tests := []struct {
		name        string
		args        []string
		wantParam   string
		wantParams  []string
		wantMessage string
		wantHint    string
	}{
		{
			name:        "RejectsCreateWithoutNameWithStructuredParam",
			args:        []string{"apps", "+role-create", "--app-id", "app_role_e2e", "--description", "Can inspect BI reports", "--dry-run"},
			wantParam:   "--name",
			wantMessage: "--name is required",
			wantHint:    "do not infer a name",
		},
		{
			name:        "RejectsInvalidRoleID",
			args:        []string{"apps", "+role-create", "--app-id", "app_role_e2e", "--role-id", "../bad", "--name", "Bad", "--dry-run"},
			wantParam:   "--role-id",
			wantMessage: "--role-id must match [A-Za-z0-9_-]{1,64}",
		},
		{
			name:        "RejectsLarkCredentialAppID",
			args:        []string{"apps", "+role-list", "--app-id", "cli_role_e2e", "--dry-run"},
			wantParam:   "--app-id",
			wantMessage: "Miaoda app_id",
		},
		{
			name:        "RejectsAppIDWithoutMiaodaPrefix",
			args:        []string{"apps", "+role-list", "--app-id", "plain_role_e2e", "--dry-run"},
			wantParam:   "--app-id",
			wantMessage: "starting with app_",
		},
		{
			name:        "RejectsExplicitEmptyRoleListName",
			args:        []string{"apps", "+role-list", "--app-id", "app_role_e2e", "--name", "", "--dry-run"},
			wantParam:   "--name",
			wantMessage: "--name must not be empty when provided",
		},
		{
			name:        "RejectsPageSizeAboveLimit",
			args:        []string{"apps", "+role-list", "--app-id", "app_role_e2e", "--page-size", "101", "--dry-run"},
			wantParam:   "--page-size",
			wantMessage: "--page-size must be between 1 and 100",
		},
		{
			name:        "RejectsNonNumericPageToken",
			args:        []string{"apps", "+role-list", "--app-id", "app_role_e2e", "--page-token", "next", "--dry-run"},
			wantParam:   "--page-token",
			wantMessage: "--page-token",
		},
		{
			name:        "RejectsUpdateWithoutPatchFields",
			args:        []string{"apps", "+role-update", "--app-id", "app_role_e2e", "--role-id", "role_analyst", "--dry-run"},
			wantParams:  []string{"--name", "--description"},
			wantMessage: "--name or --description is required",
		},
		{
			name:        "RejectsMemberAddWithoutMembers",
			args:        []string{"apps", "+role-member-add", "--app-id", "app_role_e2e", "--role-id", "role_analyst", "--dry-run"},
			wantParams:  []string{"--users", "--departments", "--chats"},
			wantMessage: "at least one of --users, --departments, or --chats is required",
		},
		{
			name:        "RejectsMoreThan100MembersBeforeDryRun",
			args:        []string{"apps", "+role-member-add", "--app-id", "app_role_e2e", "--role-id", "role_analyst", "--users", strings.TrimSuffix(strings.Repeat("ou_a,", 101), ","), "--dry-run"},
			wantParams:  []string{"--users"},
			wantMessage: "role members cannot exceed 100",
		},
		{
			name:        "RejectsRemoveAllWithExplicitMembers",
			args:        []string{"apps", "+role-member-remove", "--app-id", "app_role_e2e", "--role-id", "role_analyst", "--all", "--users", "ou_a", "--yes", "--dry-run"},
			wantParams:  []string{"--all", "--users"},
			wantMessage: "--all",
		},
		{
			name:        "RejectsRemoveWithoutMembersOrAll",
			args:        []string{"apps", "+role-member-remove", "--app-id", "app_role_e2e", "--role-id", "role_analyst", "--yes", "--dry-run"},
			wantParams:  []string{"--users", "--departments", "--chats", "--all"},
			wantMessage: "specify members to remove",
		},
		{
			name:        "RejectsBlankUserIDForMatchList",
			args:        []string{"apps", "+role-match-list", "--app-id", "app_role_e2e", "--user-id", " ", "--dry-run"},
			wantParam:   "--user-id",
			wantMessage: "user-id",
		},
		{
			name:        "RejectsEmailUserIDForMatchList",
			args:        []string{"apps", "+role-match-list", "--app-id", "app_role_e2e", "--user-id", "alice@example.com", "--dry-run"},
			wantParam:   "--user-id",
			wantMessage: "ou_",
		},
		{
			name:        "RejectsWrongDepartmentIDPrefix",
			args:        []string{"apps", "+role-member-add", "--app-id", "app_role_e2e", "--role-id", "role_analyst", "--departments", "ou_user", "--dry-run"},
			wantParam:   "--departments",
			wantMessage: "od-",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tc.args,
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			envelope := validationEnvelope(result)
			assert.Equal(t, "validation", gjson.Get(envelope, "error.type").String(), "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			assert.Equal(t, "invalid_argument", gjson.Get(envelope, "error.subtype").String(), "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			if len(tc.wantParams) > 0 {
				assert.False(t, gjson.Get(envelope, "error.param").Exists(), "scalar param must be omitted: %s", envelope)
				gotParams := make([]string, 0, len(tc.wantParams))
				for _, param := range gjson.Get(envelope, "error.params.#.name").Array() {
					gotParams = append(gotParams, param.String())
				}
				assert.Equal(t, tc.wantParams, gotParams, "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			} else {
				assert.Equal(t, tc.wantParam, gjson.Get(envelope, "error.param").String(), "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			}
			assert.Contains(t, envelope, tc.wantMessage, "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			if tc.wantHint != "" {
				assert.Contains(t, gjson.Get(envelope, "error.hint").String(), tc.wantHint, "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			}
		})
	}
}

func TestAppsRoleManagementLiveWorkflow(t *testing.T) {
	requireLiveRoleFixture(t)

	appID := os.Getenv("LARK_CLI_E2E_APPS_ROLE_APP_ID")
	roleID := liveAppsRoleFixtureID()
	member := requireLiveRoleMemberFixture(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	baselineResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"apps", "+role-member-list", "--app-id", appID, "--role-id", roleID, "--member-type", member.memberType},
		DefaultAs: "user",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	baselineResult.AssertExitCode(t, 0)
	baselineResult.AssertStdoutStatus(t, true)
	if jsonStringArrayContains(baselineResult.Stdout, member.dataPath, member.id) {
		t.Skipf("FIXTURE: member %s already belongs to role %s; refusing to mutate pre-existing state", member.id, roleID)
	}
	needsMemberCleanup := false
	t.Cleanup(func() {
		if !needsMemberCleanup {
			return
		}
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		removeResult, removeErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"apps", "+role-member-remove", "--app-id", appID, "--role-id", roleID, member.flag, member.id},
			DefaultAs: "user",
			Yes:       true,
		})
		clie2e.ReportCleanupFailure(t, "remove added apps role member "+member.id, removeResult, removeErr)
	})

	t.Run("read fixture role", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args:      []string{"apps", "+role-get", "--app-id", appID, "--role-id", roleID},
			DefaultAs: "user",
		}, clie2e.RetryOptions{})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, roleID, gjson.Get(result.Stdout, "data.role.role_id").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("add list and remove fixture member", func(t *testing.T) {
		// Arm cleanup before the write so a transport failure after a committed
		// request cannot leak the member into the shared fixture role.
		needsMemberCleanup = true
		addResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+role-member-add", "--app-id", appID, "--role-id", roleID, member.flag, member.id},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		addResult.AssertExitCode(t, 0)
		addResult.AssertStdoutStatus(t, true)
		assert.True(t, jsonStringArrayContains(addResult.Stdout, member.dataPath, member.id), "stdout:\n%s", addResult.Stdout)

		listResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args:      []string{"apps", "+role-member-list", "--app-id", appID, "--role-id", roleID, "--member-type", member.memberType},
			DefaultAs: "user",
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil || result.ExitCode != 0 {
					return true
				}
				return !jsonStringArrayContains(result.Stdout, member.dataPath, member.id)
			},
		})
		require.NoError(t, err)
		listResult.AssertExitCode(t, 0)
		listResult.AssertStdoutStatus(t, true)
		assert.True(t, jsonStringArrayContains(listResult.Stdout, member.dataPath, member.id), "stdout:\n%s", listResult.Stdout)

		removeResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+role-member-remove", "--app-id", appID, "--role-id", roleID, member.flag, member.id},
			DefaultAs: "user",
			Yes:       true,
		})
		require.NoError(t, err)
		removeResult.AssertExitCode(t, 0)
		removeResult.AssertStdoutStatus(t, true)
		assert.True(t, jsonStringArrayContains(removeResult.Stdout, member.dataPath, member.id), "stdout:\n%s", removeResult.Stdout)

		removedReadback, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args:      []string{"apps", "+role-member-list", "--app-id", appID, "--role-id", roleID, "--member-type", member.memberType},
			DefaultAs: "user",
		}, clie2e.RetryOptions{
			ShouldRetry: func(result *clie2e.Result) bool {
				return result == nil || result.ExitCode != 0 || jsonStringArrayContains(result.Stdout, member.dataPath, member.id)
			},
		})
		require.NoError(t, err)
		removedReadback.AssertExitCode(t, 0)
		removedReadback.AssertStdoutStatus(t, true)
		require.False(t, jsonStringArrayContains(removedReadback.Stdout, member.dataPath, member.id), "stdout:\n%s", removedReadback.Stdout)
		needsMemberCleanup = false
	})
}

func TestAppsRoleLifecycleLiveWorkflow(t *testing.T) {
	requireLiveRoleLifecycleFixture(t)

	appID := strings.TrimSpace(os.Getenv("LARK_CLI_E2E_APPS_ROLE_APP_ID"))
	member := requireLiveRoleMemberFixture(t)
	suffix := strings.ReplaceAll(clie2e.GenerateSuffix(), "-", "_")
	roleID := "role_e2e_" + suffix
	roleName := "CLI Role E2E " + suffix
	createdDescription := "created by role lifecycle e2e"
	updatedDescription := "updated by role lifecycle e2e"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	roleMayExist := true
	t.Cleanup(func() {
		if !roleMayExist {
			return
		}
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		listResult, listErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"apps", "+role-list", "--app-id", appID, "--name", roleName},
			DefaultAs: "user",
		})
		if listErr != nil || listResult == nil || listResult.ExitCode != 0 {
			clie2e.ReportCleanupFailure(t, "locate transient apps role "+roleID, listResult, listErr)
			return
		}
		if !roleListContainsID(listResult.Stdout, roleID) {
			return
		}

		clearResult, clearErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"apps", "+role-member-remove", "--app-id", appID, "--role-id", roleID, "--all"},
			DefaultAs: "user",
			Yes:       true,
		})
		clie2e.ReportCleanupFailure(t, "clear transient apps role members "+roleID, clearResult, clearErr)

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"apps", "+role-delete", "--app-id", appID, "--role-id", roleID},
			DefaultAs: "user",
			Yes:       true,
		})
		clie2e.ReportCleanupFailure(t, "delete transient apps role "+roleID, deleteResult, deleteErr)
	})

	createResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"apps", "+role-create", "--app-id", appID, "--role-id", roleID,
			"--name", roleName, "--description", createdDescription,
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	createResult.AssertExitCode(t, 0)
	createResult.AssertStdoutStatus(t, true)
	assert.Equal(t, roleID, gjson.Get(createResult.Stdout, "data.role.role_id").String(), "stdout:\n%s", createResult.Stdout)

	createdReadback := readRoleUntil(t, ctx, appID, roleID, func(result *clie2e.Result) bool {
		return gjson.Get(result.Stdout, "data.role.name").String() == roleName &&
			gjson.Get(result.Stdout, "data.role.description").String() == createdDescription
	})
	assert.Equal(t, roleID, gjson.Get(createdReadback.Stdout, "data.role.role_id").String(), "stdout:\n%s", createdReadback.Stdout)

	updateResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"apps", "+role-update", "--app-id", appID, "--role-id", roleID,
			"--description", updatedDescription,
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	updateResult.AssertExitCode(t, 0)
	updateResult.AssertStdoutStatus(t, true)
	assert.Equal(t, roleID, gjson.Get(updateResult.Stdout, "data.role.role_id").String(), "stdout:\n%s", updateResult.Stdout)
	readRoleUntil(t, ctx, appID, roleID, func(result *clie2e.Result) bool {
		return gjson.Get(result.Stdout, "data.role.description").String() == updatedDescription
	})

	addResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"apps", "+role-member-add", "--app-id", appID, "--role-id", roleID, member.flag, member.id},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	addResult.AssertExitCode(t, 0)
	addResult.AssertStdoutStatus(t, true)

	membersResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"apps", "+role-member-list", "--app-id", appID, "--role-id", roleID},
		DefaultAs: "user",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0 || !jsonStringArrayContains(result.Stdout, member.dataPath, member.id)
		},
	})
	require.NoError(t, err)
	membersResult.AssertExitCode(t, 0)
	membersResult.AssertStdoutStatus(t, true)
	assert.True(t, jsonStringArrayContains(membersResult.Stdout, member.dataPath, member.id), "stdout:\n%s", membersResult.Stdout)

	clearResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"apps", "+role-member-remove", "--app-id", appID, "--role-id", roleID, "--all"},
		DefaultAs: "user",
		Yes:       true,
	})
	require.NoError(t, err)
	clearResult.AssertExitCode(t, 0)
	clearResult.AssertStdoutStatus(t, true)

	clearedReadback, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"apps", "+role-member-list", "--app-id", appID, "--role-id", roleID},
		DefaultAs: "user",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0 || !allRoleMemberGroupsEmpty(result.Stdout)
		},
	})
	require.NoError(t, err)
	clearedReadback.AssertExitCode(t, 0)
	clearedReadback.AssertStdoutStatus(t, true)
	assert.True(t, allRoleMemberGroupsEmpty(clearedReadback.Stdout), "stdout:\n%s", clearedReadback.Stdout)
	readRoleUntil(t, ctx, appID, roleID, func(result *clie2e.Result) bool {
		return gjson.Get(result.Stdout, "data.role.role_id").String() == roleID
	})

	deleteResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"apps", "+role-delete", "--app-id", appID, "--role-id", roleID},
		DefaultAs: "user",
		Yes:       true,
	})
	require.NoError(t, err)
	deleteResult.AssertExitCode(t, 0)
	deleteResult.AssertStdoutStatus(t, true)
	assert.Equal(t, roleID, gjson.Get(deleteResult.Stdout, "data.role_id").String(), "stdout:\n%s", deleteResult.Stdout)
	assert.True(t, gjson.Get(deleteResult.Stdout, "data.deleted").Bool(), "stdout:\n%s", deleteResult.Stdout)

	absentResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"apps", "+role-list", "--app-id", appID, "--name", roleName},
		DefaultAs: "user",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0 || roleListContainsID(result.Stdout, roleID)
		},
	})
	require.NoError(t, err)
	absentResult.AssertExitCode(t, 0)
	absentResult.AssertStdoutStatus(t, true)
	require.False(t, roleListContainsID(absentResult.Stdout, roleID), "stdout:\n%s", absentResult.Stdout)
	roleMayExist = false
}

func TestAppsRoleMatchListLiveWorkflow(t *testing.T) {
	requireLiveRoleFixture(t)
	if os.Getenv("LARK_CLI_E2E_APPS_ROLE_MATCH_READY") != "1" {
		t.Skip("FIXTURE: Set LARK_CLI_E2E_APPS_ROLE_MATCH_READY=1 when backend user_role_list is ready for live role-match-list proof")
	}

	appID := os.Getenv("LARK_CLI_E2E_APPS_ROLE_APP_ID")
	userID := requireLiveRoleUserFixture(t)
	roleID := liveAppsRoleFixtureID()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	baselineResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"apps", "+role-member-list", "--app-id", appID, "--role-id", roleID, "--member-type", "user"},
		DefaultAs: "user",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	baselineResult.AssertExitCode(t, 0)
	baselineResult.AssertStdoutStatus(t, true)
	if jsonStringArrayContains(baselineResult.Stdout, "data.users", userID) {
		t.Skipf("FIXTURE: user %s already belongs to role %s; refusing to mutate pre-existing state", userID, roleID)
	}
	needsMemberCleanup := false
	t.Cleanup(func() {
		if !needsMemberCleanup {
			return
		}
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		removeResult, removeErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"apps", "+role-member-remove", "--app-id", appID, "--role-id", roleID, "--users", userID},
			DefaultAs: "user",
			Yes:       true,
		})
		clie2e.ReportCleanupFailure(t, "remove added apps role user "+userID, removeResult, removeErr)
	})

	// Arm cleanup before the write so a transport failure after a committed
	// request cannot leak the user into the shared fixture role.
	needsMemberCleanup = true
	addResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"apps", "+role-member-add", "--app-id", appID, "--role-id", roleID, "--users", userID},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	addResult.AssertExitCode(t, 0)
	addResult.AssertStdoutStatus(t, true)

	matchResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"apps", "+role-match-list", "--app-id", appID, "--user-id", userID},
		DefaultAs: "user",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			if result == nil || result.ExitCode != 0 {
				return true
			}
			return !gjson.Get(result.Stdout, `data.roles.#(role_id=="`+roleID+`")`).Exists()
		},
	})
	require.NoError(t, err)
	matchResult.AssertExitCode(t, 0)
	matchResult.AssertStdoutStatus(t, true)
	assert.True(t, gjson.Get(matchResult.Stdout, `data.roles.#(role_id=="`+roleID+`")`).Exists(), "stdout:\n%s", matchResult.Stdout)
}

func jsonStringArrayContains(raw, path, want string) bool {
	for _, item := range gjson.Get(raw, path).Array() {
		if item.String() == want {
			return true
		}
	}
	return false
}

func roleListContainsID(raw, roleID string) bool {
	for _, item := range gjson.Get(raw, "data.items").Array() {
		if item.Get("role_id").String() == roleID {
			return true
		}
	}
	return false
}

func allRoleMemberGroupsEmpty(raw string) bool {
	for _, path := range []string{"data.users", "data.departments", "data.chats"} {
		group := gjson.Get(raw, path)
		if !group.Exists() || !group.IsArray() || len(group.Array()) != 0 {
			return false
		}
	}
	return true
}

func readRoleUntil(t *testing.T, ctx context.Context, appID, roleID string, ready func(*clie2e.Result) bool) *clie2e.Result {
	t.Helper()
	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"apps", "+role-get", "--app-id", appID, "--role-id", roleID},
		DefaultAs: "user",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0 || !ready(result)
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	return result
}

func setAppsRoleDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "apps_role_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "apps_role_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

func validationEnvelope(result *clie2e.Result) string {
	if result.Stdout != "" {
		return result.Stdout
	}
	return result.Stderr
}

func requireLiveRoleFixture(t *testing.T) {
	t.Helper()
	if os.Getenv("LARKSUITE_CLI_CONFIG_DIR") == "" {
		t.Skip("FIXTURE: Set LARKSUITE_CLI_CONFIG_DIR to an isolated test config such as $HOME/.lark-cli-test; this live workflow must not use the default human profile")
	}
	if os.Getenv("LARK_CLI_E2E_APPS_ROLE_APP_ID") == "" {
		t.Skip("FIXTURE: Set LARK_CLI_E2E_APPS_ROLE_APP_ID to a dedicated test app where the test user can create/update/delete roles")
	}
	if liveAppsRoleFixtureID() == "" {
		t.Skip("FIXTURE: Set LARK_CLI_E2E_APPS_ROLE_ID to the dedicated test role")
	}
}

func requireLiveRoleLifecycleFixture(t *testing.T) {
	t.Helper()
	if os.Getenv("LARKSUITE_CLI_CONFIG_DIR") == "" {
		t.Skip("FIXTURE: Set LARKSUITE_CLI_CONFIG_DIR to an isolated test config; lifecycle E2E must not use the default human profile")
	}
	if strings.TrimSpace(os.Getenv("LARK_CLI_E2E_APPS_ROLE_APP_ID")) == "" {
		t.Skip("FIXTURE: Set LARK_CLI_E2E_APPS_ROLE_APP_ID to a dedicated test app where the test user can create/update/delete roles")
	}
	if strings.TrimSpace(os.Getenv("LARK_CLI_E2E_APPS_ROLE_CHAT_OPEN_ID")) == "" &&
		strings.TrimSpace(os.Getenv("LARK_CLI_E2E_APPS_ROLE_USER_OPEN_ID")) == "" {
		t.Skip("FIXTURE: Set a chat or user open ID for the transient role lifecycle member step")
	}
}

func liveAppsRoleFixtureID() string {
	return strings.TrimSpace(os.Getenv("LARK_CLI_E2E_APPS_ROLE_ID"))
}

type liveRoleMemberFixture struct {
	flag       string
	id         string
	memberType string
	dataPath   string
}

func requireLiveRoleMemberFixture(t *testing.T) liveRoleMemberFixture {
	t.Helper()
	if chatID := strings.TrimSpace(os.Getenv("LARK_CLI_E2E_APPS_ROLE_CHAT_OPEN_ID")); chatID != "" {
		return liveRoleMemberFixture{
			flag:       "--chats",
			id:         chatID,
			memberType: "chat",
			dataPath:   "data.chats",
		}
	}
	if userID := strings.TrimSpace(os.Getenv("LARK_CLI_E2E_APPS_ROLE_USER_OPEN_ID")); userID != "" {
		return liveRoleMemberFixture{
			flag:       "--users",
			id:         userID,
			memberType: "user",
			dataPath:   "data.users",
		}
	}
	t.Skip("FIXTURE: Set LARK_CLI_E2E_APPS_ROLE_CHAT_OPEN_ID for chat-member live E2E, or LARK_CLI_E2E_APPS_ROLE_USER_OPEN_ID for user-member fallback")
	return liveRoleMemberFixture{}
}

func requireLiveRoleUserFixture(t *testing.T) string {
	t.Helper()
	if userID := strings.TrimSpace(os.Getenv("LARK_CLI_E2E_APPS_ROLE_USER_OPEN_ID")); userID != "" {
		return userID
	}
	t.Skip("FIXTURE: Set LARK_CLI_E2E_APPS_ROLE_USER_OPEN_ID for live +role-match-list proof")
	return ""
}
