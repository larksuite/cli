// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// --- parseListCommentsDocRef ---

func TestParseListCommentsDocRef_DocxURL(t *testing.T) {
	t.Parallel()
	ref, ok := parseListCommentsDocRef("https://example.feishu.cn/docx/doxAbc123", "")
	if !ok {
		t.Fatal("expected ok=true for docx URL")
	}
	if ref.Type != "docx" || ref.Token != "doxAbc123" {
		t.Fatalf("got %+v", ref)
	}
}

func TestParseListCommentsDocRef_WikiURL(t *testing.T) {
	t.Parallel()
	ref, ok := parseListCommentsDocRef("https://example.feishu.cn/wiki/wikXyz789", "")
	if !ok {
		t.Fatal("expected ok=true for wiki URL")
	}
	if ref.Type != "wiki" || ref.Token != "wikXyz789" {
		t.Fatalf("got %+v", ref)
	}
}

func TestParseListCommentsDocRef_BareTokenRequiresType(t *testing.T) {
	t.Parallel()
	_, ok := parseListCommentsDocRef("doxAbc123", "")
	if ok {
		t.Fatal("expected ok=false for bare token without --type")
	}
	ref, ok := parseListCommentsDocRef("doxAbc123", "docx")
	if !ok {
		t.Fatal("expected ok=true for bare token with --type=docx")
	}
	if ref.Type != "docx" || ref.Token != "doxAbc123" {
		t.Fatalf("got %+v", ref)
	}
}

func TestParseListCommentsDocRef_RejectsUnsupportedType(t *testing.T) {
	t.Parallel()
	_, ok := parseListCommentsDocRef("token", "sheet")
	if ok {
		t.Fatal("expected ok=false for --type=sheet (not in MVP)")
	}
}

// --- validateListComments ---

func newListCommentsCmd(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "drive +list-comments"}
	cmd.Flags().String("doc", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().Bool("include-orphaned", false, "")
	cmd.Flags().Bool("include-resolved", false, "")
	cmd.Flags().Bool("no-reactions", false, "")
	cmd.Flags().String("order", "anchor", "")
	for k, v := range flags {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set --%s=%q: %v", k, v, err)
		}
	}
	return cmd
}

func TestValidateListComments_EmptyDoc(t *testing.T) {
	t.Parallel()
	cmd := newListCommentsCmd(t, nil)
	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	err := validateListComments(context.Background(), runtime)
	if err == nil || !strings.Contains(err.Error(), "--doc cannot be empty") {
		t.Fatalf("err = %v, want empty-doc error", err)
	}
}

func TestValidateListComments_UnsupportedURL(t *testing.T) {
	t.Parallel()
	cmd := newListCommentsCmd(t, map[string]string{"doc": "https://example.feishu.cn/sheets/shtAbc"})
	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	err := validateListComments(context.Background(), runtime)
	if err == nil || !strings.Contains(err.Error(), "must resolve to docx") {
		t.Fatalf("err = %v, want docx-only error", err)
	}
}

func TestValidateListComments_BareTokenWithoutType(t *testing.T) {
	t.Parallel()
	cmd := newListCommentsCmd(t, map[string]string{"doc": "doxAbc123"})
	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	err := validateListComments(context.Background(), runtime)
	if err == nil || !strings.Contains(err.Error(), "--type is required") {
		t.Fatalf("err = %v, want missing-type error", err)
	}
}

