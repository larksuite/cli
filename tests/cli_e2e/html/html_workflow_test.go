// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package html

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// The fixture must be an explicitly provisioned, reusable HTML entry. There is
// no CLI deletion endpoint, so the test never creates a disposable publication.
func TestHTMLPublishFetchWorkflow(t *testing.T) {
	entry := os.Getenv("LARK_CLI_TEST_HTML_ENTRY")
	if entry == "" {
		t.Skip("requires a reusable HTML entry fixture and deployed publish/download APIs; see coverage.md")
	}
	clie2e.SkipWithoutUserToken(t)
	abs, err := filepath.Abs(entry)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"html", "+publish", "--file-path", filepath.Base(abs)}, DefaultAs: "user", WorkDir: filepath.Dir(abs)})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	token := gjson.Get(result.Stdout, "data.token").String()
	require.NotEmpty(t, token)
	// Repeating the same entry must update the same document, including retries.
	repeated, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"html", "+publish", "--file-path", filepath.Base(abs)}, DefaultAs: "user", WorkDir: filepath.Dir(abs)})
	require.NoError(t, err)
	repeated.AssertExitCode(t, 0)
	assert.Equal(t, token, gjson.Get(repeated.Stdout, "data.token").String())
	dir := t.TempDir()
	fetched, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"html", "+fetch", "--token", token, "--output", "downloads/source.zip"}, DefaultAs: "user", WorkDir: dir})
	require.NoError(t, err)
	fetched.AssertExitCode(t, 0)
	raw, err := os.ReadFile(filepath.Join(dir, "downloads/source.zip"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(raw), 4)
	assert.Equal(t, "PK", string(raw[:2]))
}
