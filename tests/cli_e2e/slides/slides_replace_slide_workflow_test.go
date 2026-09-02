// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSlides_ReplaceSlideAliasWorkflowAsUser proves the compatibility aliases
// survive a real API round trip. Dry-run already pins the canonical wire body;
// this workflow verifies the backend applies it to only the requested block and
// honors the insertion anchor.
func TestSlides_ReplaceSlideAliasWorkflowAsUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	parentT := t
	suffix := clie2e.GenerateSuffix()
	title := "slides-replace-e2e-" + suffix
	originalMarker := "Original target " + suffix
	controlMarker := "Control block " + suffix
	replacementMarker := "Replacement target " + suffix
	insertedMarker := "Inserted block " + suffix
	slideXML := `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>` +
		replaceWorkflowShapeXML(originalMarker, 80) +
		replaceWorkflowShapeXML(controlMarker, 300) +
		`</data></slide>`

	var presentationID, slideID string

	readContent := func(t *testing.T) string {
		t.Helper()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"api", "get", "/open-apis/slides_ai/v1/xml_presentations/" + presentationID},
			DefaultAs: "user",
			Params:    map[string]any{"revision_id": -1},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		return gjson.Get(result.Stdout, "data.xml_presentation.content").String()
	}

	t.Run("create presentation and capture server block ids", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"slides", "+create",
				"--title", title,
				"--slides", mustMarshalSlidesJSON(t, []string{slideXML}),
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		presentationID = gjson.Get(result.Stdout, "data.xml_presentation_id").String()
		slideID = gjson.Get(result.Stdout, "data.slide_ids.0").String()
		require.NotEmpty(t, presentationID, "stdout:\n%s", result.Stdout)
		require.NotEmpty(t, slideID, "stdout:\n%s", result.Stdout)

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
				DefaultAs: "user",
			})
			clie2e.ReportCleanupFailure(parentT, "delete presentation "+presentationID, deleteResult, deleteErr)
		})
	})

	t.Run("replace and insert through compatibility aliases", func(t *testing.T) {
		require.NotEmpty(t, presentationID)
		require.NotEmpty(t, slideID)

		originalContent := readContent(t)
		targetID := slidesHistoryShapeID(t, originalContent, originalMarker)
		controlID := slidesHistoryShapeID(t, originalContent, controlMarker)
		parts := []map[string]string{
			{
				"action":    "replace",
				"target_id": targetID,
				"content":   replaceWorkflowShapeXML(replacementMarker, 80),
			},
			{
				"action":                 "insert",
				"element":                replaceWorkflowShapeXML(insertedMarker, 220),
				"insert_before_block_id": controlID,
			},
		}
		rawParts, err := json.Marshal(parts)
		require.NoError(t, err)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"slides", "+replace-slide",
				"--presentation", presentationID,
				"--slide-id", slideID,
				"--parts", string(rawParts),
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, int64(2), gjson.Get(result.Stdout, "data.parts_count").Int(), "stdout:\n%s", result.Stdout)
		requireReplaceSlideAliasNormalizations(
			t,
			gjson.Get(result.Stdout, "data.normalizations").Array(),
			result.Stdout,
		)

		updatedContent := readContent(t)
		require.NotContains(t, updatedContent, originalMarker, "target block was not replaced:\n%s", updatedContent)
		require.Contains(t, updatedContent, replacementMarker, "replacement did not persist:\n%s", updatedContent)
		require.Contains(t, updatedContent, insertedMarker, "inserted block did not persist:\n%s", updatedContent)
		require.Contains(t, updatedContent, controlMarker, "control block was modified:\n%s", updatedContent)
		require.Equal(t, targetID, slidesHistoryShapeID(t, updatedContent, replacementMarker),
			"replacement must retain the target block id")
		require.Less(t, strings.Index(updatedContent, insertedMarker), strings.Index(updatedContent, controlMarker),
			"insert_before_block_id must place the inserted block before the control block:\n%s", updatedContent)
	})
}

func replaceWorkflowShapeXML(marker string, top int) string {
	return `<shape type="text" topLeftX="80" topLeftY="` +
		fmt.Sprintf("%d", top) +
		`" width="800" height="120"><content textType="body"><p>` +
		slidesHistoryWorkflowXMLEscape(marker) +
		`</p></content></shape>`
}
