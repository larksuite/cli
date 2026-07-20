// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
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

func TestBuildCreateBodyPrependsTitleToContent(t *testing.T) {
	t.Parallel()

	runtime := newCreateBodyTestRuntime(context.Background())
	if err := runtime.Cmd.Flags().Set("title", "A & B <C>"); err != nil {
		t.Fatalf("set title: %v", err)
	}
	if err := runtime.Cmd.Flags().Set("content", "## Body"); err != nil {
		t.Fatalf("set content: %v", err)
	}

	body := buildCreateBody(runtime)
	if got, want := body["content"], "<title>A &amp; B &lt;C&gt;</title>\n## Body"; got != want {
		t.Fatalf("content = %#v, want %q", got, want)
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

func TestBuildFetchBodyIncludesRevisionAndFullDetail(t *testing.T) {
	t.Parallel()

	runtime := newFetchBodyTestRuntime(context.Background())
	mustSetFetchFlag(t, runtime, "revision-id", "42")
	mustSetFetchFlag(t, runtime, "detail", "full")

	body := buildFetchBody(runtime)
	if got := body["revision_id"]; got != 42 {
		t.Fatalf("revision_id = %#v, want 42", got)
	}
	exportOption, _ := body["export_option"].(map[string]interface{})
	want := map[string]interface{}{
		"export_block_id":        true,
		"export_style_attrs":     true,
		"export_cite_extra_data": true,
	}
	if !reflect.DeepEqual(exportOption, want) {
		t.Fatalf("export_option = %#v, want %#v", exportOption, want)
	}
}

func TestBuildFetchBodyIncludesWithIDsDetail(t *testing.T) {
	t.Parallel()

	runtime := newFetchBodyTestRuntime(context.Background())
	mustSetFetchFlag(t, runtime, "detail", "with-ids")

	body := buildFetchBody(runtime)
	exportOption, _ := body["export_option"].(map[string]interface{})
	want := map[string]interface{}{
		"export_block_id": true,
	}
	if !reflect.DeepEqual(exportOption, want) {
		t.Fatalf("export_option = %#v, want %#v", exportOption, want)
	}
}

func TestBuildFetchBodyIncludesReadOption(t *testing.T) {
	t.Parallel()

	runtime := newFetchBodyTestRuntime(context.Background())
	mustSetFetchFlag(t, runtime, "scope", "section")
	mustSetFetchFlag(t, runtime, "start-block-id", "blk_heading")

	body := buildFetchBody(runtime)
	want := map[string]interface{}{
		"read_mode":      "section",
		"start_block_id": "blk_heading",
	}
	if got := body["read_option"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("read_option = %#v, want %#v", got, want)
	}
}

func TestBuildFetchBodyUsesSelectionAnchorFragmentAsRangeStart(t *testing.T) {
	t.Parallel()

	runtime := newFetchBodyTestRuntime(context.Background())
	mustSetFetchFlag(t, runtime, "doc", "https://example.larksuite.com/wiki/wikcnToken#share-CUE3d6Ykno2fkexEvt8cGF8Wnse")

	body := buildFetchBody(runtime)
	want := map[string]interface{}{
		"read_mode":      "range",
		"start_block_id": "share-CUE3d6Ykno2fkexEvt8cGF8Wnse",
	}
	if got := body["read_option"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("read_option = %#v, want %#v", got, want)
	}
}

func TestBuildFetchBodyExplicitFullIgnoresSelectionAnchorFragment(t *testing.T) {
	t.Parallel()

	runtime := newFetchBodyTestRuntime(context.Background())
	mustSetFetchFlag(t, runtime, "doc", "https://example.larksuite.com/wiki/wikcnToken#share-CUE3d6Ykno2fkexEvt8cGF8Wnse")
	mustSetFetchFlag(t, runtime, "scope", "full")

	body := buildFetchBody(runtime)
	if _, ok := body["read_option"]; ok {
		t.Fatalf("did not expect read_option for explicit full scope: %#v", body["read_option"])
	}
}

func TestBuildFetchBodyDoesNotAutoReadOrdinaryFragment(t *testing.T) {
	t.Parallel()

	runtime := newFetchBodyTestRuntime(context.Background())
	mustSetFetchFlag(t, runtime, "doc", "https://example.larksuite.com/wiki/wikcnToken#blk_plain")

	body := buildFetchBody(runtime)
	if _, ok := body["read_option"]; ok {
		t.Fatalf("did not expect read_option for ordinary URL fragment: %#v", body["read_option"])
	}
}

func TestBuildFetchBodyDoesNotAutoReadUnsupportedSelectionAnchorFragments(t *testing.T) {
	t.Parallel()

	for _, doc := range []string{
		"https://example.larksuite.com/wiki/wikcnToken#part-CUE3d6Ykno2fkexEvt8cGF8Wnse",
		"https://example.larksuite.com/wiki/wikcnToken#share-",
	} {
		runtime := newFetchBodyTestRuntime(context.Background())
		mustSetFetchFlag(t, runtime, "doc", doc)

		body := buildFetchBody(runtime)
		if _, ok := body["read_option"]; ok {
			t.Fatalf("did not expect read_option for unsupported URL fragment %q: %#v", doc, body["read_option"])
		}
	}
}

func TestBuildReadOptionModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setFlags map[string]string
		want     map[string]interface{}
	}{
		{
			name: "full omits read option",
			setFlags: map[string]string{
				"scope": "full",
			},
			want: nil,
		},
		{
			name: "outline with max depth",
			setFlags: map[string]string{
				"scope":     "outline",
				"max-depth": "3",
			},
			want: map[string]interface{}{
				"read_mode": "outline",
				"max_depth": "3",
			},
		},
		{
			name: "range with block ids and context",
			setFlags: map[string]string{
				"scope":          "range",
				"start-block-id": "blk_start",
				"end-block-id":   "blk_end",
				"context-before": "2",
				"context-after":  "1",
				"max-depth":      "0",
			},
			want: map[string]interface{}{
				"read_mode":      "range",
				"start_block_id": "blk_start",
				"end_block_id":   "blk_end",
				"context_before": "2",
				"context_after":  "1",
				"max_depth":      "0",
			},
		},
		{
			name: "keyword with query",
			setFlags: map[string]string{
				"scope":          "keyword",
				"keyword":        "foo|bar",
				"context-before": "1",
			},
			want: map[string]interface{}{
				"read_mode":      "keyword",
				"keyword":        "foo|bar",
				"context_before": "1",
			},
		},
		{
			name: "section keeps unlimited depth omitted",
			setFlags: map[string]string{
				"scope":          "section",
				"start-block-id": "blk_heading",
				"max-depth":      "-1",
			},
			want: map[string]interface{}{
				"read_mode":      "section",
				"start_block_id": "blk_heading",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newFetchBodyTestRuntime(context.Background())
			for name, value := range tt.setFlags {
				mustSetFetchFlag(t, runtime, name, value)
			}

			if got := buildReadOption(runtime); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("buildReadOption() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestValidateReadModeFlagsRejectsInvalidScopeOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setFlags   map[string]string
		wantParam  string
		wantParams []string
	}{
		{
			name: "negative context before",
			setFlags: map[string]string{
				"scope":          "range",
				"start-block-id": "blk_start",
				"context-before": "-1",
			},
			wantParam: "--context-before",
		},
		{
			name: "negative context after",
			setFlags: map[string]string{
				"scope":          "range",
				"start-block-id": "blk_start",
				"context-after":  "-1",
			},
			wantParam: "--context-after",
		},
		{
			name: "max depth below unlimited sentinel",
			setFlags: map[string]string{
				"scope":          "range",
				"start-block-id": "blk_start",
				"max-depth":      "-2",
			},
			wantParam: "--max-depth",
		},
		{
			name: "range needs boundary",
			setFlags: map[string]string{
				"scope": "range",
			},
			wantParams: []string{
				"--start-block-id",
				"--end-block-id",
			},
		},
		{
			name: "keyword needs keyword",
			setFlags: map[string]string{
				"scope": "keyword",
			},
			wantParam: "--keyword",
		},
		{
			name: "section needs start block",
			setFlags: map[string]string{
				"scope": "section",
			},
			wantParam: "--start-block-id",
		},
		{
			name: "unknown scope",
			setFlags: map[string]string{
				"scope": "bad",
			},
			wantParam: "--scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newFetchBodyTestRuntime(context.Background())
			for name, value := range tt.setFlags {
				mustSetFetchFlag(t, runtime, name, value)
			}

			err := validateReadModeFlags(runtime)
			if err == nil {
				t.Fatal("validateReadModeFlags() succeeded, want error")
			}
			assertValidationContract(t, err, errs.SubtypeInvalidArgument, tt.wantParam, tt.wantParams...)
		})
	}
}

func TestValidateReadModeFlagsAcceptsValidScopeOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setFlags map[string]string
	}{
		{
			name: "outline",
			setFlags: map[string]string{
				"scope": "outline",
			},
		},
		{
			name: "range with end block",
			setFlags: map[string]string{
				"scope":        "range",
				"end-block-id": "blk_end",
			},
		},
		{
			name: "default scope with selection anchor fragment",
			setFlags: map[string]string{
				"doc": "https://example.larksuite.com/wiki/wikcnToken#share-CUE3d6Ykno2fkexEvt8cGF8Wnse",
			},
		},
		{
			name: "keyword with keyword",
			setFlags: map[string]string{
				"scope":   "keyword",
				"keyword": "bug|error",
			},
		},
		{
			name: "section with start block",
			setFlags: map[string]string{
				"scope":          "section",
				"start-block-id": "blk_heading",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newFetchBodyTestRuntime(context.Background())
			for name, value := range tt.setFlags {
				mustSetFetchFlag(t, runtime, name, value)
			}

			if err := validateReadModeFlags(runtime); err != nil {
				t.Fatalf("validateReadModeFlags() error = %v", err)
			}
		})
	}
}

