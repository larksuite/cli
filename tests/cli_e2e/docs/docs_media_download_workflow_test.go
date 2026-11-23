// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/vfs"
	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDocs_MediaDownloadWorkflow creates a document-owned image fixture, then
// downloads the same media through docs +media-download and verifies its bytes.
func TestDocs_MediaDownloadWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	const defaultAs = "bot"
	suffix := clie2e.GenerateSuffix()
	folderToken := drive.CreateDriveFolder(t, parentT, ctx, "lark-cli-e2e-docs-media-"+suffix, defaultAs, "")
	docToken := createDocWithRetry(t, parentT, ctx, folderToken, "lark-cli-e2e-docs-media-"+suffix, "media fixture "+suffix, defaultAs)
	workDir := t.TempDir()

	imageBytes := buildMediaDownloadFixture(t)
	fixtureName := "fixture-" + suffix + ".png"
	require.NoError(t, vfs.WriteFile(filepath.Join(workDir, fixtureName), imageBytes, 0o600))

	insertResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+media-insert",
			"--doc", docToken,
			"--file", fixtureName,
			"--type", "image",
		},
		WorkDir:   workDir,
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	insertResult.AssertExitCode(t, 0)
	insertResult.AssertStdoutStatus(t, true)

	mediaToken := gjson.Get(insertResult.Stdout, "data.file_token").String()
	require.NotEmpty(t, mediaToken, "media insert should return data.file_token; stdout:\n%s", insertResult.Stdout)

	downloadResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+media-download",
			"--token", mediaToken,
			"--output", "downloaded.png",
		},
		WorkDir:   workDir,
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	downloadResult.AssertExitCode(t, 0)
	downloadResult.AssertStdoutStatus(t, true)

	savedPath := gjson.Get(downloadResult.Stdout, "data.saved_path").String()
	require.NotEmpty(t, savedPath, "media download should return data.saved_path; stdout:\n%s", downloadResult.Stdout)
	require.Equal(t, "downloaded.png", filepath.Base(savedPath))
	relPath, err := filepath.Rel(workDir, savedPath)
	require.NoError(t, err)
	require.NotEqual(t, "..", relPath)
	require.NotContains(t, relPath, ".."+string(filepath.Separator))

	downloaded, err := vfs.ReadFile(savedPath)
	require.NoError(t, err)
	require.Equal(t, imageBytes, downloaded)
	require.Equal(t, int64(len(downloaded)), gjson.Get(downloadResult.Stdout, "data.size_bytes").Int())
	require.Contains(t, gjson.Get(downloadResult.Stdout, "data.content_type").String(), "image/")
}

func buildMediaDownloadFixture(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 0xff, A: 0xff})
	img.Set(1, 0, color.RGBA{G: 0xff, A: 0xff})
	img.Set(0, 1, color.RGBA{B: 0xff, A: 0xff})
	img.Set(1, 1, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})

	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, img))
	return encoded.Bytes()
}