func TestValidateListComments_DocxURLAccepted(t *testing.T) {
	t.Parallel()
	cmd := newListCommentsCmd(t, map[string]string{"doc": "https://example.feishu.cn/docx/doxAbc123"})
	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	if err := validateListComments(context.Background(), runtime); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateListComments_WikiURLAccepted(t *testing.T) {
	t.Parallel()
	cmd := newListCommentsCmd(t, map[string]string{"doc": "https://example.feishu.cn/wiki/wikAbc"})
	runtime := common.TestNewRuntimeContext(cmd, &core.CliConfig{})
	if err := validateListComments(context.Background(), runtime); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- normalizeDocContent / normalizeQuoteNeedle ---

func TestNormalizeDocContent_StripsTagsEntitiesAndWhitespace(t *testing.T) {
	t.Parallel()
	in := `<p>由 <b>P2 / P3 事件聚类</b>触发&#x2F;关联</p>`
	got := normalizeDocContent(in)
	// Tags stripped, entity decoded, ALL whitespace removed.
	// Verifies that text broken by an inline <b> tag is recombined and matches.
	want := "由P2/P3事件聚类触发/关联"
	if !strings.Contains(got, want) {
		t.Fatalf("normalize failed: %q, want substring %q", got, want)
	}
}

func TestNormalizeQuoteNeedle_UsesFirstLine(t *testing.T) {
	t.Parallel()
	multiline := "体验驱动\n随单/年度满意度调查\n舆情反馈"
	got := normalizeQuoteNeedle(multiline)
	if got != "体验驱动" {
		t.Fatalf("got %q, want 体验驱动", got)
	}
}

func TestNormalizeQuoteNeedle_RemovesAllWhitespace(t *testing.T) {
	t.Parallel()
	in := "  hello    world  "
	got := normalizeQuoteNeedle(in)
	if got != "helloworld" {
		t.Fatalf("got %q, want helloworld", got)
	}
}

func TestNormalizeQuoteNeedle_MatchesAcrossInlineTags(t *testing.T) {
	t.Parallel()
	// This test documents the cross-tag matching behavior: a needle that has
	// whitespace in the original quote text must match content that was
	// broken by inline tags in the document XML.
	quote := "由 P2 / P3 事件聚类触发"
	docContent := normalizeDocContent(`<p>由 <b>P2 / P3 事件聚类</b>触发：详细描述...</p>`)
	needle := normalizeQuoteNeedle(quote)
	if !strings.Contains(docContent, needle) {
		t.Fatalf("needle %q not found in normalized doc %q (cross-tag match should succeed)", needle, docContent)
	}
}

func TestNormalizeQuoteNeedle_EmptyQuote(t *testing.T) {
	t.Parallel()
	if got := normalizeQuoteNeedle(""); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
	if got := normalizeQuoteNeedle("\n\n  \n"); got != "" {
		t.Fatalf("got %q, want empty for whitespace-only", got)
	}
}

// --- buildCommentItem ---

func TestBuildCommentItem_WholeDocComment(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"comment_id":  "c1",
		"is_whole":    true,
		"create_time": float64(1700000000),
	}
	it := buildCommentItem(raw, "doc body", false, -1)
	if it.AnchorState != "valid" || it.AnchorPosition != 0 {
		t.Fatalf("got state=%q pos=%d, want valid/0", it.AnchorState, it.AnchorPosition)
	}
}

func TestBuildCommentItem_StickyWithReadonlyBlockPresent(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"comment_id":  "c2",
		"is_whole":    false,
		"quote":       stickyNoteQuote,
		"create_time": float64(1700000000),
	}
	it := buildCommentItem(raw, "ignored", true, 4800)
	if it.AnchorState != "structural" {
		t.Fatalf("got state=%q, want structural", it.AnchorState)
	}
	if it.AnchorPosition != 4800 {
		t.Fatalf("got pos=%d, want 4800", it.AnchorPosition)
	}
}

func TestBuildCommentItem_StickyOrphan(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"comment_id": "c3",
		"is_whole":   false,
		"quote":      stickyNoteQuote,
	}
	it := buildCommentItem(raw, "ignored", false, -1)
	if it.AnchorState != "orphaned" {
		t.Fatalf("got state=%q, want orphaned", it.AnchorState)
	}
}

func TestBuildCommentItem_TextQuoteFound(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"comment_id": "c4",
		"is_whole":   false,
		"quote":      "硬件、软件",
	}
	doc := "some intro text 硬件、软件 trailing text"
	it := buildCommentItem(raw, doc, false, -1)
	if it.AnchorState != "valid" {
		t.Fatalf("got state=%q, want valid", it.AnchorState)
	}
	wantPos := int64(strings.Index(doc, "硬件、软件"))
	if it.AnchorPosition != wantPos {
		t.Fatalf("got pos=%d, want %d", it.AnchorPosition, wantPos)
	}
}

func TestBuildCommentItem_TextQuoteOrphan(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"comment_id": "c5",
		"is_whole":   false,
		"quote":      "Initiate Project",
	}
	doc := "this doc does not mention it"
	it := buildCommentItem(raw, doc, false, -1)
	if it.AnchorState != "orphaned" || it.AnchorPosition != -1 {
		t.Fatalf("got state=%q pos=%d, want orphaned/-1", it.AnchorState, it.AnchorPosition)
	}
}

func TestBuildCommentItem_MultilineQuoteUsesFirstLine(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"comment_id": "c6",
		"is_whole":   false,
		"quote":      "体验驱动\n随单/年度满意度调查\n舆情反馈",
	}
	// Doc still contains 体验驱动 elsewhere but not the sub-bullets — should
	// still classify as valid based on first-line match.
	doc := "earlier 体验驱动和管理驱动 later"
	it := buildCommentItem(raw, doc, false, -1)
	if it.AnchorState != "valid" {
		t.Fatalf("got state=%q, want valid (first-line match)", it.AnchorState)
	}
}

// --- sortCommentItems ---

