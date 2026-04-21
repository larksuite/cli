// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestBuildCreateBlockDataUsesConcreteAppendIndex(t *testing.T) {
	t.Parallel()

	got := buildCreateBlockData("image", 3, 0)
	want := map[string]interface{}{
		"children": []interface{}{
			map[string]interface{}{
				"block_type": 27,
				"image":      map[string]interface{}{},
			},
		},
		"index": 3,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildCreateBlockData() = %#v, want %#v", got, want)
	}
}

func TestBuildCreateBlockDataForFileIncludesFilePayload(t *testing.T) {
	t.Parallel()

	got := buildCreateBlockData("file", 1, 0)
	want := map[string]interface{}{
		"children": []interface{}{
			map[string]interface{}{
				"block_type": 23,
				"file":       map[string]interface{}{},
			},
		},
		"index": 1,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildCreateBlockData(file) = %#v, want %#v", got, want)
	}
}

// The `--file-view card` path sends a different request shape than
// omitting the flag entirely: omitting produces `file: {}`, while
// `card` produces `file: {view_type: 1}`. The two are intended to be
// semantically equivalent at the API level, but the on-the-wire payload
// is different and is part of the public flag contract, so pin it down.
func TestBuildCreateBlockDataForFileWithCardView(t *testing.T) {
	t.Parallel()

	got := buildCreateBlockData("file", 0, 1) // card
	want := map[string]interface{}{
		"children": []interface{}{
			map[string]interface{}{
				"block_type": 23,
				"file": map[string]interface{}{
					"view_type": 1,
				},
			},
		},
		"index": 0,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildCreateBlockData(file, card) = %#v, want %#v", got, want)
	}
}

func TestBuildCreateBlockDataForFileWithPreviewView(t *testing.T) {
	t.Parallel()

	got := buildCreateBlockData("file", 0, 2) // preview
	want := map[string]interface{}{
		"children": []interface{}{
			map[string]interface{}{
				"block_type": 23,
				"file": map[string]interface{}{
					"view_type": 2,
				},
			},
		},
		"index": 0,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildCreateBlockData(file, preview) = %#v, want %#v", got, want)
	}
}

func TestBuildCreateBlockDataForFileWithInlineView(t *testing.T) {
	t.Parallel()

	got := buildCreateBlockData("file", 0, 3) // inline
	want := map[string]interface{}{
		"children": []interface{}{
			map[string]interface{}{
				"block_type": 23,
				"file": map[string]interface{}{
					"view_type": 3,
				},
			},
		},
		"index": 0,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildCreateBlockData(file, inline) = %#v, want %#v", got, want)
	}
}

// view_type must never leak into non-file blocks even if the caller
// accidentally passes a non-zero fileViewType alongside --type=image.
func TestBuildCreateBlockDataForImageIgnoresFileViewType(t *testing.T) {
	t.Parallel()

	got := buildCreateBlockData("image", 0, 2)
	want := map[string]interface{}{
		"children": []interface{}{
			map[string]interface{}{
				"block_type": 27,
				"image":      map[string]interface{}{},
			},
		},
		"index": 0,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildCreateBlockData(image, preview) = %#v, want %#v", got, want)
	}
}

func TestFileViewMapCoversDocumentedValues(t *testing.T) {
	t.Parallel()

	// Assert only the documented keys — leave room for future aliases
	// (e.g. a "player" synonym for preview) without breaking this test.
	want := map[string]int{
		"card":    1,
		"preview": 2,
		"inline":  3,
	}
	for key, expected := range want {
		got, ok := fileViewMap[key]
		if !ok {
			t.Errorf("fileViewMap missing required key %q", key)
			continue
		}
		if got != expected {
			t.Errorf("fileViewMap[%q] = %d, want %d", key, got, expected)
		}
	}
}

func TestBuildDeleteBlockDataUsesHalfOpenInterval(t *testing.T) {
	t.Parallel()

	got := buildDeleteBlockData(5)
	want := map[string]interface{}{
		"start_index": 5,
		"end_index":   6,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildDeleteBlockData() = %#v, want %#v", got, want)
	}
}

