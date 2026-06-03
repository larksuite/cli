// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestTriageQueryFilterFieldsIncludesSearchFields(t *testing.T) {
	filter := triageFilter{
		From:          []string{"alice@example.com"},
		To:            []string{"team@example.com"},
		CC:            []string{"cc@example.com"},
		BCC:           []string{"bcc@example.com"},
		Subject:       "合同审批",
		HasAttachment: boolPtr(true),
		IsUnread:      boolPtr(true),
		TimeRange:     &triageTimeRange{StartTime: "2026-03-01T00:00:00+08:00"},
	}

	got := triageQueryFilterFields(filter)
	// is_unread is handled by buildListParams (only_unread param), not a search-path trigger
	want := []string{"bcc", "cc", "from", "has_attachment", "subject", "time_range", "to"}
	if len(got) != len(want) {
		t.Fatalf("field count mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("field[%d] mismatch\nwant: %#v\ngot:  %#v", i, want, got)
		}
	}
}

func TestBuildSearchParamsDryRunConvertsSystemFolderAndLabel(t *testing.T) {
	// When both folder and system label are specified, system label takes priority
	// (converted to folder value "flagged"), original folder_id is dropped.
	runtime := runtimeForMailTriageTest(t, map[string]string{
		"query":  "合同审批",
		"filter": `{"folder_id":"INBOX","label_id":"FLAGGED","subject":"合同审批","is_unread":true}`,
	})
	filter, err := parseTriageFilter(runtime.Str("filter"))
	if err != nil {
		t.Fatalf("parse filter failed: %v", err)
	}

	resolvedFilter, err := resolveSearchFilter(runtime, "me", filter, true)
	if err != nil {
		t.Fatalf("resolveSearchFilter failed: %v", err)
	}
	params, body, err := buildSearchParams(runtime, "me", runtime.Str("query"), resolvedFilter, 15, "", true)
	if err != nil {
		t.Fatalf("buildSearchParams failed: %v", err)
	}

	if got := params["page_size"]; got != 15 {
		t.Fatalf("page_size mismatch, got %#v", got)
	}
	if got := body["query"]; got != "合同审批" {
		t.Fatalf("query mismatch, got %#v", got)
	}

	filterBody, ok := body["filter"].(map[string]interface{})
	if !ok {
		t.Fatalf("filter body missing, got %#v", body["filter"])
	}
	// System label FLAGGED is converted to folder="flagged" in search API.
	if got := firstString(filterBody["folder"]); got != "flagged" {
		t.Fatalf("folder mismatch, got %#v", filterBody["folder"])
	}
	if filterBody["label"] != nil {
		t.Fatalf("expected label to be absent (system label moved to folder), got %#v", filterBody["label"])
	}
	if got := filterBody["subject"]; got != "合同审批" {
		t.Fatalf("subject mismatch, got %#v", got)
	}
	if got, ok := filterBody["is_unread"].(bool); !ok || !got {
		t.Fatalf("is_unread mismatch, got %#v", filterBody["is_unread"])
	}
}

func TestBuildSearchParamsSystemLabelAsFolder(t *testing.T) {
	// System label alone (no folder) should be placed in the folder field.
	runtime := runtimeForMailTriageTest(t, map[string]string{
		"query":  "test",
		"filter": `{"label":"important"}`,
	})
	filter, err := parseTriageFilter(runtime.Str("filter"))
	if err != nil {
		t.Fatalf("parse filter failed: %v", err)
	}

	resolvedFilter, err := resolveSearchFilter(runtime, "me", filter, true)
	if err != nil {
		t.Fatalf("resolveSearchFilter failed: %v", err)
	}
	_, body, err := buildSearchParams(runtime, "me", runtime.Str("query"), resolvedFilter, 15, "", true)
	if err != nil {
		t.Fatalf("buildSearchParams failed: %v", err)
	}

	filterBody, ok := body["filter"].(map[string]interface{})
	if !ok {
		t.Fatalf("filter body missing, got %#v", body["filter"])
	}
	if got := firstString(filterBody["folder"]); got != "priority" {
		t.Fatalf("expected folder='priority' (system label as folder), got %#v", filterBody["folder"])
	}
	if filterBody["label"] != nil {
		t.Fatalf("expected label to be absent, got %#v", filterBody["label"])
	}
}

func TestMailTriageRejectsDangerousQueryWithTypedValidation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailTriage, []string{
		"+triage", "--as", "user", "--query", "bad\x01",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected dangerous --query to return an error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryValidation {
		t.Errorf("category = %q, want %q", p.Category, errs.CategoryValidation)
	}
	if p.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidArgument)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if validationErr.Param != "--query" {
		t.Errorf("param = %q, want --query", validationErr.Param)
	}
	if !strings.Contains(p.Message, "control character") {
		t.Errorf("message should mention control character, got: %s", p.Message)
	}
}

func TestSystemLabelViaFolderField(t *testing.T) {
	// System label passed via folder field should also be converted to search folder value.
	runtime := runtimeForMailTriageTest(t, map[string]string{
		"query":  "test",
		"filter": `{"folder":"flagged"}`,
	})
	filter, err := parseTriageFilter(runtime.Str("filter"))
	if err != nil {
		t.Fatalf("parse filter failed: %v", err)
	}
	if !usesTriageSearchPath(runtime.Str("query"), filter) {
		t.Fatalf("expected search path for folder=flagged")
	}
	resolvedFilter, err := resolveSearchFilter(runtime, "me", filter, true)
	if err != nil {
		t.Fatalf("resolveSearchFilter failed: %v", err)
	}
	_, body, err := buildSearchParams(runtime, "me", runtime.Str("query"), resolvedFilter, 15, "", true)
	if err != nil {
		t.Fatalf("buildSearchParams failed: %v", err)
	}
	filterBody, _ := body["filter"].(map[string]interface{})
	if got := firstString(filterBody["folder"]); got != "flagged" {
		t.Fatalf("expected folder='flagged', got %#v", filterBody["folder"])
	}
}

func TestSystemLabelChineseAlias(t *testing.T) {
	// Chinese aliases should resolve to system labels.
	cases := []struct {
		input    string
		expected string
	}{
		{`{"label":"重要邮件"}`, "priority"},
		{`{"folder":"已加旗标"}`, "flagged"},
		{`{"label":"其他邮件"}`, "other"},
	}
	for _, tc := range cases {
		runtime := runtimeForMailTriageTest(t, map[string]string{
			"query":  "test",
			"filter": tc.input,
		})
		filter, err := parseTriageFilter(runtime.Str("filter"))
		if err != nil {
			t.Fatalf("parse filter %s failed: %v", tc.input, err)
		}
		resolvedFilter, err := resolveSearchFilter(runtime, "me", filter, true)
		if err != nil {
			t.Fatalf("resolveSearchFilter %s failed: %v", tc.input, err)
		}
		_, body, err := buildSearchParams(runtime, "me", runtime.Str("query"), resolvedFilter, 15, "", true)
		if err != nil {
			t.Fatalf("buildSearchParams %s failed: %v", tc.input, err)
		}
		filterBody, _ := body["filter"].(map[string]interface{})
		if got := firstString(filterBody["folder"]); got != tc.expected {
			t.Fatalf("input %s: expected folder=%q, got %#v", tc.input, tc.expected, filterBody["folder"])
		}
	}
}

