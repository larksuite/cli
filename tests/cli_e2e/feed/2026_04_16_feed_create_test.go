// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package feed

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func discoverFeedRecipientOpenID(t *testing.T, ctx context.Context) string {
	t.Helper()

	// Get the authenticated user's own open_id from auth status.
	// This works in sandbox environments where the bot may not have contact:list permission.
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"auth", "status"},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	openID := gjson.Get(result.Stdout, "userOpenId").String()
	require.NotEmpty(t, openID, "expected to get userOpenId from auth status; stdout:\n%s", result.Stdout)
	return openID
}

// TestFeed_CreateBasic covers Scenario 1: create a feed card with required flags only.
func TestFeed_CreateBasic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	recipientOpenID := discoverFeedRecipientOpenID(t, ctx)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"feed", "+create",
			"--user-ids", recipientOpenID,
			"--title", "测试卡片",
			"--link", "https://www.feishu.cn/",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	bizID := gjson.Get(result.Stdout, "data.biz_id").String()
	assert.NotEmpty(t, bizID, "stdout should contain non-empty biz_id:\n%s", result.Stdout)

	failedCards := gjson.Get(result.Stdout, "data.failed_cards")
	assert.True(t, failedCards.IsArray(), "failed_cards should be an array:\n%s", result.Stdout)
	assert.Equal(t, 0, len(failedCards.Array()), "failed_cards should be empty:\n%s", result.Stdout)
}

// TestFeed_CreateWithOptionalFields covers Scenario 2: create a feed card with --preview and --time-sensitive.
func TestFeed_CreateWithOptionalFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	recipientOpenID := discoverFeedRecipientOpenID(t, ctx)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"feed", "+create",
			"--user-ids", recipientOpenID,
			"--title", "带预览",
			"--link", "https://www.feishu.cn/",
			"--preview", "这是预览文字",
			"--time-sensitive",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	bizID := gjson.Get(result.Stdout, "data.biz_id").String()
	assert.NotEmpty(t, bizID, "stdout should contain non-empty biz_id:\n%s", result.Stdout)

	failedCards := gjson.Get(result.Stdout, "data.failed_cards")
	assert.True(t, failedCards.IsArray(), "failed_cards should be an array:\n%s", result.Stdout)
	assert.Equal(t, 0, len(failedCards.Array()), "failed_cards should be empty:\n%s", result.Stdout)
}

// TestFeed_CreateMissingUserIDs covers Scenario 3: --user-ids is missing.
func TestFeed_CreateMissingUserIDs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"feed", "+create",
			"--title", "测试",
			"--link", "https://www.feishu.cn/",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	assert.NotEqual(t, 0, result.ExitCode, "exit code should be non-zero when --user-ids is missing")
	assert.True(t, strings.Contains(result.Stderr, "user-ids"),
		"stderr should mention 'user-ids':\n%s", result.Stderr)
}

// TestFeed_CreateHTTPLinkRejected covers Scenario 4: --link with HTTP (non-HTTPS) is rejected.
func TestFeed_CreateHTTPLinkRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	recipientOpenID := discoverFeedRecipientOpenID(t, ctx)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"feed", "+create",
			"--user-ids", recipientOpenID,
			"--title", "测试",
			"--link", "http://www.feishu.cn/",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	assert.NotEqual(t, 0, result.ExitCode, "exit code should be non-zero for non-HTTPS link")
	assert.True(t, strings.Contains(result.Stderr, "https"),
		"stderr should mention 'https':\n%s", result.Stderr)
}

// TestFeed_CreateInvalidUserIDFormat covers Scenario 5: --user-ids with invalid format.
func TestFeed_CreateInvalidUserIDFormat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"feed", "+create",
			"--user-ids", "invalid_id",
			"--title", "测试",
			"--link", "https://www.feishu.cn/",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	assert.NotEqual(t, 0, result.ExitCode, "exit code should be non-zero for invalid user-ids format")
	assert.True(t,
		strings.Contains(result.Stderr, "open_id") || strings.Contains(result.Stderr, "ou_"),
		"stderr should mention 'open_id' or 'ou_':\n%s", result.Stderr)
}

// TestFeed_CreateDryRun covers Scenario 6: --dry-run outputs the correct API path.
// Uses a static test user ID since dry-run doesn't actually call the API.
func TestFeed_CreateDryRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"feed", "+create",
			"--user-ids", "ou_test_dry_run_static",
			"--title", "测试",
			"--link", "https://www.feishu.cn/",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	assert.True(t, strings.Contains(result.Stdout, "/open-apis/im/v2/app_feed_card"),
		"stdout should contain the API path '/open-apis/im/v2/app_feed_card':\n%s", result.Stdout)
}
