// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// newFeedShortcutCreateCmd builds a cobra.Command pre-wired with the flags
// ImFeedShortcutCreate registers at runtime. Mirrors the helper used by other
// shortcut tests so tests can exercise the typed Bool/StrSlice accessors.
func newFeedShortcutCreateCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringSlice("chat-id", nil, "")
	cmd.Flags().Bool("head", false, "")
	cmd.Flags().Bool("tail", false, "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	return cmd
}

func newFeedShortcutRemoveCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringSlice("chat-id", nil, "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	return cmd
}

func newFeedShortcutListCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("page-token", "", "")
	// Default true (skip enrichment) in tests so non-enrichment-focused tests
	// don't trigger the batch_query path; tests that exercise detail
	// enrichment flip this off.
	cmd.Flags().Bool("no-detail", true, "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	return cmd
}

func TestCollectChatIDs(t *testing.T) {
	tests := []struct {
		name      string
		input     []string
		want      []string
		wantErr   bool
		errSubstr string
	}{
		{name: "single id", input: []string{"oc_abc"}, want: []string{"oc_abc"}},
		{name: "two repeated flags", input: []string{"oc_abc", "oc_def"}, want: []string{"oc_abc", "oc_def"}},
		// StringSlice handles comma splitting itself, but extra whitespace and
		// duplicates should still be normalized inside collectChatIDs.
		{name: "trims whitespace", input: []string{" oc_abc ", "oc_def"}, want: []string{"oc_abc", "oc_def"}},
		{name: "dedupes", input: []string{"oc_abc", "oc_abc", "oc_def"}, want: []string{"oc_abc", "oc_def"}},
		{name: "rejects empty list", input: nil, wantErr: true, errSubstr: "--chat-id is required"},
		{name: "rejects bad prefix", input: []string{"om_abc"}, wantErr: true, errSubstr: "must be an open_chat_id"},
		{
			name: "rejects over limit",
			input: []string{
				"oc_1", "oc_2", "oc_3", "oc_4", "oc_5",
				"oc_6", "oc_7", "oc_8", "oc_9", "oc_10", "oc_11",
			},
			wantErr:   true,
			errSubstr: "too many --chat-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newFeedShortcutCreateCmd(t)
			for _, v := range tt.input {
				if err := cmd.Flags().Set("chat-id", v); err != nil {
					t.Fatalf("Set chat-id %q error = %v", v, err)
				}
			}
			runtime := &common.RuntimeContext{Cmd: cmd}

			got, err := collectChatIDs(runtime)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("collectChatIDs() expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("collectChatIDs() error = %q, want substring %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("collectChatIDs() unexpected error: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("collectChatIDs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildShortcutItems(t *testing.T) {
	got := buildShortcutItems([]string{"oc_a", "oc_b"})
	if len(got) != 2 {
		t.Fatalf("buildShortcutItems() len = %d, want 2", len(got))
	}
	for i, it := range got {
		if it.Type != int(ShortcutTypeChat) {
			t.Fatalf("item %d type = %d, want %d", i, it.Type, ShortcutTypeChat)
		}
	}
	if got[0].FeedCardID != "oc_a" || got[1].FeedCardID != "oc_b" {
		t.Fatalf("buildShortcutItems() ids = %+v, want oc_a,oc_b", got)
	}
}

func TestShortcutFailedReasonString(t *testing.T) {
	tests := []struct {
		reason int
		want   string
	}{
		{0, "unknown"},
		{1, "no_permission"},
		{2, "invalid_item"},
		{3, "has_pending_delete"},
		{4, "type_not_support"},
		{5, "internal_error"},
		{99, "unknown"},
	}
	for _, tt := range tests {
		if got := shortcutFailedReasonString(tt.reason); got != tt.want {
			t.Fatalf("shortcutFailedReasonString(%d) = %q, want %q", tt.reason, got, tt.want)
		}
	}
}

func TestAnnotateFailedShortcuts(t *testing.T) {
	data := map[string]any{
		"failed_shortcuts": []any{
			map[string]any{"reason": float64(1)},
			map[string]any{"reason": float64(4)},
			map[string]any{"other": "field"}, // no reason field — should be left alone
		},
	}
	annotateFailedShortcuts(data)

	items := data["failed_shortcuts"].([]any)
	if got := items[0].(map[string]any)["reason_label"]; got != "no_permission" {
		t.Fatalf("item 0 reason_label = %v, want no_permission", got)
	}
	if got := items[1].(map[string]any)["reason_label"]; got != "type_not_support" {
		t.Fatalf("item 1 reason_label = %v, want type_not_support", got)
	}
	if _, ok := items[2].(map[string]any)["reason_label"]; ok {
		t.Fatalf("item 2 should not have reason_label set")
	}
}

func TestHasFailedShortcuts(t *testing.T) {
	if hasFailedShortcuts(map[string]any{}) {
		t.Fatalf("missing failed_shortcuts should not count as failure")
	}
	if hasFailedShortcuts(map[string]any{"failed_shortcuts": []any{}}) {
		t.Fatalf("empty failed_shortcuts should not count as failure")
	}
	if !hasFailedShortcuts(map[string]any{"failed_shortcuts": []any{map[string]any{"reason": float64(2)}}}) {
		t.Fatalf("non-empty failed_shortcuts should count as failure")
	}
}

func TestAddFeedShortcutWriteLedger(t *testing.T) {
	data := map[string]any{
		"failed_shortcuts": []any{
			map[string]any{
				"reason": float64(2),
				"shortcut": map[string]any{
					"feed_card_id": "oc_b",
					"type":         float64(1),
				},
			},
		},
	}
	addFeedShortcutWriteLedger(data, []shortcutItem{
		{FeedCardID: "oc_a", Type: int(ShortcutTypeChat)},
		{FeedCardID: "oc_b", Type: int(ShortcutTypeChat)},
	})

	if data["total"] != 2 || data["success_count"] != 1 || data["failure_count"] != 1 {
		t.Fatalf("ledger counts = total:%v success:%v failure:%v",
			data["total"], data["success_count"], data["failure_count"])
	}
	succeeded := data["succeeded_shortcuts"].([]shortcutItem)
	if len(succeeded) != 1 || succeeded[0].FeedCardID != "oc_a" {
		t.Fatalf("succeeded_shortcuts = %+v, want only oc_a", succeeded)
	}
}

func TestAddFeedShortcutWriteLedgerFailedEchoMissingType(t *testing.T) {
	// A failed echo whose shortcut omits `type` (or sends 0) must still
	// exclude its item from the success list: matching is by feed_card_id
	// alone, so the ledger invariant success+failure==total holds.
	data := map[string]any{
		"failed_shortcuts": []any{
			map[string]any{
				"reason":   float64(4),
				"shortcut": map[string]any{"feed_card_id": "oc_b"},
			},
		},
	}
	addFeedShortcutWriteLedger(data, []shortcutItem{
		{FeedCardID: "oc_a", Type: int(ShortcutTypeChat)},
		{FeedCardID: "oc_b", Type: int(ShortcutTypeChat)},
	})

	if data["total"] != 2 || data["success_count"] != 1 || data["failure_count"] != 1 {
		t.Fatalf("ledger counts = total:%v success:%v failure:%v, want 2/1/1",
			data["total"], data["success_count"], data["failure_count"])
	}
	succeeded := data["succeeded_shortcuts"].([]shortcutItem)
	if len(succeeded) != 1 || succeeded[0].FeedCardID != "oc_a" {
		t.Fatalf("succeeded_shortcuts = %+v, want only oc_a", succeeded)
	}
}

func TestAddFeedShortcutWriteLedgerDuplicateFailedEcho(t *testing.T) {
	// A server that echoes the same failed shortcut twice must not break the
	// success+failure==total invariant: counts derive from requested-item
	// accounting, while failed_shortcuts keeps the raw (duplicated) report.
	dup := map[string]any{
		"reason":   float64(2),
		"shortcut": map[string]any{"feed_card_id": "oc_b", "type": float64(1)},
	}
	data := map[string]any{"failed_shortcuts": []any{dup, dup}}
	addFeedShortcutWriteLedger(data, []shortcutItem{
		{FeedCardID: "oc_a", Type: int(ShortcutTypeChat)},
		{FeedCardID: "oc_b", Type: int(ShortcutTypeChat)},
	})

	if data["total"] != 2 || data["success_count"] != 1 || data["failure_count"] != 1 {
		t.Fatalf("ledger counts = total:%v success:%v failure:%v, want 2/1/1",
			data["total"], data["success_count"], data["failure_count"])
	}
}

func TestAnnotateFailedShortcutsNoOpWhenMissing(t *testing.T) {
	// Must not panic when failed_shortcuts is missing or wrong type.
	annotateFailedShortcuts(map[string]any{})
	annotateFailedShortcuts(map[string]any{"failed_shortcuts": "not-a-list"})
}

func TestResolveIsHeader(t *testing.T) {
	tests := []struct {
		name    string
		head    bool
		tail    bool
		want    bool
		wantErr bool
	}{
		{name: "default is head", want: true},
		{name: "--head explicit", head: true, want: true},
		{name: "--tail", tail: true, want: false},
		{name: "both set errors", head: true, tail: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newFeedShortcutCreateCmd(t)
			if tt.head {
				if err := cmd.Flags().Set("head", "true"); err != nil {
					t.Fatalf("Set head error = %v", err)
				}
			}
			if tt.tail {
				if err := cmd.Flags().Set("tail", "true"); err != nil {
					t.Fatalf("Set tail error = %v", err)
				}
			}
			rt := &common.RuntimeContext{Cmd: cmd}
			got, err := resolveIsHeader(rt)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveIsHeader() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveIsHeader() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveIsHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFeedShortcutStaticScopes(t *testing.T) {
	if got := ImFeedShortcutCreate.ScopesForIdentity("user"); len(got) != 1 || got[0] != feedShortcutWriteScope {
		t.Fatalf("ImFeedShortcutCreate scopes = %v, want only %s", got, feedShortcutWriteScope)
	}
	if got := ImFeedShortcutRemove.ScopesForIdentity("user"); len(got) != 1 || got[0] != feedShortcutWriteScope {
		t.Fatalf("ImFeedShortcutRemove scopes = %v, want only %s", got, feedShortcutWriteScope)
	}
	if got := ImFeedShortcutList.ScopesForIdentity("user"); len(got) != 1 || got[0] != feedShortcutReadScope {
		t.Fatalf("ImFeedShortcutList scopes = %v, want only %s", got, feedShortcutReadScope)
	}
}

func TestFeedShortcutAuthTypesUserOnly(t *testing.T) {
	for _, sc := range []common.Shortcut{ImFeedShortcutCreate, ImFeedShortcutRemove, ImFeedShortcutList} {
		if len(sc.AuthTypes) != 1 || sc.AuthTypes[0] != "user" {
			t.Fatalf("shortcut %s AuthTypes = %v, want [user]", sc.Command, sc.AuthTypes)
		}
	}
}

func TestImFeedShortcutCreateDryRunReportsValidationError(t *testing.T) {
	cmd := newFeedShortcutCreateCmd(t)
	// no chat-id set → validation error surfaced in DryRun output
	rt := &common.RuntimeContext{Cmd: cmd}
	got := ImFeedShortcutCreate.DryRun(context.Background(), rt).Format()
	if !strings.Contains(got, "--chat-id is required") {
		t.Fatalf("DryRun output = %q, want validation error", got)
	}
	if strings.Contains(got, "feed_shortcuts") {
		t.Fatalf("DryRun output = %q, should not include request for invalid input", got)
	}
}

func TestImFeedShortcutCreateDryRunRendersBody(t *testing.T) {
	cmd := newFeedShortcutCreateCmd(t)
	if err := cmd.Flags().Set("chat-id", "oc_abc"); err != nil {
		t.Fatalf("Set chat-id error = %v", err)
	}
	if err := cmd.Flags().Set("chat-id", "oc_def"); err != nil {
		t.Fatalf("Set chat-id error = %v", err)
	}
	if err := cmd.Flags().Set("tail", "true"); err != nil {
		t.Fatalf("Set tail error = %v", err)
	}
	rt := &common.RuntimeContext{Cmd: cmd}
	got := ImFeedShortcutCreate.DryRun(context.Background(), rt).Format()
	for _, want := range []string{
		"POST",
		"/open-apis/im/v2/feed_shortcuts",
		`"feed_card_id"`,
		"oc_abc",
		"oc_def",
		`"is_header"`,
		`false`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("DryRun output = %s, want %q", got, want)
		}
	}
}

func TestImFeedShortcutCreateExecuteCallsAPI(t *testing.T) {
	var gotBody []byte
	var gotPath string
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/open-apis/im/v2/feed_shortcuts") {
			return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
		}
		// Reject the remove suffix — confirms create uses the bare path.
		if strings.HasSuffix(req.URL.Path, "/remove") {
			return nil, fmt.Errorf("create should not call /remove: %s", req.URL.Path)
		}
		body, _ := io.ReadAll(req.Body)
		gotBody = body
		gotPath = req.URL.Path
		return shortcutJSONResponse(200, map[string]any{
			"code": 0,
			"data": map[string]any{
				"failed_shortcuts": []any{
					map[string]any{
						"reason": float64(2),
						"shortcut": map[string]any{
							"feed_card_id": "oc_abc",
							"type":         float64(1),
						},
					},
				},
			},
		}), nil
	}))

	cmd := newFeedShortcutCreateCmd(t)
	if err := cmd.Flags().Set("chat-id", "oc_abc"); err != nil {
		t.Fatalf("Set chat-id error = %v", err)
	}
	setRuntimeField(t, rt, "Cmd", cmd)

	err := ImFeedShortcutCreate.Execute(context.Background(), rt)
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("Execute() error = %T %v, want partial failure", err, err)
	}
	// Lock the documented exit-code contract: partial failure exits 1 (ExitAPI).
	if pfErr.Code != output.ExitAPI {
		t.Fatalf("partial failure exit code = %d, want %d (ExitAPI)", pfErr.Code, output.ExitAPI)
	}
	if !strings.HasSuffix(gotPath, "/open-apis/im/v2/feed_shortcuts") {
		t.Fatalf("Execute() path = %q, want /open-apis/im/v2/feed_shortcuts", gotPath)
	}
	if !strings.Contains(string(gotBody), `"feed_card_id":"oc_abc"`) {
		t.Fatalf("Execute() body = %s, want feed_card_id oc_abc", gotBody)
	}
	if !strings.Contains(string(gotBody), `"is_header":true`) {
		t.Fatalf("Execute() body = %s, want is_header true (default)", gotBody)
	}

	out := rt.Factory.IOStreams.Out.(interface{ String() string }).String()
	if !strings.Contains(out, `"ok": false`) {
		t.Fatalf("stdout = %s, want ok:false partial-failure envelope", out)
	}
	for _, want := range []string{
		`"total": 1`,
		`"success_count": 0`,
		`"failure_count": 1`,
		`"succeeded_shortcuts": []`,
		`"reason_label": "invalid_item"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout = %s, want %q", out, want)
		}
	}
}

func TestEmitFeedShortcutWriteResultSuccess(t *testing.T) {
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("must not call API")
		return nil, nil
	}))
	setRuntimeField(t, rt, "Cmd", newFeedShortcutCreateCmd(t))
	err := emitFeedShortcutWriteResult(rt, []shortcutItem{
		{FeedCardID: "oc_a", Type: int(ShortcutTypeChat)},
	}, map[string]any{"failed_shortcuts": []any{}})
	if err != nil {
		t.Fatalf("emitFeedShortcutWriteResult() error = %v, want nil", err)
	}
	out := rt.Factory.IOStreams.Out.(interface{ String() string }).String()
	for _, want := range []string{
		`"ok": true`,
		`"total": 1`,
		`"success_count": 1`,
		`"failure_count": 0`,
		`"feed_card_id": "oc_a"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout = %s, want %q", out, want)
		}
	}
}

func TestEmitFeedShortcutWriteResultNilData(t *testing.T) {
	// A fully-successful write can come back as code:0 with data:null, which
	// DoAPIJSON surfaces as a nil map. The emitter must still produce the
	// ledger instead of panicking on a nil-map write.
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("must not call API")
		return nil, nil
	}))
	setRuntimeField(t, rt, "Cmd", newFeedShortcutCreateCmd(t))
	err := emitFeedShortcutWriteResult(rt, []shortcutItem{
		{FeedCardID: "oc_a", Type: int(ShortcutTypeChat)},
	}, nil)
	if err != nil {
		t.Fatalf("emitFeedShortcutWriteResult(nil data) error = %v, want nil", err)
	}
	out := rt.Factory.IOStreams.Out.(interface{ String() string }).String()
	for _, want := range []string{
		`"ok": true`,
		`"total": 1`,
		`"success_count": 1`,
		`"failure_count": 0`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout = %s, want %q", out, want)
		}
	}
}

