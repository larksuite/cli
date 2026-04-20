// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIM_AppFeedCardCreateDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	result, err := clie2e.RunCmd(context.Background(), clie2e.Request{
		Args: []string{
			"im", "+app-feed-card-create",
			"--user-ids", "ou_dryrun",
			"--title", "Dry run app feed card",
			"--preview", "Preview",
			"--link", "https://example.com/card",
			"--button-text", "Open",
			"--button-url", "https://example.com/open",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	entry := firstIMDryRunRequest(t, result.Stdout)
	assert.Equal(t, "POST", entry["method"])
	assert.Equal(t, "/open-apis/im/v2/app_feed_card", entry["url"])
	assert.Equal(t, map[string]any{"user_id_type": "open_id"}, entry["params"])

	body, ok := entry["body"].(map[string]any)
	require.True(t, ok, "body should be an object: %#v", entry["body"])
	assert.Equal(t, []any{"ou_dryrun"}, body["user_ids"])

	card, ok := body["app_feed_card"].(map[string]any)
	require.True(t, ok, "app_feed_card should be an object: %#v", body["app_feed_card"])
	assert.Equal(t, "Dry run app feed card", card["title"])
	link, ok := card["link"].(map[string]any)
	require.True(t, ok, "link should be an object: %#v", card["link"])
	assert.Equal(t, "https://example.com/card", link["link"])
}

