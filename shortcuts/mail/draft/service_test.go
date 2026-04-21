// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func draftServiceTestRuntime(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
	t.Helper()
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())

	cfg := &core.CliConfig{
		AppID:      "test-app",
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_testuser",
		UserName:   "Test User",
	}
	token := &auth.StoredUAToken{
		UserOpenId:       cfg.UserOpenId,
		AppId:            cfg.AppID,
		AccessToken:      "test-user-access-token",
		RefreshToken:     "test-refresh-token",
		ExpiresAt:        time.Now().Add(1 * time.Hour).UnixMilli(),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour).UnixMilli(),
		Scope:            "mail:user_mailbox.messages:write mail:user_mailbox.messages:read mail:user_mailbox.message:modify mail:user_mailbox.message:readonly mail:user_mailbox.message.address:read mail:user_mailbox.message.subject:read mail:user_mailbox.message.body:read mail:user_mailbox:readonly",
		GrantedAt:        time.Now().Add(-1 * time.Hour).UnixMilli(),
	}
	if err := auth.SetStoredToken(token); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}
	t.Cleanup(func() {
		_ = auth.RemoveStoredToken(cfg.AppID, cfg.UserOpenId)
	})

	factory, _, _, reg := cmdutil.TestFactory(t, cfg)
	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), &cobra.Command{Use: "test"}, cfg)
	runtime.Factory = factory
	return runtime, reg
}

func TestExtractReference(t *testing.T) {
	t.Run("top-level reference", func(t *testing.T) {
		data := map[string]interface{}{"reference": "https://example.com/draft/1"}
		if got := extractReference(data); got != "https://example.com/draft/1" {
			t.Fatalf("extractReference() = %q, want %q", got, "https://example.com/draft/1")
		}
	})

	t.Run("nested draft reference", func(t *testing.T) {
		data := map[string]interface{}{
			"draft": map[string]interface{}{
				"reference": "https://example.com/draft/2",
			},
		}
		if got := extractReference(data); got != "https://example.com/draft/2" {
			t.Fatalf("extractReference() = %q, want %q", got, "https://example.com/draft/2")
		}
	})

	t.Run("missing reference", func(t *testing.T) {
		if got := extractReference(nil); got != "" {
			t.Fatalf("extractReference(nil) = %q, want empty string", got)
		}
	})
}

func TestCreateWithRawReturnsDraftResultWithReference(t *testing.T) {
	runtime, reg := draftServiceTestRuntime(t)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"draft_id":  "draft_001",
				"reference": "https://www.feishu.cn/mail?draftId=draft_001",
			},
		},
	})

	got, err := CreateWithRaw(runtime, "me", "raw-eml")
	if err != nil {
		t.Fatalf("CreateWithRaw() error = %v", err)
	}
	if got.DraftID != "draft_001" {
		t.Fatalf("DraftID = %q, want %q", got.DraftID, "draft_001")
	}
	if got.Reference != "https://www.feishu.cn/mail?draftId=draft_001" {
		t.Fatalf("Reference = %q, want %q", got.Reference, "https://www.feishu.cn/mail?draftId=draft_001")
	}
}

func TestUpdateWithRawFallsBackToInputDraftIDAndReturnsReference(t *testing.T) {
	runtime, reg := draftServiceTestRuntime(t)

	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/drafts/draft_002",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"reference": "https://www.feishu.cn/mail?draftId=draft_002",
			},
		},
	})

	got, err := UpdateWithRaw(runtime, "me", "draft_002", "raw-eml")
	if err != nil {
		t.Fatalf("UpdateWithRaw() error = %v", err)
	}
	if got.DraftID != "draft_002" {
		t.Fatalf("DraftID = %q, want fallback %q", got.DraftID, "draft_002")
	}
	if got.Reference != "https://www.feishu.cn/mail?draftId=draft_002" {
		t.Fatalf("Reference = %q, want %q", got.Reference, "https://www.feishu.cn/mail?draftId=draft_002")
	}
}
