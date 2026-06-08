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

// --- relation / block location helpers ---

func TestExtractRelationBlockID(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"relation": map[string]interface{}{
			"content_deleted": false,
			"relation":        `{"22-dox":{"objType":22,"positionInfo":{"blockID":"blk_123"}}}`,
		},
	}
	blockID, deleted, ok := extractCommentRelation(raw)
	if !ok {
		t.Fatal("expected relation to parse")
	}
	if deleted {
		t.Fatal("content_deleted should be false")
	}
	if blockID != "blk_123" {
		t.Fatalf("blockID = %q, want blk_123", blockID)
	}
}

func TestExtractRelationBlockID_MalformedRelationDoesNotDelete(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"relation": map[string]interface{}{
			"content_deleted": true,
			"relation":        `{not-json`,
		},
	}
	blockID, deleted, ok := extractCommentRelation(raw)
	if ok {
		t.Fatal("malformed relation JSON should not report a parsed block id")
	}
	if !deleted {
		t.Fatal("content_deleted should still be honored when relation JSON is malformed")
	}
	if blockID != "" {
		t.Fatalf("blockID = %q, want empty", blockID)
	}
}

func TestBuildDocBlockIndex_OrdersBlockIDsAndEmbeddedTokens(t *testing.T) {
	t.Parallel()
	xml := `<doc>
		<paragraph id="blk_a">A</paragraph>
		<whiteboard id="blk_whiteboard" token="wbd_123"></whiteboard>
		<paragraph block_id="blk_b">B</paragraph>
	</doc>`
	idx := buildDocBlockIndex(xml)
	if got := idx.orderOfBlock("blk_a"); got != 0 {
		t.Fatalf("blk_a order = %d, want 0", got)
	}
	if got := idx.orderOfBlock("blk_b"); got != 2 {
		t.Fatalf("blk_b order = %d, want 2", got)
	}
	blockID, order, ok := idx.lookupEmbeddedToken("wbd_123")
	if !ok {
		t.Fatal("expected embedded token lookup to succeed")
	}
	if blockID != "blk_whiteboard" || order != 1 {
		t.Fatalf("embedded lookup = (%q,%d), want (blk_whiteboard,1)", blockID, order)
	}
}

func TestBuildCommentItem_ContentDeletedIsOrphaned(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"comment_id": "c_deleted",
		"relation": map[string]interface{}{
			"content_deleted": true,
			"relation":        `{"22-dox":{"positionInfo":{"blockID":"blk_deleted"}}}`,
		},
	}
	idx := buildDocBlockIndex(`<paragraph id="blk_deleted">deleted anchor gone</paragraph>`)
	it := buildCommentItem(raw, idx)
	if it.AnchorState != "orphaned" || it.LocationAccuracy != "content_deleted" {
		t.Fatalf("got state=%q accuracy=%q, want orphaned/content_deleted", it.AnchorState, it.LocationAccuracy)
	}
}

func TestBuildCommentItem_RelationExact(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"comment_id":  "c_exact",
		"create_time": float64(1700000000),
		"relation": map[string]interface{}{
			"content_deleted": false,
			"relation":        `{"22-dox":{"positionInfo":{"blockID":"blk_a"}}}`,
		},
	}
	idx := buildDocBlockIndex(`<paragraph id="blk_a">A</paragraph>`)
	it := buildCommentItem(raw, idx)
	if it.AnchorState != "valid" || it.LocationAccuracy != "relation_exact" {
		t.Fatalf("got state=%q accuracy=%q, want valid/relation_exact", it.AnchorState, it.LocationAccuracy)
	}
	if it.AnchorBlockID != "blk_a" || it.AnchorPosition != 0 {
		t.Fatalf("got block=%q pos=%d, want blk_a/0", it.AnchorBlockID, it.AnchorPosition)
	}
}

func TestBuildCommentItem_ParentResourceExact(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"comment_id":   "c_embed",
		"parent_type":  "WHITEBOARD_BLOCK",
		"parent_token": "wbd_123",
	}
	idx := buildDocBlockIndex(`<whiteboard id="blk_whiteboard" token="wbd_123"></whiteboard>`)
	it := buildCommentItem(raw, idx)
	if it.AnchorState != "structural" || it.LocationAccuracy != "parent_resource_exact" {
		t.Fatalf("got state=%q accuracy=%q, want structural/parent_resource_exact", it.AnchorState, it.LocationAccuracy)
	}
	if it.AnchorBlockID != "blk_whiteboard" || it.AnchorPosition != 0 {
		t.Fatalf("got block=%q pos=%d, want blk_whiteboard/0", it.AnchorBlockID, it.AnchorPosition)
	}
}

