// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIM_MessagesSendAudioDryRunRejectsNonOpus(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "im_audio_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "im_audio_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	workDir := t.TempDir()
	audioPath := filepath.Join(workDir, "voice.mp3")
	require.NoError(t, os.WriteFile(audioPath, []byte("not real mp3; validation checks extension before upload"), 0o600))

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"im", "+messages-send",
			"--chat-id", "oc_123",
			"--audio", "./voice.mp3",
			"--dry-run",
		},
		DefaultAs: "bot",
		WorkDir:   workDir,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)

	if got := gjson.Get(result.Stderr, "error.type").String(); got != "validation" {
		t.Fatalf("error.type = %q, want validation\nstderr:\n%s", got, result.Stderr)
	}
	if got := gjson.Get(result.Stderr, "error.subtype").String(); got != "invalid_argument" {
		t.Fatalf("error.subtype = %q, want invalid_argument\nstderr:\n%s", got, result.Stderr)
	}
	if got := gjson.Get(result.Stderr, "error.param").String(); got != "--audio" {
		t.Fatalf("error.param = %q, want --audio\nstderr:\n%s", got, result.Stderr)
	}
	message := gjson.Get(result.Stderr, "error.message").String()
	if !strings.Contains(message, "--audio supports only Opus audio files") {
		t.Fatalf("error.message = %q, want Opus guidance\nstderr:\n%s", message, result.Stderr)
	}
	hint := gjson.Get(result.Stderr, "error.hint").String()
	if !strings.Contains(hint, "--file") || !strings.Contains(hint, "ffmpeg") {
		t.Fatalf("error.hint = %q, want --file and ffmpeg guidance\nstderr:\n%s", hint, result.Stderr)
	}
}
