// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package feed

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func feedTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	suffix := strings.NewReplacer("/", "-", " ", "-", ":", "-", "\t", "-").Replace(t.Name())
	return &core.CliConfig{
		AppID:      "test-app-" + suffix,
		AppSecret:  "test-secret-" + suffix,
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_testuser",
		UserName:   "Test User",
	}
}

func feedShortcutTestFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	return cmdutil.TestFactory(t, feedTestConfig(t))
}

func warmTenantToken(t *testing.T, f *cmdutil.Factory, reg *httpmock.Registry) {
	t.Helper()
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/test/v1/warm",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{},
		},
	})
	s := common.Shortcut{
		Service:   "test",
		Command:   "+warm-token",
		AuthTypes: []string{"bot"},
		Execute: func(_ context.Context, rctx *common.RuntimeContext) error {
			_, err := rctx.CallAPI("GET", "/open-apis/test/v1/warm", nil, nil)
			return err
		},
	}
	parent := &cobra.Command{Use: "test"}
	s.Mount(parent, f)
	parent.SetArgs([]string{"+warm-token", "--as", "bot"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if err := parent.Execute(); err != nil {
		t.Fatalf("warm tenant token: %v", err)
	}
}

func runMountedFeedShortcut(t *testing.T, shortcut common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "test"}
	shortcut.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func TestFeedCreate_Success(t *testing.T) {
	f, stdout, _, reg := feedShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/im/v2/app_feed_card",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"biz_id":       "test-biz-id-123",
				"failed_cards": []interface{}{},
			},
		},
	})

	args := []string{"+create", "--user-ids", "ou_abc123", "--title", "Test Card", "--link", "https://www.feishu.cn/", "--as", "bot"}
	err := runMountedFeedShortcut(t, FeedCreate, args, f, stdout)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "biz_id") {
		t.Errorf("expected biz_id in output, got: %s", out)
	}
	if !strings.Contains(out, "test-biz-id-123") {
		t.Errorf("expected biz_id value in output, got: %s", out)
	}
	if !strings.Contains(out, "failed_cards") {
		t.Errorf("expected failed_cards in output, got: %s", out)
	}
}

func TestFeedCreate_SuccessWithOptionalFields(t *testing.T) {
	f, stdout, _, reg := feedShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/im/v2/app_feed_card",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"biz_id":       "biz-optional-456",
				"failed_cards": []interface{}{},
			},
		},
	})

	args := []string{
		"+create",
		"--user-ids", "ou_abc123",
		"--title", "带预览",
		"--link", "https://www.feishu.cn/",
		"--preview", "这是预览文字",
		"--time-sensitive",
		"--as", "bot",
	}
	err := runMountedFeedShortcut(t, FeedCreate, args, f, stdout)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "biz-optional-456") {
		t.Errorf("expected biz_id value in output, got: %s", out)
	}
}

func TestFeedShortcuts(t *testing.T) {
	shortcuts := Shortcuts()
	if len(shortcuts) != 1 {
		t.Fatalf("Shortcuts() len = %d, want 1", len(shortcuts))
	}
	if shortcuts[0].Command != "+create" {
		t.Errorf("Shortcuts()[0].Command = %q, want +create", shortcuts[0].Command)
	}
}

func TestFeedCreate_Validate(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "invalid user-id format",
			args:    []string{"+create", "--user-ids", "invalid_id", "--title", "Test", "--link", "https://www.feishu.cn/", "--as", "bot"},
			wantErr: "ou_",
		},
		{
			name:    "link uses http not https",
			args:    []string{"+create", "--user-ids", "ou_abc", "--title", "Test", "--link", "http://www.feishu.cn/", "--as", "bot"},
			wantErr: "https",
		},
		{
			name:    "title too long",
			args:    []string{"+create", "--user-ids", "ou_abc", "--title", strings.Repeat("a", 61), "--link", "https://www.feishu.cn/", "--as", "bot"},
			wantErr: "title",
		},
		{
			name:    "preview too long",
			args:    []string{"+create", "--user-ids", "ou_abc", "--title", "Test", "--link", "https://www.feishu.cn/", "--preview", strings.Repeat("a", 121), "--as", "bot"},
			wantErr: "preview",
		},
		{
			name: "too many user-ids",
			args: func() []string {
				base := []string{"+create", "--title", "T", "--link", "https://example.com/", "--as", "bot"}
				for i := 0; i < 21; i++ {
					base = append(base, "--user-ids", fmt.Sprintf("ou_%02d", i))
				}
				return base
			}(),
			wantErr: "20",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := feedShortcutTestFactory(t)
			err := runMountedFeedShortcut(t, FeedCreate, tt.args, f, stdout)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil (stdout=%s)", tt.wantErr, stdout.String())
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}
