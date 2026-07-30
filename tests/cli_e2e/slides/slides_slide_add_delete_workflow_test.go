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

// slideXMLWithBody renders a one-shape page carrying a caller-supplied marker,
// so a readback can tell the pages apart by content instead of by index.
func slideXMLWithBody(body string) string {
	return `<slide xmlns="http://www.larkoffice.com/sml/2.0"><data>` +
		`<shape type="text" topLeftX="80" topLeftY="80" width="800" height="120">` +
		`<content textType="title"><p>` + body + `</p></content></shape></data></slide>`
}

// TestSlides_SlideAddDeleteWorkflowAsUser exercises the +add-slide /
// +delete-slide round trip against the real API.
//
// The dry-run E2E already pins the request shapes, so what only a live run can
// prove is the part the request shape cannot express: that the returned
// slide_id addresses a page that actually exists, that --before-slide-id
// positions the new page rather than merely being forwarded, and that the
// deleted page is gone from the presentation afterwards. Every assertion is
// therefore made against a readback of the deck, not against the write's own
// response.
func TestSlides_SlideAddDeleteWorkflowAsUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	clie2e.SkipWithoutUserToken(t)

	parentT := t
	suffix := clie2e.GenerateSuffix()
	title := "slides-slide-e2e-" + suffix
	bodyFirst := "First " + suffix
	bodyAppended := "Appended " + suffix
	bodyInserted := "Inserted " + suffix

	var presentationID, appendedSlideID, insertedSlideID string

	// readContent fetches the whole deck at the latest revision. Reading back
	// after every write is what makes the ordering and deletion assertions
	// meaningful.
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

	t.Run("create the presentation to add pages to", func(t *testing.T) {
		slideXML := slideXMLWithBody(bodyFirst)
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

	t.Run("append a page with +add-slide", func(t *testing.T) {
		require.NotEmpty(t, presentationID, "presentation should be created first")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"slides", "+add-slide",
				"--presentation", presentationID,
				"--slide", slideXMLWithBody(bodyAppended),
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		appendedSlideID = gjson.Get(result.Stdout, "data.slide_id").String()
		require.NotEmpty(t, appendedSlideID, "stdout:\n%s", result.Stdout)
		require.Equal(t, presentationID, gjson.Get(result.Stdout, "data.xml_presentation_id").String(), "stdout:\n%s", result.Stdout)
		// Appending must not report a position it was never given.
		require.False(t, gjson.Get(result.Stdout, "data.before_slide_id").Exists(), "stdout:\n%s", result.Stdout)

		content := readContent(t)
		// Both markers must be asserted present before their positions are
		// compared: strings.Index returns -1 for an absent substring, so a
		// missing page would make the ordering check pass for the wrong reason.
		require.Contains(t, content, bodyFirst, "content:\n%s", content)
		require.Contains(t, content, bodyAppended, "content:\n%s", content)
		require.Less(t, strings.Index(content, bodyFirst), strings.Index(content, bodyAppended),
			"an appended page must land after the existing one, content:\n%s", content)
	})

	t.Run("insert a page before another with --before-slide-id", func(t *testing.T) {
		require.NotEmpty(t, appendedSlideID, "the append step must succeed first")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"slides", "+add-slide",
				"--presentation", presentationID,
				"--slide", slideXMLWithBody(bodyInserted),
				"--before-slide-id", appendedSlideID,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		insertedSlideID = gjson.Get(result.Stdout, "data.slide_id").String()
		require.NotEmpty(t, insertedSlideID, "stdout:\n%s", result.Stdout)
		require.NotEqual(t, appendedSlideID, insertedSlideID, "stdout:\n%s", result.Stdout)
		require.Equal(t, appendedSlideID, gjson.Get(result.Stdout, "data.before_slide_id").String(), "stdout:\n%s", result.Stdout)

		// The point of the flag: the new page sits between the other two, not
		// at the end. Only a readback can tell those apart.
		content := readContent(t)
		require.Contains(t, content, bodyFirst, "content:\n%s", content)
		require.Contains(t, content, bodyInserted, "content:\n%s", content)
		require.Contains(t, content, bodyAppended, "content:\n%s", content)
		require.Less(t, strings.Index(content, bodyFirst), strings.Index(content, bodyInserted),
			"content:\n%s", content)
		require.Less(t, strings.Index(content, bodyInserted), strings.Index(content, bodyAppended),
			"--before-slide-id must place the page ahead of its anchor, content:\n%s", content)
	})

	t.Run("remove a page with +delete-slide", func(t *testing.T) {
		require.NotEmpty(t, insertedSlideID, "the insert step must succeed first")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"slides", "+delete-slide",
				"--presentation", presentationID,
				"--slide-id", insertedSlideID,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		require.True(t, gjson.Get(result.Stdout, "data.deleted").Bool(), "stdout:\n%s", result.Stdout)
		require.Equal(t, insertedSlideID, gjson.Get(result.Stdout, "data.slide_id").String(), "stdout:\n%s", result.Stdout)

		content := readContent(t)
		require.NotContains(t, content, bodyInserted, "the deleted page must be gone, content:\n%s", content)
		// The neighbours must survive: a delete that took out the wrong page
		// would otherwise pass the assertion above.
		require.Contains(t, content, bodyFirst, "content:\n%s", content)
		require.Contains(t, content, bodyAppended, "content:\n%s", content)
	})
}
