// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDrive_MemberRemoveWorkflowAsUser(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	clie2e.SkipWithoutUserToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	folderToken := CreateDriveFolder(t, t, ctx, "lark-cli-e2e-member-remove-"+suffix, "user", "")
	docToken := createMemberRemoveWorkflowDoc(t, ctx, folderToken, suffix)
	memberOpenID := requireDriveMemberFixture(t)

	// The doc is freshly created by this run, so the fixture user cannot be a
	// pre-existing collaborator; a user member is also observable via member-list
	// (unlike a bot, which the permission backend never returns), so this round
	// trip can verify real state rather than only the synthesized receipt.
	addResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+member-add",
			"--token", docToken,
			"--type", "docx",
			"--member-id", memberOpenID,
			"--member-type", "openid",
			"--perm", "view",
			"--yes",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	addResult.AssertExitCode(t, 0)
	addResult.AssertStdoutStatus(t, true)
	require.Equal(t, memberOpenID, gjson.Get(addResult.Stdout, "data.member_id").String(), "stdout:\n%s", addResult.Stdout)

	// Prove the add actually granted access; the add receipt echoes request fields
	// and cannot by itself prove the collaborator was created.
	requireDriveMemberListMembership(t, ctx, docToken, "docx", memberOpenID, true)

	removeResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+member-remove",
			"--token", docToken,
			"--type", "docx",
			"--member-id", memberOpenID,
			"--member-type", "openid",
			"--yes",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	removeResult.AssertExitCode(t, 0)
	removeResult.AssertStdoutStatus(t, true)
	require.True(t, gjson.Get(removeResult.Stdout, "data.removed").Bool(), "stdout:\n%s", removeResult.Stdout)
	require.Equal(t, docToken, gjson.Get(removeResult.Stdout, "data.resource_token").String(), "stdout:\n%s", removeResult.Stdout)
	require.Equal(t, "docx", gjson.Get(removeResult.Stdout, "data.resource_type").String(), "stdout:\n%s", removeResult.Stdout)
	require.Equal(t, memberOpenID, gjson.Get(removeResult.Stdout, "data.member_id").String(), "stdout:\n%s", removeResult.Stdout)
	require.Equal(t, "openid", gjson.Get(removeResult.Stdout, "data.member_type").String(), "stdout:\n%s", removeResult.Stdout)

	// Prove the removal changed observable state, not just that the call exited 0.
	requireDriveMemberListMembership(t, ctx, docToken, "docx", memberOpenID, false)
}

// TestDrive_MemberRemoveAppsWorkflowAsUser exercises the Miaoda apps branch of
// drive +member-remove against a real backend. The docx workflow above never
// touches --type=apps, so this is the only live proof that the server accepts
// DELETE /permissions/{token}/members/{id}?type=apps; the dry-run tests only
// cover client-side URL inference and request shaping.
//
// The collaborator is a USER (not a bot): the permission member-list backend
// only returns supported member types and deliberately excludes app/bot owners,
// so a bot collaborator can never be observed there and its membership can never
// be verified. A user member is observable, which lets this test use an
// authoritative baseline and a real add -> verify -> remove -> verify round
// trip. Both the app and the collaborator user are injected via fixture env vars
// (never real tokens in source); the test only mutates that one collaborator and
// cleans it up. The app's existing collaborators are never touched.
func TestDrive_MemberRemoveAppsWorkflowAsUser(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)
	appToken := requireDriveAppsFixture(t)
	memberOpenID := requireDriveMemberFixture(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	// Authoritative baseline: the collaborator must be a supported (user) member
	// type that member-list actually returns, and it must be absent before the
	// run. If it is already present, skip rather than risk deleting a pre-existing
	// collaborator during cleanup. Because user members ARE observable, absence
	// here is authoritative (unlike a bot, which is never listed).
	baselineResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"drive", "+member-list", "--token", appToken, "--type", "apps"},
		DefaultAs: "user",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	baselineResult.AssertExitCode(t, 0)
	baselineResult.AssertStdoutStatus(t, true)
	if driveMemberListContains(baselineResult.Stdout, memberOpenID) {
		t.Skipf("FIXTURE: user %s already collaborates on the fixture app; refusing to mutate pre-existing state", memberOpenID)
	}

	// Arm cleanup only for the membership this run creates. needsCleanup is set
	// true just before the add write, so a transport failure after a committed
	// request still triggers cleanup, while an early skip/failure never removes a
	// collaborator this run did not add. Cleanup failures are reported, not
	// discarded.
	needsCleanup := false
	t.Cleanup(func() {
		if !needsCleanup {
			return
		}
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		removeResult, removeErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{
				"drive", "+member-remove",
				"--token", appToken,
				"--type", "apps",
				"--member-id", memberOpenID,
				"--member-type", "openid",
				"--yes",
			},
			DefaultAs: "user",
		})
		clie2e.ReportCleanupFailure(t, "remove added apps collaborator "+memberOpenID, removeResult, removeErr)
	})

	needsCleanup = true
	addResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+member-add",
			"--token", appToken,
			"--type", "apps",
			"--member-id", memberOpenID,
			"--member-type", "openid",
			"--perm", "view",
			"--yes",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	addResult.AssertExitCode(t, 0)
	addResult.AssertStdoutStatus(t, true)
	require.Equal(t, "apps", gjson.Get(addResult.Stdout, "data.resource_type").String(), "stdout:\n%s", addResult.Stdout)
	require.Equal(t, memberOpenID, gjson.Get(addResult.Stdout, "data.member_id").String(), "stdout:\n%s", addResult.Stdout)

	// Prove the add is observable (user members are returned by member-list), so
	// the round trip verifies real state rather than just an exit code.
	requireDriveMemberListMembership(t, ctx, appToken, "apps", memberOpenID, true)

	removeResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+member-remove",
			"--token", appToken,
			"--type", "apps",
			"--member-id", memberOpenID,
			"--member-type", "openid",
			"--yes",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	removeResult.AssertExitCode(t, 0)
	removeResult.AssertStdoutStatus(t, true)
	require.True(t, gjson.Get(removeResult.Stdout, "data.removed").Bool(), "stdout:\n%s", removeResult.Stdout)
	require.Equal(t, "apps", gjson.Get(removeResult.Stdout, "data.resource_type").String(), "stdout:\n%s", removeResult.Stdout)
	require.Equal(t, memberOpenID, gjson.Get(removeResult.Stdout, "data.member_id").String(), "stdout:\n%s", removeResult.Stdout)
	require.Equal(t, "openid", gjson.Get(removeResult.Stdout, "data.member_type").String(), "stdout:\n%s", removeResult.Stdout)

	// Prove the removal changed observable state (the user we just confirmed
	// present is now gone), not merely that the call exited 0.
	requireDriveMemberListMembership(t, ctx, appToken, "apps", memberOpenID, false)

	// The explicit remove above already deleted the membership; a redundant
	// cleanup delete is unnecessary and would only add noise.
	needsCleanup = false
}