func TestSortCommentItems_OrphansAtEnd(t *testing.T) {
	t.Parallel()
	items := []commentItem{
		{CommentID: "a", AnchorState: "valid", AnchorPosition: 100, CreateTime: 1},
		{CommentID: "b", AnchorState: "orphaned", AnchorPosition: -1, CreateTime: 2},
		{CommentID: "c", AnchorState: "valid", AnchorPosition: 50, CreateTime: 3},
		{CommentID: "d", AnchorState: "structural", AnchorPosition: 75, CreateTime: 4},
	}
	sortCommentItems(items, "anchor")
	gotIDs := make([]string, len(items))
	for i, it := range items {
		gotIDs[i] = it.CommentID
	}
	// Expected: c (50), d (75 structural), a (100), b (orphan)
	wantIDs := []string{"c", "d", "a", "b"}
	for i, want := range wantIDs {
		if gotIDs[i] != want {
			t.Fatalf("position %d: got %q want %q (full: %v)", i, gotIDs[i], want, gotIDs)
		}
	}
}

func TestSortCommentItems_CreatedOrder(t *testing.T) {
	t.Parallel()
	items := []commentItem{
		{CommentID: "a", AnchorState: "valid", AnchorPosition: 100, CreateTime: 3},
		{CommentID: "b", AnchorState: "valid", AnchorPosition: 50, CreateTime: 1},
		{CommentID: "c", AnchorState: "orphaned", AnchorPosition: -1, CreateTime: 2},
	}
	sortCommentItems(items, "created")
	// Expected: b (1), a (3), c (orphan at end despite time=2)
	gotIDs := make([]string, len(items))
	for i, it := range items {
		gotIDs[i] = it.CommentID
	}
	wantIDs := []string{"b", "a", "c"}
	for i, want := range wantIDs {
		if gotIDs[i] != want {
			t.Fatalf("position %d: got %q want %q (full: %v)", i, gotIDs[i], want, gotIDs)
		}
	}
}

// --- projectRawPosToNormalized ---

func TestProjectRawPosToNormalized_ExactPrefixNormalize(t *testing.T) {
	t.Parallel()
	raw := "<p>01234</p><readonly-block></readonly-block><p>56789</p>"
	got := projectRawPosToNormalized(raw, "", "<readonly-block")
	// normalizeDocContent strips tags and ALL whitespace. The prefix
	// "<p>01234</p>" normalizes to "01234" (length 5), so the marker's
	// position in normalized space is 5.
	if got != 5 {
		t.Fatalf("got %d, want 5", got)
	}
}

func TestProjectRawPosToNormalized_NotFound(t *testing.T) {
	t.Parallel()
	got := projectRawPosToNormalized("no marker here", "normalized", "<readonly-block")
	if got != -1 {
		t.Fatalf("got %d, want -1", got)
	}
}

// --- DryRun ---