func TestImFeedShortcutRemoveExecuteCallsRemovePath(t *testing.T) {
	var gotPath string
	var gotBody []byte
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/open-apis/im/v2/feed_shortcuts/remove") {
			return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
		}
		body, _ := io.ReadAll(req.Body)
		gotBody = body
		gotPath = req.URL.Path
		return shortcutJSONResponse(200, map[string]any{
			"code": 0,
			"data": map[string]any{"failed_shortcuts": []any{}},
		}), nil
	}))

	cmd := newFeedShortcutRemoveCmd(t)
	if err := cmd.Flags().Set("chat-id", "oc_abc,oc_def"); err != nil {
		t.Fatalf("Set chat-id error = %v", err)
	}
	setRuntimeField(t, rt, "Cmd", cmd)

	if err := ImFeedShortcutRemove.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.HasSuffix(gotPath, "/open-apis/im/v2/feed_shortcuts/remove") {
		t.Fatalf("Execute() path = %q, want /open-apis/im/v2/feed_shortcuts/remove", gotPath)
	}
	if !strings.Contains(string(gotBody), `"feed_card_id":"oc_abc"`) {
		t.Fatalf("Execute() body = %s, want feed_card_id oc_abc", gotBody)
	}
	if !strings.Contains(string(gotBody), `"feed_card_id":"oc_def"`) {
		t.Fatalf("Execute() body = %s, want feed_card_id oc_def", gotBody)
	}
	// Remove must not send is_header — that's a create-only field.
	if strings.Contains(string(gotBody), "is_header") {
		t.Fatalf("Execute() body = %s, should NOT contain is_header", gotBody)
	}
}

