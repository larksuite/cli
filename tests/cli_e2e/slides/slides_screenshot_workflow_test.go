// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/vfs"
	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	drivee2e "github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSlidesScreenshotAliasesLiveE2E(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	title := "slides-screenshot-alias-e2e-" + suffix
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
	slideID := gjson.Get(createResult.Stdout, "data.slide_ids.0").String()
	require.NotEmpty(t, slideID, "stdout:\n%s", createResult.Stdout)

	workDir := t.TempDir()
	screenshotResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+screenshot",
			"--presentation-id", presentationID,
			"--slides", slideID,
			"--output-dir", "shots",
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
	})
	require.NoError(t, err)
	screenshotResult.AssertExitCode(t, 0)
	screenshotResult.AssertStdoutStatus(t, true)

	require.Equal(t, presentationID, gjson.Get(screenshotResult.Stdout, "data.xml_presentation_id").String(), "stdout:\n%s", screenshotResult.Stdout)
	require.Equal(t, "shots", gjson.Get(screenshotResult.Stdout, "data.output_dir").String(), "stdout:\n%s", screenshotResult.Stdout)
	screenshots := gjson.Get(screenshotResult.Stdout, "data.screenshots").Array()
	require.Len(t, screenshots, 1, "stdout:\n%s", screenshotResult.Stdout)
	require.Equal(t, slideID, screenshots[0].Get("slide_id").String(), "stdout:\n%s", screenshotResult.Stdout)
	require.Equal(t, int64(1), screenshots[0].Get("slide_number").Int(), "stdout:\n%s", screenshotResult.Stdout)

	imagePath := screenshots[0].Get("path").String()
	require.True(t, filepath.IsAbs(imagePath), "path must be absolute: %s", imagePath)
	requireScreenshotPathUnderDir(t, imagePath, filepath.Join(workDir, "shots"))

	imageFile, err := vfs.Open(imagePath)
	require.NoError(t, err)
	defer imageFile.Close()
	config, format, err := image.DecodeConfig(imageFile)
	require.NoError(t, err)
	require.Positive(t, config.Width)
	require.Positive(t, config.Height)
	require.Contains(t, []string{"jpeg", "png"}, format)
	require.Equal(t, format, screenshots[0].Get("format").String(), "stdout:\n%s", screenshotResult.Stdout)

	info, err := imageFile.Stat()
	require.NoError(t, err)
	require.Equal(t, info.Size(), screenshots[0].Get("size").Int(), "stdout:\n%s", screenshotResult.Stdout)
	require.False(t, screenshots[0].Get("data").Exists(), "stdout must not expose screenshot payload: %s", screenshotResult.Stdout)
	require.NotContains(t, screenshotResult.Stdout, "data:image/", "stdout must not expose screenshot payload")

	requestedExt := ".png"
	if format == "png" {
		requestedExt = ".jpg"
	}
	requestedOutput := filepath.Join("fixed", "cover"+requestedExt)
	fixedOutputResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+screenshot",
			"--presentation", presentationID,
			"--slide-id", slideID,
			"--output", requestedOutput,
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
	})
	require.NoError(t, err)
	fixedOutputResult.AssertExitCode(t, 0)
	fixedOutputResult.AssertStdoutStatus(t, true)

	actualFormat := gjson.Get(fixedOutputResult.Stdout, "data.screenshots.0.format").String()
	wantExt := ".png"
	if actualFormat == "jpeg" {
		wantExt = ".jpg"
	}
	wantOutput := filepath.Join(workDir, "fixed", "cover"+wantExt)
	wantOutput, err = vfs.EvalSymlinks(wantOutput)
	require.NoError(t, err)
	require.Equal(t, wantOutput, gjson.Get(fixedOutputResult.Stdout, "data.output").String(), fixedOutputResult.Stdout)
	require.False(t, gjson.Get(fixedOutputResult.Stdout, "data.output_dir").Exists(), fixedOutputResult.Stdout)
	require.Equal(t, wantOutput, gjson.Get(fixedOutputResult.Stdout, "data.screenshots.0.path").String(), fixedOutputResult.Stdout)
	if requestedExt != wantExt {
		require.Equal(t, requestedOutput, gjson.Get(fixedOutputResult.Stdout, "data.requested_output").String(), fixedOutputResult.Stdout)
		require.True(t, gjson.Get(fixedOutputResult.Stdout, "data.output_adjusted").Bool(), fixedOutputResult.Stdout)
	}
	fixedImageFile, err := vfs.Open(wantOutput)
	require.NoError(t, err)
	defer fixedImageFile.Close()
	_, decodedFormat, err := image.DecodeConfig(fixedImageFile)
	require.NoError(t, err)
	require.Equal(t, actualFormat, decodedFormat, fixedOutputResult.Stdout)
}

func requireScreenshotPathUnderDir(t *testing.T, path, dir string) {
	t.Helper()
	canonicalPath, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	canonicalDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	rel, err := filepath.Rel(canonicalDir, canonicalPath)
	require.NoError(t, err)
	require.False(t, rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)), "path %s escapes %s", path, dir)
}
