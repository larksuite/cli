// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"
)

func draftTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	return &core.CliConfig{
		AppID:     "test-okr-draft",
		AppSecret: "secret-okr-draft",
		Brand:     core.BrandFeishu,
	}
}

func runDraftShortcut(t *testing.T, shortcut common.Shortcut, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	shortcut.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	stdout.Reset()
	return parent.Execute()
}

func TestPublishDraftDryRunUsesExactPath(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, draftTestConfig(t))
	err := runDraftShortcut(t, OKRPublishDraft, f, stdout, []string{
		"+publish-draft", "--objective-id", "123", "--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if got := gjson.Get(output, "data.api.0.method").String(); got != "POST" {
		t.Fatalf("method = %q, want POST; output: %s", got, output)
	}
	if got := gjson.Get(output, "data.api.0.url").String(); got != "/open-apis/okr/v2/objectives/123/draft_publish" {
		t.Fatalf("url = %q, want draft_publish path; output: %s", got, output)
	}
}

func TestDropDraftDryRunUsesExactPathAndHighRisk(t *testing.T) {
	t.Parallel()
	if OKRDropDraft.Risk != "high-risk-write" {
		t.Fatalf("Risk = %q, want high-risk-write", OKRDropDraft.Risk)
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, draftTestConfig(t))
	err := runDraftShortcut(t, OKRDropDraft, f, stdout, []string{
		"+drop-draft", "--objective-id", "456", "--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if got := gjson.Get(output, "data.api.0.method").String(); got != "POST" {
		t.Fatalf("method = %q, want POST; output: %s", got, output)
	}
	if got := gjson.Get(output, "data.api.0.url").String(); got != "/open-apis/okr/v2/objectives/456/draft_drop" {
		t.Fatalf("url = %q, want draft_drop path; output: %s", got, output)
	}
}

func TestDraftShortcutRejectsInvalidObjectiveID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, draftTestConfig(t))
	err := runDraftShortcut(t, OKRPublishDraft, f, stdout, []string{
		"+publish-draft", "--objective-id", "invalid",
	})
	if err == nil || !strings.Contains(err.Error(), "--objective-id must be a positive int64") {
		t.Fatalf("expected objective ID validation error, got: %v", err)
	}
}

func TestPublishDraftExecute(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, draftTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/objectives/123/draft_publish",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"objective_id": "123"},
		},
	})
	err := runDraftShortcut(t, OKRPublishDraft, f, stdout, []string{
		"+publish-draft", "--objective-id", "123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := gjson.Get(stdout.String(), "data.objective_id").String(); got != "123" {
		t.Fatalf("objective_id = %q, want 123; output: %s", got, stdout.String())
	}
}

func TestDropDraftExecuteRequiresYes(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, draftTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/objectives/456/draft_drop",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"objective_id": "456"},
		},
	})
	if err := runDraftShortcut(t, OKRDropDraft, f, stdout, []string{
		"+drop-draft", "--objective-id", "456",
	}); err == nil {
		t.Fatal("expected --yes confirmation error")
	}
	if err := runDraftShortcut(t, OKRDropDraft, f, stdout, []string{
		"+drop-draft", "--objective-id", "456", "--yes",
	}); err != nil {
		t.Fatalf("unexpected error with --yes: %v", err)
	}
}

func TestDraftActionResponseProjection(t *testing.T) {
	for _, shortcut := range []common.Shortcut{OKRPublishDraft, OKRDropDraft} {
		for _, tc := range []struct {
			name      string
			data      map[string]interface{}
			wantID    string
			wantError bool
		}{
			{"returned_id", map[string]interface{}{"objective_id": "999"}, "999", false},
			{"omitted_id", map[string]interface{}{}, "123", false},
			{"empty_id", map[string]interface{}{"objective_id": ""}, "123", false},
			{"null_id", map[string]interface{}{"objective_id": nil}, "123", false},
			{"invalid_type", map[string]interface{}{"objective_id": 123}, "", true},
		} {
			t.Run(shortcut.Command+"/"+tc.name, func(t *testing.T) {
				f, stdout, _, reg := cmdutil.TestFactory(t, draftTestConfig(t))
				action := "draft_publish"
				args := []string{shortcut.Command, "--objective-id", "123"}
				if shortcut.Command == "+drop-draft" {
					action = "draft_drop"
					args = append(args, "--yes")
				}
				reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/okr/v2/objectives/123/" + action, Body: map[string]interface{}{"code": 0, "data": tc.data}})
				err := runDraftShortcut(t, shortcut, f, stdout, args)
				if tc.wantError {
					var internalErr *errs.InternalError
					if !errors.As(err, &internalErr) || internalErr.Subtype != errs.SubtypeInvalidResponse {
						t.Fatalf("want typed invalid_response, got %v", err)
					}
					var cause *json.UnmarshalTypeError
					if !errors.As(err, &cause) {
						t.Fatalf("decode error cause was lost: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got := gjson.Get(stdout.String(), "data.objective_id").String(); got != tc.wantID {
					t.Fatalf("objective_id = %q, want %q", got, tc.wantID)
				}
				if got := gjson.Get(stdout.String(), "data.action").String(); got != action {
					t.Fatalf("action = %q, want %q", got, action)
				}
			})
		}
	}
}