func TestImFeedShortcutListDryRunRendersGet(t *testing.T) {
	cmd := newFeedShortcutListCmd(t)
	rt := &common.RuntimeContext{Cmd: cmd}
	got := ImFeedShortcutList.DryRun(context.Background(), rt).Format()
	for _, want := range []string{
		"GET",
		"/open-apis/im/v2/feed_shortcuts",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("DryRun output = %s, want %q", got, want)
		}
	}
	if strings.Contains(got, "page_token") {
		t.Fatalf("DryRun output = %s, should omit page_token on first-page request", got)
	}
}

func TestImFeedShortcutListDryRunIncludesNonEmptyPageToken(t *testing.T) {
	cmd := newFeedShortcutListCmd(t)
	if err := cmd.Flags().Set("page-token", "tok1"); err != nil {
		t.Fatalf("Set page-token error = %v", err)
	}
	rt := &common.RuntimeContext{Cmd: cmd}
	got := ImFeedShortcutList.DryRun(context.Background(), rt).Format()
	if !strings.Contains(got, "page_token=tok1") {
		t.Fatalf("DryRun output = %s, want page_token=tok1", got)
	}
}

func TestImFeedShortcutListHelpDoesNotTreatDetailAsArgName(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "app", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	parent := &cobra.Command{Use: "im"}
	ImFeedShortcutList.Mount(parent, f)

	cmd, _, err := parent.Find([]string{"+feed-shortcut-list"})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Help(); err != nil {
		t.Fatalf("Help() error = %v", err)
	}
	got := out.String()
	if strings.Contains(got, "--no-detail detail") {
		t.Fatalf("help output treats `detail` as a flag arg name:\n%s", got)
	}
	if !strings.Contains(got, "--no-detail") {
		t.Fatalf("help output missing --no-detail:\n%s", got)
	}
}