func TestDryRunListComments_DocxURL(t *testing.T) {
	t.Parallel()
	cmd := newListCommentsCmd(t, map[string]string{"doc": "https://example.feishu.cn/docx/doxAbc"})
	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	dry := DriveListComments.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}
	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got struct {
		API []struct {
			URL    string                 `json:"url"`
			Method string                 `json:"method"`
			Params map[string]interface{} `json:"params"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Expect 2 calls (no wiki resolution): list comments, fetch doc.
	if len(got.API) != 2 {
		t.Fatalf("expected 2 API calls, got %d (%v)", len(got.API), got.API)
	}
	if !strings.Contains(got.API[0].URL, "/comments") {
		t.Fatalf("first call should hit comments endpoint: %q", got.API[0].URL)
	}
	if got.API[0].Params["is_solved"] != false {
		t.Fatalf("default should include is_solved=false: %#v", got.API[0].Params)
	}
	if got.API[0].Params["need_reaction"] != true {
		t.Fatalf("default should include need_reaction=true: %#v", got.API[0].Params)
	}
	if !strings.Contains(got.API[1].URL, "/fetch") {
		t.Fatalf("second call should hit fetch endpoint: %q", got.API[1].URL)
	}
	if got.API[1].Body["format"] != "xml" {
		t.Fatalf("fetch should use format=xml: %#v", got.API[1].Body)
	}
}

func TestDryRunListComments_WikiURL_AddsResolveStep(t *testing.T) {
	t.Parallel()
	cmd := newListCommentsCmd(t, map[string]string{"doc": "https://example.feishu.cn/wiki/wikAbc"})
	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	dry := DriveListComments.DryRun(context.Background(), runtime)
	data, _ := json.Marshal(dry)
	var got struct {
		API []struct {
			URL string `json:"url"`
		} `json:"api"`
	}
	_ = json.Unmarshal(data, &got)
	if len(got.API) != 3 {
		t.Fatalf("expected 3 API calls with wiki, got %d", len(got.API))
	}
	if !strings.Contains(got.API[0].URL, "/wiki/v2/spaces/get_node") {
		t.Fatalf("first call should resolve wiki: %q", got.API[0].URL)
	}
}

func TestDryRunListComments_IncludeResolvedDropsFilter(t *testing.T) {
	t.Parallel()
	cmd := newListCommentsCmd(t, map[string]string{
		"doc":              "https://example.feishu.cn/docx/doxAbc",
		"include-resolved": "true",
	})
	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	dry := DriveListComments.DryRun(context.Background(), runtime)
	data, _ := json.Marshal(dry)
	var got struct {
		API []struct {
			URL    string                 `json:"url"`
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	_ = json.Unmarshal(data, &got)
	if _, ok := got.API[0].Params["is_solved"]; ok {
		t.Fatalf("include-resolved=true should omit is_solved: %#v", got.API[0].Params)
	}
}

func TestDryRunListComments_NoReactionsDropsParam(t *testing.T) {
	t.Parallel()
	cmd := newListCommentsCmd(t, map[string]string{
		"doc":          "https://example.feishu.cn/docx/doxAbc",
		"no-reactions": "true",
	})
	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	dry := DriveListComments.DryRun(context.Background(), runtime)
	data, _ := json.Marshal(dry)
	var got struct {
		API []struct {
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	_ = json.Unmarshal(data, &got)
	if _, ok := got.API[0].Params["need_reaction"]; ok {
		t.Fatalf("no-reactions=true should omit need_reaction: %#v", got.API[0].Params)
	}
}

// --- End-to-end execution with httpmock ---

func TestDriveListComments_E2E_FiltersOrphanedByDefault(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := &core.CliConfig{AppID: "drive-listcomments-e2e", AppSecret: "test-secret", Brand: core.BrandFeishu}
	f, stdout, _, reg := cmdutil.TestFactory(t, cfg)

	// 3 comments: one valid quote, one orphaned, one sticky (with readonly-block present).
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files/doxTest/comments",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"has_more":   false,
				"page_token": "",
				"items": []map[string]interface{}{
					{
						"comment_id":  "v1",
						"quote":       "硬件、软件",
						"is_whole":    false,
						"is_solved":   false,
						"create_time": float64(1700000001),
						"reply_list":  map[string]interface{}{"replies": []interface{}{}},
					},
					{
						"comment_id":  "o1",
						"quote":       "this text is not in doc",
						"is_whole":    false,
						"is_solved":   false,
						"create_time": float64(1700000002),
						"reply_list":  map[string]interface{}{"replies": []interface{}{}},
					},
					{
						"comment_id":  "s1",
						"quote":       stickyNoteQuote,
						"is_whole":    false,
						"is_solved":   false,
						"create_time": float64(1700000003),
						"reply_list":  map[string]interface{}{"replies": []interface{}{}},
					},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/doxTest/fetch",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"document": map[string]interface{}{
					"content": `<p>introduction</p><p>硬件、软件 are technical</p><readonly-block type="isv"></readonly-block><p>trailing</p>`,
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveListComments, []string{
		"+list-comments",
		"--doc", "doxTest",
		"--type", "docx",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	out := decodeDriveEnvelope(t, stdout)
	items, ok := out["items"].([]interface{})
	if !ok {
		t.Fatalf("missing items array: %#v", out)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items after orphan filter, got %d", len(items))
	}
	counts, _ := out["counts"].(map[string]interface{})
	if counts["total"] != float64(3) {
		t.Fatalf("counts.total = %#v, want 3", counts["total"])
	}
	if counts["valid"] != float64(1) {
		t.Fatalf("counts.valid = %#v, want 1", counts["valid"])
	}
	if counts["structural"] != float64(1) {
		t.Fatalf("counts.structural = %#v, want 1", counts["structural"])
	}
	if counts["orphaned"] != float64(1) {
		t.Fatalf("counts.orphaned = %#v, want 1", counts["orphaned"])
	}
}

func TestDriveListComments_E2E_IncludeOrphanedKeepsAll(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cfg := &core.CliConfig{AppID: "drive-listcomments-include-orphan", AppSecret: "test-secret", Brand: core.BrandFeishu}
	f, stdout, _, reg := cmdutil.TestFactory(t, cfg)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files/doxTest2/comments",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"has_more": false,
				"items": []map[string]interface{}{
					{"comment_id": "v1", "quote": "硬件", "is_whole": false, "create_time": float64(1)},
					{"comment_id": "o1", "quote": "missing", "is_whole": false, "create_time": float64(2)},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/doxTest2/fetch",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"document": map[string]interface{}{
					"content": "<p>硬件 only</p>",
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveListComments, []string{
		"+list-comments",
		"--doc", "doxTest2",
		"--type", "docx",
		"--include-orphaned",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	out := decodeDriveEnvelope(t, stdout)
	items, _ := out["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("expected 2 items with --include-orphaned, got %d", len(items))
	}
}