func TestValidateFetchV2RejectsInvalidDocAndScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setFlags  map[string]string
		wantParam string
	}{
		{
			name: "invalid doc",
			setFlags: map[string]string{
				"doc": "https://example.com/sheets/sht_token",
			},
			wantParam: "--doc",
		},
		{
			name: "invalid scope",
			setFlags: map[string]string{
				"scope": "bad",
			},
			wantParam: "--scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newFetchShortcutTestRuntime(t, "", tt.setFlags)
			err := validateFetchV2(context.Background(), runtime)
			if err == nil {
				t.Fatal("validateFetchV2() succeeded, want error")
			}
			assertValidationContract(t, err, errs.SubtypeInvalidArgument, tt.wantParam)
		})
	}
}

func TestAddFetchDetailDowngradeWarningNoops(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setFlags map[string]string
	}{
		{
			name: "xml format",
			setFlags: map[string]string{
				"doc-format": "xml",
				"detail":     "full",
			},
		},
		{
			name: "markdown simple detail",
			setFlags: map[string]string{
				"doc-format": "markdown",
				"detail":     "simple",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newFetchBodyTestRuntime(context.Background())
			for name, value := range tt.setFlags {
				mustSetFetchFlag(t, runtime, name, value)
			}

			data := map[string]interface{}{}
			if got := addFetchDetailDowngradeWarning(runtime, data); got != "" {
				t.Fatalf("warning = %q, want empty", got)
			}
			if _, ok := data["warnings"]; ok {
				t.Fatalf("unexpected warnings: %#v", data["warnings"])
			}
		})
	}
}

