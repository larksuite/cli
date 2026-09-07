// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/shortcuts/common"
)

func TestDetectImageDimensionsSupportsBMPAndTIFF(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		image  []byte
		width  int
		height int
	}{
		{name: "BMP", image: testBMPImage(2, 3), width: 2, height: 3},
		{name: "TIFF", image: testTIFFImage(4, 5), width: 4, height: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, height, err := detectImageDimensions(bytes.NewReader(tt.image))
			if err != nil {
				t.Fatalf("detectImageDimensions() error: %v", err)
			}
			if width != tt.width || height != tt.height {
				t.Fatalf("dimensions = %dx%d, want %dx%d", width, height, tt.width, tt.height)
			}
		})
	}
}

func TestDocumentImageMIMETypesAndExtensions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		contentType string
		ext         string
	}{
		{contentType: "image/bmp", ext: ".bmp"},
		{contentType: "image/gif", ext: ".gif"},
		{contentType: "image/jpeg", ext: ".jpg"},
		{contentType: "image/png", ext: ".png"},
		{contentType: "image/tiff", ext: ".tiff"},
		{contentType: "image/webp", ext: ".webp"},
	}
	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			if got := docCoverAllowedContentTypes[tt.contentType]; got != tt.ext {
				t.Fatalf("allowed extension = %q, want %q", got, tt.ext)
			}
			resolution := docMediaExtensionByContentType(tt.contentType + "; charset=binary")
			if resolution == nil || resolution.Ext != tt.ext {
				t.Fatalf("extension resolution = %#v, want %q", resolution, tt.ext)
			}
		})
	}
}

func testBMPImage(width, height int) []byte {
	rowSize := (width*3 + 3) &^ 3
	pixelBytes := rowSize * height
	data := make([]byte, 54+pixelBytes)
	copy(data[:2], "BM")
	binary.LittleEndian.PutUint32(data[2:6], uint32(len(data)))
	binary.LittleEndian.PutUint32(data[10:14], 54)
	binary.LittleEndian.PutUint32(data[14:18], 40)
	binary.LittleEndian.PutUint32(data[18:22], uint32(width))
	binary.LittleEndian.PutUint32(data[22:26], uint32(height))
	binary.LittleEndian.PutUint16(data[26:28], 1)
	binary.LittleEndian.PutUint16(data[28:30], 24)
	binary.LittleEndian.PutUint32(data[34:38], uint32(pixelBytes))
	return data
}

func testTIFFImage(width, height int) []byte {
	const entryCount = 4
	data := make([]byte, 8+2+entryCount*12)
	copy(data[:4], []byte{'I', 'I', 42, 0})
	binary.LittleEndian.PutUint32(data[4:8], 8)
	binary.LittleEndian.PutUint16(data[8:10], entryCount)

	entries := []struct {
		tag, fieldType uint16
		value          uint32
	}{
		{tag: 256, fieldType: 4, value: uint32(width)},  // ImageWidth, LONG
		{tag: 257, fieldType: 4, value: uint32(height)}, // ImageLength, LONG
		{tag: 258, fieldType: 3, value: 8},              // BitsPerSample, SHORT
		{tag: 262, fieldType: 3, value: 1},              // PhotometricInterpretation, SHORT
	}
	for i, entry := range entries {
		offset := 10 + i*12
		binary.LittleEndian.PutUint16(data[offset:offset+2], entry.tag)
		binary.LittleEndian.PutUint16(data[offset+2:offset+4], entry.fieldType)
		binary.LittleEndian.PutUint32(data[offset+4:offset+8], 1)
		binary.LittleEndian.PutUint32(data[offset+8:offset+12], entry.value)
	}
	return data
}

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

	got := buildBatchUpdateData("blk_1", "image", "file_tok", "center", "caption text", 0, 0)
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

	got := buildBatchUpdateData("blk_2", "file", "file_tok", "", "", 0, 0)
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

