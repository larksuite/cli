// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"reflect"
	"testing"
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

func TestExtractBlockPlainTextParagraph(t *testing.T) {
	t.Parallel()

	block := map[string]interface{}{
		"block_type": 2,
		"text": map[string]interface{}{
			"elements": []interface{}{
				map[string]interface{}{"text_run": map[string]interface{}{"content": "Hello "}},
				map[string]interface{}{"text_run": map[string]interface{}{"content": "World"}},
			},
		},
	}
	got := extractBlockPlainText(block)
	if got != "Hello World" {
		t.Fatalf("extractBlockPlainText() = %q, want %q", got, "Hello World")
	}
}

func TestExtractBlockPlainTextHeading(t *testing.T) {
	t.Parallel()

	block := map[string]interface{}{
		"block_type": 3,
		"heading1": map[string]interface{}{
			"elements": []interface{}{
				map[string]interface{}{"text_run": map[string]interface{}{"content": "My Section"}},
			},
		},
	}
	got := extractBlockPlainText(block)
	if got != "My Section" {
		t.Fatalf("extractBlockPlainText() = %q, want %q", got, "My Section")
	}
}

func TestExtractBlockPlainTextBulletOrderedTodo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key     string
		content string
	}{
		{"bullet", "太空山"},
		{"ordered", "第一步操作"},
		{"todo", "完成任务"},
	}
	for _, tc := range cases {
		block := map[string]interface{}{
			"block_type": 0,
			tc.key: map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{"text_run": map[string]interface{}{"content": tc.content}},
				},
			},
		}
		got := extractBlockPlainText(block)
		if got != tc.content {
			t.Errorf("extractBlockPlainText(%q) = %q, want %q", tc.key, got, tc.content)
		}
	}
}

func TestExtractBlockPlainTextEmpty(t *testing.T) {
	t.Parallel()

	block := map[string]interface{}{"block_type": 27, "image": map[string]interface{}{}}
	if got := extractBlockPlainText(block); got != "" {
		t.Fatalf("extractBlockPlainText(image) = %q, want empty", got)
	}
}

func TestFindInsertIndexByKeywordFindsAfterBlock(t *testing.T) {
	t.Parallel()

	blocks := []map[string]interface{}{
		{
			"block_id":  "root",
			"parent_id": "",
		},
		{
			"block_id":  "blk_a",
			"parent_id": "root",
			"heading1": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{"text_run": map[string]interface{}{"content": "Introduction"}},
				},
			},
		},
		{
			"block_id":  "blk_b",
			"parent_id": "root",
			"heading1": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{"text_run": map[string]interface{}{"content": "Architecture"}},
				},
			},
		},
	}
	rootChildren := []interface{}{"blk_a", "blk_b"}

	idx, err := findInsertIndexByKeyword(blocks, rootChildren, "Introduction", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != 1 {
		t.Fatalf("findInsertIndexByKeyword() = %d, want 1", idx)
	}
}

func TestFindInsertIndexByKeywordCaseInsensitive(t *testing.T) {
	t.Parallel()

	blocks := []map[string]interface{}{
		{
			"block_id":  "blk_a",
			"parent_id": "root",
			"heading2": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{"text_run": map[string]interface{}{"content": "Core Architecture"}},
				},
			},
		},
	}
	rootChildren := []interface{}{"blk_a"}

	idx, err := findInsertIndexByKeyword(blocks, rootChildren, "core architecture", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != 1 {
		t.Fatalf("findInsertIndexByKeyword() = %d, want 1", idx)
	}
}

func TestFindInsertIndexByKeywordNestedBlock(t *testing.T) {
	t.Parallel()

	// Nested block: blk_child is inside blk_section (root child)
	blocks := []map[string]interface{}{
		{
			"block_id":  "blk_section",
			"parent_id": "root",
			"heading1": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{"text_run": map[string]interface{}{"content": "Section"}},
				},
			},
		},
		{
			"block_id":  "blk_child",
			"parent_id": "blk_section",
			"text": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{"text_run": map[string]interface{}{"content": "nested content here"}},
				},
			},
		},
	}
	rootChildren := []interface{}{"blk_section"}

	// Matching a nested block should insert after its root-level ancestor.
	idx, err := findInsertIndexByKeyword(blocks, rootChildren, "nested content", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != 1 {
		t.Fatalf("findInsertIndexByKeyword() nested = %d, want 1", idx)
	}
}

