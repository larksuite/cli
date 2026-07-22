// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSlides_HistoryWorkflow(t *testing.T) {
	if os.Getenv("LARK_SLIDES_HISTORY_E2E") != "1" {
		t.Skip("set LARK_SLIDES_HISTORY_E2E=1 to run slides history live workflow")
	}
	clie2e.SkipWithoutUserToken(t)

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	title := "lark-cli-e2e-slides-history-" + suffix
	originalMarker := "original history marker " + suffix
	updatedMarker := "updated history marker " + suffix
	const defaultAs = "user"

	originalSlideXML := slidesHistoryWorkflowSlideXML(title, originalMarker)
	updatedSlideXML := slidesHistoryWorkflowSlideXML(title, updatedMarker)
	slidesJSON := mustMarshalSlidesJSON(t, []string{originalSlideXML})

	var presentationID string
	var slideID string
	createResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+create",
			"--title", title,
			"--slides", slidesJSON,
		},
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	createResult.AssertExitCode(t, 0)
	createResult.AssertStdoutStatus(t, true)

	presentationID = gjson.Get(createResult.Stdout, "data.xml_presentation_id").String()
	require.NotEmpty(t, presentationID, "stdout:\n%s", createResult.Stdout)
	slideID = gjson.Get(createResult.Stdout, "data.slide_ids.0").String()
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
			DefaultAs: defaultAs,
		})
		clie2e.ReportCleanupFailure(parentT, "delete presentation "+presentationID, deleteResult, deleteErr)
	})

	fetchOriginal, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"api", "get", "/open-apis/slides_ai/v1/xml_presentations/" + presentationID},
		DefaultAs: defaultAs,
		Params:    map[string]any{"revision_id": -1},
	})
	require.NoError(t, err)
	fetchOriginal.AssertExitCode(t, 0)
	fetchOriginal.AssertStdoutStatus(t, true)
	originalContent := gjson.Get(fetchOriginal.Stdout, "data.xml_presentation.content").String()
	assert.Contains(t, originalContent, originalMarker, "stdout:\n%s", fetchOriginal.Stdout)
	originalRevision := gjson.Get(fetchOriginal.Stdout, "data.xml_presentation.revision_id").Int()
	require.Greater(t, originalRevision, int64(0), "stdout:\n%s", fetchOriginal.Stdout)

	pagesJSON := mustMarshalPagesJSON(t, []slidesHistoryWorkflowPage{
		{SlideID: slideID, Content: updatedSlideXML},
	})
	updateResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+replace-pages",
			"--presentation", presentationID,
			"--pages", pagesJSON,
		},
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	updateResult.AssertExitCode(t, 0)
	updateResult.AssertStdoutStatus(t, true)

	fetchUpdated, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"api", "get", "/open-apis/slides_ai/v1/xml_presentations/" + presentationID},
		DefaultAs: defaultAs,
		Params:    map[string]any{"revision_id": -1},
	})
	require.NoError(t, err)
	fetchUpdated.AssertExitCode(t, 0)
	fetchUpdated.AssertStdoutStatus(t, true)
	updatedContent := gjson.Get(fetchUpdated.Stdout, "data.xml_presentation.content").String()
	assert.Contains(t, updatedContent, updatedMarker, "stdout:\n%s", fetchUpdated.Stdout)
	assert.NotContains(t, updatedContent, originalMarker, "stdout:\n%s", fetchUpdated.Stdout)
	currentRevision := gjson.Get(fetchUpdated.Stdout, "data.xml_presentation.revision_id").Int()
	require.Greater(t, currentRevision, originalRevision, "stdout:\n%s", fetchUpdated.Stdout)

	var revertHistoryVersionID string
	require.Eventually(t, func() bool {
		listResult, listErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"slides", "+history-list",
				"--presentation", presentationID,
				"--page-size", "20",
			},
			DefaultAs: defaultAs,
		})
		if listErr != nil || listResult.ExitCode != 0 {
			return false
		}
		for _, entry := range gjson.Get(listResult.Stdout, "data.entries").Array() {
			revisionID := entry.Get("revision_id").Int()
			historyVersionID := entry.Get("history_version_id").String()
			if revisionID == originalRevision && historyVersionID != "" {
				revertHistoryVersionID = historyVersionID
				return true
			}
		}
		return false
	}, 45*time.Second, 3*time.Second, "history list did not expose original revision %d", originalRevision)
	require.NotEmpty(t, revertHistoryVersionID)

	revertResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+history-revert",
			"--presentation", presentationID,
			"--history-version-id", revertHistoryVersionID,
		},
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	revertResult.AssertExitCode(t, 0)
	revertResult.AssertStdoutStatus(t, true)

	status := gjson.Get(revertResult.Stdout, "data.status").String()
	taskID := gjson.Get(revertResult.Stdout, "data.task_id").String()
	if status == "running" {
		require.NotEmpty(t, taskID, "stdout:\n%s", revertResult.Stdout)
		require.Eventually(t, func() bool {
			statusResult, statusErr := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"slides", "+history-revert-status",
					"--presentation", presentationID,
					"--task-id", taskID,
				},
				DefaultAs: defaultAs,
			})
			if statusErr != nil || statusResult.ExitCode != 0 {
				return false
			}
			status = gjson.Get(statusResult.Stdout, "data.status").String()
			return status != "" && status != "running"
		}, 60*time.Second, 5*time.Second, "history revert task did not finish")
	}
	require.Equal(t, "done", status, "revert stdout:\n%s", revertResult.Stdout)

	fetchReverted, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"api", "get", "/open-apis/slides_ai/v1/xml_presentations/" + presentationID},
		DefaultAs: defaultAs,
		Params:    map[string]any{"revision_id": -1},
	})
	require.NoError(t, err)
	fetchReverted.AssertExitCode(t, 0)
	fetchReverted.AssertStdoutStatus(t, true)
	revertedContent := gjson.Get(fetchReverted.Stdout, "data.xml_presentation.content").String()
	assert.Contains(t, revertedContent, originalMarker, "stdout:\n%s", fetchReverted.Stdout)
	assert.NotContains(t, revertedContent, updatedMarker, "stdout:\n%s", fetchReverted.Stdout)
}

type slidesHistoryWorkflowPage struct {
	SlideID string `json:"slide_id"`
	Content string `json:"content"`
}

func slidesHistoryWorkflowSlideXML(title, marker string) string {
	return `<slide xmlns="http://www.larkoffice.com/sml/2.0"><data>` +
		`<shape type="text" topLeftX="80" topLeftY="80" width="800" height="120"><content textType="title"><p>` + slidesHistoryWorkflowXMLEscape(title) + `</p></content></shape>` +
		`<shape type="text" topLeftX="80" topLeftY="220" width="800" height="180"><content textType="body"><p>` + slidesHistoryWorkflowXMLEscape(marker) + `</p></content></shape>` +
		`</data></slide>`
}

func slidesHistoryWorkflowXMLEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}

func mustMarshalSlidesJSON(t *testing.T, slides []string) string {
	t.Helper()
	raw, err := json.Marshal(slides)
	if err != nil {
		t.Fatalf("marshal slides JSON: %v", err)
	}
	return string(raw)
}

func mustMarshalPagesJSON(t *testing.T, pages []slidesHistoryWorkflowPage) string {
	t.Helper()
	raw, err := json.Marshal(pages)
	if err != nil {
		t.Fatalf("marshal pages JSON: %v", err)
	}
	return strings.TrimSpace(string(raw))
}