func TestBuildBatchUpdateDataForImageWithWidthHeight(t *testing.T) {
	t.Parallel()

	got := buildBatchUpdateData("blk_1", "image", "file_tok", "center", "caption text", 800, 447)
	want := map[string]interface{}{
		"requests": []interface{}{
			map[string]interface{}{
				"block_id": "blk_1",
				"replace_image": map[string]interface{}{
					"token":   "file_tok",
					"width":   800,
					"height":  447,
					"align":   2,
					"caption": map[string]interface{}{"content": "caption text"},
				},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildBatchUpdateData(image, 800, 447) = %#v, want %#v", got, want)
	}
}

func TestBuildBatchUpdateDataForFileIgnoresWidthHeight(t *testing.T) {
	t.Parallel()

	got := buildBatchUpdateData("blk_2", "file", "file_tok", "", "", 800, 600)
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
		t.Fatalf("buildBatchUpdateData(file, 800, 600) = %#v, want %#v", got, want)
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

func TestDocMediaInsertDoesNotExposeTextLocationFlags(t *testing.T) {
	t.Parallel()

	for _, flag := range DocMediaInsert.Flags {
		if flag.Name == "selection-with-ellipsis" || flag.Name == "before" {
			t.Fatalf("docs +media-insert still exposes removed flag --%s", flag.Name)
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

func newMediaInsertValidateRuntimeWithSize(t *testing.T, doc, mediaType string, width, height int, setWidth, setHeight bool) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "docs +media-insert"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().Bool("from-clipboard", false, "")
	cmd.Flags().String("doc", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("file-view", "", "")
	cmd.Flags().Int("width", 0, "")
	cmd.Flags().Int("height", 0, "")
	if err := cmd.Flags().Set("file", "dummy.bin"); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := cmd.Flags().Set("doc", doc); err != nil {
		t.Fatalf("set --doc: %v", err)
	}
	if err := cmd.Flags().Set("type", mediaType); err != nil {
		t.Fatalf("set --type: %v", err)
	}
	if setWidth {
		if err := cmd.Flags().Set("width", fmt.Sprintf("%d", width)); err != nil {
			t.Fatalf("set --width: %v", err)
		}
	}
	if setHeight {
		if err := cmd.Flags().Set("height", fmt.Sprintf("%d", height)); err != nil {
			t.Fatalf("set --height: %v", err)
		}
	}
	return common.TestNewRuntimeContext(cmd, nil)
}

func TestDocMediaInsertValidateWidthHeightOnlyForImage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mediaType string
		width     int
		height    int
		setWidth  bool
		setHeight bool
		wantErr   string
	}{
		{
			name:      "width with file type is rejected",
			mediaType: "file",
			width:     800,
			setWidth:  true,
			wantErr:   "--width/--height only apply when --type=image",
		},
		{
			name:      "height with file type is rejected",
			mediaType: "file",
			height:    600,
			setHeight: true,
			wantErr:   "--width/--height only apply when --type=image",
		},
		{
			name:      "explicit zero width is rejected",
			mediaType: "image",
			width:     0,
			setWidth:  true,
			wantErr:   "--width must be a positive integer",
		},
		{
			name:      "negative width is rejected",
			mediaType: "image",
			width:     -1,
			setWidth:  true,
			wantErr:   "--width must be a positive integer",
		},
		{
			name:      "negative height is rejected",
			mediaType: "image",
			height:    -5,
			setHeight: true,
			wantErr:   "--height must be a positive integer",
		},
		{
			name:      "valid width with image type is accepted",
			mediaType: "image",
			width:     800,
			setWidth:  true,
		},
		{
			name:      "valid width and height with image type is accepted",
			mediaType: "image",
			width:     800,
			height:    600,
			setWidth:  true,
			setHeight: true,
		},
	}

	for _, ttTemp := range tests {
		tt := ttTemp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rt := newMediaInsertValidateRuntimeWithSize(t, "doxcnValidateSize", tt.mediaType, tt.width, tt.height, tt.setWidth, tt.setHeight)
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

func TestDocMediaInsertValidateNoWidthHeightIsValid(t *testing.T) {
	t.Parallel()

	rt := newMediaInsertValidateRuntimeWithSize(t, "doxcnNoSize", "image", 0, 0, false, false)
	err := DocMediaInsert.Validate(context.Background(), rt)
	if err != nil {
		t.Fatalf("Validate() unexpected error when neither --width nor --height passed: %v", err)
	}
}

func TestAutoAspectRatioFromWidth(t *testing.T) {
	t.Parallel()

	// Native image: 1200x800 (3:2 ratio)
	// User provides width=600 → expected height = 600 * 800 / 1200 = 400
	got := computeMissingDimension(600, 0, 1200, 800)
	wantWidth, wantHeight := 600, 400
	if got.width != wantWidth || got.height != wantHeight {
		t.Fatalf("computeMissingDimension(600, 0, 1200, 800) = (%d, %d), want (%d, %d)", got.width, got.height, wantWidth, wantHeight)
	}
}

func TestAutoAspectRatioFromHeight(t *testing.T) {
	t.Parallel()

	// Native image: 1200x800 (3:2 ratio)
	// User provides height=400 → expected width = 400 * 1200 / 800 = 600
	got := computeMissingDimension(0, 400, 1200, 800)
	wantWidth, wantHeight := 600, 400
	if got.width != wantWidth || got.height != wantHeight {
		t.Fatalf("computeMissingDimension(0, 400, 1200, 800) = (%d, %d), want (%d, %d)", got.width, got.height, wantWidth, wantHeight)
	}
}

func TestComputeMissingDimensionBothProvided(t *testing.T) {
	t.Parallel()
	got := computeMissingDimension(800, 600, 1200, 900)
	if got.width != 800 || got.height != 600 {
		t.Fatalf("computeMissingDimension(800, 600, 1200, 900) = (%d, %d), want (800, 600)", got.width, got.height)
	}
}

func TestComputeMissingDimensionNeitherProvided(t *testing.T) {
	t.Parallel()
	got := computeMissingDimension(0, 0, 1200, 900)
	if got.width != 0 || got.height != 0 {
		t.Fatalf("computeMissingDimension(0, 0, 1200, 900) = (%d, %d), want (0, 0)", got.width, got.height)
	}
}

func TestComputeMissingDimensionZeroNativeWidth(t *testing.T) {
	t.Parallel()
	got := computeMissingDimension(600, 0, 0, 800)
	if got.width != 600 || got.height != 0 {
		t.Fatalf("computeMissingDimension(600, 0, 0, 800) = (%d, %d), want (600, 0)", got.width, got.height)
	}
}

func TestComputeMissingDimensionZeroNativeHeight(t *testing.T) {
	t.Parallel()
	got := computeMissingDimension(0, 400, 1200, 0)
	if got.width != 0 || got.height != 400 {
		t.Fatalf("computeMissingDimension(0, 400, 1200, 0) = (%d, %d), want (0, 400)", got.width, got.height)
	}
}

func TestComputeMissingDimensionRounding(t *testing.T) {
	t.Parallel()
	got := computeMissingDimension(999, 0, 1000, 333)
	want := (999*333 + 500) / 1000
	if got.height != want {
		t.Fatalf("computeMissingDimension(999, 0, 1000, 333).height = %d, want %d (rounded)", got.height, want)
	}
}

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
