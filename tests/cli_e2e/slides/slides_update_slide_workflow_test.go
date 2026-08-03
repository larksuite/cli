// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// skipWithoutCleanupScopes refuses to create a deck this run cannot delete,
// when that is knowable. AGENTS.md wants live workflows self-contained
// (create → use → cleanup); a cleanup that fails on missing scopes both fails
// the package after the workflow already passed and leaks one presentation per
// run. The capability is probed up front with --dry-run, which runs the same
// scope pre-flight as the real cleanup without touching anything remote.
//
// The probe is authoritative only for stored credentials, whose scope grants
// the pre-flight can read. A token injected through the environment
// (TEST_USER_ACCESS_TOKEN / LARKSUITE_CLI_USER_ACCESS_TOKEN) carries no scope
// metadata, and the pre-flight deliberately skips when scopes are unknown —
// exit 0 from the probe proves nothing there. No API exposes a token's grants
// without exercising them, so for that path the run proceeds on the documented
// requirement that the CI identity is provisioned with the cleanup scopes
// (coverage.md), and a cleanup failure stays fatal and visible.
func skipWithoutCleanupScopes(ctx context.Context, t *testing.T) {
	t.Helper()
	if os.Getenv("TEST_USER_ACCESS_TOKEN") != "" || os.Getenv("LARKSUITE_CLI_USER_ACCESS_TOKEN") != "" {
		t.Log("cleanup-scope probe skipped: environment tokens carry no scope metadata, so a dry-run pre-flight cannot prove anything; the CI identity must be provisioned with space:document:delete and drive:drive.metadata:readonly (see coverage.md)")
		return
	}
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"drive", "+delete", "--file-token", "cleanup_scope_probe", "--type", "slides", "--yes", "--dry-run"},
		DefaultAs: "user",
	})
	require.NoError(t, err, "the CLI must launch for the scope probe")
	switch {
	case result.ExitCode == 0:
		// Stored credential with the scopes present.
	case strings.Contains(result.Stderr, "missing_scope"):
		t.Skipf("user token lacks the cleanup scopes (space:document:delete, drive:drive.metadata:readonly); refusing to create a deck the run cannot delete\nstderr:\n%s", result.Stderr)
	default:
		// Anything else is a broken probe, not a known-good capability;
		// proceeding would risk creating a deck under unknown conditions.
		t.Fatalf("cleanup-scope probe failed unexpectedly (exit %d)\nstdout:\n%s\nstderr:\n%s", result.ExitCode, result.Stdout, result.Stderr)
	}
}

