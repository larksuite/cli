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

// TestSlidesScreenshotSlideIDCSVDryRunE2E pins the CSV multi-value parsing for
// --slide-id through the built CLI: a single comma-separated flag value must
// expand into the same slide_ids request body that repeating the flag would
// produce.
func TestSlidesScreenshotSlideIDCSVDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+screenshot",
			"--presentation", "presScreenshotDryRun",
			"--slide-id", "slide_1,slide_2",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t, "POST", gjson.Get(result.Stdout, "data.api.0.method").String(), result.Stdout)
	require.Equal(t,
		"/open-apis/slides_ai/v1/xml_presentations/presScreenshotDryRun/slide_images",
		gjson.Get(result.Stdout, "data.api.0.url").String(),
		result.Stdout,
	)

	slideIDs := gjson.Get(result.Stdout, "data.api.0.body.slide_ids").Array()
	require.Len(t, slideIDs, 2, result.Stdout)
	require.Equal(t, "slide_1", slideIDs[0].String(), result.Stdout)
	require.Equal(t, "slide_2", slideIDs[1].String(), result.Stdout)
}

func TestSlidesScreenshotAliasesDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+screenshot",
			"--presentation-id", "presScreenshotAlias",
			"--slides", "slide_1,slide_2",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t,
		"/open-apis/slides_ai/v1/xml_presentations/presScreenshotAlias/slide_images",
		gjson.Get(result.Stdout, "data.api.0.url").String(),
		result.Stdout,
	)
	slideIDs := gjson.Get(result.Stdout, "data.api.0.body.slide_ids").Array()
	require.Len(t, slideIDs, 2, result.Stdout)
	require.Equal(t, "slide_1", slideIDs[0].String(), result.Stdout)
	require.Equal(t, "slide_2", slideIDs[1].String(), result.Stdout)
}

func TestSlidesScreenshotMixedSelectorsDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+screenshot",
			"--presentation", "presScreenshotMixed",
			"--slide-id", "pII",
			"--slide-number", "2",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "cannot be used together")
	require.Equal(t, int64(2), gjson.Get(result.Stderr, "error.params.#").Int(), result.Stderr)
	require.Equal(t, "--slide-id", gjson.Get(result.Stderr, "error.params.0.name").String(), result.Stderr)
	require.Equal(t, "--slide-number", gjson.Get(result.Stderr, "error.params.1.name").String(), result.Stderr)
	require.Empty(t, result.Stdout)
}

func TestSlidesScreenshotValidationPriorityDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+screenshot",
			"--presentation", "tmp/wiki/invalid",
			"--slide-id", "pII",
			"--slide-number", "2",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--presentation", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "unsupported --presentation input")
	require.Empty(t, result.Stdout)
}

func TestSlidesScreenshotEmptySlideIDDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+screenshot",
			"--content", `<slide xmlns="http://www.larkoffice.com/sml/2.0"><data></data></slide>`,
			"--slide-id", "",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, "/open-apis/slides_ai/v1/slide_image/render", gjson.Get(result.Stdout, "data.api.0.url").String(), result.Stdout)
}