func TestImFeedShortcutListDryRunMentionsDetailScope(t *testing.T) {
	cmd := newFeedShortcutListCmd(t)
	if err := cmd.Flags().Set("no-detail", "false"); err != nil {
		t.Fatalf("Set no-detail error = %v", err)
	}
	rt := &common.RuntimeContext{Cmd: cmd}
	got := ImFeedShortcutList.DryRun(context.Background(), rt).Format()
	for _, want := range []string{
		"im:chat:read",
		"--no-detail",
		"batch_query",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("DryRun output = %s, want %q", got, want)
		}
	}
}

func TestImFeedShortcutListDoesNotExposeAutoPaginationFlags(t *testing.T) {
	// Locks in the design decision: this shortcut is a one-page wrapper.
	// If any of these reappear, callers/AI agents will assume auto-walking
	// is supported and write code that silently double-fetches.
	banned := map[string]bool{"page-all": true, "page-limit": true, "page-size": true}
	for _, fl := range ImFeedShortcutList.Flags {
		if banned[fl.Name] {
			t.Fatalf("ImFeedShortcutList must not expose --%s", fl.Name)
		}
	}
}

func TestImFeedShortcutListPageTokenIsOptional(t *testing.T) {
	// --page-token must NOT be Required: omitting it is the natural first-page
	// signal (the server treats "missing" and "" the same). Forcing an empty
	// string would just be noise.
	for _, fl := range ImFeedShortcutList.Flags {
		if fl.Name == "page-token" && fl.Required {
			t.Fatalf("--page-token must be optional; omitting it should mean first page")
		}
	}
}

