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

// TestSlidesAddSlideDryRunE2E pins the request +add-slide builds through the
// real binary. The unit tests already cover the same shapes, but only the
// built CLI proves the XML survives flag parsing intact: --slide carries a
// full XML document with quotes and angle brackets, which is exactly the layer
// that produces most 3350001 reports.
func TestSlidesAddSlideDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	const slideXML = `<slide xmlns="http://www.larkoffice.com/sml/2.0"><data></data></slide>`

	tests := []struct {
		name      string
		args      []string
		assertion func(t *testing.T, stdout string)
	}{
		{
			name: "append to end",
			args: []string{
				"slides", "+add-slide",
				"--presentation", "presAddDryRun",
				"--slide", slideXML,
				"--dry-run",
			},
			assertion: func(t *testing.T, stdout string) {
				require.Equal(t, slideXML, gjson.Get(stdout, "data.api.0.body.slide.content").String(), stdout)
				// Appending is expressed by omitting before_slide_id: an empty
				// string is rejected by the backend as an unknown slide.
				require.False(t, gjson.Get(stdout, "data.api.0.body.before_slide_id").Exists(), stdout)
				require.Equal(t, int64(-1), gjson.Get(stdout, "data.api.0.params.revision_id").Int(), stdout)
				require.Equal(t, int64(0), gjson.Get(stdout, "data.images_to_upload").Int(), stdout)
			},
		},
		{
			name: "insert before a page with locking via presentation alias",
			args: []string{
				"slides", "+add-slide",
				"--presentation-id", "presAddDryRun",
				"--slide", slideXML,
				"--before-slide-id", "slide_target",
				"--revision-id", "12",
				"--dry-run",
			},
			assertion: func(t *testing.T, stdout string) {
				require.Equal(t, "slide_target", gjson.Get(stdout, "data.api.0.body.before_slide_id").String(), stdout)
				require.Equal(t, int64(12), gjson.Get(stdout, "data.api.0.params.revision_id").Int(), stdout)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			require.Equal(t, "POST", gjson.Get(result.Stdout, "data.api.0.method").String(), result.Stdout)
			require.Equal(t,
				"/open-apis/slides_ai/v1/xml_presentations/presAddDryRun/slide",
				gjson.Get(result.Stdout, "data.api.0.url").String(),
				result.Stdout,
			)
			tt.assertion(t, result.Stdout)
		})
	}
}

// TestSlidesDeleteSlideDryRunE2E covers the delete request shape and, more
// importantly, pins that the shortcut runs without --yes. The raw
// xml_presentation.slide.delete command is high-risk-write and would exit 10
// with confirmation_required here; the shortcut is Risk "write" on purpose.
func TestSlidesDeleteSlideDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+delete-slide",
			"--presentation-token", "presDeleteDryRun",
			"--slide-id", "slide_gone",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t, "DELETE", gjson.Get(result.Stdout, "data.api.0.method").String(), result.Stdout)
	require.Equal(t,
		"/open-apis/slides_ai/v1/xml_presentations/presDeleteDryRun/slide",
		gjson.Get(result.Stdout, "data.api.0.url").String(),
		result.Stdout,
	)
	require.Equal(t, "slide_gone", gjson.Get(result.Stdout, "data.api.0.params.slide_id").String(), result.Stdout)
	require.Equal(t, int64(-1), gjson.Get(result.Stdout, "data.api.0.params.revision_id").Int(), result.Stdout)
}

// TestSlidesDeleteSlideWikiDryRunE2E proves the wiki URL is resolved as a
// declared first step instead of being sent to the slides API as a token.
func TestSlidesDeleteSlideWikiDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+delete-slide",
			"--presentation", "https://example.feishu.cn/wiki/wikcnE2ETOKEN",
			"--slide-id", "slide_gone",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t, "GET", gjson.Get(result.Stdout, "data.api.0.method").String(), result.Stdout)
	require.Equal(t, "/open-apis/wiki/v2/spaces/get_node", gjson.Get(result.Stdout, "data.api.0.url").String(), result.Stdout)
	require.Equal(t, "wikcnE2ETOKEN", gjson.Get(result.Stdout, "data.api.0.params.token").String(), result.Stdout)
	require.Equal(t, "DELETE", gjson.Get(result.Stdout, "data.api.1.method").String(), result.Stdout)
}
