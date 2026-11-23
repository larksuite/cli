// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const updateSlideDryRunPage = `<slide><style><fill type="solid" color="rgba(0,0,0,1)"/></style>` +
	`<data><shape id="bUn" type="text"><content fontFamily="思源黑体"><p>hi</p></content></shape></data></slide>`

// TestSlidesUpdateSlideDryRunE2E pins the whole contract through the built binary:
// one request, one block_replace part, and block_id carrying the PAGE id. That
// last detail is the entire design — an element id there would only replace one
// element and leave the rest of the page alone.
func TestSlidesUpdateSlideDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+update-slide",
			"--presentation", "presUpdateSlideDryRun",
			"--slide-id", "pYw",
			"--content", updateSlideDryRunPage,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	api := gjson.Get(result.Stdout, "data.api")
	require.Len(t, api.Array(), 1, "a whole-page update is one request, not a read-modify-write: %s", result.Stdout)
	require.Equal(t, "POST", gjson.Get(result.Stdout, "data.api.0.method").String(), result.Stdout)
	require.Equal(t,
		"/open-apis/slides_ai/v1/xml_presentations/presUpdateSlideDryRun/slide/replace",
		gjson.Get(result.Stdout, "data.api.0.url").String(),
		result.Stdout,
	)
	require.Equal(t, "pYw", gjson.Get(result.Stdout, "data.api.0.params.slide_id").String(), result.Stdout)
	require.Equal(t, int64(-1), gjson.Get(result.Stdout, "data.api.0.params.revision_id").Int(), result.Stdout)

	parts := gjson.Get(result.Stdout, "data.api.0.body.parts").Array()
	require.Len(t, parts, 1, "exactly one part carries the page: %s", result.Stdout)
	require.Equal(t, "block_replace", parts[0].Get("action").String(), result.Stdout)
	require.Equal(t, "pYw", parts[0].Get("block_id").String(),
		"block_id must be the page id — that is what makes the backend swap the whole <slide>: %s", result.Stdout)
	require.Contains(t, parts[0].Get("replacement").String(), `<slide id="pYw">`, result.Stdout)
	require.Contains(t, parts[0].Get("replacement").String(), "思源黑体",
		"the caller's own bytes must reach the request unchanged: %s", result.Stdout)
}

// TestSlidesUpdateSlideImageDryRunE2E pins the @path pipeline through the built
// binary: an <img src="@local"> placeholder plans an upload_all step ahead of
// the slide/replace, so the whole-page swap picks up a fresh local image the
// same way +add-slide and +create do. The upload is the irreversible half, so
// a caller who dry-runs first must see it coming.
func TestSlidesUpdateSlideImageDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "chart.png"), []byte("png-bytes"), 0o600))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	const imagePage = `<slide><data>` +
		`<img src="@./chart.png" topLeftX="10" topLeftY="10" width="100" height="100"/>` +
		`</data></slide>`

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+update-slide",
			"--presentation", "presUpdateImageDryRun",
			"--slide-id", "pYw",
			"--content", imagePage,
			"--dry-run",
		},
		DefaultAs: "bot",
		WorkDir:   dir,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.images_to_upload").Int(), result.Stdout)

	api := gjson.Get(result.Stdout, "data.api").Array()
	require.Len(t, api, 2, "an @path page plans upload then replace: %s", result.Stdout)

	require.Equal(t, "POST", gjson.Get(result.Stdout, "data.api.0.method").String(), result.Stdout)
	require.Equal(t, "/open-apis/drive/v1/medias/upload_all",
		gjson.Get(result.Stdout, "data.api.0.url").String(), result.Stdout)
	require.Equal(t, "slide_file",
		gjson.Get(result.Stdout, "data.api.0.body.parent_type").String(), result.Stdout)
	require.Equal(t, "presUpdateImageDryRun",
		gjson.Get(result.Stdout, "data.api.0.body.parent_node").String(), result.Stdout)

	require.Equal(t, "/open-apis/slides_ai/v1/xml_presentations/presUpdateImageDryRun/slide/replace",
		gjson.Get(result.Stdout, "data.api.1.url").String(), result.Stdout)
}

