// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSlides_CreateWorkflowAsUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	parentT := t
	suffix := clie2e.GenerateSuffix()
	title := "slides-e2e-" + suffix
	slideTitle := "Overview " + suffix
	slideBody := "Body " + suffix
	slideXML := `<slide xmlns="http://www.larkoffice.com/sml/2.0"><data><shape type="text" topLeftX="80" topLeftY="80" width="800" height="120"><content textType="title"><p>` + slideTitle + `</p></content></shape><shape type="text" topLeftX="80" topLeftY="200" width="800" height="180"><content textType="body"><p>` + slideBody + `</p></content></shape></data></slide>`

	var presentationID string

	t.Run("create presentation with slide as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"slides", "+create",
				"--title", title,
				"--slides", `["` + strings.ReplaceAll(slideXML, `"`, `\"`) + `"]`,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		presentationID = gjson.Get(result.Stdout, "data.xml_presentation_id").String()
		require.NotEmpty(t, presentationID, "stdout:\n%s", result.Stdout)
		require.Equal(t, title, gjson.Get(result.Stdout, "data.title").String(), "stdout:\n%s", result.Stdout)
		require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.slides_added").Int(), "stdout:\n%s", result.Stdout)
		require.Len(t, gjson.Get(result.Stdout, "data.slide_ids").Array(), 1, "stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			cleanupCtx, cancel := clie2e.CleanupContext()
			defer cancel()

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

	t.Run("get created presentation xml as user", func(t *testing.T) {
		require.NotEmpty(t, presentationID, "presentation should be created before readback")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"api", "get", "/open-apis/slides_ai/v1/xml_presentations/" + presentationID},
			DefaultAs: "user",
			Params:    map[string]any{"revision_id": -1},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		require.Equal(t, presentationID, gjson.Get(result.Stdout, "data.xml_presentation.presentation_id").String(), "stdout:\n%s", result.Stdout)
		content := gjson.Get(result.Stdout, "data.xml_presentation.content").String()
		require.Contains(t, content, "<title>"+title+"</title>", "stdout:\n%s", result.Stdout)
		require.Contains(t, content, slideTitle, "stdout:\n%s", result.Stdout)
		require.Contains(t, content, slideBody, "stdout:\n%s", result.Stdout)
	})
}
