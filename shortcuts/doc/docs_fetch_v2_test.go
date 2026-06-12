// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestBuildFetchBodyIncludesSceneFromContext(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), docsSceneContextKey, " DoubaoCLI ")
	runtime := newFetchBodyTestRuntime(ctx)

	body := buildFetchBody(runtime)
	if got := body["scene"]; got != "DoubaoCLI" {
		t.Fatalf("scene = %#v, want %q", got, "DoubaoCLI")
	}
}

func TestBuildCreateBodyIncludesSceneFromContext(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), docsSceneContextKey, "DoubaoCLI")
	runtime := newCreateBodyTestRuntime(ctx)

	body := buildCreateBody(runtime)
	if got := body["scene"]; got != "DoubaoCLI" {
		t.Fatalf("scene = %#v, want %q", got, "DoubaoCLI")
	}
}

func TestBuildUpdateBodyIncludesSceneFromContext(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), docsSceneContextKey, "DoubaoCLI")
	runtime := newUpdateBodyTestRuntime(ctx)

	body := buildUpdateBody(runtime)
	if got := body["scene"]; got != "DoubaoCLI" {
		t.Fatalf("scene = %#v, want %q", got, "DoubaoCLI")
	}
}

func TestBuildFetchBodyOmitsEmptyScene(t *testing.T) {
	t.Parallel()

	runtime := newFetchBodyTestRuntime(context.Background())

	body := buildFetchBody(runtime)
	if _, ok := body["scene"]; ok {
		t.Fatalf("did not expect empty scene in fetch body: %#v", body)
	}
}

func TestDocsFetchDryRunDefaultsToV2Endpoint(t *testing.T) {
	t.Parallel()

	runtime := newFetchShortcutTestRuntime(t, "", nil)
	if err := validateFetchV2(context.Background(), runtime); err != nil {
		t.Fatalf("validateFetchV2() error = %v", err)
	}

	dry := decodeDocDryRun(t, DocsFetch.DryRun(context.Background(), runtime))
	if len(dry.API) != 1 {
		t.Fatalf("expected 1 dry-run API call, got %d", len(dry.API))
	}
	if got, want := dry.API[0].URL, "/open-apis/docs_ai/v1/documents/doxcnFetchDryRun/fetch"; got != want {
		t.Fatalf("dry-run URL = %q, want %q", got, want)
	}
	if got, want := dry.API[0].Body["format"], "markdown"; got != want {
		t.Fatalf("dry-run format = %#v, want %q", got, want)
	}
}

func TestDocsFetchDetailWithIdsAutoUpgradesFormatToXml(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setFlags map[string]string
		wantFmt  string
		wantErr  bool
	}{
		{
			name:     "with-ids without explicit doc-format auto-upgrades to xml",
			setFlags: map[string]string{"detail": "with-ids"},
			wantFmt:  "xml",
		},
		{
			name:     "full without explicit doc-format auto-upgrades to xml",
			setFlags: map[string]string{"detail": "full"},
			wantFmt:  "xml",
		},
		{
			name:     "with-ids with explicit xml stays xml",
			setFlags: map[string]string{"detail": "with-ids", "doc-format": "xml"},
			wantFmt:  "xml",
		},
		{
			name:     "with-ids with explicit markdown errors",
			setFlags: map[string]string{"detail": "with-ids", "doc-format": "markdown"},
			wantErr:  true,
		},
		{
			name:     "simple detail keeps markdown default",
			setFlags: map[string]string{"detail": "simple"},
			wantFmt:  "markdown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newFetchShortcutTestRuntime(t, "", tt.setFlags)
			err := validateFetchV2(context.Background(), runtime)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected validation error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("validateFetchV2() error = %v", err)
			}
			if got := runtime.Str("doc-format"); got != tt.wantFmt {
				t.Fatalf("doc-format = %q, want %q", got, tt.wantFmt)
			}

			dry := decodeDocDryRun(t, DocsFetch.DryRun(context.Background(), runtime))
			if got := dry.API[0].Body["format"]; got != tt.wantFmt {
				t.Fatalf("dry-run format = %#v, want %q", got, tt.wantFmt)
			}
		})
	}
}

func TestDocsFetchAPIVersionV1StillUsesV2Endpoint(t *testing.T) {
	t.Parallel()

	runtime := newFetchShortcutTestRuntime(t, "v1", nil)
	if err := validateFetchV2(context.Background(), runtime); err != nil {
		t.Fatalf("validateFetchV2() error = %v", err)
	}

	dry := decodeDocDryRun(t, DocsFetch.DryRun(context.Background(), runtime))
	if len(dry.API) != 1 {
		t.Fatalf("expected 1 dry-run API call, got %d", len(dry.API))
	}
	if got, want := dry.API[0].URL, "/open-apis/docs_ai/v1/documents/doxcnFetchDryRun/fetch"; got != want {
		t.Fatalf("dry-run URL = %q, want %q", got, want)
	}
}