// TestSlidesUpdateAliasDryRunE2E exercises the spellings agents actually type:
// the singular `slide` service alias, the `+update` command alias and `--xml`
// instead of `--content`. All three must land on the canonical request.
func TestSlidesUpdateAliasDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slide", "+update",
			"--token", "presUpdateAliasDryRun",
			"--slide-id", "pYw",
			"--xml", updateSlideDryRunPage,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t,
		"/open-apis/slides_ai/v1/xml_presentations/presUpdateAliasDryRun/slide/replace",
		gjson.Get(result.Stdout, "data.api.0.url").String(),
		result.Stdout,
	)
	parts := gjson.Get(result.Stdout, "data.api.0.body.parts").Array()
	require.Len(t, parts, 1, result.Stdout)
	require.Equal(t, "pYw", parts[0].Get("block_id").String(), result.Stdout)
}

// TestSlidesUpdateSlideRefusesElementRootDryRunE2E: handing over a bare element
// is the frequent mistake, since +replace-slide takes those. Sending it as a page
// would delete everything else, so it must fail before any request is built.
func TestSlidesUpdateSlideRefusesElementRootDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+update-slide",
			"--presentation", "presUpdateSlideDryRun",
			"--slide-id", "pYw",
			"--content", `<shape type="text"><content><p>x</p></content></shape>`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	// RunCmd only errors when the process could not be run at all; a refusal is a
	// non-zero exit with the typed envelope on stderr.
	require.NoError(t, err)
	result.AssertExitCode(t, 2)

	envelope := errorEnvelope(t, result)
	require.False(t, gjson.Get(envelope, "ok").Bool(), envelope)
	require.Equal(t, "validation", gjson.Get(envelope, "error.type").String(), envelope)
	require.Equal(t, "invalid_argument", gjson.Get(envelope, "error.subtype").String(), envelope)
	require.Equal(t, "--content", gjson.Get(envelope, "error.param").String(), envelope)
	require.Contains(t, gjson.Get(envelope, "error.message").String(), "root must be <slide>", envelope)
}

// TestSlidesUpdateSlideRefusesCrossPageIDDryRunE2E: a root id naming a different
// page is the signature of XML read from page A about to be written over page B.
// Overriding it silently would destroy the wrong page.
func TestSlidesUpdateSlideRefusesCrossPageIDDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+update-slide",
			"--presentation", "presUpdateSlideDryRun",
			"--slide-id", "pYw",
			"--content", `<slide id="pOther"><data/></slide>`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	envelope := errorEnvelope(t, result)
	require.False(t, gjson.Get(envelope, "ok").Bool(), envelope)
	require.Equal(t, "validation", gjson.Get(envelope, "error.type").String(), envelope)
	require.Equal(t, "invalid_argument", gjson.Get(envelope, "error.subtype").String(), envelope)
	require.Equal(t, "--content", gjson.Get(envelope, "error.param").String(), envelope)
	require.Contains(t, gjson.Get(envelope, "error.message").String(),
		"pass the page you mean to replace", envelope)
}

// errorEnvelope returns the typed error envelope and asserts the stream contract
// on the way: a Validate-stage refusal exits 2, writes the envelope to stderr and
// leaves stdout empty. Accepting the envelope from either stream would hide a
// regression that prints it to stdout, and an empty stdout is also the real proof
// that no request was built — the error envelope carries no `data.api` to check.
func errorEnvelope(t *testing.T, result *clie2e.Result) string {
	t.Helper()
	require.Empty(t, result.Stdout,
		"stdout is reserved for program data; a refusal must build no request: %s", result.Stdout)
	return result.Stderr
}
