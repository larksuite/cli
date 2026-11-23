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

func TestSlidesScreenshotRequiresSelectorDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+screenshot",
			"--presentation", "presScreenshotMissingSelector",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--slide-id or --slide-number is required", gjson.Get(result.Stderr, "error.message").String(), result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.hint").String(), "--slide-id <slide_id>", result.Stderr)
	require.Empty(t, result.Stdout)
}

func TestSlidesScreenshotRejectsEmptySlideIDWithSlideNumberDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+screenshot",
			"--presentation", "presScreenshotEmptyID",
			"--slide-id", "",
			"--slide-number", "1",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--slide-id", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Equal(t, "--slide-id cannot be empty", gjson.Get(result.Stderr, "error.message").String(), result.Stderr)
	require.Empty(t, result.Stdout)
}

func TestSlidesScreenshotOutputDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+screenshot",
			"--presentation", "presScreenshotOutput",
			"--slide-number", "3",
			"--output", "./slide3.png",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t, "./slide3.png", gjson.Get(result.Stdout, "data.output").String(), result.Stdout)
	require.False(t, gjson.Get(result.Stdout, "data.output_dir").Exists(), result.Stdout)
	require.Equal(t, "POST", gjson.Get(result.Stdout, "data.api.0.method").String(), result.Stdout)
	require.Equal(t,
		"/open-apis/slides_ai/v1/xml_presentations/presScreenshotOutput/slide_images",
		gjson.Get(result.Stdout, "data.api.0.url").String(),
		result.Stdout,
	)
	require.Equal(t, int64(3), gjson.Get(result.Stdout, "data.api.0.body.slide_numbers.0").Int(), result.Stdout)
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

func TestSlidesScreenshotMixedSelectorAliasAttributionDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)
	tests := []struct {
		name       string
		args       []string
		wantParams []string
	}{
		{
			name:       "numeric slide alias",
			args:       []string{"--slides", "pII", "--slide", "2"},
			wantParams: []string{"--slides", "--slide"},
		},
		{
			name:       "ID slide alias",
			args:       []string{"--slide", "pII", "--slide-numbers", "2"},
			wantParams: []string{"--slide", "--slide-numbers"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			args := append([]string{"slides", "+screenshot", "--presentation", "presScreenshotMixedAlias"}, tt.args...)
			args = append(args, "--dry-run")
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
			require.Equal(t, int64(2), gjson.Get(result.Stderr, "error.params.#").Int(), result.Stderr)
			require.Equal(t, tt.wantParams[0], gjson.Get(result.Stderr, "error.params.0.name").String(), result.Stderr)
			require.Equal(t, tt.wantParams[1], gjson.Get(result.Stderr, "error.params.1.name").String(), result.Stderr)
			require.Empty(t, result.Stdout)
		})
	}
}

func TestSlidesScreenshotContentSlideAliasAttributionDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+screenshot",
			"--content", `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data></data></slide>`,
			"--slide", "pII",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "--content", gjson.Get(result.Stderr, "error.params.0.name").String(), result.Stderr)
	require.Equal(t, "--slide", gjson.Get(result.Stderr, "error.params.1.name").String(), result.Stderr)
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
			"--content", `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data></data></slide>`,
			"--slide-id", "",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, "/open-apis/slides_ai/v1/slide_image/render", gjson.Get(result.Stdout, "data.api.0.url").String(), result.Stdout)
}