func TestBuildFetchBodyIncludesFetchExtraParamByDefault(t *testing.T) {
	t.Parallel()

	runtime := newFetchBodyTestRuntime(context.Background())

	body := buildFetchBody(runtime)
	extraParam, ok := body["extra_param"].(string)
	if !ok || extraParam == "" {
		t.Fatalf("extra_param = %#v, want JSON string", body["extra_param"])
	}
	var got map[string]bool
	if err := json.Unmarshal([]byte(extraParam), &got); err != nil {
		t.Fatalf("decode extra_param %q: %v", extraParam, err)
	}
	if got["enable_user_cite_reference_map"] != true {
		t.Fatalf("enable_user_cite_reference_map = %#v, want true in %#v", got["enable_user_cite_reference_map"], got)
	}
	if got["return_html5_block_data"] != true {
		t.Fatalf("return_html5_block_data = %#v, want true in %#v", got["return_html5_block_data"], got)
	}
	if _, ok := got["reference_map_mode"]; ok {
		t.Fatalf("extra_param should not use legacy reference_map_mode: %#v", got)
	}
	if len(got) != 2 {
		t.Fatalf("extra_param should only contain fetch reference_map and html5 data toggles: %#v", got)
	}
}

func TestDocsFetchV2ReferenceMapFlagIsNotAvailable(t *testing.T) {
	t.Parallel()

	for _, flag := range v2FetchFlags() {
		if flag.Name == "reference-map" {
			t.Fatal("fetch should not expose reference-map flag")
		}
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

func TestDocsFetchAPIVersionCompatFlagIsIgnored(t *testing.T) {
	t.Parallel()

	runtime := newFetchShortcutTestRuntime(t, "legacy", nil)
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

func TestDocsFetchIMMarkdownRequestsMarkdownFromAPI(t *testing.T) {
	t.Parallel()

	runtime := newFetchShortcutTestRuntime(t, "", map[string]string{
		"doc-format": "im-markdown",
	})
	if err := validateFetchV2(context.Background(), runtime); err != nil {
		t.Fatalf("validateFetchV2() error = %v", err)
	}

	dry := decodeDocDryRun(t, DocsFetch.DryRun(context.Background(), runtime))
	if got, want := dry.API[0].Body["format"], "markdown"; got != want {
		t.Fatalf("dry-run format = %#v, want %q", got, want)
	}
}

func TestDocsFetchIMMarkdownIgnoresHTML5BlockInsideCodeFence(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-im-markdown-code-fence"))
	registerDocsAIStub(reg, "POST", "/open-apis/docs_ai/v1/documents/doxcnFetchIMMarkdownFence/fetch", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcnFetchIMMarkdownFence",
			"revision_id": float64(1),
			"content":     "```xml\n<html5-block data-ref=\"html5_1\"></html5-block>\n```\n",
		},
	})

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", "doxcnFetchIMMarkdownFence",
		"--doc-format", "im-markdown",
		"--format", "json",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\nraw=%s", err, stdout.String())
	}
	if errField, ok := envelope["error"]; ok {
		t.Fatalf("fetch output should not contain error: %#v", errField)
	}
	data, _ := envelope["data"].(map[string]interface{})
	doc, _ := data["document"].(map[string]interface{})
	content, _ := doc["content"].(string)
	if !strings.Contains(content, "```xml\n<html5-block data-ref=\"html5_1\"></html5-block>\n```") {
		t.Fatalf("fenced html5-block should stay in content, got:\n%s", content)
	}
	if _, ok := doc["reference_map"]; ok {
		t.Fatalf("fenced html5-block should not create reference_map side effects: %#v", doc["reference_map"])
	}
}

