// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestWhiteboardParseImageDryRun_RequestShapes(t *testing.T) {
	setWhiteboardDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("submit", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"whiteboard", "+parse-image",
				"--whiteboard-token", "wbcnParseImageDryRun",
				"--image", "./input.png",
				"--mode", "agentic",
				"--idempotent-token", "parse-token-12345",
				"--overwrite",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		out := result.Stdout
		if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "POST" {
			t.Fatalf("method=%q, want POST\nstdout:\n%s", got, out)
		}
		gotURL := clie2e.DryRunGet(out, "api.0.url").String()
		if !strings.HasPrefix(gotURL, "/open-apis/board/v1/whiteboards/") || !strings.HasSuffix(gotURL, "/parse_image") {
			t.Fatalf("url=%q, want parse_image submit URL\nstdout:\n%s", gotURL, out)
		}
		if got := clie2e.DryRunGet(out, "api.0.body.image_file").String(); got != "@./input.png" {
			t.Fatalf("body.image_file=%q, want @./input.png\nstdout:\n%s", got, out)
		}
		if got := clie2e.DryRunGet(out, "api.0.body.client_token").String(); got != "parse-token-12345" {
			t.Fatalf("body.client_token=%q, want parse-token-12345\nstdout:\n%s", got, out)
		}
		if got := clie2e.DryRunGet(out, "api.0.body.overwrite").Bool(); !got {
			t.Fatalf("body.overwrite=%v, want true\nstdout:\n%s", got, out)
		}
		if got := clie2e.DryRunGet(out, "api.0.body.mode").String(); got != "agentic" {
			t.Fatalf("body.mode=%q, want agentic\nstdout:\n%s", got, out)
		}
	})

	t.Run("result", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"whiteboard", "+parse-image-result",
				"--whiteboard-token", "wbcnParseImageDryRun",
				"--task-id", "7670001",
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
		gotURL := clie2e.DryRunGet(out, "api.0.url").String()
		if !strings.HasPrefix(gotURL, "/open-apis/board/v1/whiteboards/") || !strings.HasSuffix(gotURL, "/parse_image/7670001") {
			t.Fatalf("url=%q, want parse_image result URL\nstdout:\n%s", gotURL, out)
		}
	})
}
