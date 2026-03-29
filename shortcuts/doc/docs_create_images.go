// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var imageRefRegex = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)\)`)

var allowedImageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".bmp": true, ".webp": true,
}

type imageRef struct {
	fullMatch string
	path      string
}

// parseImageRefs extracts all markdown image references from the given text.
func parseImageRefs(markdown string) []imageRef {
	matches := imageRefRegex.FindAllStringSubmatch(markdown, -1)
	var refs []imageRef
	for _, m := range matches {
		refs = append(refs, imageRef{
			fullMatch: m[0],
			path:      m[1],
		})
	}
	return refs
}

// isLocalPath returns true if the path is not an HTTP(S) URL.
func isLocalPath(p string) bool {
	return !strings.HasPrefix(p, "http://") && !strings.HasPrefix(p, "https://")
}

// hasLocalImages checks whether the markdown contains any local image references.
func hasLocalImages(markdown string) bool {
	for _, ref := range parseImageRefs(markdown) {
		if isLocalPath(ref.path) {
			return true
		}
	}
	return false
}

// safeImagePath resolves an image path relative to baseDir and validates it.
// It rejects absolute paths, prevents traversal outside baseDir, resolves
// symlinks, and checks the file exists.
func safeImagePath(imgPath, baseDir string) (string, error) {
	if filepath.IsAbs(imgPath) {
		return "", fmt.Errorf("absolute image path not allowed: %s", imgPath)
	}
	if err := validate.RejectControlChars(imgPath, "image path"); err != nil {
		return "", err
	}

	cleaned := filepath.Clean(imgPath)
	resolved := filepath.Join(baseDir, cleaned)

	// Resolve symlinks for the actual path
	real, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %s: %w", imgPath, err)
	}

	// Ensure the resolved path stays under baseDir
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("cannot resolve base directory: %w", err)
	}
	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return "", fmt.Errorf("cannot resolve base directory: %w", err)
	}

	rel, err := filepath.Rel(realBase, real)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("image path %q resolves outside base directory", imgPath)
	}

	return real, nil
}

// validateImageFile checks that the file has an allowed extension and is within size limits.
func validateImageFile(path string) (os.FileInfo, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if !allowedImageExts[ext] {
		return nil, fmt.Errorf("unsupported image format %q (allowed: jpg, jpeg, png, gif, bmp, webp)", ext)
	}

	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if stat.Size() > maxFileSize {
		return nil, fmt.Errorf("file %.1fMB exceeds 20MB limit", float64(stat.Size())/1024/1024)
	}
	return stat, nil
}

// processMarkdownImages implements two-phase document creation:
// 1. Create document with title only
// 2. Upload local images and replace paths with file tokens
// 3. Update document with processed markdown (overwrite mode)
func processMarkdownImages(ctx context.Context, runtime *common.RuntimeContext, markdown, baseDir string, createArgs map[string]interface{}) (map[string]interface{}, error) {
	// Phase 1: Create document with minimal content
	titleArgs := make(map[string]interface{})
	for k, v := range createArgs {
		if k != "markdown" {
			titleArgs[k] = v
		}
	}
	titleArgs["markdown"] = " "

	result, err := common.CallMCPTool(runtime, "create-doc", titleArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to create document: %w", err)
	}

	documentID := extractDocumentID(result)
	if documentID == "" {
		return nil, fmt.Errorf("create-doc did not return document_id")
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Document created: %s, uploading local images...\n", common.MaskToken(documentID))

	// Upload images and collect replacements
	processedMarkdown, uploadCount, err := uploadAndReplaceImages(ctx, runtime, markdown, baseDir, documentID)
	if err != nil {
		return result, fmt.Errorf("image upload failed: %w", err)
	}

	if uploadCount == 0 {
		// No images were uploaded, just update with original markdown
		processedMarkdown = markdown
	}

	// Phase 2: Update document with processed markdown
	updateArgs := map[string]interface{}{
		"doc_id":   documentID,
		"mode":     "overwrite",
		"markdown": processedMarkdown,
	}

	_, err = common.CallMCPTool(runtime, "update-doc", updateArgs)
	if err != nil {
		return result, fmt.Errorf("failed to update document content: %w", err)
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Document content updated with %d uploaded image(s)\n", uploadCount)
	return result, nil
}

// uploadAndReplaceImages uploads local images and returns the markdown with paths replaced.
func uploadAndReplaceImages(ctx context.Context, runtime *common.RuntimeContext, markdown, baseDir, documentID string) (string, int, error) {
	refs := parseImageRefs(markdown)
	replacements := make(map[string]string) // path -> file_token (dedup)

	// Get document root block
	rootData, err := runtime.CallAPI("GET",
		fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s",
			validate.EncodePathSegment(documentID), validate.EncodePathSegment(documentID)),
		nil, nil)
	if err != nil {
		return markdown, 0, fmt.Errorf("failed to get document root: %w", err)
	}

	parentBlockID, insertIndex, err := extractAppendTarget(rootData, documentID)
	if err != nil {
		return markdown, 0, err
	}

	for _, ref := range refs {
		if !isLocalPath(ref.path) {
			continue
		}

		// Skip duplicates
		if _, ok := replacements[ref.path]; ok {
			continue
		}

		resolved, err := safeImagePath(ref.path, baseDir)
		if err != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "Warning: skipping image %s: %v\n", ref.path, err)
			continue
		}

		if _, err := validateImageFile(resolved); err != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "Warning: skipping image %s: %v\n", ref.path, err)
			continue
		}

		// Create empty image block as upload target
		createData, err := runtime.CallAPI("POST",
			fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s/children",
				validate.EncodePathSegment(documentID), validate.EncodePathSegment(parentBlockID)),
			nil, buildCreateBlockData("image", insertIndex))
		if err != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "Warning: failed to create block for %s: %v\n", ref.path, err)
			continue
		}

		_, uploadParentNode, _ := extractCreatedBlockTargets(createData, "image")
		if uploadParentNode == "" {
			fmt.Fprintf(runtime.IO().ErrOut, "Warning: failed to create block for %s\n", ref.path)
			continue
		}
		insertIndex++

		// Upload file
		fileName := filepath.Base(resolved)
		fileToken, err := uploadMediaFile(ctx, runtime, resolved, fileName, "image", uploadParentNode, documentID)
		if err != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "Warning: failed to upload %s: %v\n", ref.path, err)
			continue
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Uploaded: %s -> %s\n", ref.path, fileToken)
		replacements[ref.path] = fileToken
	}

	// Replace paths in markdown
	processed := markdown
	for oldPath, fileToken := range replacements {
		processed = strings.ReplaceAll(processed, "]("+oldPath+")", "]("+fileToken+")")
	}

	return processed, len(replacements), nil
}

// extractDocumentID tries to get document_id from a create-doc MCP result.
func extractDocumentID(result map[string]interface{}) string {
	if id := common.GetString(result, "document_id"); id != "" {
		return id
	}
	if id := common.GetString(result, "url"); id != "" {
		// Try to extract from URL
		if ref, err := parseDocumentRef(id); err == nil {
			return ref.Token
		}
	}
	return ""
}