func TestParseTriageFilterUnknownFieldHintUnread(t *testing.T) {
	_, err := parseTriageFilter(`{"unread":true}`)
	if err == nil {
		t.Fatalf("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), `did you mean "is_unread"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildSearchParamsDoesNotSetUserMailboxIDInBody(t *testing.T) {
	runtime := runtimeForMailTriageTest(t, map[string]string{"query": "hello"})
	params, body, err := buildSearchParams(runtime, "", runtime.Str("query"), triageFilter{}, 15, "", true)
	if err != nil {
		t.Fatalf("buildSearchParams failed: %v", err)
	}
	if got := params["page_size"]; got != 15 {
		t.Fatalf("page_size mismatch, got %#v", got)
	}
	if _, ok := body["user_mailbox_id"]; ok {
		t.Fatalf("user_mailbox_id should not be included in request body for user_mailbox.search")
	}
}

func TestMailTriageDryRunQueryWithoutLabelsUsesSearchOnly(t *testing.T) {
	runtime := runtimeForMailTriageTest(t, map[string]string{
		"query": "合同审批",
	})

	apis := dryRunAPIsForMailTriageTest(t, MailTriage.DryRun(context.Background(), runtime))
	if len(apis) != 1 {
		t.Fatalf("expected 1 dry-run api, got %d", len(apis))
	}
	if apis[0].URL != mailboxPath("me", "search") {
		t.Fatalf("unexpected url: %s", apis[0].URL)
	}
	if apis[0].Method != "POST" {
		t.Fatalf("unexpected method: %s", apis[0].Method)
	}
}

func TestMailTriageDryRunQueryWithLabelsAddsBatchGet(t *testing.T) {
	runtime := runtimeForMailTriageTest(t, map[string]string{
		"query":  "合同审批",
		"labels": "true",
	})

	apis := dryRunAPIsForMailTriageTest(t, MailTriage.DryRun(context.Background(), runtime))
	if len(apis) != 2 {
		t.Fatalf("expected 2 dry-run apis, got %d", len(apis))
	}
	if apis[0].URL != mailboxPath("me", "search") {
		t.Fatalf("search url mismatch, got %s", apis[0].URL)
	}
	if apis[1].URL != mailboxPath("me", "messages", "batch_get") {
		t.Fatalf("batch_get url mismatch, got %s", apis[1].URL)
	}
}

func TestMailTriageDryRunListPathUsesMessagesAndBatchGet(t *testing.T) {
	runtime := runtimeForMailTriageTest(t, map[string]string{
		"filter": `{"folder_id":"INBOX"}`,
	})

	apis := dryRunAPIsForMailTriageTest(t, MailTriage.DryRun(context.Background(), runtime))
	if len(apis) != 2 {
		t.Fatalf("expected 2 dry-run apis, got %d", len(apis))
	}
	if apis[0].URL != mailboxPath("me", "messages") {
		t.Fatalf("messages url mismatch, got %s", apis[0].URL)
	}
	if apis[1].URL != mailboxPath("me", "messages", "batch_get") {
		t.Fatalf("batch_get url mismatch, got %s", apis[1].URL)
	}
}

func TestMailTriageDryRunListPathCapsPageSizeAtAPILimit(t *testing.T) {
	runtime := runtimeForMailTriageTest(t, map[string]string{
		"max":    "50",
		"filter": `{"folder_id":"INBOX"}`,
	})

	apis := dryRunAPIsForMailTriageTest(t, MailTriage.DryRun(context.Background(), runtime))
	if len(apis) < 1 {
		t.Fatalf("expected at least 1 dry-run api, got %d", len(apis))
	}
	got, ok := apis[0].Params["page_size"].(float64)
	if !ok {
		t.Fatalf("page_size type mismatch, got %#v", apis[0].Params["page_size"])
	}
	if int(got) != 20 {
		t.Fatalf("page_size should be capped at 20, got %#v", got)
	}
}

func TestBuildTriageMessagesFromSearchItems(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{
			"id": "search_index_id_ignored",
			"meta_data": map[string]interface{}{
				"message_biz_id": "biz_msg_123",
				"title":          "合同审批",
				"thread_id":      "thread_1",
				"create_time":    "2026-03-21T10:00:00+08:00",
				"from": map[string]interface{}{
					"name":         "Alice",
					"mail_address": "alice@example.com",
				},
			},
		},
	}

	got := buildTriageMessagesFromSearchItems(raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	if got[0]["message_id"] != "biz_msg_123" {
		t.Fatalf("message_id mismatch, got %#v", got[0]["message_id"])
	}
	if got[0]["subject"] != "合同审批" {
		t.Fatalf("subject mismatch, got %#v", got[0]["subject"])
	}
	if got[0]["thread_id"] != "thread_1" {
		t.Fatalf("thread_id mismatch, got %#v", got[0]["thread_id"])
	}
	if got[0]["date"] != "2026-03-21T10:00:00+08:00" {
		t.Fatalf("date mismatch, got %#v", got[0]["date"])
	}
	if got[0]["from"] != "Alice <alice@example.com>" {
		t.Fatalf("from mismatch, got %#v", got[0]["from"])
	}
	if got[0]["labels"] != "" {
		t.Fatalf("labels should default to empty string, got %#v", got[0]["labels"])
	}
}

type triageDryRunPayload struct {
	API []struct {
		Method string                 `json:"method"`
		URL    string                 `json:"url"`
		Params map[string]interface{} `json:"params,omitempty"`
		Body   interface{}            `json:"body"`
	} `json:"api"`
}

func runtimeForMailTriageTest(t *testing.T, values map[string]string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	for _, fl := range MailTriage.Flags {
		switch fl.Type {
		case "bool":
			cmd.Flags().Bool(fl.Name, fl.Default == "true", "")
		case "int":
			cmd.Flags().Int(fl.Name, 0, "")
		default:
			cmd.Flags().String(fl.Name, fl.Default, "")
		}
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("parse flags failed: %v", err)
	}
	for k, v := range values {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set flag --%s failed: %v", k, err)
		}
	}
	return &common.RuntimeContext{Cmd: cmd}
}

func dryRunAPIsForMailTriageTest(t *testing.T, dry *common.DryRunAPI) []struct {
	Method string                 `json:"method"`
	URL    string                 `json:"url"`
	Params map[string]interface{} `json:"params,omitempty"`
	Body   interface{}            `json:"body"`
} {
	t.Helper()
	var payload triageDryRunPayload
	b, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry-run failed: %v", err)
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("unmarshal dry-run failed: %v\njson=%s", err, string(b))
	}
	return payload.API
}

