// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const updateSlideDryRunPageXML = `<slide id="piy"><style><fill id="fiy"><fillColor color="rgba(255, 255, 255, 1)"/></fill></style><data><shape id="bRU" type="text" topLeftX="46" topLeftY="34" width="400" height="36"><content textType="headline" fontSize="28"><p>Overview</p></content></shape></data><note id="bno"><content/></note></slide>`

// TestSlidesUpdateSlideDryRunE2E pins the shape of the orchestration through the
// built CLI: the command reads the page before writing it, because the parts it
// sends are derived from the page's current state rather than from --content
// alone. Dry-run deliberately cannot show the parts — computing them would
// require the read it is not allowed to perform.
func TestSlidesUpdateSlideDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+update-slide",
			"--presentation", "presUpdateSlideDryRun",
			"--slide-id", "piy",
			"--content", updateSlideDryRunPageXML,
			"--revision-id", "17",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	api := gjson.Get(result.Stdout, "data.api").Array()
	require.Len(t, api, 2, "the command reads then writes\n%s", result.Stdout)

	require.Equal(t, "GET", api[0].Get("method").String(), result.Stdout)
	require.Equal(t,
		"/open-apis/slides_ai/v1/xml_presentations/presUpdateSlideDryRun/slide",
		api[0].Get("url").String(), result.Stdout,
	)
	require.Equal(t, "POST", api[1].Get("method").String(), result.Stdout)
	require.Equal(t,
		"/open-apis/slides_ai/v1/xml_presentations/presUpdateSlideDryRun/slide/replace",
		api[1].Get("url").String(), result.Stdout,
	)

	// The same revision is used for both calls so the parts apply to the
	// snapshot they were diffed against.
	for i := range api {
		require.Equal(t, "piy", api[i].Get("params.slide_id").String(), result.Stdout)
		require.Equal(t, int64(17), api[i].Get("params.revision_id").Int(), result.Stdout)
		require.False(t, api[i].Get("params.tid").Exists(), "tid must be omitted when empty\n%s", result.Stdout)
	}

	require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.wanted_element_count").Int(), result.Stdout)
}

// TestSlidesUpdateAliasDryRunE2E proves the hidden `+update` spelling reaches
// the same logic through the real CLI, along with the hidden --token / --xml
// flag spellings.
func TestSlidesUpdateAliasDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+update",
			"--token", "presUpdateSlideDryRun",
			"--slide-id", "piy",
			"--xml", updateSlideDryRunPageXML,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t,
		"/open-apis/slides_ai/v1/xml_presentations/presUpdateSlideDryRun/slide/replace",
		gjson.Get(result.Stdout, "data.api.1.url").String(),
		result.Stdout,
	)
	require.Equal(t, int64(-1), gjson.Get(result.Stdout, "data.api.1.params.revision_id").Int(),
		"-1 is the default: apply against the latest revision\n%s", result.Stdout)
}

// TestSlidesUpdateSlideRejectsBadContentDryRunE2E is the guardrail check
// through the built CLI: inputs whose failure mode is data loss must be
// refused before any request is built, with the full typed error contract —
// agents branch on type/subtype/param, not on prose.
func TestSlidesUpdateSlideRejectsBadContentDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	for _, tt := range []struct {
		name        string
		content     string
		wantMessage string
	}{
		{
			// An element-level fragment would mean "the page should contain
			// only this" and delete everything else.
			name:        "element_root",
			content:     `<shape type="text"><content><p>oops</p></content></shape>`,
			wantMessage: "+replace-slide",
		},
		{
			// XML fetched for page A posted against page B.
			name:        "root_id_mismatch",
			content:     `<slide id="pother"><data/></slide>`,
			wantMessage: "read from a different page",
		},
		{
			// Slide-level structure the diff cannot represent would be
			// silently dropped — possibly reported as `unchanged`.
			name:        "unknown_slide_child",
			content:     `<slide id="piy"><data/><foo requestedChange="true"/></slide>`,
			wantMessage: "unknown <foo>",
		},
		{
			// Container attributes have no element-level part to travel in.
			name:        "root_attribute",
			content:     `<slide id="piy" requestedChange="true"><data/></slide>`,
			wantMessage: "unsupported attribute",
		},
		{
			// A namespace binding inherited from the root changes what every
			// descendant name means; only the official SML declaration passes.
			name:        "wrong_default_xmlns",
			content:     `<slide xmlns="urn:not-sml" id="piy"><data/></slide>`,
			wantMessage: "unsupported xmlns",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"slides", "+update-slide",
					"--presentation", "presUpdateSlideDryRun",
					"--slide-id", "piy",
					"--content", tt.content,
					"--dry-run",
				},
				DefaultAs: "bot",
			})
			// RunCmd errors only when the CLI could not be launched; a normal
			// non-zero exit lands in result.ExitCode. Discarding this error
			// would turn a broken harness into a nil-pointer panic.
			require.NoError(t, err, "the CLI must launch")
			// Validation errors have a fixed process contract: exit code 2,
			// nothing on stdout, the typed envelope on stderr.
			require.Equal(t, 2, result.ExitCode,
				"stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			require.Empty(t, result.Stdout, "a refused command must not emit a result")

			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
			require.Equal(t, "--content", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
			require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), tt.wantMessage, result.Stderr)
		})
	}
}
