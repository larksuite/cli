// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// page1 is deliberately multi-line and quote-heavy. Getting this into a JSON
// array from a shell is the whole reason the file inputs exist: callers reached
// for jq to do the escaping, and every environment without jq turned the
// substitution into an empty argument.
const (
	page1 = "<slide xmlns=\"https://www.larkoffice.com/sml/2.0\">\n  <data>\n    <shape type=\"text\"><content>Q1 \"results\" &amp; outlook</content></shape>\n  </data>\n</slide>"
	page2 = `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data></data></slide>`
)

// writeSlidePages writes the two fixture pages into a temp dir and returns it.
func writeSlidePages(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{"slide-01.xml": page1, "slide-02.xml": page2} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	return dir
}

// mustMarshalDeck renders pages as the --slides JSON array.
func mustMarshalDeck(t *testing.T, pages ...string) []byte {
	t.Helper()
	deck, err := json.Marshal(pages)
	require.NoError(t, err)
	return deck
}

// TestSlidesCreateRepeatedSlideFileDryRunE2E proves the assembled array through
// the real binary, which is the only layer that shows the file bytes reaching
// the request unchanged: no shell escaping, no JSON encoder in the caller's
// hands, and flag order preserved as page order.
func TestSlidesCreateRepeatedSlideFileDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+create",
			"--title", "Assembled Deck",
			"--slide", "@./slide-01.xml",
			"--slide", "@./slide-02.xml",
			"--dry-run",
		},
		WorkDir:   writeSlidePages(t),
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t, "POST", gjson.Get(result.Stdout, "data.api.0.method").String(), result.Stdout)
	require.Equal(t, "/open-apis/slides_ai/v1/xml_presentations", gjson.Get(result.Stdout, "data.api.0.url").String(), result.Stdout)
	require.Equal(t, page1, gjson.Get(result.Stdout, "data.api.1.body.slide.content").String(), result.Stdout)
	require.Equal(t, page2, gjson.Get(result.Stdout, "data.api.2.body.slide.content").String(), result.Stdout)
	require.False(t, gjson.Get(result.Stdout, "data.api.3").Exists(), result.Stdout)
}

// TestSlidesCreateSlidesFileAndStdinDryRunE2E covers the other input form for
// callers who already hold the finished array.
func TestSlidesCreateSlidesFileAndStdinDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	deck := mustMarshalDeck(t, page2)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deck.json"), deck, 0o600))

	tests := []struct {
		name  string
		args  []string
		stdin []byte
	}{
		{name: "from file", args: []string{"--slides", "@./deck.json"}},
		{name: "from stdin", args: []string{"--slides", "-"}, stdin: deck},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      append([]string{"slides", "+create", "--title", "Deck", "--dry-run"}, tt.args...),
				Stdin:     tt.stdin,
				WorkDir:   dir,
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Equal(t, page2, gjson.Get(result.Stdout, "data.api.1.body.slide.content").String(), result.Stdout)
		})
	}
}

// TestSlidesCreateRejectsBothSlideFormsDryRunE2E pins the user-visible refusal.
// The package test sees the Go error directly and never runs the dispatcher, so
// only this proves the exit code and the typed envelope an agent parses to
// repair its own command.
func TestSlidesCreateRejectsBothSlideFormsDryRunE2E(t *testing.T) {
	setSlidesDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"slides", "+create",
			"--title", "Both",
			"--slides", string(mustMarshalDeck(t, page2)),
			"--slide", page2,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)

	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--slide", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "cannot be combined", result.Stderr)
	// The refusal happens before any plan exists, so stdout carries no partial
	// dry-run payload a caller could mistake for a validated command.
	require.Empty(t, result.Stdout, result.Stdout)
}
