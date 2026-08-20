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

func TestSlidesScreenshotOverviewAndRegionLiveE2E(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	title := "slides-screenshot-overview-region-e2e-" + suffix
	slideXML := func(marker string) string {
		return `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data><shape type="text" topLeftX="80" topLeftY="80" width="800" height="120"><content textType="title"><p>` + marker + `</p></content></shape></data></slide>`
	}
	slidesJSON, err := json.Marshal([]string{slideXML(title + "-one"), slideXML(title + "-two")})
	require.NoError(t, err)

	createResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"slides", "+create", "--title", title, "--slides", string(slidesJSON)},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	createResult.AssertExitCode(t, 0)
	createResult.AssertStdoutStatus(t, true)
	presentationID := gjson.Get(createResult.Stdout, "data.xml_presentation_id").String()
	require.NotEmpty(t, presentationID, createResult.Stdout)
	parentT.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"drive", "+delete", "--file-token", presentationID, "--type", "slides", "--yes"},
			DefaultAs: "bot",
		})
		clie2e.ReportCleanupFailure(parentT, "delete presentation "+presentationID, deleteResult, deleteErr)
	})
	slideIDs := gjson.Get(createResult.Stdout, "data.slide_ids").Array()
	require.Len(t, slideIDs, 2, createResult.Stdout)

	workDir := t.TempDir()
	overviewResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"slides", "+screenshot", "--presentation", presentationID, "--overview", "--output", "overview.png"},
		DefaultAs: "bot",
		WorkDir:   workDir,
	})
	require.NoError(t, err)
	overviewResult.AssertExitCode(t, 0)
	overviewResult.AssertStdoutStatus(t, true)
	overview := gjson.Get(overviewResult.Stdout, "data.overview")
	require.Equal(t, int64(4), overview.Get("columns").Int(), overviewResult.Stdout)
	require.Equal(t, int64(2), overview.Get("total_slides").Int(), overviewResult.Stdout)
	require.Equal(t, int64(1), overview.Get("overview_page").Int(), overviewResult.Stdout)
	slides := overview.Get("slides").Array()
	require.Len(t, slides, 2, overviewResult.Stdout)
	for i, slideID := range slideIDs {
		require.Equal(t, int64(i+1), slides[i].Get("index").Int(), overviewResult.Stdout)
		require.Equal(t, slideID.String(), slides[i].Get("slide_id").String(), overviewResult.Stdout)
		require.Equal(t, int64(0), slides[i].Get("row").Int(), overviewResult.Stdout)
		require.Equal(t, int64(i), slides[i].Get("column").Int(), overviewResult.Stdout)
	}
	overviewPath := overview.Get("path").String()
	require.Equal(t, overviewPath, gjson.Get(overviewResult.Stdout, "data.output").String(), overviewResult.Stdout)
	requireScreenshotPathUnderDir(t, overviewPath, workDir)
	overviewFile, err := vfs.Open(overviewPath)
	require.NoError(t, err)
	defer overviewFile.Close()
	overviewConfig, overviewFormat, err := image.DecodeConfig(overviewFile)
	require.NoError(t, err)
	require.Equal(t, "png", overviewFormat, overviewResult.Stdout)
	require.Equal(t, 1360, overviewConfig.Width, overviewResult.Stdout)
	require.Equal(t, 236, overviewConfig.Height, overviewResult.Stdout)
	require.Equal(t, int64(1360), overview.Get("image_size.width").Int(), overviewResult.Stdout)
	require.Equal(t, int64(236), overview.Get("image_size.height").Int(), overviewResult.Stdout)

	regionResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+screenshot", "--presentation", presentationID,
			"--slide-id", slideIDs[0].String(), "--region", "0,0,240,135", "--output", "region.png",
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
	})
	require.NoError(t, err)
	regionResult.AssertExitCode(t, 0)
	regionResult.AssertStdoutStatus(t, true)
	region := gjson.Get(regionResult.Stdout, "data.region")
	require.Equal(t, int64(0), region.Get("requested_pixel_rect.x").Int(), regionResult.Stdout)
	require.Equal(t, int64(0), region.Get("requested_pixel_rect.y").Int(), regionResult.Stdout)
	require.Equal(t, int64(240), region.Get("requested_pixel_rect.width").Int(), regionResult.Stdout)
	require.Equal(t, int64(135), region.Get("requested_pixel_rect.height").Int(), regionResult.Stdout)
	require.GreaterOrEqual(t, region.Get("source_image_size.width").Int(), int64(240), regionResult.Stdout)
	require.GreaterOrEqual(t, region.Get("source_image_size.height").Int(), int64(135), regionResult.Stdout)
	screenshot := gjson.Get(regionResult.Stdout, "data.screenshots.0")
	require.Equal(t, slideIDs[0].String(), screenshot.Get("slide_id").String(), regionResult.Stdout)
	require.Equal(t, "png", screenshot.Get("format").String(), regionResult.Stdout)
	require.Equal(t, int64(240), screenshot.Get("image_size.width").Int(), regionResult.Stdout)
	require.Equal(t, int64(135), screenshot.Get("image_size.height").Int(), regionResult.Stdout)
	regionPath := screenshot.Get("path").String()
	require.Equal(t, regionPath, gjson.Get(regionResult.Stdout, "data.output").String(), regionResult.Stdout)
	requireScreenshotPathUnderDir(t, regionPath, workDir)
	regionFile, err := vfs.Open(regionPath)
	require.NoError(t, err)
	defer regionFile.Close()
	regionConfig, regionFormat, err := image.DecodeConfig(regionFile)
	require.NoError(t, err)
	require.Equal(t, "png", regionFormat, regionResult.Stdout)
	require.Equal(t, 240, regionConfig.Width, regionResult.Stdout)
	require.Equal(t, 135, regionConfig.Height, regionResult.Stdout)
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