func TestDocsFetchRejectsLegacyFlags(t *testing.T) {
	tests := []struct {
		name     string
		setFlags map[string]string
		want     []string
	}{
		{
			name:     "legacy offset",
			setFlags: map[string]string{"offset": "10"},
			want: []string{
				"docs +fetch is v2-only",
				"the old v1 interface has been shut down",
				"legacy v1 flag(s) --offset are no longer supported",
				"--offset -> use --scope outline/range/keyword/section",
				"lark-cli skills read lark-doc references/lark-doc-fetch.md",
				"MUST NOT grep/open local SKILL.md files",
				"lark-cli docs +fetch --help",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newFetchShortcutTestRuntime(t, "", tt.setFlags)
			err := validateFetchV2(context.Background(), runtime)
			if err == nil {
				t.Fatal("expected v2-only validation error")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error missing %q: %v", want, err)
				}
			}
		})
	}
}

func newFetchBodyTestRuntime(ctx context.Context) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("doc-format", fetchDefault("doc-format"), "")
	cmd.Flags().String("detail", fetchDefault("detail"), "")
	cmd.Flags().Int("revision-id", fetchDefaultInt("revision-id"), "")
	cmd.Flags().String("scope", fetchDefault("scope"), "")
	cmd.Flags().String("start-block-id", fetchDefault("start-block-id"), "")
	cmd.Flags().String("end-block-id", fetchDefault("end-block-id"), "")
	cmd.Flags().String("keyword", fetchDefault("keyword"), "")
	cmd.Flags().Int("context-before", fetchDefaultInt("context-before"), "")
	cmd.Flags().Int("context-after", fetchDefaultInt("context-after"), "")
	cmd.Flags().Int("max-depth", fetchDefaultInt("max-depth"), "")
	return common.TestNewRuntimeContextWithCtx(ctx, cmd, nil)
}

// fetchDefault returns the declared default for a flag from the real
// v2FetchFlags definition so tests don't hardcode a stale default.
// It panics if the flag is not found, since a missing flag indicates
// a test setup error rather than a runtime condition.
func fetchDefault(name string) string {
	for _, fl := range v2FetchFlags() {
		if fl.Name == name {
			return fl.Default
		}
	}
	panic(fmt.Sprintf("fetchDefault: flag %q not found in v2FetchFlags", name))
}

// fetchDefaultInt returns the declared default for an int flag from
// v2FetchFlags, parsed as an int. It panics if the flag is not found
// or its default cannot be parsed as an int.
func fetchDefaultInt(name string) int {
	s := fetchDefault(name)
	if s == "" {
		return 0
	}
	var d int
	if _, err := fmt.Sscanf(s, "%d", &d); err != nil {
		panic(fmt.Sprintf("fetchDefaultInt: flag %q default %q is not an int", name, s))
	}
	return d
}

func newFetchShortcutTestRuntime(t *testing.T, apiVersion string, setFlags map[string]string) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("api-version", "", "")
	cmd.Flags().String("doc", "doxcnFetchDryRun", "")
	cmd.Flags().String("doc-format", fetchDefault("doc-format"), "")
	cmd.Flags().String("detail", fetchDefault("detail"), "")
	cmd.Flags().Int("revision-id", fetchDefaultInt("revision-id"), "")
	cmd.Flags().String("scope", fetchDefault("scope"), "")
	cmd.Flags().String("start-block-id", fetchDefault("start-block-id"), "")
	cmd.Flags().String("end-block-id", fetchDefault("end-block-id"), "")
	cmd.Flags().String("keyword", fetchDefault("keyword"), "")
	cmd.Flags().Int("context-before", fetchDefaultInt("context-before"), "")
	cmd.Flags().Int("context-after", fetchDefaultInt("context-after"), "")
	cmd.Flags().Int("max-depth", fetchDefaultInt("max-depth"), "")
	cmd.Flags().String("offset", "", "")
	cmd.Flags().String("limit", "", "")
	if apiVersion != "" {
		if err := cmd.Flags().Set("api-version", apiVersion); err != nil {
			t.Fatalf("set api-version: %v", err)
		}
	}
	for name, value := range setFlags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}
	return common.TestNewRuntimeContext(cmd, nil)
}

func newCreateBodyTestRuntime(ctx context.Context) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "+create"}
	cmd.Flags().String("doc-format", "xml", "")
	cmd.Flags().String("content", "<title>hello</title>", "")
	cmd.Flags().String("parent-token", "", "")
	cmd.Flags().String("parent-position", "", "")
	return common.TestNewRuntimeContextWithCtx(ctx, cmd, nil)
}

func newUpdateBodyTestRuntime(ctx context.Context) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "+update"}
	cmd.Flags().String("doc-format", "xml", "")
	cmd.Flags().String("command", "append", "")
	cmd.Flags().Int("revision-id", 0, "")
	cmd.Flags().String("content", "<p>hello</p>", "")
	cmd.Flags().String("pattern", "", "")
	cmd.Flags().String("block-id", "", "")
	cmd.Flags().String("src-block-ids", "", "")
	return common.TestNewRuntimeContextWithCtx(ctx, cmd, nil)
}
