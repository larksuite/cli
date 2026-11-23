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

// TestSlidesReplaceSlideNormalizationDryRunE2E pins the compatibility contract
// through the built binary: aliases are accepted, the request is canonical, and
// structured output records every conversion that was applied.
func TestSlidesReplaceSlideNormalizationDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	parts := `[{"action":"replace","target_id":"bUn","content":"` +
		`<shape type=\"text\"/>"},{"action":"insert","element":"<shape type=\"rect\"/>"}]`

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+replace-slide",
			"--presentation", "presReplaceSlideDryRun",
			"--slide-id", "pYw",
			"--parts", parts,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	body := gjson.Get(result.Stdout, "data.api.0.body.parts").Array()
	require.Len(t, body, 2, result.Stdout)
	require.Equal(t, "block_replace", body[0].Get("action").String(), result.Stdout)
	require.Equal(t, "bUn", body[0].Get("block_id").String(), result.Stdout)
	require.True(t, body[0].Get("replacement").Exists(), result.Stdout)
	require.False(t, body[0].Get("target_id").Exists(), result.Stdout)
	require.False(t, body[0].Get("content").Exists(), result.Stdout)
	require.Equal(t, "block_insert", body[1].Get("action").String(), result.Stdout)
	require.True(t, body[1].Get("insertion").Exists(), result.Stdout)
	require.False(t, body[1].Get("element").Exists(), result.Stdout)

	normalizations := gjson.Get(result.Stdout, "data.normalizations").Array()
	requireReplaceSlideAliasNormalizations(t, normalizations, result.Stdout)
}

func requireReplaceSlideAliasNormalizations(t *testing.T, got []gjson.Result, output string) {
	t.Helper()
	want := []struct {
		partIndex int64
		kind      string
		from      string
		to        string
	}{
		{0, "action", "replace", "block_replace"},
		{0, "field", "target_id", "block_id"},
		{0, "field", "content", "replacement"},
		{1, "action", "insert", "block_insert"},
		{1, "field", "element", "insertion"},
	}
	require.Len(t, got, len(want), output)
	for i, expected := range want {
		require.Equal(t, expected.partIndex, got[i].Get("part_index").Int(), output)
		require.Equal(t, expected.kind, got[i].Get("kind").String(), output)
		require.Equal(t, expected.from, got[i].Get("from").String(), output)
		require.Equal(t, expected.to, got[i].Get("to").String(), output)
	}
}

// TestSlidesReplaceSlideEmptyReplacementDryRunE2E guards the other side of the
// split: an actually-empty payload must keep the non-empty wording, so the two
// failures stay distinguishable to whoever reads the envelope.
func TestSlidesReplaceSlideEmptyReplacementDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+replace-slide",
			"--presentation", "presReplaceSlideDryRun",
			"--slide-id", "pYw",
			"--parts", `[{"action":"block_replace","block_id":"bUn","replacement":""}]`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)

	require.Empty(t, result.Stdout,
		"a rejected command must not print a success envelope: %s", result.Stdout)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--parts", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(),
		"requires non-empty replacement", result.Stderr)
}

// TestSlidesReplaceSlideDryRunE2E keeps the accepted path honest: the field
// whitelist must not reject a legitimate mixed batch, and the request body must
// carry both parts with the id injected into the block_replace fragment.
func TestSlidesReplaceSlideDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	parts := `[{"action":"block_replace","block_id":"bUn","replacement":"` +
		`<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\">` +
		`<content textType=\"title\"><p>hi</p></content></shape>"},` +
		`{"action":"block_insert","insertion":"<shape type=\"rect\" width=\"100\" height=\"100\"/>",` +
		`"insert_before_block_id":"bUn"}]`

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+replace-slide",
			"--presentation", "presReplaceSlideDryRun",
			"--slide-id", "pYw",
			"--parts", parts,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t, "POST", gjson.Get(result.Stdout, "data.api.0.method").String(), result.Stdout)
	require.Equal(t,
		"/open-apis/slides_ai/v1/xml_presentations/presReplaceSlideDryRun/slide/replace",
		gjson.Get(result.Stdout, "data.api.0.url").String(),
		result.Stdout,
	)
	require.Equal(t, "pYw", gjson.Get(result.Stdout, "data.api.0.params.slide_id").String(), result.Stdout)

	body := gjson.Get(result.Stdout, "data.api.0.body.parts").Array()
	require.Len(t, body, 2, "both parts of the mixed batch must survive validation: %s", result.Stdout)
	require.Equal(t, "block_replace", body[0].Get("action").String(), result.Stdout)
	require.Equal(t, "bUn", body[0].Get("block_id").String(), result.Stdout)
	require.Contains(t, body[0].Get("replacement").String(), `id="bUn"`,
		"the shortcut injects the block id into the fragment root: %s", result.Stdout)
	require.Equal(t, "block_insert", body[1].Get("action").String(), result.Stdout)
	require.Equal(t, `<shape type="rect" width="100" height="100"><content/></shape>`,
		body[1].Get("insertion").String(),
		"the insertion payload must reach the request, with <content/> filled in: %s", result.Stdout)
	require.Equal(t, "bUn", body[1].Get("insert_before_block_id").String(),
		"insert_before_block_id is a valid block_insert field and must not be rejected: %s", result.Stdout)
}
