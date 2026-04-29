// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package feed

import (
	"bytes"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func defaultConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
}

func mountAndRun(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "test"}
	s.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func TestFeedSensitive_Validate_MissingEnableDisable(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, FeedSensitive, []string{
		"+sensitive",
		"--feed-card-id", "oc_abc123",
		"--user-ids", "ou_user1",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "--enable") || !strings.Contains(err.Error(), "--disable") {
		t.Errorf("error should mention --enable and --disable, got: %v", err)
	}
}

func TestFeedSensitive_Validate_BothEnableAndDisable(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, FeedSensitive, []string{
		"+sensitive",
		"--feed-card-id", "oc_abc123",
		"--enable",
		"--disable",
		"--user-ids", "ou_user1",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention mutually exclusive, got: %v", err)
	}
}

func TestFeedSensitive_Validate_InvalidFeedCardID(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, FeedSensitive, []string{
		"+sensitive",
		"--feed-card-id", "invalid_id",
		"--enable",
		"--user-ids", "ou_user1",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "oc_") {
		t.Errorf("error should mention oc_ prefix, got: %v", err)
	}
}

func TestFeedSensitive_Validate_EmptyUserIDs(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, FeedSensitive, []string{
		"+sensitive",
		"--feed-card-id", "oc_abc123",
		"--enable",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for missing --user-ids, got nil")
	}
	if !strings.Contains(err.Error(), "user-ids") {
		t.Errorf("error should mention user-ids, got: %v", err)
	}
}

func TestFeedSensitive_Execute_AllSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/im/v2/feed_cards/oc_abc123",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"failed_user_reasons": []interface{}{},
			},
		},
	})

	err := mountAndRun(t, FeedSensitive, []string{
		"+sensitive",
		"--feed-card-id", "oc_abc123",
		"--enable",
		"--user-ids", "ou_user1",
		"--as", "bot",
		"--format", "pretty",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Time-sensitive updated for 1 user(s)") {
		t.Errorf("stdout should contain success message, got: %s", stdout.String())
	}
}

func TestFeedSensitive_Execute_PartialFailure(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/im/v2/feed_cards/oc_abc123",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"failed_user_reasons": []interface{}{
					map[string]interface{}{
						"error_code":    0,
						"error_message": "The user is not in the chat",
						"user_id":       "ou_user2",
					},
				},
			},
		},
	})

	err := mountAndRun(t, FeedSensitive, []string{
		"+sensitive",
		"--feed-card-id", "oc_abc123",
		"--enable",
		"--user-ids", "ou_user1,ou_user2",
		"--as", "bot",
		"--format", "pretty",
	}, f, stdout)

	if err == nil {
		t.Fatal("expected error for partial failure, got nil")
	}
	if !strings.Contains(stderr.String(), "warning:") {
		t.Errorf("stderr should contain warning, got: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Time-sensitive updated for 1 user(s)") {
		t.Errorf("stdout should report 1 success, got: %s", stdout.String())
	}
}

func TestFeedSensitive_Execute_AllFailed(t *testing.T) {
	f, _, stderr, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/im/v2/feed_cards/oc_abc123",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"failed_user_reasons": []interface{}{
					map[string]interface{}{
						"error_code":    0,
						"error_message": "The user is not in the chat",
						"user_id":       "ou_user1",
					},
				},
			},
		},
	})

	err := mountAndRun(t, FeedSensitive, []string{
		"+sensitive",
		"--feed-card-id", "oc_abc123",
		"--enable",
		"--user-ids", "ou_user1",
		"--as", "bot",
		"--format", "pretty",
	}, f, nil)

	if err == nil {
		t.Fatal("expected error when all users fail, got nil")
	}
	if !strings.Contains(err.Error(), "all") {
		t.Errorf("error should mention 'all', got: %v", err)
	}
	if !strings.Contains(stderr.String(), "warning:") {
		t.Errorf("stderr should contain warning, got: %s", stderr.String())
	}
}