func TestImFeedShortcutListDetailOnByDefault(t *testing.T) {
	// The real flag definition must keep detail enrichment on by default:
	// --no-detail is an opt-out bool with a false zero-value default. The
	// test-helper command flips it for isolation, so this definition-level
	// check is what actually locks the shipped default against a flip.
	for _, fl := range ImFeedShortcutList.Flags {
		if fl.Name == "no-detail" {
			if fl.Default != "" && fl.Default != "false" {
				t.Fatalf("--no-detail default = %q, want unset/false (enrichment on by default)", fl.Default)
			}
			return
		}
	}
	t.Fatalf("--no-detail flag not found on ImFeedShortcutList")
}

func TestFeedShortcutChatIDNotCobraRequired(t *testing.T) {
	// --chat-id is mandatory, but must NOT be cobra-Required: cobra would
	// intercept a missing flag before Validate runs and emit a plain-text
	// "required flag(s) not set" error (exit 1) instead of collectChatIDs'
	// structured validation envelope (exit 2).
	for _, sc := range []common.Shortcut{ImFeedShortcutCreate, ImFeedShortcutRemove} {
		for _, fl := range sc.Flags {
			if fl.Name == "chat-id" && fl.Required {
				t.Fatalf("%s: --chat-id must not be cobra-Required; requiredness is enforced by collectChatIDs", sc.Command)
			}
		}
	}
}

func TestFeedShortcutListQueryOmitsEmptyToken(t *testing.T) {
	q := feedShortcutListQuery("")
	if _, ok := q["page_token"]; ok {
		t.Fatalf("feedShortcutListQuery(\"\") = %v, want no page_token key", q)
	}
	q = feedShortcutListQuery("next")
	if v := q["page_token"]; len(v) != 1 || v[0] != "next" {
		t.Fatalf("feedShortcutListQuery(\"next\") page_token = %v, want [next]", v)
	}
}

