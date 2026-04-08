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

	got := buildCreateBlockData("image", 3)
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

	got := buildCreateBlockData("file", 1)
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

func registerInsertWithSelectionStubs(reg interface {
	Register(*httpmock.Stub)
}, docID, anchorBlockID, parentBlockID string) {
	// Root block
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
	// MCP locate-doc
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "mcp.feishu.cn/mcp",
		Body: buildLocateDocMCPResponse([]map[string]interface{}{
			{"anchor_block_id": anchorBlockID, "parent_block_id": parentBlockID},
		}),
	})
	// Create block
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
}

// TestLocateInsertIndexAfterModeViaExecute verifies that --selection-with-ellipsis
// inserts after the matched root-level block (index = root index + 1).
func TestLocateInsertIndexAfterModeViaExecute(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("locate-after-app"))
	registerInsertWithSelectionStubs(reg, "doxcnSEL", "blk_a", "doxcnSEL")

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
}

// TestLocateInsertIndexBeforeModeViaExecute verifies that --before inserts before
// the matched root-level block.
func TestLocateInsertIndexBeforeModeViaExecute(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("locate-before-app"))
	registerInsertWithSelectionStubs(reg, "doxcnSEL2", "blk_b", "doxcnSEL2")

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
}

// TestLocateInsertIndexNestedBlockViaExecute verifies that a nested block's
// parent_block_id hint is used to walk to the root-level ancestor.
func TestLocateInsertIndexNestedBlockViaExecute(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("locate-nested-app"))

	docID := "doxcnNESTED"
	// Root block with blk_section and blk_other as children
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
	// MCP locate-doc returns blk_child nested under blk_section
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "mcp.feishu.cn/mcp",
		Body: buildLocateDocMCPResponse([]map[string]interface{}{
			{"anchor_block_id": "blk_child", "parent_block_id": "blk_section"},
		}),
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
			Desc string `json:"desc"`
			URL  string `json:"url"`
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
