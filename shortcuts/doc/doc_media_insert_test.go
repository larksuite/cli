// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

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

	blockID, index, err := extractAppendTarget(rootData, "fallback")
	if err != nil {
		t.Fatalf("extractAppendTarget() unexpected error: %v", err)
	}
	if blockID != "root_block" {
		t.Fatalf("extractAppendTarget() blockID = %q, want %q", blockID, "root_block")
	}
	if index != 3 {
		t.Fatalf("extractAppendTarget() index = %d, want 3", index)
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
	cmd.Flags().String("doc", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("file-view", "", "")
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