func TestBuildCommentItem_ParentResourceTokenUsesLastUnderscore(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"comment_id":   "c_sheet_parent",
		"parent_type":  "SHEET_BLOCK",
		"parent_token": "sht_token_123_sheet1",
	}
	idx := buildDocBlockIndex(`<sheet id="blk_sheet" token="sht_token_123"></sheet>`)
	it := buildCommentItem(raw, idx)
	if it.AnchorState != "structural" || it.LocationAccuracy != "parent_resource_exact" {
		t.Fatalf("got state=%q accuracy=%q, want structural/parent_resource_exact", it.AnchorState, it.LocationAccuracy)
	}
	if it.AnchorBlockID != "blk_sheet" || it.AnchorPosition != 0 {
		t.Fatalf("got block=%q pos=%d, want blk_sheet/0", it.AnchorBlockID, it.AnchorPosition)
	}
}

func TestBuildCommentItem_EmbeddedRelationUsesParentResourceAccuracy(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"comment_id":   "c_sheet",
		"parent_type":  "SHEET_BLOCK",
		"parent_token": "sht_123_sheet1",
		"relation": map[string]interface{}{
			"content_deleted": false,
			"relation":        `{"22-dox":{"positionInfo":{"blockID":"blk_sheet"}}}`,
		},
	}
	idx := buildDocBlockIndex(`<sheet id="blk_sheet" token="sht_123"></sheet>`)
	it := buildCommentItem(raw, idx)
	if it.AnchorState != "structural" || it.LocationAccuracy != "parent_resource_exact" {
		t.Fatalf("got state=%q accuracy=%q, want structural/parent_resource_exact", it.AnchorState, it.LocationAccuracy)
	}
	if it.AnchorBlockID != "blk_sheet" || it.AnchorPosition != 0 {
		t.Fatalf("got block=%q pos=%d, want blk_sheet/0", it.AnchorBlockID, it.AnchorPosition)
	}
}

// --- sortCommentItems ---

func TestSortCommentItems_OrphansAtEnd(t *testing.T) {
	t.Parallel()
	items := []commentItem{
		{CommentID: "a", AnchorState: "valid", AnchorPosition: 2, CreateTime: 1},
		{CommentID: "b", AnchorState: "orphaned", AnchorPosition: -1, CreateTime: 2},
		{CommentID: "c", AnchorState: "valid", AnchorPosition: 0, CreateTime: 3},
		{CommentID: "d", AnchorState: "structural", AnchorPosition: 1, CreateTime: 4},
	}
	sortCommentItems(items, "anchor")
	gotIDs := make([]string, len(items))
	for i, it := range items {
		gotIDs[i] = it.CommentID
	}
	// Expected: c (0), d (1 structural), a (2), b (orphan)
	wantIDs := []string{"c", "d", "a", "b"}
	for i, want := range wantIDs {
		if gotIDs[i] != want {
			t.Fatalf("position %d: got %q want %q (full: %v)", i, gotIDs[i], want, gotIDs)
		}
	}
}