func TestImFeedShortcutListExecuteForwardsToken(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		wantSent string // value the server should see in ?page_token=
		wantKey  bool   // whether ?page_token should appear at all
	}{
		{name: "first page omits param", token: "", wantSent: "", wantKey: false},
		{name: "explicit token is forwarded", token: "tok1", wantSent: "tok1", wantKey: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls int
			var sawKey bool
			var gotToken string
			rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if !strings.Contains(req.URL.Path, "/open-apis/im/v2/feed_shortcuts") {
					return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
				}
				calls++
				_, sawKey = req.URL.Query()["page_token"]
				gotToken = req.URL.Query().Get("page_token")
				return shortcutJSONResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"shortcuts":  []any{map[string]any{"feed_card_id": "oc_a", "type": float64(1)}},
						"has_more":   false,
						"page_token": "end",
					},
				}), nil
			}))

			cmd := newFeedShortcutListCmd(t)
			if err := cmd.Flags().Set("page-token", tt.token); err != nil {
				t.Fatalf("Set page-token error = %v", err)
			}
			setRuntimeField(t, rt, "Cmd", cmd)

			if err := ImFeedShortcutList.Execute(context.Background(), rt); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if calls != 1 {
				t.Fatalf("expected 1 API call, got %d", calls)
			}
			if sawKey != tt.wantKey {
				t.Fatalf("page_token query key present = %v, want %v", sawKey, tt.wantKey)
			}
			if gotToken != tt.wantSent {
				t.Fatalf("page_token sent = %q, want %q", gotToken, tt.wantSent)
			}
		})
	}
}

func TestShortcutTypeFromValue(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want ShortcutType
	}{
		{name: "float64 1 → chat", v: float64(1), want: ShortcutTypeChat},
		{name: "int 1 → chat", v: 1, want: ShortcutTypeChat},
		{name: "float64 0 → unknown", v: float64(0), want: ShortcutTypeUnknown},
		{name: "unknown numeric → unknown ShortcutType(99)", v: float64(99), want: ShortcutType(99)},
		{name: "string defaults to unknown", v: "1", want: ShortcutTypeUnknown},
		{name: "nil defaults to unknown", v: nil, want: ShortcutTypeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shortcutTypeFromValue(tt.v); got != tt.want {
				t.Fatalf("shortcutTypeFromValue(%v) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

func TestResolveChatDetailBatchesAt50(t *testing.T) {
	var calls int
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/open-apis/im/v1/chats/batch_query") {
			return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
		}
		calls++
		// Echo each requested chat_id back with a synthetic name so we can
		// confirm both that batching happened and that the response was
		// parsed correctly.
		body, _ := io.ReadAll(req.Body)
		var parsed struct {
			ChatIDs []string `json:"chat_ids"`
		}
		_ = json.Unmarshal(body, &parsed)
		items := make([]any, 0, len(parsed.ChatIDs))
		for _, id := range parsed.ChatIDs {
			items = append(items, map[string]any{"chat_id": id, "name": "group-" + id})
		}
		return shortcutJSONResponse(200, map[string]any{
			"code": 0,
			"data": map[string]any{"items": items},
		}), nil
	}))
	setRuntimeScopes(t, rt, chatBatchQueryScope)

	ids := make([]string, 120) // 50 + 50 + 20 → 3 batches
	for i := range ids {
		ids[i] = fmt.Sprintf("oc_%d", i)
	}
	got, err := resolveChatDetail(rt, ids)
	if err != nil {
		t.Fatalf("resolveChatDetail() error = %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (120 ids / 50 batch size)", calls)
	}
	if len(got) != 120 {
		t.Fatalf("resolved size = %d, want 120", len(got))
	}
	first := got["oc_0"]
	last := got["oc_119"]
	if first == nil || last == nil {
		t.Fatalf("Items missing boundary entries: first=%v last=%v", first, last)
	}
	if first["name"] != "group-oc_0" || last["name"] != "group-oc_119" {
		t.Fatalf("expected name passthrough; got first=%v last=%v", first["name"], last["name"])
	}
}

func TestResolveChatDetailIncludesP2PChats(t *testing.T) {
	// Unlike the old title-only resolver, the detail resolver keeps p2p chats
	// in the result map (their full object carries chat_mode/p2p_target_id);
	// only `name` is empty. Locks in that the empty-name skip was removed
	// when we switched from `title` (string) to `detail` (full object).
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return shortcutJSONResponse(200, map[string]any{
			"code": 0,
			"data": map[string]any{
				"items": []any{
					map[string]any{"chat_id": "oc_group", "name": "Engineering", "chat_mode": "group"},
					map[string]any{"chat_id": "oc_p2p", "name": "", "chat_mode": "p2p", "p2p_target_id": "ou_x"},
				},
			},
		}), nil
	}))
	setRuntimeScopes(t, rt, chatBatchQueryScope)

	got, err := resolveChatDetail(rt, []string{"oc_group", "oc_p2p"})
	if err != nil {
		t.Fatalf("resolveChatDetail() error = %v", err)
	}
	if got["oc_group"]["name"] != "Engineering" {
		t.Fatalf("oc_group name = %v, want Engineering", got["oc_group"]["name"])
	}
	p2p, ok := got["oc_p2p"]
	if !ok {
		t.Fatalf("oc_p2p must be in Items even though name is empty (caller decides what to show)")
	}
	if p2p["chat_mode"] != "p2p" || p2p["p2p_target_id"] != "ou_x" {
		t.Fatalf("p2p detail = %v, want chat_mode=p2p with p2p_target_id passthrough", p2p)
	}
}