func TestBuildBatchUpdateDataForImage(t *testing.T) {
	t.Parallel()

	got := buildBatchUpdateData("blk_1", "image", "file_tok", "center", "caption text")
	want := map[string]interface{}{
		"requests": []interface{}{
			map[string]interface{}{
				"block_id": "blk_1",
				"replace_image": map[string]interface{}{
					"token": "file_tok",
					"align": 2,
					"caption": map[string]interface{}{
						"content": "caption text",
					},
				},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildBatchUpdateData(image) = %#v, want %#v", got, want)
	}
}

func TestBuildBatchUpdateDataForFile(t *testing.T) {
	t.Parallel()

	got := buildBatchUpdateData("blk_2", "file", "file_tok", "", "")
	want := map[string]interface{}{
		"requests": []interface{}{
			map[string]interface{}{
				"block_id": "blk_2",
				"replace_file": map[string]interface{}{
					"token": "file_tok",
				},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildBatchUpdateData(file) = %#v, want %#v", got, want)
	}
}

func TestExtractAppendTargetUsesRootChildrenCount(t *testing.T) {
	t.Parallel()

	rootData := map[string]interface{}{
		"block": map[string]interface{}{
			"block_id": "root_block",
			"children": []interface{}{"c1", "c2", "c3"},
		},
	}

	blockID, index, children, err := extractAppendTarget(rootData, "fallback")
	if err != nil {
		t.Fatalf("extractAppendTarget() unexpected error: %v", err)
	}
	if blockID != "root_block" {
		t.Fatalf("extractAppendTarget() blockID = %q, want %q", blockID, "root_block")
	}
	if index != 3 {
		t.Fatalf("extractAppendTarget() index = %d, want 3", index)
	}
	if len(children) != 3 {
		t.Fatalf("extractAppendTarget() children len = %d, want 3", len(children))
	}
}

// buildLocateDocMCPResponse builds a JSON-RPC 2.0 response for a locate-doc MCP call.
func buildLocateDocMCPResponse(matches []map[string]interface{}) map[string]interface{} {
	resultJSON, _ := json.Marshal(map[string]interface{}{"matches": matches})
	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "test-id",
		"result": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": string(resultJSON),
				},
			},
		},
	}
}

// registerInsertWithSelectionStubs wires the minimal stub set for the
// --selection-with-ellipsis happy path. Returns the create-block stub so
// callers can inspect the request body (e.g. to verify the computed index).
func registerInsertWithSelectionStubs(reg interface {
	Register(*httpmock.Stub)
}, docID, anchorBlockID, parentBlockID string, rootChildren []interface{}) *httpmock.Stub {
	// Root block
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/" + docID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"block": map[string]interface{}{
					"block_id": docID,
					"children": rootChildren,
				},
			},
		},
	})
	// MCP locate-doc
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "mcp.feishu.cn/mcp",
		Body: buildLocateDocMCPResponse([]map[string]interface{}{
			{"anchor_block_id": anchorBlockID, "parent_block_id": parentBlockID},
		}),
	})
	// Create block — returned so the test can inspect index in CapturedBody.
	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/" + docID + "/children",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"children": []interface{}{
					map[string]interface{}{"block_id": "blk_new", "block_type": 27, "image": map[string]interface{}{}},
				},
			},
		},
	}
	reg.Register(createStub)
	// Upload
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "ftok_test"},
		},
	})
	// Batch update
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/batch_update",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	})
	return createStub
}

// assertCreateBlockIndex decodes the create-block request body and asserts the
// `index` field equals want. Fails the test if the body is missing or wrong.
func assertCreateBlockIndex(t *testing.T, stub *httpmock.Stub, want int) {
	t.Helper()
	if stub.CapturedBody == nil {
		t.Fatalf("create-block stub captured no body")
	}
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode create-block body: %v (raw: %s)", err, stub.CapturedBody)
	}
	got, _ := body["index"].(float64)
	if int(got) != want {
		t.Fatalf("create-block index = %v, want %d (body: %s)", body["index"], want, stub.CapturedBody)
	}
}

