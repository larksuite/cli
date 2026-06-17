// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
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

func TestBuildFetchBodyIncludesExplicitLang(t *testing.T) {
	t.Parallel()

	runtime := newFetchBodyTestRuntime(context.Background())
	if err := runtime.Cmd.Flags().Set("lang", "en-US"); err != nil {
		t.Fatalf("set lang: %v", err)
	}

	body := buildFetchBody(runtime)
	if got := body["lang"]; got != "en-US" {
		t.Fatalf("lang = %#v, want %q", got, "en-US")
	}
}

func TestBuildFetchBodyUsesRuntimeConfigLang(t *testing.T) {
	t.Parallel()

	runtime := newFetchBodyTestRuntime(context.Background())
	runtime.Config = &core.CliConfig{Lang: "zh_cn"}

	body := buildFetchBody(runtime)
	if got := body["lang"]; got != "zh_cn" {
		t.Fatalf("lang = %#v, want %q", got, "zh_cn")
	}
}

func TestBuildFetchBodyExplicitBlankLangOmitsLang(t *testing.T) {
	t.Parallel()

	runtime := newFetchBodyTestRuntime(context.Background())
	runtime.Config = &core.CliConfig{Lang: "zh_cn"}
	if err := runtime.Cmd.Flags().Set("lang", ""); err != nil {
		t.Fatalf("set lang: %v", err)
	}

	body := buildFetchBody(runtime)
	if _, ok := body["lang"]; ok {
		t.Fatalf("did not expect blank explicit lang in fetch body: %#v", body)
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
	if got, want := dry.API[0].Body["format"], "xml"; got != want {
		t.Fatalf("dry-run format = %#v, want %q", got, want)
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

func TestDocsFetchMarkdownDetailDowngradesToSimple(t *testing.T) {
	t.Parallel()

	for _, detail := range []string{"with-ids", "full"} {
		t.Run(detail, func(t *testing.T) {
			t.Parallel()

			runtime := newFetchShortcutTestRuntime(t, "", map[string]string{
				"doc-format": "markdown",
				"detail":     detail,
			})
			if err := validateFetchV2(context.Background(), runtime); err != nil {
				t.Fatalf("validateFetchV2() error = %v", err)
			}

			dry := decodeDocDryRun(t, DocsFetch.DryRun(context.Background(), runtime))
			exportOption, _ := dry.API[0].Body["export_option"].(map[string]interface{})
			if exportOption == nil {
				t.Fatalf("missing export_option: %#v", dry.API[0].Body)
			}
			if got := exportOption["export_block_id"]; got != false {
				t.Fatalf("export_block_id = %#v, want false after markdown detail downgrade", got)
			}
			if got := exportOption["export_style_attrs"]; got != false {
				t.Fatalf("export_style_attrs = %#v, want false after markdown detail downgrade", got)
			}
			if got := exportOption["export_cite_extra_data"]; got != false {
				t.Fatalf("export_cite_extra_data = %#v, want false after markdown detail downgrade", got)
			}
		})
	}
}

func TestDocsFetchMarkdownDetailDowngradeWarnsInOutput(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-detail-warning"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/doxcnFetchWarning/fetch",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"document": map[string]interface{}{
					"document_id": "doxcnFetchWarning",
					"revision_id": float64(1),
					"content":     "# hello",
				},
			},
		},
	})

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", "doxcnFetchWarning",
		"--doc-format", "markdown",
		"--detail", "with-ids",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\nraw=%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	warnings, _ := data["warnings"].([]interface{})
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want one downgrade warning", data["warnings"])
	}
	if got, _ := warnings[0].(string); !strings.Contains(got, "returning markdown output") || !strings.Contains(got, "ignoring the unsupported detail option") {
		t.Fatalf("unexpected warning: %q", got)
	}
}

func TestDocsFetchMarkdownDetailDowngradeWarnsInPrettyOutput(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, stderr, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-detail-pretty-warning"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/doxcnFetchPrettyWarning/fetch",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"document": map[string]interface{}{
					"document_id": "doxcnFetchPrettyWarning",
					"revision_id": float64(1),
					"content":     "# hello",
				},
			},
		},
	})

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", "doxcnFetchPrettyWarning",
		"--doc-format", "markdown",
		"--detail", "full",
		"--format", "pretty",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := stdout.String(); got != "# hello\n" {
		t.Fatalf("stdout = %q, want markdown content only", got)
	}
	if got := stderr.String(); !strings.Contains(got, "warning: --detail full is only supported with --doc-format xml") ||
		!strings.Contains(got, "returning markdown output") ||
		!strings.Contains(got, "ignoring the unsupported detail option") {
		t.Fatalf("stderr missing downgrade warning: %q", got)
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
	cmd.Flags().String("doc-format", "xml", "")
	cmd.Flags().String("detail", "simple", "")
	cmd.Flags().String("lang", "", "")
	cmd.Flags().Int("revision-id", -1, "")
	cmd.Flags().String("scope", "full", "")
	cmd.Flags().String("start-block-id", "", "")
	cmd.Flags().String("end-block-id", "", "")
	cmd.Flags().String("keyword", "", "")
	cmd.Flags().Int("context-before", 0, "")
	cmd.Flags().Int("context-after", 0, "")
	cmd.Flags().Int("max-depth", -1, "")
	return common.TestNewRuntimeContextWithCtx(ctx, cmd, nil)
}

func newFetchShortcutTestRuntime(t *testing.T, apiVersion string, setFlags map[string]string) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("api-version", "", "")
	cmd.Flags().String("doc", "doxcnFetchDryRun", "")
	cmd.Flags().String("doc-format", "xml", "")
	cmd.Flags().String("detail", "simple", "")
	cmd.Flags().String("lang", "", "")
	cmd.Flags().Int("revision-id", -1, "")
	cmd.Flags().String("scope", "full", "")
	cmd.Flags().String("start-block-id", "", "")
	cmd.Flags().String("end-block-id", "", "")
	cmd.Flags().String("keyword", "", "")
	cmd.Flags().Int("context-before", 0, "")
	cmd.Flags().Int("context-after", 0, "")
	cmd.Flags().Int("max-depth", -1, "")
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