// driveMemberListContains reports whether a +member-list response includes a
// collaborator with the given member id.
func driveMemberListContains(stdout, memberID string) bool {
	for _, item := range gjson.Get(stdout, "data.items").Array() {
		if item.Get("member_id").String() == memberID {
			return true
		}
	}
	return false
}

// requireDriveMemberListMembership polls +member-list until the member's
// presence matches want, tolerating eventual consistency. It fails the test if
// the expected state is not reached before the deadline, so a removal that did
// not actually change server state cannot pass silently.
func requireDriveMemberListMembership(t *testing.T, ctx context.Context, token, resourceType, memberID string, want bool) {
	t.Helper()
	err := clie2e.WaitForCondition(ctx, clie2e.WaitOptions{
		Timeout:  30 * time.Second,
		Interval: 2 * time.Second,
		TimeoutError: func() error {
			return fmt.Errorf("member %s presence in %s member-list never became %v", memberID, resourceType, want)
		},
	}, func() (bool, error) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"drive", "+member-list", "--token", token, "--type", resourceType},
			DefaultAs: "user",
		})
		if err != nil {
			return false, err
		}
		if result.ExitCode != 0 {
			return false, fmt.Errorf("member-list failed: exit=%d\nstdout:\n%s\nstderr:\n%s", result.ExitCode, result.Stdout, result.Stderr)
		}
		return driveMemberListContains(result.Stdout, memberID) == want, nil
	})
	require.NoError(t, err)
}

// requireDriveAppsFixture returns the Miaoda app token/URL used as the live
// apps target, or skips when it is unset. The token is never hard-coded in
// source: it identifies a real tenant resource and must be supplied by the
// runner. Accepts either a /page/ URL or a bare app token (both resolve to
// --type=apps).
func requireDriveAppsFixture(t *testing.T) string {
	t.Helper()
	appToken := strings.TrimSpace(os.Getenv("LARK_CLI_E2E_DRIVE_APPS_TOKEN"))
	if appToken == "" {
		t.Skip("FIXTURE: Set LARK_CLI_E2E_DRIVE_APPS_TOKEN to a Miaoda app /page/ URL or bare app token whose collaborators the test user can manage")
	}
	return appToken
}

// requireDriveMemberFixture returns the open id of a USER collaborator to add and
// remove in the live workflows, or skips when unset. A user (not a bot) is
// required because the permission member-list backend only returns supported
// member types and excludes app/bot owners, so only a user member can be
// verified via member-list. The id must be a disposable test user that is NOT a
// pre-existing collaborator of the target resource. Shared by the docx and apps
// workflows; mirrors the LARK_CLI_E2E_APPS_ROLE_USER_OPEN_ID fixture used by the
// apps role live test.
func requireDriveMemberFixture(t *testing.T) string {
	t.Helper()
	memberOpenID := strings.TrimSpace(os.Getenv("LARK_CLI_E2E_DRIVE_MEMBER_OPEN_ID"))
	if memberOpenID == "" {
		t.Skip("FIXTURE: Set LARK_CLI_E2E_DRIVE_MEMBER_OPEN_ID to a disposable test user's open id (ou_...) that is not already a collaborator on the target resource")
	}
	return memberOpenID
}

func createMemberRemoveWorkflowDoc(t *testing.T, ctx context.Context, folderToken, suffix string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+create",
			"--parent-token", folderToken,
			"--doc-format", "markdown",
			"--content", "# member-remove-" + suffix + "\n\nTemporary permission workflow fixture.",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	docToken := gjson.Get(result.Stdout, "data.document.document_id").String()
	require.NotEmpty(t, docToken, "stdout:\n%s", result.Stdout)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		deleteResult, deleteErr := DeleteDriveResourceAndVerify(cleanupCtx, docToken, "docx", "user")
		clie2e.ReportCleanupFailure(t, "delete member-remove workflow doc "+docToken, deleteResult, deleteErr)
	})
	return docToken
}
