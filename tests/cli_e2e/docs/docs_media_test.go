// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDocs_MediaWorkflow tests the complete media workflow: insert and download.
func TestDocs_MediaWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	docTitle := "lark-cli-e2e-media-" + suffix

	var docToken string
	var fileToken string

	// Create a temp image file for testing (relative path for CLI safety)
	tmpFile := "test-image-" + suffix + ".png"
	tmpOutput := "test-image-downloaded-" + suffix + ".png"
	t.Cleanup(func() {
		os.Remove(tmpFile)
		os.Remove(tmpOutput)
	})

	t.Run("create-temp-image", func(t *testing.T) {
		// Create a minimal PNG file (1x1 transparent pixel)
		pngData := []byte{
			0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
			0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR length + type
			0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // width=1, height=1
			0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, // bit depth, color type, etc.
			0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41, // IDAT length + type
			0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00, // compressed data
			0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, // end of IDAT
			0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, // IEND length + type
			0x42, 0x60, 0x82, // IEND CRC
		}
		err := os.WriteFile(tmpFile, pngData, 0644)
		require.NoError(t, err)
	})

	t.Run("create-document", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+create",
				"--title", docTitle,
				"--markdown", "# Test Document",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		docToken = gjson.Get(result.Stdout, "data.doc_id").String()
		require.NotEmpty(t, docToken, "stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			// best-effort cleanup
		})
	})

	t.Run("media-insert", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before media-insert")
		require.FileExists(t, tmpFile)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+media-insert",
				"--doc", docToken,
				"--file", tmpFile,
				"--type", "image",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		fileToken = gjson.Get(result.Stdout, "data.file_token").String()
		require.NotEmpty(t, fileToken, "file_token should be returned, stdout:\n%s", result.Stdout)
	})

	t.Run("media-download", func(t *testing.T) {
		require.NotEmpty(t, fileToken, "file_token should be available from media-insert")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+media-download",
				"--token", fileToken,
				"--output", tmpOutput,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		// Verify the file was downloaded
		downloadPath := gjson.Get(result.Stdout, "data.download_path").String()
		if downloadPath == "" {
			downloadPath = tmpOutput
		}
		absPath, err := filepath.Abs(downloadPath)
		require.NoError(t, err)
		assert.FileExists(t, absPath, "downloaded file should exist at %s", absPath)
	})
}