func TestDocsFetchMarkdownDetailDowngradesToSimple(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"markdown", "im-markdown"} {
		for _, detail := range []string{"with-ids", "full"} {
			t.Run(format+"/"+detail, func(t *testing.T) {
				t.Parallel()

				runtime := newFetchShortcutTestRuntime(t, "", map[string]string{
					"doc-format": format,
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

func TestDocsFetchV2ReturnsAPIError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-api-error"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/doxcnFetchAPIError/fetch",
		Body: map[string]interface{}{
			"code": 999999,
			"msg":  "fetch failed",
		},
	})

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", "doxcnFetchAPIError",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("mountAndRunDocs() succeeded, want API error")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *errs.APIError (%v)", err, err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf() ok = false for %T (%v)", err, err)
	}
	if p.Category != errs.CategoryAPI {
		t.Errorf("category = %q, want %q", p.Category, errs.CategoryAPI)
	}
	if p.Subtype != errs.SubtypeUnknown {
		t.Errorf("subtype = %q, want %q", p.Subtype, errs.SubtypeUnknown)
	}
	if p.Code != 999999 {
		t.Errorf("code = %d, want 999999", p.Code)
	}
	if p.Message != "fetch failed" {
		t.Errorf("message = %q, want %q", p.Message, "fetch failed")
	}
	if cause := errors.Unwrap(err); cause != nil {
		t.Fatalf("unexpected wrapped cause for API response error: %T %v", cause, cause)
	}
}

func TestDocsFetchIMMarkdownConvertsContentInJSONOutput(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-im-markdown"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/doxcnFetchIMMarkdown/fetch",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"document": map[string]interface{}{
					"document_id": "doxcnFetchIMMarkdown",
					"revision_id": float64(1),
					"content": strings.Join([]string{
						`<title>Doc Title</title>`,
						`<callout emoji="💡">Read **this**.</callout>`,
						`<bookmark name="Example" href="https://example.com"></bookmark>`,
					}, "\n\n"),
				},
			},
		},
	})

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", "doxcnFetchIMMarkdown",
		"--doc-format", "im-markdown",
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
	doc, _ := data["document"].(map[string]interface{})
	content, _ := doc["content"].(string)
	for _, want := range []string{
		"# Doc Title",
		"---\n💡 Read **this**.\n---",
		"[Example](https://example.com)",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("converted content missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "<title>") || strings.Contains(content, "<callout") || strings.Contains(content, "<bookmark") {
		t.Fatalf("converted content still contains downgraded XML tags:\n%s", content)
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
			assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--offset")
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
	cmd.Flags().String("doc", "doxcnFetchDryRun", "")
	cmd.Flags().String("doc-format", fetchDefault("doc-format"), "")
	cmd.Flags().String("detail", fetchDefault("detail"), "")
	cmd.Flags().String("lang", fetchDefault("lang"), "")
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

func mustSetFetchFlag(t *testing.T, runtime *common.RuntimeContext, name, value string) {
	t.Helper()

	if err := runtime.Cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("set %s: %v", name, err)
	}
}

func newFetchShortcutTestRuntime(t *testing.T, apiVersion string, setFlags map[string]string) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().String("api-version", "", "")
	cmd.Flags().String("doc", "doxcnFetchDryRun", "")
	cmd.Flags().String("doc-format", fetchDefault("doc-format"), "")
	cmd.Flags().String("detail", fetchDefault("detail"), "")
	cmd.Flags().String("lang", fetchDefault("lang"), "")
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
	cmd.Flags().String("title", "", "")
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
	cmd.Flags().String("reference-map", "", "")
	cmd.Flags().String("pattern", "", "")
	cmd.Flags().String("block-id", "", "")
	cmd.Flags().String("src-block-ids", "", "")
	return common.TestNewRuntimeContextWithCtx(ctx, cmd, nil)
}
