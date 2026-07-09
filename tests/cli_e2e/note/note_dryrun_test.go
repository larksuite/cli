// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package note

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestNoteDetailDryRun(t *testing.T) {
	setNoteDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"note", "+detail",
			"--note-id", "note_dryrun",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "GET" {
		t.Fatalf("method=%q, want GET\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/vc/v1/notes/note_dryrun" {
		t.Fatalf("url=%q, want note detail endpoint\nstdout:\n%s", got, out)
	}
}

func TestNoteTranscriptDryRun(t *testing.T) {
	setNoteDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"note", "+transcript",
			"--note-id", "note_dryrun",
			"--transcript-format", "plain_text",
			"--dry-run",
		},
		DefaultAs: "user",
		Format:    "json",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.#").Int(); got != 2 {
		t.Fatalf("api count=%d, want 2\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "GET" {
		t.Fatalf("detail method=%q, want GET\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/vc/v1/notes/note_dryrun" {
		t.Fatalf("detail url=%q, want note detail endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.method").String(); got != "GET" {
		t.Fatalf("transcript method=%q, want GET\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/vc/v1/notes/note_dryrun/unified_note_transcript" {
		t.Fatalf("transcript url=%q, want unified transcript endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.params.format").String(); got != "plain_text" {
		t.Fatalf("transcript API format=%q, want plain_text\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.params.page_size").Int(); got != 200 {
		t.Fatalf("page_size=%d, want 200\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.params.locale").String(); got != "zh_cn" {
		t.Fatalf("locale=%q, want zh_cn\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "transcript_format").String(); got != "plain_text" {
		t.Fatalf("transcript_format=%q, want plain_text\nstdout:\n%s", got, out)
	}
}

func setNoteDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "note_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "note_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