func TestIM_AppFeedCardBatchDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	updateResult, err := clie2e.RunCmd(context.Background(), clie2e.Request{
		Args: []string{
			"im", "+app-feed-card-update",
			"--user-ids", "ou_dryrun",
			"--biz-id", "biz_dryrun",
			"--title", "Updated app feed card",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	updateResult.AssertExitCode(t, 0)

	updateEntry := firstIMDryRunRequest(t, updateResult.Stdout)
	assert.Equal(t, "PUT", updateEntry["method"])
	assert.Equal(t, "/open-apis/im/v2/app_feed_card/batch", updateEntry["url"])
	updateBody, ok := updateEntry["body"].(map[string]any)
	require.True(t, ok, "update body should be an object: %#v", updateEntry["body"])
	updateCards, ok := updateBody["feed_cards"].([]any)
	require.True(t, ok, "feed_cards should be an array: %#v", updateBody["feed_cards"])
	require.Len(t, updateCards, 1)
	updateCard, ok := updateCards[0].(map[string]any)
	require.True(t, ok, "feed_cards[0] should be an object: %#v", updateCards[0])
	assert.Equal(t, []any{"1"}, updateCard["update_fields"])

	deleteResult, err := clie2e.RunCmd(context.Background(), clie2e.Request{
		Args: []string{
			"im", "+app-feed-card-delete",
			"--user-ids", "ou_dryrun",
			"--biz-id", "biz_dryrun",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	deleteResult.AssertExitCode(t, 0)

	deleteEntry := firstIMDryRunRequest(t, deleteResult.Stdout)
	assert.Equal(t, "DELETE", deleteEntry["method"])
	assert.Equal(t, "/open-apis/im/v2/app_feed_card/batch", deleteEntry["url"])
	deleteBody, ok := deleteEntry["body"].(map[string]any)
	require.True(t, ok, "delete body should be an object: %#v", deleteEntry["body"])
	deleteCards, ok := deleteBody["feed_cards"].([]any)
	require.True(t, ok, "feed_cards should be an array: %#v", deleteBody["feed_cards"])
	require.Len(t, deleteCards, 1)
	deleteCard, ok := deleteCards[0].(map[string]any)
	require.True(t, ok, "feed_cards[0] should be an object: %#v", deleteCards[0])
	assert.Equal(t, "biz_dryrun", deleteCard["biz_id"])
}

func TestIM_FeedCardTimeSensitiveDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	cardResult, err := clie2e.RunCmd(context.Background(), clie2e.Request{
		Args: []string{
			"im", "+feed-card-time-sensitive",
			"--feed-card-id", "oc_dryrun",
			"--user-ids", "ou_dryrun",
			"--time-sensitive", "false",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	cardResult.AssertExitCode(t, 0)

	cardEntry := firstIMDryRunRequest(t, cardResult.Stdout)
	assert.Equal(t, "PATCH", cardEntry["method"])
	assert.Equal(t, "/open-apis/im/v2/feed_cards/oc_dryrun", cardEntry["url"])
	assert.Equal(t, map[string]any{"user_id_type": "open_id"}, cardEntry["params"])
	cardBody, ok := cardEntry["body"].(map[string]any)
	require.True(t, ok, "card body should be an object: %#v", cardEntry["body"])
	assert.Equal(t, []any{"ou_dryrun"}, cardBody["user_ids"])
	assert.Equal(t, false, cardBody["time_sensitive"])

	invalidCardResult, err := clie2e.RunCmd(context.Background(), clie2e.Request{
		Args: []string{
			"im", "+feed-card-time-sensitive",
			"--feed-card-id", "om_dryrun",
			"--user-ids", "ou_dryrun",
			"--time-sensitive", "true",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	invalidCardResult.AssertExitCode(t, 2)
	assert.Equal(t, "validation", gjson.Get(invalidCardResult.Stderr, "error.type").String(), "stderr:\n%s", invalidCardResult.Stderr)
	assert.Contains(t, gjson.Get(invalidCardResult.Stderr, "error.message").String(), `starting with "oc_"`)
}

func TestIM_AppFeedCardCreateWorkflowAsBot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	selfOpenID := getCurrentUserOpenIDForIM(t, ctx)
	bizID := "lark-cli-e2e-feed-" + clie2e.GenerateSuffix()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"im", "+app-feed-card-create",
			"--user-ids", selfOpenID,
			"--biz-id", bizID,
			"--title", "lark-cli e2e app feed card",
			"--preview", "created by lark-cli e2e",
			"--link", "https://www.larksuite.com/",
			"--close-notify",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 && isAppFeedPermissionFailure(result) {
		t.Skipf("skipped: app feed card API permission is unavailable in this environment: %s", result.Stderr)
	}
	result.AssertExitCode(t, 0)

	cleanupBizID := bizID
	deleted := false
	t.Cleanup(func() {
		if deleted {
			return
		}
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		cleanupResult, cleanupErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"api", "DELETE", "/open-apis/im/v2/app_feed_card/batch"},
			DefaultAs: "bot",
			Params:    map[string]any{"user_id_type": "open_id"},
			Data: map[string]any{
				"feed_cards": []map[string]string{{
					"biz_id":  cleanupBizID,
					"user_id": selfOpenID,
				}},
			},
		})
		clie2e.ReportCleanupFailure(t, "delete app feed card", cleanupResult, cleanupErr)
	})

	result.AssertStdoutStatus(t, true)
	returnedBizID := gjson.Get(result.Stdout, "data.biz_id").String()
	if returnedBizID != "" {
		cleanupBizID = returnedBizID
	}
	require.NotEmpty(t, returnedBizID, "stdout:\n%s", result.Stdout)

	updateResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"im", "+app-feed-card-update",
			"--user-ids", selfOpenID,
			"--biz-id", returnedBizID,
			"--title", "lark-cli e2e app feed card updated",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if updateResult.ExitCode != 0 && isAppFeedPermissionFailure(updateResult) {
		t.Skipf("skipped: app feed card update permission is unavailable in this environment: %s", updateResult.Stderr)
	}
	updateResult.AssertExitCode(t, 0)
	updateResult.AssertStdoutStatus(t, true)

	deleteResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"im", "+app-feed-card-delete",
			"--user-ids", selfOpenID,
			"--biz-id", returnedBizID,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if deleteResult.ExitCode != 0 && isAppFeedPermissionFailure(deleteResult) {
		t.Skipf("skipped: app feed card delete permission is unavailable in this environment: %s", deleteResult.Stderr)
	}
	deleteResult.AssertExitCode(t, 0)
	deleteResult.AssertStdoutStatus(t, true)
	deleted = true
}

func getCurrentUserOpenIDForIM(t *testing.T, ctx context.Context) string {
	t.Helper()
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"contact", "+get-user"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	openID := gjson.Get(result.Stdout, "data.user.open_id").String()
	require.NotEmpty(t, openID, "stdout:\n%s", result.Stdout)
	return openID
}

func firstIMDryRunRequest(t *testing.T, stdout string) map[string]any {
	t.Helper()

	const prefix = "=== Dry Run ===\n"
	stdout = strings.TrimPrefix(stdout, prefix)
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("parse dry-run payload: %v\nstdout:\n%s", err, stdout)
	}

	apiEntries, ok := payload["api"].([]any)
	require.True(t, ok, "payload missing api array: %#v", payload)
	require.Len(t, apiEntries, 1)

	entry, ok := apiEntries[0].(map[string]any)
	require.True(t, ok, "api entry is not an object: %#v", apiEntries[0])
	return entry
}

func isAppFeedPermissionFailure(result *clie2e.Result) bool {
	raw := strings.ToLower(result.Stdout + "\n" + result.Stderr)
	return strings.Contains(raw, "permission") ||
		strings.Contains(raw, "missing_scope") ||
		strings.Contains(raw, "im:app_feed_card:write") ||
		strings.Contains(raw, "999916")
}
