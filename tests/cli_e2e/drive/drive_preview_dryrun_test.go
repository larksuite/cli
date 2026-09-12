// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// TestDrivePreviewDryRun_EntityLookup pins token identification and the
// downstream request plan in every preview mode and accepted input form.
func TestDrivePreviewDryRun_EntityLookup(t *testing.T) {
	for _, source := range []struct {
		name, flag, input string
		wiki              bool
	}{
		{"file", "--file-token", "inputToken", false},
		{"file URL", "--url", "https://example.com/file/inputToken", false},
		{"wiki", "--wiki-token", "inputToken", true},
		{"wiki URL", "--url", "https://example.com/wiki/inputToken", true},
	} {
		for _, mode := range []string{"list", "source_file", "pdf"} {
			t.Run(source.name+"/"+mode, func(t *testing.T) {
				setDriveDryRunConfigEnv(t)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				t.Cleanup(cancel)
				args := []string{"drive", "+preview", source.flag, source.input, "--version", "12", "--dry-run"}
				if mode == "list" {
					args = append(args, "--list-only")
				} else {
					args = append(args, "--type", mode, "--output", "./artifacts/preview")
				}
				result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				out := result.Stdout
				require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String())
				require.Equal(t, "/open-apis/drive/v2/files/query_by_token", clie2e.DryRunGet(out, "api.0.url").String())
				require.Equal(t, "inputToken", clie2e.DryRunGet(out, "api.0.params.token").String())
				require.Contains(t, clie2e.DryRunGet(out, "api.0.desc").String(), "Best-effort")
				index := 1
				if source.wiki {
					require.Equal(t, "GET", clie2e.DryRunGet(out, "api.1.method").String())
					require.Equal(t, "/open-apis/wiki/v2/spaces/get_node", clie2e.DryRunGet(out, "api.1.url").String())
					require.Equal(t, "inputToken", clie2e.DryRunGet(out, "api.1.params.token").String())
					require.Contains(t, clie2e.DryRunGet(out, "api.1.desc").String(), "Only if entity lookup fails")
					require.Equal(t, "inputToken", clie2e.DryRunGet(out, "wiki_token").String())
					index++
				} else {
					require.Equal(t, "inputToken", clie2e.DryRunGet(out, "fallback_file_token").String())
				}
				field := func(suffix string) string { return fmt.Sprintf("api.%d.%s", index, suffix) }
				if mode != "source_file" {
					require.Equal(t, "POST", clie2e.DryRunGet(out, field("method")).String())
					require.Equal(t, "/open-apis/drive/v1/medias/resolved_file_token/preview_result", clie2e.DryRunGet(out, field("url")).String())
					require.Equal(t, "12", clie2e.DryRunGet(out, field("body.version")).String())
					index++
				}
				if mode == "list" {
					require.Equal(t, "list", clie2e.DryRunGet(out, "mode").String())
				} else {
					require.Equal(t, "download", clie2e.DryRunGet(out, "mode").String())
					require.Equal(t, mode, clie2e.DryRunGet(out, "requested_type").String())
					require.Equal(t, "./artifacts/preview", clie2e.DryRunGet(out, "output").String())
					require.Equal(t, "GET", clie2e.DryRunGet(out, field("method")).String())
					require.Equal(t, "/open-apis/drive/v1/medias/resolved_file_token/preview_download", clie2e.DryRunGet(out, field("url")).String())
					require.Equal(t, "12", clie2e.DryRunGet(out, field("params.version")).String())
					wantType := "<selected type_code from preview_result>"
					if mode == "source_file" {
						wantType = "16"
						require.Equal(t, "source_file", clie2e.DryRunGet(out, "selected_type").String())
						require.Equal(t, "16", clie2e.DryRunGet(out, "selected_type_code").String())
					}
					require.Equal(t, wantType, clie2e.DryRunGet(out, field("params.preview_type")).String())
					index++
				}
				require.Equal(t, int64(index), clie2e.DryRunGet(out, "api.#").Int())
			})
		}
	}
}

// TestDriveCoverDryRun_Download verifies cover dry-run request structure for
// download mode.
func TestDriveCoverDryRun_Download(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+cover",
			"--file-token", "fileDryRunCover",
			"--spec", "square",
			"--output", "./artifacts/cover",
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
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/medias/fileDryRunCover/preview_download" {
		t.Fatalf("url=%q, want preview_download endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.preview_type").String(); got != "1" {
		t.Fatalf("preview_type=%q, want 1\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.bus_type").Exists(); got {
		t.Fatalf("bus_type should be omitted for square crop flow\nstdout:\n%s", out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.platform").Exists(); got {
		t.Fatalf("platform should be omitted when using default platform\nstdout:\n%s", out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.width").String(); got != "360" {
		t.Fatalf("width=%q, want 360\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.height").String(); got != "360" {
		t.Fatalf("height=%q, want 360\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.policy").String(); got != "near" {
		t.Fatalf("policy=%q, want near\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "selected_spec").String(); got != "square" {
		t.Fatalf("selected_spec=%q, want square\nstdout:\n%s", got, out)
	}
}