func firstString(v interface{}) string {
	if items, ok := v.([]string); ok {
		if len(items) == 0 {
			return ""
		}
		return items[0]
	}
	items, _ := v.([]interface{})
	if len(items) == 0 {
		return ""
	}
	s, _ := items[0].(string)
	return s
}

func TestBuildTriageMessageMetaOmitsAbsentBodyFields(t *testing.T) {
	msg := map[string]interface{}{
		"message_id": "msg_456",
		"subject":    "No body",
	}
	got := buildTriageMessageMeta(msg, "msg_456")
	if _, ok := got["body_html"]; ok {
		t.Fatalf("body_html should be absent when not in API response")
	}
	if _, ok := got["body_plain_text"]; ok {
		t.Fatalf("body_plain_text should be absent when not in API response")
	}
}

func TestBuildTriageMessagesFromSearchItemsDecodesBodyFields(t *testing.T) {
	htmlEncoded := base64.URLEncoding.EncodeToString([]byte("<h1>Report</h1>"))
	plainEncoded := base64.URLEncoding.EncodeToString([]byte("Report plain"))

	raw := []interface{}{
		map[string]interface{}{
			"meta_data": map[string]interface{}{
				"message_biz_id":  "biz_msg_789",
				"title":           "Report",
				"body_html":       htmlEncoded,
				"body_plain_text": plainEncoded,
			},
		},
	}

	got := buildTriageMessagesFromSearchItems(raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	if got[0]["body_html"] != "<h1>Report</h1>" {
		t.Fatalf("body_html not decoded: %#v", got[0]["body_html"])
	}
	if got[0]["body_plain_text"] != "Report plain" {
		t.Fatalf("body_plain_text not decoded: %#v", got[0]["body_plain_text"])
	}
}

// --- usesTriageSearchPath ---

func TestUsesTriageSearchPathWithQuery(t *testing.T) {
	if !usesTriageSearchPath("hello", triageFilter{}) {
		t.Fatal("expected search path when query is set")
	}
}

func TestUsesTriageSearchPathWithSearchFields(t *testing.T) {
	cases := []triageFilter{
		{From: []string{"a@b.com"}},
		{To: []string{"a@b.com"}},
		{CC: []string{"a@b.com"}},
		{BCC: []string{"a@b.com"}},
		{Subject: "test"},
		{HasAttachment: boolPtr(true)},
		{TimeRange: &triageTimeRange{StartTime: "2026-01-01T00:00:00+08:00"}},
	}
	for _, f := range cases {
		if !usesTriageSearchPath("", f) {
			t.Fatalf("expected search path for filter %+v", f)
		}
	}
}

func TestUsesTriageSearchPathSystemLabelViaFolder(t *testing.T) {
	cases := []string{"flagged", "priority", "other", "FLAGGED", "已加旗标", "重要邮件", "其他邮件"}
	for _, v := range cases {
		if !usesTriageSearchPath("", triageFilter{Folder: v}) {
			t.Fatalf("expected search path for folder=%q", v)
		}
	}
}

func TestUsesTriageSearchPathSystemLabelViaLabel(t *testing.T) {
	cases := []string{"important", "IMPORTANT", "flagged", "other", "priority"}
	for _, v := range cases {
		if !usesTriageSearchPath("", triageFilter{Label: v}) {
			t.Fatalf("expected search path for label=%q", v)
		}
	}
}

func TestUsesTriageSearchPathSystemLabelViaLabelID(t *testing.T) {
	for _, v := range []string{"FLAGGED", "IMPORTANT", "OTHER"} {
		if !usesTriageSearchPath("", triageFilter{LabelID: v}) {
			t.Fatalf("expected search path for label_id=%q", v)
		}
	}
}

func TestUsesTriageSearchPathScheduled(t *testing.T) {
	if !usesTriageSearchPath("", triageFilter{Folder: "scheduled"}) {
		t.Fatal("expected search path for folder=scheduled")
	}
}

func TestUsesTriageSearchPathListPath(t *testing.T) {
	// Plain folder/label without system labels → list path.
	cases := []triageFilter{
		{Folder: "inbox"},
		{FolderID: "INBOX"},
		{Label: "custom-label"},
		{LabelID: "12345"},
		{},
	}
	for _, f := range cases {
		if usesTriageSearchPath("", f) {
			t.Fatalf("expected list path for filter %+v", f)
		}
	}
}

// --- resolveSystemLabel ---

func TestResolveSystemLabelAliases(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"important", "IMPORTANT"},
		{"IMPORTANT", "IMPORTANT"},
		{"priority", "IMPORTANT"},
		{"重要邮件", "IMPORTANT"},
		{"flagged", "FLAGGED"},
		{"FLAGGED", "FLAGGED"},
		{"已加旗标", "FLAGGED"},
		{"other", "OTHER"},
		{"OTHER", "OTHER"},
		{"其他邮件", "OTHER"},
	}
	for _, tc := range cases {
		got, ok := resolveSystemLabel(tc.input)
		if !ok {
			t.Fatalf("resolveSystemLabel(%q) returned false", tc.input)
		}
		if got != tc.want {
			t.Fatalf("resolveSystemLabel(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestResolveSystemLabelNotSystemLabel(t *testing.T) {
	for _, v := range []string{"inbox", "custom", "INBOX", "", "  "} {
		if _, ok := resolveSystemLabel(v); ok {
			t.Fatalf("resolveSystemLabel(%q) should return false", v)
		}
	}
}

// --- resolveListFilter (dry-run) ---

func TestResolveListFilterDryRunFolderSystemAlias(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{Folder: "inbox"}
	got, err := resolveListFilter(rt, "me", f, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.FolderID != "INBOX" {
		t.Fatalf("expected FolderID=INBOX, got %q", got.FolderID)
	}
	if got.Folder != "" {
		t.Fatalf("expected Folder cleared, got %q", got.Folder)
	}
}

func TestResolveListFilterDryRunFolderID(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{FolderID: "SENT"}
	got, err := resolveListFilter(rt, "me", f, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.FolderID != "SENT" {
		t.Fatalf("expected FolderID=SENT, got %q", got.FolderID)
	}
}

func TestResolveListFilterDryRunLabelSystemAlias(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{Label: "flagged"}
	got, err := resolveListFilter(rt, "me", f, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.LabelID != "FLAGGED" {
		t.Fatalf("expected LabelID=FLAGGED, got %q", got.LabelID)
	}
	if got.Label != "" {
		t.Fatalf("expected Label cleared, got %q", got.Label)
	}
}

func TestResolveListFilterDryRunLabelID(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{LabelID: "IMPORTANT"}
	got, err := resolveListFilter(rt, "me", f, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.LabelID != "IMPORTANT" {
		t.Fatalf("expected LabelID=IMPORTANT, got %q", got.LabelID)
	}
}

func TestResolveListFilterDryRunCustomFolderID(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{FolderID: "754000000000093"}
	got, err := resolveListFilter(rt, "me", f, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.FolderID != "754000000000093" {
		t.Fatalf("expected custom FolderID preserved, got %q", got.FolderID)
	}
}

// --- buildSearchCreateTime ---

func TestBuildSearchCreateTimeNil(t *testing.T) {
	got := buildSearchCreateTime(nil)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestBuildSearchCreateTimeBoth(t *testing.T) {
	got := buildSearchCreateTime(&triageTimeRange{
		StartTime: "2026-01-01T00:00:00+08:00",
		EndTime:   "2026-12-31T23:59:59+08:00",
	})
	if got["start_time"] != "2026-01-01T00:00:00+08:00" {
		t.Fatalf("start_time mismatch: %v", got)
	}
	if got["end_time"] != "2026-12-31T23:59:59+08:00" {
		t.Fatalf("end_time mismatch: %v", got)
	}
}

func TestBuildSearchCreateTimeStartOnly(t *testing.T) {
	got := buildSearchCreateTime(&triageTimeRange{StartTime: "2026-01-01T00:00:00+08:00"})
	if got["start_time"] != "2026-01-01T00:00:00+08:00" {
		t.Fatalf("start_time mismatch: %v", got)
	}
	if _, ok := got["end_time"]; ok {
		t.Fatalf("end_time should be absent")
	}
}

func TestBuildSearchCreateTimeEmpty(t *testing.T) {
	got := buildSearchCreateTime(&triageTimeRange{})
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}

// --- normalizeTriageMax ---

func TestNormalizeTriageMax(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 20}, {-1, 20}, {1, 1}, {20, 20}, {400, 400}, {401, 400}, {999, 400},
	}
	for _, tc := range cases {
		if got := normalizeTriageMax(tc.in); got != tc.want {
			t.Fatalf("normalizeTriageMax(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// --- trimStringList ---

func TestTrimStringList(t *testing.T) {
	got := trimStringList([]string{"  alice@b.com ", "", " bob@b.com", "  "})
	if len(got) != 2 || got[0] != "alice@b.com" || got[1] != "bob@b.com" {
		t.Fatalf("unexpected result: %v", got)
	}
}

func TestTrimStringListEmpty(t *testing.T) {
	got := trimStringList([]string{"", "  "})
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestTrimStringListNil(t *testing.T) {
	got := trimStringList(nil)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

// --- formatAddress ---

func TestFormatAddressNameAndEmail(t *testing.T) {
	got := formatAddress(map[string]interface{}{"name": "Alice", "mail_address": "alice@a.com"})
	if got != "Alice <alice@a.com>" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatAddressEmailOnly(t *testing.T) {
	got := formatAddress(map[string]interface{}{"mail_address": "alice@a.com"})
	if got != "alice@a.com" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatAddressNameOnly(t *testing.T) {
	got := formatAddress(map[string]interface{}{"name": "Alice"})
	if got != "Alice" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatAddressFallbackToAddress(t *testing.T) {
	got := formatAddress(map[string]interface{}{"address": "bob@b.com"})
	if got != "bob@b.com" {
		t.Fatalf("got %q", got)
	}
}

func TestShouldRetryTriageAPIError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "rate limit",
			err:  errs.NewAPIError(errs.SubtypeRateLimit, "too many requests"),
			want: true,
		},
		{
			name: "network",
			err:  errs.NewNetworkError(errs.SubtypeNetworkTransport, "dial timeout"),
			want: true,
		},
		{
			name: "validation",
			err:  errs.NewValidationError(errs.SubtypeInvalidArgument, "bad query"),
			want: false,
		},
		{
			name: "plain",
			err:  assertErr("legacy plain error"),
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRetryTriageAPIError(tc.err); got != tc.want {
				t.Fatalf("shouldRetryTriageAPIError() = %v, want %v", got, tc.want)
			}
		})
	}
}

// --- extractTriageMessageIDs ---

func TestExtractTriageMessageIDsStringItems(t *testing.T) {
	raw := []interface{}{"msg_1", "msg_2", ""}
	got := extractTriageMessageIDs(raw)
	if len(got) != 2 || got[0] != "msg_1" || got[1] != "msg_2" {
		t.Fatalf("unexpected: %v", got)
	}
}

func TestExtractTriageMessageIDsMapItems(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{"message_id": "msg_a"},
		map[string]interface{}{"id": "msg_b"},
		map[string]interface{}{"other": "no_id"},
	}
	got := extractTriageMessageIDs(raw)
	if len(got) != 2 || got[0] != "msg_a" || got[1] != "msg_b" {
		t.Fatalf("unexpected: %v", got)
	}
}

func TestExtractTriageMessageIDsNil(t *testing.T) {
	got := extractTriageMessageIDs(nil)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

// --- mergeTriageLabels ---

func TestMergeTriageLabels(t *testing.T) {
	messages := []map[string]interface{}{
		{"message_id": "m1", "labels": ""},
		{"message_id": "m2", "labels": ""},
		{"message_id": "m3", "labels": ""},
	}
	enriched := []map[string]interface{}{
		{"message_id": "m1", "labels": "IMPORTANT,FLAGGED"},
		{"message_id": "m3", "labels": "OTHER"},
	}
	mergeTriageLabels(messages, enriched)
	if messages[0]["labels"] != "IMPORTANT,FLAGGED" {
		t.Fatalf("m1 labels mismatch: %v", messages[0]["labels"])
	}
	if messages[1]["labels"] != "" {
		t.Fatalf("m2 labels should remain empty: %v", messages[1]["labels"])
	}
	if messages[2]["labels"] != "OTHER" {
		t.Fatalf("m3 labels mismatch: %v", messages[2]["labels"])
	}
}

// --- printTriageFilterSchema ---

func TestPrintTriageFilterSchema(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	var buf strings.Builder
	rt.Factory = &cmdutil.Factory{IOStreams: &cmdutil.IOStreams{Out: &buf, ErrOut: &buf}}
	printTriageFilterSchema(rt)
	if !strings.Contains(buf.String(), "folder") {
		t.Fatal("schema output should contain 'folder'")
	}
}

// --- resolveSearchFolderFilter / resolveSearchLabelFilter (dry-run) ---

func TestResolveSearchFolderFilterDryRunSystemFolder(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{Folder: "trash"}
	got, err := resolveSearchFolderFilter(rt, "me", f, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "trash" {
		t.Fatalf("expected 'trash', got %q", got)
	}
}

func TestResolveSearchFolderFilterDryRunScheduled(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{Folder: "scheduled"}
	got, err := resolveSearchFolderFilter(rt, "me", f, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "scheduled" {
		t.Fatalf("expected 'scheduled', got %q", got)
	}
}

func TestResolveSearchFolderFilterDryRunArchive(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{Folder: "archived"}
	got, err := resolveSearchFolderFilter(rt, "me", f, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "archive" {
		t.Fatalf("expected 'archive', got %q", got)
	}
}

func TestResolveSearchFolderFilterDryRunFolderID(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{FolderID: "INBOX"}
	got, err := resolveSearchFolderFilter(rt, "me", f, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "inbox" {
		t.Fatalf("expected 'inbox', got %q", got)
	}
}

func TestResolveSearchLabelFilterDryRunCustom(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{Label: "my-custom-label"}
	got, err := resolveSearchLabelFilter(rt, "me", f, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "my-custom-label" {
		t.Fatalf("expected 'my-custom-label', got %q", got)
	}
}

func TestResolveSearchLabelFilterDryRunEmpty(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{}
	got, err := resolveSearchLabelFilter(rt, "me", f, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

// --- buildListParams (dry-run) ---

func TestBuildListParamsDryRunDefaults(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{}
	got, err := buildListParams(rt, "me", f, 20, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if got["folder_id"] != "INBOX" {
		t.Fatalf("default folder_id should be INBOX, got %v", got["folder_id"])
	}
	if got["page_size"] != 20 {
		t.Fatalf("page_size mismatch: %v", got["page_size"])
	}
}

func TestBuildListParamsDryRunWithLabel(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{LabelID: "FLAGGED"}
	got, err := buildListParams(rt, "me", f, 10, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["folder_id"]; ok {
		t.Fatalf("folder_id should not be set when label is specified, got %v", got["folder_id"])
	}
	if got["label_id"] != "FLAGGED" {
		t.Fatalf("label_id mismatch: %v", got["label_id"])
	}
}

func TestBuildListParamsDryRunWithPageToken(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{}
	got, err := buildListParams(rt, "me", f, 20, "token123", true)
	if err != nil {
		t.Fatal(err)
	}
	if got["page_token"] != "token123" {
		t.Fatalf("page_token mismatch: %v", got["page_token"])
	}
}

func TestBuildListParamsDryRunOnlyUnread(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{IsUnread: boolPtr(true)}
	got, err := buildListParams(rt, "me", f, 20, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if got["only_unread"] != true {
		t.Fatalf("only_unread should be true, got %v", got["only_unread"])
	}
}

func TestBuildListParamsDryRunFolderAlias(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{Folder: "sent"}
	got, err := buildListParams(rt, "me", f, 20, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if got["folder_id"] != "SENT" {
		t.Fatalf("expected folder_id=SENT, got %v", got["folder_id"])
	}
}

func TestBuildListParamsDryRunLabelAlias(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{Label: "flagged"}
	got, err := buildListParams(rt, "me", f, 10, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if got["label_id"] != "FLAGGED" {
		t.Fatalf("expected label_id=FLAGGED, got %v", got["label_id"])
	}
}

// --- buildSearchParams additional coverage ---

func TestBuildSearchParamsAllFilterFields(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	f := triageFilter{
		Folder:        "inbox",
		From:          []string{"alice@a.com"},
		To:            []string{"bob@b.com"},
		CC:            []string{"cc@c.com"},
		BCC:           []string{"bcc@d.com"},
		Subject:       "report",
		HasAttachment: boolPtr(true),
		IsUnread:      boolPtr(false),
	}
	resolved, _ := resolveSearchFilter(rt, "me", f, true)
	_, body, err := buildSearchParams(rt, "me", "keyword", resolved, 10, "tok", true)
	if err != nil {
		t.Fatal(err)
	}
	fb, _ := body["filter"].(map[string]interface{})
	if fb["subject"] != "report" {
		t.Fatalf("subject mismatch: %v", fb["subject"])
	}
	if fb["has_attachment"] != true {
		t.Fatalf("has_attachment mismatch: %v", fb["has_attachment"])
	}
	if fb["is_unread"] != false {
		t.Fatalf("is_unread mismatch: %v", fb["is_unread"])
	}
	if body["query"] != "keyword" {
		t.Fatalf("query mismatch: %v", body["query"])
	}
}

func TestBuildSearchParamsPageToken(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil)
	params, _, _ := buildSearchParams(rt, "me", "q", triageFilter{}, 10, "next_page", true)
	if params["page_token"] != "next_page" {
		t.Fatalf("page_token mismatch: %v", params["page_token"])
	}
}

// --- resolveTriagePageSize ---

func TestResolveTriagePageSizeDefaultMax(t *testing.T) {
	rt := runtimeForMailTriageTest(t, nil) // max=0 (unset) → normalizeTriageMax returns 20
	got := resolveTriagePageSize(rt)
	if got != 20 {
		t.Fatalf("expected 20, got %d", got)
	}
}

func TestResolveTriagePageSizeFromMax(t *testing.T) {
	rt := runtimeForMailTriageTest(t, map[string]string{"max": "30"})
	got := resolveTriagePageSize(rt)
	if got != 30 {
		t.Fatalf("expected 30, got %d", got)
	}
}

func TestResolveTriagePageSizeFromPageSize(t *testing.T) {
	rt := runtimeForMailTriageTest(t, map[string]string{"page-size": "10"})
	got := resolveTriagePageSize(rt)
	if got != 10 {
		t.Fatalf("expected 10, got %d", got)
	}
}

func TestResolveTriagePageSizePageSizeOverridesMax(t *testing.T) {
	rt := runtimeForMailTriageTest(t, map[string]string{"max": "30", "page-size": "5"})
	got := resolveTriagePageSize(rt)
	if got != 5 {
		t.Fatalf("expected page-size=5 to override max=30, got %d", got)
	}
}

func TestResolveTriagePageSizeClamped(t *testing.T) {
	rt := runtimeForMailTriageTest(t, map[string]string{"page-size": "999"})
	got := resolveTriagePageSize(rt)
	if got != 400 {
		t.Fatalf("expected clamped to 400, got %d", got)
	}
}

// --- page-token path validation ---

func TestResolveTriagePathSearchTokenContinuation(t *testing.T) {
	// search: token without --query is valid (continuation)
	useSearch, err := resolveTriagePath(mustParseTriagePageToken(t, "search:abc123"), "", triageFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !useSearch {
		t.Fatal("search: prefix should select search path")
	}
}

func TestResolveTriagePathListTokenConflictsWithQuery(t *testing.T) {
	// list: token + --query → error (query would be silently ignored)
	_, err := resolveTriagePath(mustParseTriagePageToken(t, "list:abc123"), "hello", triageFilter{})
	if err == nil {
		t.Fatal("expected error for list: token with --query")
	}
}

func TestResolveTriagePathListTokenConflictsWithSearchFilter(t *testing.T) {
	// list: token + search-only filter field → error
	_, err := resolveTriagePath(mustParseTriagePageToken(t, "list:abc123"), "", triageFilter{From: []string{"a@b.com"}})
	if err == nil {
		t.Fatal("expected error for list: token with search-only filter")
	}
}

func TestResolveTriagePathListTokenWithListFilter(t *testing.T) {
	// list: token + list-compatible filter → OK
	useSearch, err := resolveTriagePath(mustParseTriagePageToken(t, "list:abc123"), "", triageFilter{Folder: "inbox"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if useSearch {
		t.Fatal("list: prefix should select list path")
	}
}

func TestResolveTriagePathBareTokenRejected(t *testing.T) {
	// Bare tokens are rejected at parse time, not at resolveTriagePath time
	_, err := parseTriagePageToken("baretoken123")
	if err == nil {
		t.Fatal("expected error for bare token without prefix")
	}
	if !strings.Contains(err.Error(), "prefix") {
		t.Fatalf("error should mention prefix, got: %v", err)
	}
}

func TestResolveTriagePathEmptyToken(t *testing.T) {
	// No token → falls back to usesTriageSearchPath
	useSearch, err := resolveTriagePath(triagePageToken{}, "hello", triageFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !useSearch {
		t.Fatal("query present → should use search path")
	}

	useSearch, err = resolveTriagePath(triagePageToken{}, "", triageFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if useSearch {
		t.Fatal("no query → should use list path")
	}
}

func TestPageTokenSearchPrefixStripped(t *testing.T) {
	raw := "search:72d98412d30aa6af"
	got := strings.TrimPrefix(raw, "search:")
	if got != "72d98412d30aa6af" {
		t.Fatalf("expected stripped token, got %q", got)
	}
}

func TestPageTokenListPrefixStripped(t *testing.T) {
	raw := "list:FfccvoqPd_loLhtcRx8cx"
	got := strings.TrimPrefix(raw, "list:")
	if got != "FfccvoqPd_loLhtcRx8cx" {
		t.Fatalf("expected stripped token, got %q", got)
	}
}

func TestPageTokenBareTokenRejected(t *testing.T) {
	_, err := parseTriagePageToken("FfccvoqPd_loLhtcRx8cx")
	if err == nil {
		t.Fatal("expected error for bare token without prefix")
	}
	if !strings.Contains(err.Error(), "prefix") {
		t.Fatalf("error should mention prefix requirement, got: %v", err)
	}
}

// --- DryRun with page-size ---

func TestMailTriageDryRunPageSizeOverridesMax(t *testing.T) {
	runtime := runtimeForMailTriageTest(t, map[string]string{
		"max":       "50",
		"page-size": "8",
		"filter":    `{"folder_id":"INBOX"}`,
	})
	apis := dryRunAPIsForMailTriageTest(t, MailTriage.DryRun(context.Background(), runtime))
	if len(apis) < 1 {
		t.Fatalf("expected at least 1 dry-run api, got %d", len(apis))
	}
	got, ok := apis[0].Params["page_size"].(float64)
	if !ok {
		t.Fatalf("page_size type mismatch, got %#v", apis[0].Params["page_size"])
	}
	if int(got) != 8 {
		t.Fatalf("expected page_size=8 (from --page-size), got %d", int(got))
	}
}

func TestMailTriageDryRunSearchPathCapsPageSizeAt15(t *testing.T) {
	runtime := runtimeForMailTriageTest(t, map[string]string{
		"query":     "hello",
		"page-size": "30",
	})
	apis := dryRunAPIsForMailTriageTest(t, MailTriage.DryRun(context.Background(), runtime))
	if len(apis) < 1 {
		t.Fatalf("expected at least 1 dry-run api, got %d", len(apis))
	}
	got, ok := apis[0].Params["page_size"].(float64)
	if !ok {
		t.Fatalf("page_size type mismatch, got %#v", apis[0].Params["page_size"])
	}
	if int(got) != searchPageMax {
		t.Fatalf("expected page_size capped at %d, got %d", searchPageMax, int(got))
	}
}

// --- DryRun with page-token ---

func TestMailTriageDryRunListPathWithPageToken(t *testing.T) {
	runtime := runtimeForMailTriageTest(t, map[string]string{
		"filter":     `{"folder_id":"INBOX"}`,
		"page-token": "list:abc123token",
	})
	apis := dryRunAPIsForMailTriageTest(t, MailTriage.DryRun(context.Background(), runtime))
	if len(apis) < 1 {
		t.Fatalf("expected at least 1 dry-run api, got %d", len(apis))
	}
	got, ok := apis[0].Params["page_token"]
	if !ok {
		t.Fatalf("expected page_token in params")
	}
	if got != "abc123token" {
		t.Fatalf("expected stripped page_token='abc123token', got %v", got)
	}
}

func TestMailTriageDryRunSearchPathWithPageToken(t *testing.T) {
	runtime := runtimeForMailTriageTest(t, map[string]string{
		"query":      "test",
		"page-token": "search:def456token",
	})
	apis := dryRunAPIsForMailTriageTest(t, MailTriage.DryRun(context.Background(), runtime))
	if len(apis) < 1 {
		t.Fatalf("expected at least 1 dry-run api, got %d", len(apis))
	}
	got, ok := apis[0].Params["page_token"]
	if !ok {
		t.Fatalf("expected page_token in params")
	}
	if got != "def456token" {
		t.Fatalf("expected stripped page_token='def456token', got %v", got)
	}
}

func TestMailTriageDryRunBarePageTokenErrors(t *testing.T) {
	runtime := runtimeForMailTriageTest(t, map[string]string{
		"filter":     `{"folder_id":"INBOX"}`,
		"page-token": "baretoken123",
	})
	dry := MailTriage.DryRun(context.Background(), runtime)
	b, _ := json.Marshal(dry)
	s := string(b)
	if !strings.Contains(s, "filter_error") {
		t.Fatalf("expected filter_error for bare token, got %s", s)
	}
}

// --- resolveTriagePath ---

func TestResolveTriagePathSearchPrefixWithoutQuery(t *testing.T) {
	useSearch, err := resolveTriagePath(mustParseTriagePageToken(t, "search:abc"), "", triageFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !useSearch {
		t.Fatal("search: prefix should select search path")
	}
}

func TestResolveTriagePathListPrefixWithoutConflict(t *testing.T) {
	useSearch, err := resolveTriagePath(mustParseTriagePageToken(t, "list:abc"), "", triageFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if useSearch {
		t.Fatal("list: prefix should select list path")
	}
}

func TestResolveTriagePathListPrefixWithQueryErrors(t *testing.T) {
	_, err := resolveTriagePath(mustParseTriagePageToken(t, "list:abc"), "hello", triageFilter{})
	if err == nil {
		t.Fatal("expected error for list: token with --query")
	}
}

func TestResolveTriagePathListPrefixWithSearchFilterErrors(t *testing.T) {
	_, err := resolveTriagePath(mustParseTriagePageToken(t, "list:abc"), "", triageFilter{Subject: "test"})
	if err == nil {
		t.Fatal("expected error for list: token with search-only filter field")
	}
}

func TestResolveTriagePathBareTokenErrors(t *testing.T) {
	_, err := parseTriagePageToken("baretoken")
	if err == nil {
		t.Fatal("expected error for bare token")
	}
}

func TestResolveTriagePathEmptyTokenFallsBack(t *testing.T) {
	useSearch, err := resolveTriagePath(triagePageToken{}, "", triageFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if useSearch {
		t.Fatal("no query → should use list path")
	}

	useSearch, err = resolveTriagePath(triagePageToken{}, "keyword", triageFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !useSearch {
		t.Fatal("query present → should use search path")
	}
}

// --- DryRun: token prefix overrides path ---

func TestMailTriageDryRunSearchTokenWithoutQueryUsesSearchPath(t *testing.T) {
	runtime := runtimeForMailTriageTest(t, map[string]string{
		"page-token": "search:abc123",
	})
	apis := dryRunAPIsForMailTriageTest(t, MailTriage.DryRun(context.Background(), runtime))
	if len(apis) < 1 {
		t.Fatalf("expected at least 1 dry-run api, got %d", len(apis))
	}
	if apis[0].URL != mailboxPath("me", "search") {
		t.Fatalf("search: prefix should force search path, got url %s", apis[0].URL)
	}
}

func TestMailTriageDryRunListTokenWithQueryErrors(t *testing.T) {
	runtime := runtimeForMailTriageTest(t, map[string]string{
		"query":      "hello",
		"page-token": "list:abc123",
	})
	dry := MailTriage.DryRun(context.Background(), runtime)
	b, _ := json.Marshal(dry)
	s := string(b)
	if !strings.Contains(s, "filter_error") {
		t.Fatalf("expected filter_error for list token with query, got %s", s)
	}
}

// --- DryRun with no page-token has no page_token param ---

func TestMailTriageDryRunNoPageTokenOmitsParam(t *testing.T) {
	runtime := runtimeForMailTriageTest(t, map[string]string{
		"filter": `{"folder_id":"INBOX"}`,
	})
	apis := dryRunAPIsForMailTriageTest(t, MailTriage.DryRun(context.Background(), runtime))
	if len(apis) < 1 {
		t.Fatalf("expected at least 1 dry-run api, got %d", len(apis))
	}
	if _, ok := apis[0].Params["page_token"]; ok {
		t.Fatalf("page_token should not be present when --page-token is empty")
	}
}

// --- Flag definition checks ---

func TestMailTriageFlagsIncludePageTokenAndPageSize(t *testing.T) {
	flagNames := make(map[string]bool)
	for _, fl := range MailTriage.Flags {
		flagNames[fl.Name] = true
	}
	for _, name := range []string{"page-token", "page-size", "max"} {
		if !flagNames[name] {
			t.Fatalf("expected flag --%s to be defined", name)
		}
	}
}

func mustParseTriagePageToken(t *testing.T, token string) triagePageToken {
	t.Helper()
	parsed, err := parseTriagePageToken(token)
	if err != nil {
		t.Fatalf("parseTriagePageToken(%q) failed: %v", token, err)
	}
	return parsed
}

// --- parseTriagePageToken / encodeTriagePageToken ---

func TestEncodeTriagePageToken(t *testing.T) {
	got := encodeTriagePageToken("search", "abc123")
	if got != "search:abc123" {
		t.Fatalf("expected search:abc123, got %q", got)
	}
}

func TestEncodeTriagePageTokenEmpty(t *testing.T) {
	got := encodeTriagePageToken("search", "")
	if got != "" {
		t.Fatalf("expected empty for empty raw token, got %q", got)
	}
}

func TestParseTriagePageTokenSearch(t *testing.T) {
	parsed, err := parseTriagePageToken("search:abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Path != "search" || parsed.RawToken != "abc123" {
		t.Fatalf("unexpected parsed: %+v", parsed)
	}
}

func TestParseTriagePageTokenList(t *testing.T) {
	parsed, err := parseTriagePageToken("list:longtoken123xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Path != "list" || parsed.RawToken != "longtoken123xyz" {
		t.Fatalf("unexpected parsed: %+v", parsed)
	}
}

func TestParseTriagePageTokenWithColonsInRawToken(t *testing.T) {
	// Raw token may contain colons
	parsed, err := parseTriagePageToken("search:abc:def:ghi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Path != "search" || parsed.RawToken != "abc:def:ghi" {
		t.Fatalf("unexpected parsed: %+v", parsed)
	}
}

func TestParseTriagePageTokenBareRejected(t *testing.T) {
	_, err := parseTriagePageToken("baretoken")
	if err == nil {
		t.Fatal("expected error for bare token")
	}
}

func TestParseTriagePageTokenEmptyRawTokenRejected(t *testing.T) {
	_, err := parseTriagePageToken("search:")
	if err == nil {
		t.Fatal("expected error for empty raw token after prefix")
	}
	_, err = parseTriagePageToken("list:")
	if err == nil {
		t.Fatal("expected error for empty raw token after prefix")
	}
}

func TestParseTriagePageTokenEmpty(t *testing.T) {
	parsed, err := parseTriagePageToken("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.RawToken != "" {
		t.Fatalf("expected empty parsed, got %+v", parsed)
	}
}

func TestParseTriagePageTokenInvalidPrefix(t *testing.T) {
	_, err := parseTriagePageToken("unknown:abc123")
	if err == nil {
		t.Fatal("expected error for unknown prefix")
	}
}

func boolPtr(v bool) *bool { return &v }

// --- mailbox_id preservation tests ---

func TestMailTriageStructuredOutputPreservesMailboxID(t *testing.T) {
	tests := []struct {
		name      string
		mailbox   string
		format    string
		args      []string
		register  func(*httpmock.Registry, string)
		wantCount int
	}{
		{
			name:    "list json default mailbox",
			mailbox: "me",
			format:  "json",
			args:    []string{"--filter", `{"folder_id":"INBOX"}`},
			register: func(reg *httpmock.Registry, mailbox string) {
				registerMailTriageListStub(reg, mailbox, []string{"msg_001", "msg_002"}, false, "")
				registerMailTriageBatchStub(reg, mailbox, []map[string]interface{}{
					mailTriageBatchMessage("msg_001", "Subject 1"),
					mailTriageBatchMessage("msg_002", "Subject 2"),
				})
			},
			wantCount: 2,
		},
		{
			name:    "list data public mailbox",
			mailbox: "shared@company.com",
			format:  "data",
			args:    []string{"--filter", `{"folder_id":"INBOX"}`},
			register: func(reg *httpmock.Registry, mailbox string) {
				registerMailTriageListStub(reg, mailbox, []string{"msg_pub_001"}, false, "")
				registerMailTriageBatchStub(reg, mailbox, []map[string]interface{}{
					mailTriageBatchMessage("msg_pub_001", "Shared mailbox message"),
				})
			},
			wantCount: 1,
		},
		{
			name:    "search json public mailbox",
			mailbox: "shared@corp.com",
			format:  "json",
			args:    []string{"--query", "shared keyword"},
			register: func(reg *httpmock.Registry, mailbox string) {
				registerMailTriageSearchStub(reg, mailbox, []interface{}{
					mailTriageSearchItem("search_pub_001", "Shared search"),
				}, false, "")
			},
			wantCount: 1,
		},
		{
			name:    "empty list json keeps top-level mailbox",
			mailbox: "me",
			format:  "json",
			args:    []string{"--filter", `{"folder_id":"INBOX"}`},
			register: func(reg *httpmock.Registry, mailbox string) {
				registerMailTriageListStub(reg, mailbox, nil, false, "")
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := mailShortcutTestFactory(t)
			defer reg.Verify(t)

			tt.register(reg, tt.mailbox)

			args := []string{"+triage", "--format", tt.format}
			if tt.mailbox != "me" {
				args = append(args, "--mailbox", tt.mailbox)
			}
			args = append(args, tt.args...)

			if err := runMountedMailShortcut(t, MailTriage, args, f, stdout); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			data := decodeMailTriageJSONOutput(t, stdout)
			if data["mailbox_id"] != tt.mailbox {
				t.Fatalf("top-level mailbox_id mismatch: got %v, want %q", data["mailbox_id"], tt.mailbox)
			}
			messages := mailTriageMessagesFromOutput(t, data)
			if len(messages) != tt.wantCount {
				t.Fatalf("message count mismatch: got %d, want %d", len(messages), tt.wantCount)
			}
			for i, msg := range messages {
				if msg["mailbox_id"] != tt.mailbox {
					t.Fatalf("message[%d] mailbox_id mismatch: got %v, want %q", i, msg["mailbox_id"], tt.mailbox)
				}
			}
		})
	}
}

func TestMailTriageMissingMessageMetadataStillGetsMailboxID(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	defer reg.Verify(t)

	registerMailTriageListStub(reg, "me", []string{"msg_ok", "msg_missing"}, false, "")
	registerMailTriageBatchStub(reg, "me", []map[string]interface{}{
		mailTriageBatchMessage("msg_ok", "Present"),
	})

	err := runMountedMailShortcut(t, MailTriage, []string{
		"+triage",
		"--format", "json",
		"--filter", `{"folder_id":"INBOX"}`,
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	messages := mailTriageMessagesFromOutput(t, decodeMailTriageJSONOutput(t, stdout))
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	for i, msg := range messages {
		if msg["mailbox_id"] != "me" {
			t.Fatalf("message[%d] mailbox_id mismatch: got %v, want me", i, msg["mailbox_id"])
		}
	}
	if messages[1]["message_id"] != "msg_missing" || messages[1]["error"] == nil {
		t.Fatalf("missing metadata placeholder mismatch: %#v", messages[1])
	}
}

func TestMailTriageTableOutputPreservesMailboxContext(t *testing.T) {
	tests := []struct {
		name              string
		mailbox           string
		hasMore           bool
		wantMailboxColumn bool
		wantMailboxHint   bool
	}{
		{name: "default mailbox", mailbox: "me"},
		{name: "public mailbox", mailbox: "shared@company.com", hasMore: true, wantMailboxColumn: true, wantMailboxHint: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, stderr, reg := mailShortcutTestFactory(t)
			defer reg.Verify(t)

			registerMailTriageListStub(reg, tt.mailbox, []string{"msg_001"}, tt.hasMore, "next_page_token")
			registerMailTriageBatchStub(reg, tt.mailbox, []map[string]interface{}{
				mailTriageBatchMessage("msg_001", "Table message"),
			})

			args := []string{"+triage", "--max", "1", "--filter", `{"folder_id":"INBOX"}`}
			if tt.mailbox != "me" {
				args = append(args, "--mailbox", tt.mailbox)
			}
			if err := runMountedMailShortcut(t, MailTriage, args, f, stdout); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			out := stdout.String()
			if got := strings.Contains(out, "mailbox_id"); got != tt.wantMailboxColumn {
				t.Fatalf("mailbox_id column presence mismatch: got %v, want %v\nstdout:\n%s", got, tt.wantMailboxColumn, out)
			}
			if tt.wantMailboxColumn && !strings.Contains(out, tt.mailbox) {
				t.Fatalf("table output should contain mailbox %q, stdout:\n%s", tt.mailbox, out)
			}

			errOut := stderr.String()
			quotedMailbox := shellQuote(tt.mailbox)
			if got := strings.Contains(errOut, "--mailbox "+quotedMailbox); got != tt.wantMailboxHint {
				t.Fatalf("mailbox hint presence mismatch: got %v, want %v\nstderr:\n%s", got, tt.wantMailboxHint, errOut)
			}
			if !strings.Contains(errOut, "mail +message") {
				t.Fatalf("stderr should contain mail +message tip, got:\n%s", errOut)
			}
		})
	}
}

func decodeMailTriageJSONOutput(t *testing.T, stdout interface{ Bytes() []byte }) map[string]interface{} {
	t.Helper()
	var data map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	return data
}

func mailTriageMessagesFromOutput(t *testing.T, data map[string]interface{}) []map[string]interface{} {
	t.Helper()
	rawMessages, ok := data["messages"].([]interface{})
	if !ok {
		t.Fatalf("messages type mismatch: %T", data["messages"])
	}
	messages := make([]map[string]interface{}, 0, len(rawMessages))
	for i, item := range rawMessages {
		msg, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("messages[%d] type mismatch: %T", i, item)
		}
		messages = append(messages, msg)
	}
	return messages
}

func registerMailTriageListStub(reg *httpmock.Registry, mailbox string, items []string, hasMore bool, pageToken string) {
	data := map[string]interface{}{
		"items":    items,
		"has_more": hasMore,
	}
	if pageToken != "" {
		data["page_token"] = pageToken
	}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    mailboxPath(mailbox, "messages") + "?",
		Body: map[string]interface{}{
			"code": 0,
			"data": data,
		},
	})
}

func registerMailTriageBatchStub(reg *httpmock.Registry, mailbox string, messages []map[string]interface{}) {
	rawMessages := make([]interface{}, 0, len(messages))
	for _, msg := range messages {
		rawMessages = append(rawMessages, msg)
	}
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    mailboxPath(mailbox, "messages", "batch_get"),
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"messages": rawMessages,
			},
		},
	})
}

func registerMailTriageSearchStub(reg *httpmock.Registry, mailbox string, items []interface{}, hasMore bool, pageToken string) {
	data := map[string]interface{}{
		"items":    items,
		"has_more": hasMore,
	}
	if pageToken != "" {
		data["page_token"] = pageToken
	}
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    mailboxPath(mailbox, "search"),
		Body: map[string]interface{}{
			"code": 0,
			"data": data,
		},
	})
}

func mailTriageBatchMessage(messageID, subject string) map[string]interface{} {
	return map[string]interface{}{
		"message_id": messageID,
		"subject":    subject,
		"head_from":  map[string]interface{}{"name": "Alice", "mail_address": "alice@example.com"},
		"folder_id":  "INBOX",
	}
}

func mailTriageSearchItem(messageID, subject string) map[string]interface{} {
	return map[string]interface{}{
		"meta_data": map[string]interface{}{
			"message_biz_id": messageID,
			"title":          subject,
			"from":           map[string]interface{}{"name": "Alice", "mail_address": "alice@example.com"},
		},
	}
}