// TestFindInsertIndexByKeywordDuplicateUsesFirst verifies that when the same
// keyword appears in multiple blocks, the function always anchors to the
// first matching block in document order (the slice iteration order of blocks).
func TestFindInsertIndexByKeywordDuplicateUsesFirst(t *testing.T) {
	t.Parallel()

	// Three root-level blocks, all containing "overview".
	// Document order: blk_a (index 0) → blk_b (index 1) → blk_c (index 2).
	blocks := []map[string]interface{}{
		{
			"block_id":  "blk_a",
			"parent_id": "root",
			"text": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{"text_run": map[string]interface{}{"content": "overview: section one"}},
				},
			},
		},
		{
			"block_id":  "blk_b",
			"parent_id": "root",
			"text": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{"text_run": map[string]interface{}{"content": "overview: section two"}},
				},
			},
		},
		{
			"block_id":  "blk_c",
			"parent_id": "root",
			"text": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{"text_run": map[string]interface{}{"content": "overview: section three"}},
				},
			},
		},
	}
	rootChildren := []interface{}{"blk_a", "blk_b", "blk_c"}

	// --after-keyword: should insert after blk_a (index 0 → return 1), not blk_b or blk_c.
	afterIdx, err := findInsertIndexByKeyword(blocks, rootChildren, "overview", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if afterIdx != 1 {
		t.Fatalf("after: got index %d, want 1 (after first match blk_a)", afterIdx)
	}

	// --before-keyword: should insert before blk_a (index 0 → return 0).
	beforeIdx, err := findInsertIndexByKeyword(blocks, rootChildren, "overview", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if beforeIdx != 0 {
		t.Fatalf("before: got index %d, want 0 (before first match blk_a)", beforeIdx)
	}
}

// TestFindInsertIndexByKeywordDuplicateNestedAndTopLevel verifies that when the
// keyword appears in both a nested block (inside blk_a) and a later top-level
// block (blk_b), the function uses the earlier document-order match — which
// resolves upward to blk_a.
func TestFindInsertIndexByKeywordDuplicateNestedAndTopLevel(t *testing.T) {
	t.Parallel()

	blocks := []map[string]interface{}{
		// blk_a is a top-level section
		{
			"block_id":  "blk_a",
			"parent_id": "root",
			"heading1": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{"text_run": map[string]interface{}{"content": "Section A"}},
				},
			},
		},
		// blk_child is nested inside blk_a and contains the keyword first
		{
			"block_id":  "blk_child",
			"parent_id": "blk_a",
			"text": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{"text_run": map[string]interface{}{"content": "architecture diagram"}},
				},
			},
		},
		// blk_b is a second top-level block that also contains the keyword
		{
			"block_id":  "blk_b",
			"parent_id": "root",
			"text": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{"text_run": map[string]interface{}{"content": "architecture diagram"}},
				},
			},
		},
	}
	rootChildren := []interface{}{"blk_a", "blk_b"}

	// blk_child appears before blk_b in blocks slice → first match is blk_child,
	// which walks up to blk_a (rootChildren index 0).
	// after → insert at index 1 (after blk_a)
	afterIdx, err := findInsertIndexByKeyword(blocks, rootChildren, "architecture diagram", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if afterIdx != 1 {
		t.Fatalf("after: got %d, want 1 (after blk_a, ancestor of first-matched blk_child)", afterIdx)
	}

	// before → insert at index 0 (before blk_a)
	beforeIdx, err := findInsertIndexByKeyword(blocks, rootChildren, "architecture diagram", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if beforeIdx != 0 {
		t.Fatalf("before: got %d, want 0 (before blk_a, ancestor of first-matched blk_child)", beforeIdx)
	}
}

func TestFindInsertIndexByKeywordNotFound(t *testing.T) {
	t.Parallel()

	blocks := []map[string]interface{}{
		{
			"block_id":  "blk_a",
			"parent_id": "root",
			"text": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{"text_run": map[string]interface{}{"content": "hello world"}},
				},
			},
		},
	}
	rootChildren := []interface{}{"blk_a"}

	_, err := findInsertIndexByKeyword(blocks, rootChildren, "nonexistent keyword", false)
	if err == nil {
		t.Fatal("expected error for missing keyword, got nil")
	}
}

func TestFindInsertIndexByKeywordBeforeMode(t *testing.T) {
	t.Parallel()

	blocks := []map[string]interface{}{
		{
			"block_id":  "blk_a",
			"parent_id": "root",
			"heading1": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{"text_run": map[string]interface{}{"content": "Introduction"}},
				},
			},
		},
		{
			"block_id":  "blk_b",
			"parent_id": "root",
			"heading1": map[string]interface{}{
				"elements": []interface{}{
					map[string]interface{}{"text_run": map[string]interface{}{"content": "Architecture"}},
				},
			},
		},
	}
	rootChildren := []interface{}{"blk_a", "blk_b"}

	// before=true: should return index 1 (before blk_b, which is rootChildren[1])
	idx, err := findInsertIndexByKeyword(blocks, rootChildren, "Architecture", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != 1 {
		t.Fatalf("findInsertIndexByKeyword(before) = %d, want 1", idx)
	}

	// before=false: should return index 2 (after blk_b)
	idx, err = findInsertIndexByKeyword(blocks, rootChildren, "Architecture", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idx != 2 {
		t.Fatalf("findInsertIndexByKeyword(after) = %d, want 2", idx)
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