func TestResolveChatDetailDropsItemsWithoutChatID(t *testing.T) {
	// Defensive: the server should always echo chat_id back, but if it ever
	// returns an item missing chat_id we must not write a "" → object entry
	// into the map and end up attaching nonsense to entries.
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return shortcutJSONResponse(200, map[string]any{
			"code": 0,
			"data": map[string]any{
				"items": []any{
					map[string]any{"chat_id": "oc_ok", "name": "ok"},
					map[string]any{"name": "no chat_id"},
				},
			},
		}), nil
	}))
	setRuntimeScopes(t, rt, chatBatchQueryScope)

	got, err := resolveChatDetail(rt, []string{"oc_ok"})
	if err != nil {
		t.Fatalf("resolveChatDetail() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("resolved size = %d, want 1 (entry without chat_id must be dropped)", len(got))
	}
	if _, ok := got[""]; ok {
		t.Fatalf("got[\"\"] must not exist; got %v", got[""])
	}
}

func TestResolveChatDetailPropagatesScopeError(t *testing.T) {
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("resolver should fail scope pre-flight before calling API: %s", req.URL.Path)
		return nil, nil
	}))
	// Token resolves with a known-but-wrong scope set so the missing-scope
	// branch (not the unknown-metadata warning branch) fires.
	setRuntimeScopes(t, rt, "search:message")

	_, err := resolveChatDetail(rt, []string{"oc_abc"})
	if err == nil {
		t.Fatalf("resolveChatDetail() expected scope error, got nil")
	}
	if !strings.Contains(err.Error(), chatBatchQueryScope) {
		t.Fatalf("resolveChatDetail() error = %v, want mention of %s", err, chatBatchQueryScope)
	}
}

func TestEnrichFeedShortcutDetailAttachesAndDedupes(t *testing.T) {
	var calls int
	var capturedIDs []string
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/open-apis/im/v1/chats/batch_query") {
			return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
		}
		calls++
		body, _ := io.ReadAll(req.Body)
		var parsed struct {
			ChatIDs []string `json:"chat_ids"`
		}
		_ = json.Unmarshal(body, &parsed)
		capturedIDs = append(capturedIDs, parsed.ChatIDs...)
		items := make([]any, 0, len(parsed.ChatIDs))
		for _, id := range parsed.ChatIDs {
			items = append(items, map[string]any{
				"chat_id":   id,
				"name":      "name-of-" + id,
				"chat_mode": "group",
			})
		}
		return shortcutJSONResponse(200, map[string]any{
			"code": 0,
			"data": map[string]any{"items": items},
		}), nil
	}))
	setRuntimeScopes(t, rt, chatBatchQueryScope)

	data := map[string]any{
		"shortcuts": []any{
			map[string]any{"feed_card_id": "oc_a", "type": float64(1)},
			map[string]any{"feed_card_id": "oc_b", "type": float64(1)},
			map[string]any{"feed_card_id": "oc_a", "type": float64(1)}, // duplicate
			// Unknown type — must be skipped without aborting the whole call.
			map[string]any{"feed_card_id": "doc_xxx", "type": float64(3)},
		},
	}
	if err := enrichFeedShortcutDetail(rt, data); err != nil {
		t.Fatalf("enrichFeedShortcutDetail() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (single batch covers all CHAT ids)", calls)
	}
	if len(capturedIDs) != 2 {
		t.Fatalf("server saw chat_ids = %v, want 2 dedup'd ids", capturedIDs)
	}

	items := data["shortcuts"].([]any)
	for _, ix := range []int{0, 1, 2} { // 2 is the duplicate of 0
		detail, ok := items[ix].(map[string]any)["detail"].(map[string]any)
		if !ok {
			t.Fatalf("item[%d] missing detail field; got %v", ix, items[ix])
		}
		// The full chat object is passed through verbatim — not just a name.
		if detail["chat_mode"] != "group" {
			t.Fatalf("item[%d] detail.chat_mode = %v, want group (full object passthrough)", ix, detail["chat_mode"])
		}
		wantName := "name-of-" + items[ix].(map[string]any)["feed_card_id"].(string)
		if detail["name"] != wantName {
			t.Fatalf("item[%d] detail.name = %v, want %q", ix, detail["name"], wantName)
		}
	}
	if _, ok := items[3].(map[string]any)["detail"]; ok {
		t.Fatalf("item[3] (unknown type) should not have detail set")
	}
}

