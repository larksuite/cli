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
	"github.com/tidwall/gjson"
)

func TestWhiteboardExportDryRun_RequestShapes(t *testing.T) {
	setWhiteboardDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		name       string
		args       []string
		wantMethod string
		wantSuffix string
		wantBody   map[string]string
	}{
		{
			name: "preview",
			args: []string{
				"whiteboard", "+export",
				"--whiteboard-token", "wbcnDryRunPreview",
				"--output-type", "preview",
				"--output", "preview",
				"--dry-run",
			},
			wantMethod: "GET",
			wantSuffix: "/download_as_image",
		},
		{
			name: "svg",
			args: []string{
				"whiteboard", "+export",
				"--whiteboard-token", "wbcnDryRunSvg",
				"--output-type", "svg",
				"--dry-run",
			},
			wantMethod: "POST",
			wantSuffix: "/export",
			wantBody: map[string]string{
				"export_type": "svg",
			},
		},
		{
			name: "source",
			args: []string{
				"whiteboard", "+export",
				"--whiteboard-token", "wbcnDryRunSource",
				"--output-type", "source",
				"--dry-run",
			},
			wantMethod: "GET",
			wantSuffix: "/nodes",
		},
		{
			name: "raw",
			args: []string{
				"whiteboard", "+export",
				"--whiteboard-token", "wbcnDryRunRaw",
				"--output-type", "raw",
				"--dry-run",
			},
			wantMethod: "GET",
			wantSuffix: "/nodes",
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

			out := result.Stdout
			if got := clie2e.DryRunGet(out, "api.#").Int(); got != 1 {
				t.Fatalf("api count=%d, want 1\nstdout:\n%s", got, out)
			}
			if got := clie2e.DryRunGet(out, "api.0.method").String(); got != tt.wantMethod {
				t.Fatalf("method=%q, want %q\nstdout:\n%s", got, tt.wantMethod, out)
			}
			gotURL := clie2e.DryRunGet(out, "api.0.url").String()
			if !strings.HasPrefix(gotURL, "/open-apis/board/v1/whiteboards/") || !strings.HasSuffix(gotURL, tt.wantSuffix) {
				t.Fatalf("url=%q, want board whiteboard URL ending %q\nstdout:\n%s", gotURL, tt.wantSuffix, out)
			}
			for key, want := range tt.wantBody {
				if got := clie2e.DryRunGet(out, "api.0.body."+key).String(); got != want {
					t.Fatalf("body.%s=%q, want %q\nstdout:\n%s", key, got, want, out)
				}
			}
		})
	}
}

func TestWhiteboardQueryDryRun_LegacySmoke(t *testing.T) {
	setWhiteboardDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"whiteboard", "+query",
			"--whiteboard-token", "wbcnDryRunLegacy",
			"--output_as", "image",
			"--output", "preview",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "GET" {
		t.Fatalf("method=%q, want GET\nstdout:\n%s", got, out)
	}
	gotURL := clie2e.DryRunGet(out, "api.0.url").String()
	if !strings.HasPrefix(gotURL, "/open-apis/board/v1/whiteboards/") || !strings.HasSuffix(gotURL, "/download_as_image") {
		t.Fatalf("url=%q, want preview download\nstdout:\n%s", gotURL, out)
	}
}

func TestWhiteboardExportSelectorRequiredBeforeAuth(t *testing.T) {
	setWhiteboardDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("export requires output-type", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"whiteboard", "+export",
				"--whiteboard-token", "wbcnMissingSelector",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		output := result.Stdout + "\n" + result.Stderr
		if got := gjson.Get(output, "error.type").String(); got != "validation" {
			t.Fatalf("error.type=%q, want validation\nstdout:\n%s\nstderr:\n%s", got, result.Stdout, result.Stderr)
		}
		if got := gjson.Get(output, "error.message").String(); !strings.Contains(got, "output-type") {
			t.Fatalf("error.message=%q, want output-type\nstdout:\n%s\nstderr:\n%s", got, result.Stdout, result.Stderr)
		}
	})

	t.Run("legacy query requires output_as", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"whiteboard", "+query",
				"--whiteboard-token", "wbcnMissingSelector",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		output := result.Stdout + "\n" + result.Stderr
		if got := gjson.Get(output, "error.type").String(); got != "validation" {
			t.Fatalf("error.type=%q, want validation\nstdout:\n%s\nstderr:\n%s", got, result.Stdout, result.Stderr)
		}
		if got := gjson.Get(output, "error.message").String(); !strings.Contains(got, "output_as") {
			t.Fatalf("error.message=%q, want output_as\nstdout:\n%s\nstderr:\n%s", got, result.Stdout, result.Stderr)
		}
	})
}

func setWhiteboardDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "whiteboard_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "whiteboard_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