// TestSlides_UpdateSlideWorkflowAsUser is the only test that can prove this
// command works, and it exists because an earlier version of it did not: the
// whole design once rested on sending a single part covering the page, which
// unit stubs happily accepted and the real API rejects outright (block_id is
// validated as a short ELEMENT id, so a page id cannot be addressed). Stubs
// prove the request shape; only a live round trip proves the request is legal
// and does what it claims.
//
// It walks every operation the diff can emit — replace, insert, delete, and the
// no-op — then checks the two things element-level parts cannot express are
// refused rather than silently dropped.
func TestSlides_UpdateSlideWorkflowAsUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	// Without a user token the load-bearing backend behavior goes unverified in
	// this run; say so rather than skipping quietly.
	clie2e.SkipWithoutUserToken(t)
	skipWithoutCleanupScopes(ctx, t)

	parentT := t
	suffix := clie2e.GenerateSuffix()
	title := "slides-update-e2e-" + suffix
	controlText := "Control " + suffix
	originalText := "Original " + suffix

	page := func(body string) string {
		return `<slide xmlns="http://www.larkoffice.com/sml/2.0"><data>` +
			`<shape type="text" topLeftX="80" topLeftY="80" width="800" height="120">` +
			`<content textType="title"><p>` + body + `</p></content></shape></data></slide>`
	}
	jsonArray := func(xmls ...string) string {
		quoted := make([]string, 0, len(xmls))
		for _, xml := range xmls {
			quoted = append(quoted, `"`+strings.ReplaceAll(xml, `"`, `\"`)+`"`)
		}
		return "[" + strings.Join(quoted, ",") + "]"
	}
	readPage := func(t *testing.T, presentationID, slideID string) string {
		t.Helper()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"slides", "+xml-get", "--presentation", presentationID, "--slide-id", slideID},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		content := gjson.Get(result.Stdout, "data.slide.content").String()
		require.NotEmpty(t, content, "stdout:\n%s", result.Stdout)
		return content
	}
	update := func(t *testing.T, presentationID, slideID, content string) *clie2e.Result {
		t.Helper()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"slides", "+update-slide",
				"--presentation", presentationID,
				"--slide-id", slideID,
				"--content", content,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		return result
	}

	var presentationID, targetSlideID string

	t.Run("create a two-page presentation as user", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"slides", "+create",
				"--title", title,
				"--slides", jsonArray(page(controlText), page(originalText)),
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		presentationID = gjson.Get(result.Stdout, "data.xml_presentation_id").String()
		require.NotEmpty(t, presentationID, "stdout:\n%s", result.Stdout)
		slideIDs := gjson.Get(result.Stdout, "data.slide_ids").Array()
		require.Len(t, slideIDs, 2, "stdout:\n%s", result.Stdout)
		targetSlideID = slideIDs[1].String()

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
			// Deleting needs space:document:delete, which a token scoped only
			// for slides does not carry; report rather than fail so a scope gap
			// does not mask the workflow result. The deck is named with the
			// run suffix so a leftover is identifiable.
			clie2e.ReportCleanupFailure(parentT, "delete presentation "+presentationID, deleteResult, deleteErr)
		})
	})

	t.Run("restyle an element: one replace part", func(t *testing.T) {
		require.NotEmpty(t, targetSlideID, "presentation should be created first")

		// Change the font on every element, which is the edit the command
		// exists for. Only <content> is touched, so exactly one element differs.
		current := readPage(t, presentationID, targetSlideID)
		wanted := regexp.MustCompile(`fontFamily="[^"]*"`).ReplaceAllString(current, `fontFamily="楷体"`)
		require.NotEqual(t, current, wanted, "the page should have had a fontFamily to change:\n%s", current)

		result := update(t, presentationID, targetSlideID, wanted)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.replaced").Int(), result.Stdout)
		require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.parts_count").Int(), result.Stdout)
		require.Equal(t, targetSlideID, gjson.Get(result.Stdout, "data.slide_id").String(),
			"the page keeps its slide_id\n%s", result.Stdout)

		after := readPage(t, presentationID, targetSlideID)
		require.Contains(t, after, `fontFamily="楷体"`, "the restyle must be live:\n%s", after)
		require.Contains(t, after, originalText, "restyling must not change the text:\n%s", after)
	})

	t.Run("writing the same page again is a no-op", func(t *testing.T) {
		current := readPage(t, presentationID, targetSlideID)
		result := update(t, presentationID, targetSlideID, current)
		result.AssertExitCode(t, 0)
		require.True(t, gjson.Get(result.Stdout, "data.unchanged").Bool(),
			"an identical page must not be written\n%s", result.Stdout)
		require.Equal(t, int64(0), gjson.Get(result.Stdout, "data.parts_count").Int(), result.Stdout)
	})

	t.Run("add an element without an id: one insert part", func(t *testing.T) {
		current := readPage(t, presentationID, targetSlideID)
		added := `<shape type="text" topLeftX="80" topLeftY="300" width="400" height="80"><content><p>Added ` + suffix + `</p></content></shape>`
		wanted := strings.Replace(current, "</data>", added+"</data>", 1)

		result := update(t, presentationID, targetSlideID, wanted)
		result.AssertExitCode(t, 0)
		require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.inserted").Int(), result.Stdout)

		after := readPage(t, presentationID, targetSlideID)
		require.Contains(t, after, "Added "+suffix, "the new element must be live:\n%s", after)
		require.Contains(t, after, originalText, "the existing element must survive:\n%s", after)
	})

	t.Run("drop an element: one delete part", func(t *testing.T) {
		current := readPage(t, presentationID, targetSlideID)
		// Remove the original title shape, keeping the one added above.
		wanted := regexp.MustCompile(`(?s)\s*<shape[^>]*>\s*<content[^>]*>\s*<p>`+regexp.QuoteMeta(originalText)+`</p>.*?</shape>`).
			ReplaceAllString(current, "")
		require.NotEqual(t, current, wanted, "the title shape should have been removable:\n%s", current)

		result := update(t, presentationID, targetSlideID, wanted)
		result.AssertExitCode(t, 0)
		require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.deleted").Int(), result.Stdout)

		after := readPage(t, presentationID, targetSlideID)
		require.NotContains(t, after, originalText, "the dropped element must be gone:\n%s", after)
		require.Contains(t, after, "Added "+suffix, "the kept element must remain:\n%s", after)
	})

	t.Run("a background change is refused, not dropped", func(t *testing.T) {
		current := readPage(t, presentationID, targetSlideID)
		// Matches both the self-closing <style/> a plain page comes back with
		// and a populated <style>…</style>.
		styleBlock := regexp.MustCompile(`(?s)<style\s*/>|<style[^>]*>.*?</style>`)
		require.True(t, styleBlock.MatchString(current), "page should carry a <style> block:\n%s", current)
		wanted := styleBlock.ReplaceAllString(current,
			`<style><fill><fillColor color="rgba(255, 0, 0, 1)"/></fill></style>`)

		result := update(t, presentationID, targetSlideID, wanted)
		require.NotEqual(t, 0, result.ExitCode,
			"the background cannot be expressed, so it must fail loudly\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
		require.Contains(t, result.Stderr, "background", "stderr:\n%s", result.Stderr)

		// And nothing may have been written: the page is still what it was.
		require.Equal(t, current, readPage(t, presentationID, targetSlideID),
			"a refused background change must leave the page untouched")
	})

	t.Run("the other page and the page order are untouched", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"api", "get", "/open-apis/slides_ai/v1/xml_presentations/" + presentationID},
			DefaultAs: "user",
			Params:    map[string]any{"revision_id": -1},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		content := gjson.Get(result.Stdout, "data.xml_presentation.content").String()
		require.Contains(t, content, controlText, "the control page must be untouched\n%s", content)

		controlAt := strings.Index(content, controlText)
		editedAt := strings.Index(content, "Added "+suffix)
		require.GreaterOrEqual(t, controlAt, 0, content)
		require.GreaterOrEqual(t, editedAt, 0, content)
		require.Less(t, controlAt, editedAt, "the deck order must not change\n%s", content)
	})
}
