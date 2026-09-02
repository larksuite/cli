// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	drivee2e "github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSlidesMediaUploadDownloadLiveE2E(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	title := "slides-media-e2e-" + suffix
	slideXML := `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data><shape type="text" topLeftX="80" topLeftY="80" width="800" height="120"><content textType="title"><p>` + title + `</p></content></shape></data></slide>`
	slidesJSON, err := json.Marshal([]string{slideXML})
	require.NoError(t, err)

	createResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+create",
			"--title", title,
			"--slides", string(slidesJSON),
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	createResult.AssertExitCode(t, 0)
	createResult.AssertStdoutStatus(t, true)

	presentationID := gjson.Get(createResult.Stdout, "data.xml_presentation_id").String()
	require.NotEmpty(t, presentationID, "stdout:\n%s", createResult.Stdout)
	parentT.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		deleteResult, deleteErr := drivee2e.DeleteDriveResourceAndVerify(cleanupCtx, presentationID, "slides", "bot")
		clie2e.ReportCleanupFailure(parentT, "delete presentation "+presentationID, deleteResult, deleteErr)
	})

	workDir := t.TempDir()

	pngPath := filepath.Join(workDir, "test.png")
	pngBytes := generateTestPNG(t)
	require.NoError(t, os.WriteFile(pngPath, pngBytes, 0o600))

	uploadResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+media-upload",
			"--presentation", presentationID,
			"--file", "test.png",
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
	})
	require.NoError(t, err)
	uploadResult.AssertExitCode(t, 0)
	uploadResult.AssertStdoutStatus(t, true)

	fileToken := gjson.Get(uploadResult.Stdout, "data.file_token").String()
	require.NotEmpty(t, fileToken, "stdout:\n%s", uploadResult.Stdout)
	require.Equal(t, presentationID, gjson.Get(uploadResult.Stdout, "data.presentation_id").String(),
		"upload must resolve to the created presentation; stdout:\n%s", uploadResult.Stdout)
	require.Equal(t, "test.png", gjson.Get(uploadResult.Stdout, "data.file_name").String(),
		"stdout:\n%s", uploadResult.Stdout)
	require.Equal(t, int64(len(pngBytes)), gjson.Get(uploadResult.Stdout, "data.size").Int(),
		"uploaded size must match local file; stdout:\n%s", uploadResult.Stdout)

	downloadDir := filepath.Join(workDir, "downloads")
	downloadResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+media-download",
			"--file-token", fileToken,
			"--output-dir", "downloads",
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
	})
	require.NoError(t, err)
	downloadResult.AssertExitCode(t, 0)
	downloadResult.AssertStdoutStatus(t, true)

	require.Equal(t, fileToken, gjson.Get(downloadResult.Stdout, "data.file_token").String(),
		"stdout:\n%s", downloadResult.Stdout)
	downloadedPath := gjson.Get(downloadResult.Stdout, "data.path").String()
	require.NotEmpty(t, downloadedPath, "stdout:\n%s", downloadResult.Stdout)
	require.True(t, filepath.IsAbs(downloadedPath), "path must be absolute: %s", downloadedPath)
	requireScreenshotPathUnderDir(t, downloadedPath, downloadDir)
	require.Equal(t, "downloads", gjson.Get(downloadResult.Stdout, "data.output_dir").String(),
		"stdout:\n%s", downloadResult.Stdout)

	downloadedBytes, err := os.ReadFile(downloadedPath)
	require.NoError(t, err)
	require.NotEmpty(t, downloadedBytes, "downloaded file must not be empty")
	require.Equal(t, int64(len(downloadedBytes)), gjson.Get(downloadResult.Stdout, "data.size").Int(),
		"reported size must match actual bytes; stdout:\n%s", downloadResult.Stdout)
	require.Equal(t, pngBytes, downloadedBytes,
		"downloaded bytes must match uploaded PNG (Drive media upload/download is a binary round-trip)")

	contentType := gjson.Get(downloadResult.Stdout, "data.content_type").String()
	require.Contains(t, contentType, "image/", "content_type must indicate an image: %s", contentType)

	source := gjson.Get(downloadResult.Stdout, "data.source").String()
	require.Contains(t, []string{"download", "preview"}, source,
		"source must be either direct download or preview fallback: %s", source)
}

func generateTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if (x+y)%2 == 0 {
				img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			} else {
				img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
			}
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}