// TestLocateInsertIndexAfterModeViaExecute verifies that
// --selection-with-ellipsis (default after-mode) places the new block
// immediately after the matched root-level block. Uses three root children so
// the after-index (2) differs from what --before would produce (1), and
// inspects the create-block request body to prove the computed index actually
// reaches the /children API.
func TestLocateInsertIndexAfterModeViaExecute(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("locate-after-app"))
	createStub := registerInsertWithSelectionStubs(reg, "doxcnSEL", "blk_b", "doxcnSEL",
		[]interface{}{"blk_a", "blk_b", "blk_c"})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	writeSizedDocTestFile(t, "img.png", 100)

	err := mountAndRunDocs(t, DocMediaInsert, []string{
		"+media-insert",
		"--doc", "doxcnSEL",
		"--file", "img.png",
		"--selection-with-ellipsis", "Introduction",
		"--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	// after blk_b (index 1) → insert at index 2, between blk_b and blk_c.
	assertCreateBlockIndex(t, createStub, 2)
}

// TestLocateInsertIndexBeforeModeViaExecute verifies that --before inserts
// before the matched root-level block. Pairs with the after-mode test above:
// same fixture, same anchor, but --before should flip the index from 2 to 1.
// A regression that ignored --before would still pass the success check alone,
// so we assert the create-block body explicitly.
func TestLocateInsertIndexBeforeModeViaExecute(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("locate-before-app"))
	createStub := registerInsertWithSelectionStubs(reg, "doxcnSEL2", "blk_b", "doxcnSEL2",
		[]interface{}{"blk_a", "blk_b", "blk_c"})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	writeSizedDocTestFile(t, "img.png", 100)

	err := mountAndRunDocs(t, DocMediaInsert, []string{
		"+media-insert",
		"--doc", "doxcnSEL2",
		"--file", "img.png",
		"--selection-with-ellipsis", "Architecture",
		"--before",
		"--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	// before blk_b (index 1) → insert at index 1, between blk_a and blk_b.
	assertCreateBlockIndex(t, createStub, 1)
}

// TestLocateInsertIndexNestedBlockViaExecute verifies that a deeply-nested
// anchor (2+ levels below root) walks up through an intermediate block via
// the GET /blocks/{id} API to find the root-level ancestor. This exercises
// the fallback ancestor-walk path in locateInsertIndex — the parent_block_id
// hint from locate-doc is only good for one level, so deeper nesting must hit
// the block-fetch loop.
func TestLocateInsertIndexNestedBlockViaExecute(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("locate-nested-app"))

	docID := "doxcnNESTED"
	// Root children: blk_section (index 0), blk_other (index 1).
	// Anchor blk_grandchild is nested two levels deep:
	//   root → blk_section → blk_section_child → blk_grandchild
	// locate-doc gives us parent_block_id = blk_section_child (one level up);
	// the walk must fetch blk_section_child to discover its parent = blk_section
	// before it can land on a root child.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/" + docID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"block": map[string]interface{}{
					"block_id": docID,
					"children": []interface{}{"blk_section", "blk_other"},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "mcp.feishu.cn/mcp",
		Body: buildLocateDocMCPResponse([]map[string]interface{}{
			{"anchor_block_id": "blk_grandchild", "parent_block_id": "blk_section_child"},
		}),
	})
	// Intermediate block lookup — this is the key step that exercises the
	// fallback walk. Without this stub the test would fail.
	intermediateStub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/blk_section_child",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"block": map[string]interface{}{
					"block_id":  "blk_section_child",
					"parent_id": "blk_section",
				},
			},
		},
	}
	reg.Register(intermediateStub)
	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/" + docID + "/children",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"children": []interface{}{
					map[string]interface{}{"block_id": "blk_new", "block_type": 27, "image": map[string]interface{}{}},
				},
			},
		},
	}
	reg.Register(createStub)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "ftok_nested"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/batch_update",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	writeSizedDocTestFile(t, "img.png", 100)

	err := mountAndRunDocs(t, DocMediaInsert, []string{
		"+media-insert",
		"--doc", docID,
		"--file", "img.png",
		"--selection-with-ellipsis", "nested content",
		"--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	// Confirm the ancestor-walk actually fired — without this assertion a
	// regression that short-circuited the walk would still pass.
	if intermediateStub.CapturedBody == nil && intermediateStub.CapturedHeaders == nil {
		t.Errorf("expected GET /blocks/blk_section_child to be invoked by the parent-walk; stub was not hit")
	}
	// after blk_section (index 0) → insert at index 1, between blk_section and blk_other.
	assertCreateBlockIndex(t, createStub, 1)
}

// TestLocateInsertIndexNoMatchReturnsError verifies that when locate-doc returns
// no matches, Execute returns a descriptive error.
func TestLocateInsertIndexNoMatchReturnsError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("locate-nomatch-app"))

	docID := "doxcnNOMATCH"
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/" + docID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"block": map[string]interface{}{
					"block_id": docID,
					"children": []interface{}{"blk_a"},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "mcp.feishu.cn/mcp",
		Body:   buildLocateDocMCPResponse([]map[string]interface{}{}),
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	writeSizedDocTestFile(t, "img.png", 100)

	err := mountAndRunDocs(t, DocMediaInsert, []string{
		"+media-insert",
		"--doc", docID,
		"--file", "img.png",
		"--selection-with-ellipsis", "nonexistent text",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected no-match error, got nil")
	}
	if !strings.Contains(err.Error(), "no_match") && !strings.Contains(err.Error(), "did not find") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLocateInsertIndexDryRunIncludesMCPStep verifies that the dry-run output
// includes a locate-doc MCP step when --selection-with-ellipsis is provided.
func TestLocateInsertIndexDryRunIncludesMCPStep(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "docs +media-insert"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("doc", "", "")
	cmd.Flags().String("type", "image", "")
	cmd.Flags().String("align", "", "")
	cmd.Flags().String("caption", "", "")
	cmd.Flags().String("selection-with-ellipsis", "", "")
	cmd.Flags().Bool("before", false, "")
	_ = cmd.Flags().Set("file", "img.png")
	_ = cmd.Flags().Set("doc", "doxcnABCDEF")
	_ = cmd.Flags().Set("selection-with-ellipsis", "Introduction")

	rt := common.TestNewRuntimeContext(cmd, docsTestConfigWithAppID("dry-run-app"))
	dryAPI := DocMediaInsert.DryRun(context.Background(), rt)
	raw, _ := json.Marshal(dryAPI)

	var dry struct {
		Description string `json:"description"`
		API         []struct {
			Desc string                 `json:"desc"`
			URL  string                 `json:"url"`
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(raw, &dry); err != nil {
		t.Fatalf("decode dry-run: %v", err)
	}

	foundMCP := false
	for _, step := range dry.API {
		if strings.Contains(step.Desc, "locate-doc") {
			foundMCP = true
		}
	}
	if !foundMCP {
		t.Fatalf("dry-run should include a locate-doc step, got: %+v", dry.API)
	}
	if !strings.Contains(dry.Description, "locate-doc") {
		t.Fatalf("dry-run description should mention 'locate-doc', got: %s", dry.Description)
	}

	// Verify create-block step shows <locate_index> not <children_len>
	for _, step := range dry.API {
		if strings.Contains(step.URL, "/children") && step.Body != nil {
			if idx, ok := step.Body["index"]; ok {
				if idx != "<locate_index>" {
					t.Fatalf("create-block index in selection mode = %q, want <locate_index>", idx)
				}
			}
		}
	}
}

func TestExtractCreatedBlockTargetsForImage(t *testing.T) {
	t.Parallel()

	createData := map[string]interface{}{
		"children": []interface{}{
			map[string]interface{}{
				"block_id": "img_outer",
			},
		},
	}

	blockID, uploadParentNode, replaceBlockID := extractCreatedBlockTargets(createData, "image")
	if blockID != "img_outer" || uploadParentNode != "img_outer" || replaceBlockID != "img_outer" {
		t.Fatalf("extractCreatedBlockTargets(image) = (%q, %q, %q)", blockID, uploadParentNode, replaceBlockID)
	}
}

func TestExtractCreatedBlockTargetsForFileUsesNestedFileBlock(t *testing.T) {
	t.Parallel()

	createData := map[string]interface{}{
		"children": []interface{}{
			map[string]interface{}{
				"block_id": "view_outer",
				"children": []interface{}{"file_inner"},
			},
		},
	}

	blockID, uploadParentNode, replaceBlockID := extractCreatedBlockTargets(createData, "file")
	if blockID != "view_outer" {
		t.Fatalf("extractCreatedBlockTargets(file) blockID = %q, want %q", blockID, "view_outer")
	}
	if uploadParentNode != "file_inner" {
		t.Fatalf("extractCreatedBlockTargets(file) uploadParentNode = %q, want %q", uploadParentNode, "file_inner")
	}
	if replaceBlockID != "file_inner" {
		t.Fatalf("extractCreatedBlockTargets(file) replaceBlockID = %q, want %q", replaceBlockID, "file_inner")
	}
}

// newMediaInsertValidateRuntime builds a bare RuntimeContext wired with
// only the flags that DocMediaInsert.Validate reads. It exists so the
// Validate tests below can exercise the CLI contract without going
// through the full cobra command tree.
func newMediaInsertValidateRuntime(t *testing.T, doc, mediaType, fileView string) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "docs +media-insert"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().Bool("from-clipboard", false, "")
	cmd.Flags().String("doc", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("file-view", "", "")
	// A non-empty --file satisfies the file/clipboard xor check so Validate
	// reaches the --file-view logic under test below.
	if err := cmd.Flags().Set("file", "dummy.bin"); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := cmd.Flags().Set("doc", doc); err != nil {
		t.Fatalf("set --doc: %v", err)
	}
	if err := cmd.Flags().Set("type", mediaType); err != nil {
		t.Fatalf("set --type: %v", err)
	}
	if fileView != "" {
		if err := cmd.Flags().Set("file-view", fileView); err != nil {
			t.Fatalf("set --file-view: %v", err)
		}
	}
	return common.TestNewRuntimeContext(cmd, nil)
}

// Validate is the real user-facing contract for --file-view: unknown
// values must be rejected, and passing the flag alongside --type!=file
// must also be rejected. buildCreateBlockData tests alone cannot catch
// regressions here, so lock the guard logic down explicitly.
func TestDocMediaInsertValidateFileView(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mediaType string
		fileView  string
		wantErr   string // substring; empty means success expected
	}{
		{
			name:      "file with card is accepted",
			mediaType: "file",
			fileView:  "card",
		},
		{
			name:      "file with preview is accepted",
			mediaType: "file",
			fileView:  "preview",
		},
		{
			name:      "file with inline is accepted",
			mediaType: "file",
			fileView:  "inline",
		},
		{
			name:      "file without file-view is accepted",
			mediaType: "file",
			fileView:  "",
		},
		{
			name:      "unknown file-view value is rejected",
			mediaType: "file",
			fileView:  "bogus",
			wantErr:   "invalid --file-view value",
		},
		{
			name:      "file-view with image type is rejected",
			mediaType: "image",
			fileView:  "preview",
			wantErr:   "--file-view only applies when --type=file",
		},
	}

	for _, ttTemp := range tests {
		tt := ttTemp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rt := newMediaInsertValidateRuntime(t, "doxcnValidateFileView", tt.mediaType, tt.fileView)
			err := DocMediaInsert.Validate(context.Background(), rt)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() error = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestLocateInsertIndexWarnsOnMultipleMatches verifies that when locate-doc
// returns more than one match, a warning is written to stderr pointing the user
// at the 'start...end' disambiguation syntax. Silently picking the first match
// of an ambiguous selection is a real UX trap — users who edit documents with
// repeated phrases (a heading that also appears in the TOC, for example) get
// no signal that another match existed.
func TestLocateInsertIndexWarnsOnMultipleMatches(t *testing.T) {
	f, _, stderr, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("locate-multi-app"))

	docID := "doxcnMULTI"
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/" + docID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"block": map[string]interface{}{
					"block_id": docID,
					"children": []interface{}{"blk_a", "blk_b"},
				},
			},
		},
	})
	// Two matches — same selection appears in two different root-level blocks.
	// locate-doc orders matches by document position, so matches[0] is still
	// deterministic (blk_a) even with limit=2.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "mcp.feishu.cn/mcp",
		Body: buildLocateDocMCPResponse([]map[string]interface{}{
			{"anchor_block_id": "blk_a", "parent_block_id": docID},
			{"anchor_block_id": "blk_b", "parent_block_id": docID},
		}),
	})
	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/" + docID + "/children",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"children": []interface{}{
					map[string]interface{}{"block_id": "blk_new", "block_type": 27, "image": map[string]interface{}{}},
				},
			},
		},
	}
	reg.Register(createStub)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "ftok_multi"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/batch_update",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	writeSizedDocTestFile(t, "img.png", 100)

	err := mountAndRunDocs(t, DocMediaInsert, []string{
		"+media-insert",
		"--doc", docID,
		"--file", "img.png",
		"--selection-with-ellipsis", "Repeated phrase",
		"--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Warning should name the ambiguity and point at 'start...end'.
	stderrOut := stderr.String()
	if !strings.Contains(stderrOut, "matched more than one block") {
		t.Errorf("stderr missing multi-match warning; got:\n%s", stderrOut)
	}
	if !strings.Contains(stderrOut, "start...end") {
		t.Errorf("stderr missing 'start...end' disambiguation hint; got:\n%s", stderrOut)
	}
	// Should still insert at the first match (blk_a at index 0) → after ⇒ 1.
	assertCreateBlockIndex(t, createStub, 1)
}

// TestLocateInsertIndexLogsNestedAnchor verifies that when the matched block is
// nested (not a direct root child), a note is written to stderr explaining that
// the media lands at the top-level ancestor. This protects users from being
// surprised when selecting text inside a callout or table cell and seeing the
// image appear outside that container.
func TestLocateInsertIndexLogsNestedAnchor(t *testing.T) {
	f, _, stderr, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("locate-nested-log-app"))

	docID := "doxcnNESTEDLOG"
	// Same shape as TestLocateInsertIndexNestedBlockViaExecute: anchor is two
	// levels below root, so walkDepth == 2 when we hit the root ancestor.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/" + docID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"block": map[string]interface{}{
					"block_id": docID,
					"children": []interface{}{"blk_section", "blk_other"},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "mcp.feishu.cn/mcp",
		Body: buildLocateDocMCPResponse([]map[string]interface{}{
			{"anchor_block_id": "blk_grandchild", "parent_block_id": "blk_section_child"},
		}),
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/blk_section_child",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"block": map[string]interface{}{
					"block_id":  "blk_section_child",
					"parent_id": "blk_section",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/" + docID + "/children",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"children": []interface{}{
					map[string]interface{}{"block_id": "blk_new", "block_type": 27, "image": map[string]interface{}{}},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "ftok_nested_log"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/batch_update",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	writeSizedDocTestFile(t, "img.png", 100)

	err := mountAndRunDocs(t, DocMediaInsert, []string{
		"+media-insert",
		"--doc", docID,
		"--file", "img.png",
		"--selection-with-ellipsis", "nested content",
		"--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	stderrOut := stderr.String()
	if !strings.Contains(stderrOut, "nested") || !strings.Contains(stderrOut, "top-level ancestor") {
		t.Errorf("stderr missing nested-anchor note; got:\n%s", stderrOut)
	}
}

// TestLocateInsertIndexCycleDetection verifies that a malformed parent chain
// (blk_x.parent = blk_y and blk_y.parent = blk_x, neither reachable from root)
// does not spin the locate-doc walk forever. The `visited` map must break the
// cycle, and the user must see the "not reachable from document root" error
// rather than the process hanging. Without this test, a regression that broke
// cycle protection would only surface in production with a stalled CLI.
func TestLocateInsertIndexCycleDetection(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("locate-cycle-app"))

	docID := "doxcnCYCLE"
	// Root has unrelated children — neither blk_x nor blk_y reach root.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/" + docID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"block": map[string]interface{}{
					"block_id": docID,
					"children": []interface{}{"blk_unrelated_a", "blk_unrelated_b"},
				},
			},
		},
	})
	// locate-doc hints parent_block_id = blk_y for anchor blk_x (first hop consumed).
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "mcp.feishu.cn/mcp",
		Body: buildLocateDocMCPResponse([]map[string]interface{}{
			{"anchor_block_id": "blk_x", "parent_block_id": "blk_y"},
		}),
	})
	// blk_y claims blk_x as parent — closes the cycle. The walk must land here
	// exactly once before visited[blk_x] triggers a break.
	blkYStub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/" + docID + "/blocks/blk_y",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"block": map[string]interface{}{
					"block_id":  "blk_y",
					"parent_id": "blk_x",
				},
			},
		},
	}
	reg.Register(blkYStub)

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	writeSizedDocTestFile(t, "img.png", 100)

	err := mountAndRunDocs(t, DocMediaInsert, []string{
		"+media-insert",
		"--doc", docID,
		"--file", "img.png",
		"--selection-with-ellipsis", "cyclic anchor",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected 'block_not_reachable' error from cyclic parent chain; got nil")
	}
	if !strings.Contains(err.Error(), "not reachable") && !strings.Contains(err.Error(), "block_not_reachable") {
		t.Fatalf("unexpected error — want cycle-bounded 'not reachable', got: %v", err)
	}
	// blk_y should be fetched exactly once. Registering just one stub for it
	// already enforces an upper bound (httpmock errors on extra calls), so if
	// the walk looped more than once the test harness would fail differently.
	if blkYStub.CapturedHeaders == nil && blkYStub.CapturedBody == nil {
		t.Errorf("expected the walk to fetch blk_y once; stub was not hit")
	}
}
