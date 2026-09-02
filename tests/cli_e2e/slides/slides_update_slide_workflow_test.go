// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSlidesUpdateSlideLiveE2E is the required live-backend assertion for
// whole-page editing. Unlike the opt-in history workflow, this test uses the
// default live lane's tenant credential and cannot be skipped there: that lane
// preflights TEST_TENANT_ACCESS_TOKEN before running this package.
func TestSlidesUpdateSlideLiveE2E(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	title := "slides-update-live-e2e-" + suffix
	originalMarker := "original update marker " + suffix
	updatedMarker := "updated update marker " + suffix
	originalSlideXML := updateSlideLiveXML(title, originalMarker)
	slidesJSON, err := json.Marshal([]string{originalSlideXML})
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
	slideID := gjson.Get(createResult.Stdout, "data.slide_ids.0").String()
	require.NotEmpty(t, slideID, "stdout:\n%s", createResult.Stdout)
	parentT.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{
				"drive", "+delete",
				"--file-token", presentationID,
				"--type", "slides",
				"--yes",
			},
			DefaultAs: "bot",
		})
		clie2e.ReportCleanupFailure(parentT, "delete presentation "+presentationID, deleteResult, deleteErr)
	})

	updateResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+update-slide",
			"--presentation", presentationID,
			"--slide-id", slideID,
			"--content", updateSlideLiveXML(title, updatedMarker),
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	updateResult.AssertExitCode(t, 0)
	updateResult.AssertStdoutStatus(t, true)
	require.Equal(t, slideID, gjson.Get(updateResult.Stdout, "data.slide_id").String(),
		"whole-page update must preserve slide_id; stdout:\n%s", updateResult.Stdout)

	readbackResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+xml-get",
			"--presentation", presentationID,
			"--slide-id", slideID,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	readbackResult.AssertExitCode(t, 0)
	readbackResult.AssertStdoutStatus(t, true)
	require.Equal(t, slideID, gjson.Get(readbackResult.Stdout, "data.slide.slide_id").String(),
		"readback must address the updated page; stdout:\n%s", readbackResult.Stdout)
	content := gjson.Get(readbackResult.Stdout, "data.slide.content").String()
	require.Contains(t, content, updatedMarker, "stdout:\n%s", readbackResult.Stdout)
	require.NotContains(t, content, originalMarker, "stdout:\n%s", readbackResult.Stdout)
}

func updateSlideLiveXML(title, marker string) string {
	return `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>` +
		`<shape type="text" topLeftX="80" topLeftY="80" width="800" height="120"><content textType="title"><p>` + title + `</p></content></shape>` +
		`<shape type="text" topLeftX="80" topLeftY="220" width="800" height="180"><content textType="body"><p>` + marker + `</p></content></shape>` +
		`</data></slide>`
}