func TestSortCommentItems_UnknownLocationsAfterAnchoredBeforeOrphans(t *testing.T) {
	t.Parallel()
	items := []commentItem{
		{CommentID: "a", AnchorState: "valid", AnchorPosition: -1, CreateTime: 1},
		{CommentID: "b", AnchorState: "valid", AnchorPosition: 2, CreateTime: 2},
		{CommentID: "c", AnchorState: "orphaned", AnchorPosition: -1, CreateTime: 3},
		{CommentID: "d", AnchorState: "valid", AnchorPosition: 1, CreateTime: 4},
	}
	sortCommentItems(items, "anchor")
	gotIDs := make([]string, len(items))
	for i, it := range items {
		gotIDs[i] = it.CommentID
	}
	wantIDs := []string{"d", "b", "a", "c"}
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
	if got.API[0].Params["need_relation"] != true {
		t.Fatalf("default should include need_relation=true: %#v", got.API[0].Params)
	}
	if !strings.Contains(got.API[1].URL, "/fetch") {
		t.Fatalf("second call should hit fetch endpoint: %q", got.API[1].URL)
	}
	if got.API[1].Body["format"] != "xml" {
		t.Fatalf("fetch should use format=xml: %#v", got.API[1].Body)
	}
	exportOption, _ := got.API[1].Body["export_option"].(map[string]interface{})
	if exportOption["export_block_id"] != true {
		t.Fatalf("fetch should request export_block_id=true: %#v", got.API[1].Body)
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
	cfg := &core.CliConfig{AppID: "drive-listcomments-e2e", AppSecret: "test-secret", Brand: core.BrandFeishu}
	f, stdout, _, reg := cmdutil.TestFactory(t, cfg)

	// 4 comments: two relation-exact anchors, one embedded-resource anchor,
	// and one content_deleted orphan.
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
						"comment_id":  "later",
						"quote":       "later block",
						"is_whole":    false,
						"is_solved":   false,
						"create_time": float64(1700000001),
						"relation": map[string]interface{}{
							"content_deleted": false,
							"relation":        `{"22-doxTest":{"positionInfo":{"blockID":"blk_b"}}}`,
						},
						"reply_list": map[string]interface{}{"replies": []interface{}{}},
					},
					{
						"comment_id":  "deleted",
						"quote":       "deleted anchor",
						"is_whole":    false,
						"is_solved":   false,
						"create_time": float64(1700000002),
						"relation": map[string]interface{}{
							"content_deleted": true,
							"relation":        `{"22-doxTest":{"positionInfo":{"blockID":"blk_deleted"}}}`,
						},
						"reply_list": map[string]interface{}{"replies": []interface{}{}},
					},
					{
						"comment_id":   "embed",
						"quote":        "画板节点文本",
						"is_whole":     false,
						"is_solved":    false,
						"create_time":  float64(1700000003),
						"parent_type":  "WHITEBOARD_BLOCK",
						"parent_token": "wbd_123",
						"reply_list":   map[string]interface{}{"replies": []interface{}{}},
					},
					{
						"comment_id":  "earlier",
						"quote":       "first block",
						"is_whole":    false,
						"is_solved":   false,
						"create_time": float64(1700000004),
						"relation": map[string]interface{}{
							"content_deleted": false,
							"relation":        `{"22-doxTest":{"positionInfo":{"blockID":"blk_a"}}}`,
						},
						"reply_list": map[string]interface{}{"replies": []interface{}{}},
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
					"content": `<doc><paragraph id="blk_a">first</paragraph><whiteboard id="blk_whiteboard" token="wbd_123"></whiteboard><paragraph id="blk_b">later</paragraph></doc>`,
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
	if len(items) != 3 {
		t.Fatalf("expected 3 items after orphan filter, got %d", len(items))
	}
	gotIDs := make([]string, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]interface{})
		gotIDs = append(gotIDs, item["comment_id"].(string))
	}
	wantIDs := []string{"earlier", "embed", "later"}
	for i, want := range wantIDs {
		if gotIDs[i] != want {
			t.Fatalf("item order = %v, want %v", gotIDs, wantIDs)
		}
	}
	embed, _ := items[1].(map[string]interface{})
	if embed["anchor_block_id"] != "blk_whiteboard" {
		t.Fatalf("embedded anchor_block_id = %#v, want blk_whiteboard", embed["anchor_block_id"])
	}
	if embed["location_accuracy"] != "parent_resource_exact" {
		t.Fatalf("embedded location_accuracy = %#v, want parent_resource_exact", embed["location_accuracy"])
	}
	counts, _ := out["counts"].(map[string]interface{})
	if counts["total"] != float64(4) {
		t.Fatalf("counts.total = %#v, want 4", counts["total"])
	}
	if counts["valid"] != float64(2) {
		t.Fatalf("counts.valid = %#v, want 2", counts["valid"])
	}
	if counts["structural"] != float64(1) {
		t.Fatalf("counts.structural = %#v, want 1", counts["structural"])
	}
	if counts["orphaned"] != float64(1) {
		t.Fatalf("counts.orphaned = %#v, want 1", counts["orphaned"])
	}
}

func TestDriveListComments_E2E_IncludeOrphanedKeepsAll(t *testing.T) {
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
					{
						"comment_id":  "v1",
						"quote":       "硬件",
						"is_whole":    false,
						"create_time": float64(1),
						"relation": map[string]interface{}{
							"content_deleted": false,
							"relation":        `{"22-doxTest2":{"positionInfo":{"blockID":"blk_a"}}}`,
						},
					},
					{
						"comment_id":  "o1",
						"quote":       "missing",
						"is_whole":    false,
						"create_time": float64(2),
						"relation": map[string]interface{}{
							"content_deleted": true,
							"relation":        `{"22-doxTest2":{"positionInfo":{"blockID":"blk_missing"}}}`,
						},
					},
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
					"content": `<paragraph id="blk_a">硬件 only</paragraph>`,
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