func TestEnrichFeedShortcutDetailNoOpWhenEmpty(t *testing.T) {
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("must not call API for empty list: %s", req.URL.Path)
		return nil, nil
	}))
	if err := enrichFeedShortcutDetail(rt, map[string]any{}); err != nil {
		t.Fatalf("enrichFeedShortcutDetail(empty data) error = %v", err)
	}
	if err := enrichFeedShortcutDetail(rt, map[string]any{"shortcuts": []any{}}); err != nil {
		t.Fatalf("enrichFeedShortcutDetail(empty shortcuts) error = %v", err)
	}
}

func TestEnrichFeedShortcutDetailSkipsWhenNoSupportedType(t *testing.T) {
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("must not call batch_query when no resolvable types: %s", req.URL.Path)
		return nil, nil
	}))
	data := map[string]any{
		"shortcuts": []any{
			map[string]any{"feed_card_id": "doc_1", "type": float64(3)},  // DOC, not exposed
			map[string]any{"feed_card_id": "app_1", "type": float64(4)},  // OPENAPP, not exposed
			map[string]any{"feed_card_id": "biz_1", "type": float64(13)}, // APP_FEED, not exposed
		},
	}
	if err := enrichFeedShortcutDetail(rt, data); err != nil {
		t.Fatalf("enrichFeedShortcutDetail() error = %v", err)
	}
	for i, it := range data["shortcuts"].([]any) {
		if _, ok := it.(map[string]any)["detail"]; ok {
			t.Fatalf("item[%d] should not have a detail (unknown type)", i)
		}
	}
}

func TestImFeedShortcutListExecuteEnrichesDetailByDefault(t *testing.T) {
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v2/feed_shortcuts"):
			return shortcutJSONResponse(200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"shortcuts": []any{
						map[string]any{"feed_card_id": "oc_a", "type": float64(1)},
					},
					"has_more":   false,
					"page_token": "",
				},
			}), nil
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/chats/batch_query"):
			return shortcutJSONResponse(200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"items": []any{
						map[string]any{
							"chat_id":   "oc_a",
							"name":      "Team Alpha",
							"chat_mode": "group",
						},
					},
				},
			}), nil
		}
		return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
	}))
	setRuntimeScopes(t, rt, feedShortcutReadScope+" "+chatBatchQueryScope)

	cmd := newFeedShortcutListCmd(t)
	if err := cmd.Flags().Set("no-detail", "false"); err != nil {
		t.Fatalf("Set no-detail error = %v", err)
	}
	setRuntimeField(t, rt, "Cmd", cmd)

	if err := ImFeedShortcutList.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	out := rt.Factory.IOStreams.Out.(interface{ String() string }).String()
	// Verify both the attach-field name and the full-object passthrough,
	// so future regressions that drop fields (e.g. only keeping `name`)
	// fail loudly here.
	for _, want := range []string{
		`"detail":`,
		`"chat_mode": "group"`,
		`"name": "Team Alpha"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q, got:\n%s", want, out)
		}
	}
}

func TestImFeedShortcutListExecuteWarnsOnEnrichFailure(t *testing.T) {
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v2/feed_shortcuts"):
			return shortcutJSONResponse(200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"shortcuts": []any{
						map[string]any{"feed_card_id": "oc_a", "type": float64(1)},
					},
					"has_more":   false,
					"page_token": "",
				},
			}), nil
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/chats/batch_query"):
			return nil, fmt.Errorf("batch_query network failure")
		}
		return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
	}))
	setRuntimeScopes(t, rt, feedShortcutReadScope+" "+chatBatchQueryScope)

	cmd := newFeedShortcutListCmd(t)
	if err := cmd.Flags().Set("no-detail", "false"); err != nil {
		t.Fatalf("Set no-detail error = %v", err)
	}
	setRuntimeField(t, rt, "Cmd", cmd)

	// Listing should still succeed even when enrichment can't reach the API —
	// failure becomes a stderr warning, not a hard exit.
	if err := ImFeedShortcutList.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	stderr := rt.Factory.IOStreams.ErrOut.(interface{ String() string }).String()
	if !strings.Contains(stderr, "detail enrichment failed") {
		t.Fatalf("stderr = %q, want enrichment warning", stderr)
	}
	// And the shortcut itself still appears, just without `detail`.
	stdout := rt.Factory.IOStreams.Out.(interface{ String() string }).String()
	if !strings.Contains(stdout, `"feed_card_id": "oc_a"`) {
		t.Fatalf("stdout should still contain the bare shortcut entry; got:\n%s", stdout)
	}
	if strings.Contains(stdout, `"detail"`) {
		t.Fatalf("stdout should NOT contain detail when enrichment failed; got:\n%s", stdout)
	}
	// The degradation is mirrored as a machine-readable data field so
	// stdout-only consumers can tell "skipped" from "nothing to enrich".
	if !strings.Contains(stdout, `"_notice": "detail enrichment skipped`) {
		t.Fatalf("stdout should carry the _notice degradation marker; got:\n%s", stdout)
	}
}
