// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
package doc

import (
	"bytes"
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

// ── V2 tests ──

func TestValidCommandsV2(t *testing.T) {
	expected := map[string]bool{
		"str_replace":             true,
		"block_delete":            true,
		"block_insert_after":      true,
		"block_copy_insert_after": true,
		"block_replace":           true,
		"block_move_after":        true,
		"overwrite":               true,
		"append":                  true,
	}
	if len(validCommandsV2) != len(expected) {
		t.Fatalf("expected %d commands, got %d", len(expected), len(validCommandsV2))
	}
	for cmd := range validCommandsV2 {
		if !expected[cmd] {
			t.Fatalf("unexpected command %q in validCommandsV2", cmd)
		}
	}
}

func TestDocsUpdateDryRunAcceptsDeprecatedAPIVersionValues(t *testing.T) {
	for _, apiVersion := range []string{"v1", "v2"} {
		t.Run(apiVersion, func(t *testing.T) {
			t.Parallel()

			runtime := newUpdateShortcutTestRuntime(t, apiVersion, nil)
			if err := validateUpdateV2(context.Background(), runtime); err != nil {
				t.Fatalf("validateUpdateV2() error = %v", err)
			}

			dry := decodeDocDryRun(t, DocsUpdate.DryRun(context.Background(), runtime))
			if len(dry.API) != 1 {
				t.Fatalf("expected 1 dry-run API call, got %d", len(dry.API))
			}
			if got, want := dry.API[0].URL, "/open-apis/docs_ai/v1/documents/doxcnUpdateDryRun"; got != want {
				t.Fatalf("dry-run URL = %q, want %q", got, want)
			}
			if got, want := dry.API[0].Body["command"], "block_insert_after"; got != want {
				t.Fatalf("dry-run command = %#v, want %q", got, want)
			}
			if got, want := dry.API[0].Body["block_id"], "-1"; got != want {
				t.Fatalf("dry-run block_id = %#v, want %q", got, want)
			}
		})
	}
}

func TestDocsUpdateRejectsLegacyFlags(t *testing.T) {
	tests := []struct {
		name     string
		setFlags map[string]string
		want     []string
	}{
		{
			name:     "legacy mode",
			setFlags: map[string]string{"mode": "overwrite"},
			want: []string{
				"docs +update is v2-only",
				"the old v1 interface has been shut down",
				"legacy v1 flag(s) --mode are no longer supported",
				"--mode -> use --command",
				"lark-cli skills read lark-doc references/lark-doc-update.md",
				"lark-cli skills read lark-doc references/lark-doc-xml.md",
				"lark-cli skills read lark-doc references/lark-doc-md.md",
				"follow the latest format rules",
				"MUST NOT grep/open local SKILL.md files",
				"lark-cli docs +update --help",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newUpdateShortcutTestRuntime(t, "", tt.setFlags)
			err := validateUpdateV2(context.Background(), runtime)
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

func newUpdateShortcutTestRuntime(t *testing.T, apiVersion string, setFlags map[string]string) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "+update"}
	cmd.Flags().String("api-version", "", "")
	cmd.Flags().String("doc", "doxcnUpdateDryRun", "")
	cmd.Flags().String("doc-format", "xml", "")
	cmd.Flags().String("command", "append", "")
	cmd.Flags().Int("revision-id", -1, "")
	cmd.Flags().String("content", "<p>hello</p>", "")
	cmd.Flags().String("pattern", "", "")
	cmd.Flags().String("block-id", "", "")
	cmd.Flags().String("src-block-ids", "", "")
	cmd.Flags().String("mode", "", "")
	cmd.Flags().String("markdown", "", "")
	cmd.Flags().String("selection-with-ellipsis", "", "")
	cmd.Flags().String("selection-by-title", "", "")
	cmd.Flags().String("new-title", "", "")
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

// ── Integration tests (full pipeline with mocked HTTP) ──

func TestDocsUpdateV2WarnsOnPreWithoutCode(t *testing.T) {
	t.Parallel()

	f, stdout, stderr, reg := cmdutil.TestFactory(t, docsUpdateTestConfig(t))
	registerDocsUpdateAPIStub(reg, map[string]interface{}{
		"document": map[string]interface{}{
			"revision_id": float64(2),
			"url":         "https://example.feishu.cn/docx/doxcnUpdateDryRun",
		},
		"result": "success",
	})

	err := runDocsUpdateShortcut(t, f, stdout, []string{
		"+update",
		"--api-version", "v2",
		"--doc", "doxcnUpdateDryRun",
		"--command", "block_insert_after",
		"--block-id", "doxcnAnchor",
		"--content", "<pre lang=\"text\">no code tag</pre>",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "missing a <code> child element") {
		t.Fatalf("expected stderr warning about missing <code>, got:\n%s", stderrStr)
	}
}

func TestDocsUpdateV2BlockInsertAfterHint(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, docsUpdateTestConfig(t))
	registerDocsUpdateAPIStub(reg, map[string]interface{}{
		"document": map[string]interface{}{
			"revision_id": float64(2),
			"url":         "https://example.feishu.cn/docx/doxcnUpdateDryRun",
		},
		"result": "success",
	})

	err := runDocsUpdateShortcut(t, f, stdout, []string{
		"+update",
		"--api-version", "v2",
		"--doc", "doxcnUpdateDryRun",
		"--command", "block_insert_after",
		"--block-id", "doxcnAnchor",
		"--content", "<p>hello</p>",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode output: %v\nraw=%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	if data == nil {
		t.Fatalf("missing data in output envelope: %#v", envelope)
	}
	hint, _ := data["_hint"].(string)
	if hint == "" {
		t.Fatalf("expected _hint in block_insert_after response, got: %#v", data)
	}
	if !strings.Contains(hint, "docs +fetch") {
		t.Fatalf("_hint should mention docs +fetch, got: %s", hint)
	}
}

func TestDocsUpdateV2AppendHint(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, docsUpdateTestConfig(t))
	registerDocsUpdateAPIStub(reg, map[string]interface{}{
		"document": map[string]interface{}{
			"revision_id": float64(2),
			"url":         "https://example.feishu.cn/docx/doxcnUpdateDryRun",
		},
		"result": "success",
	})

	err := runDocsUpdateShortcut(t, f, stdout, []string{
		"+update",
		"--api-version", "v2",
		"--doc", "doxcnUpdateDryRun",
		"--command", "append",
		"--content", "<p>hello</p>",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode output: %v\nraw=%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	if data == nil {
		t.Fatalf("missing data in output envelope: %#v", envelope)
	}
	hint, _ := data["_hint"].(string)
	if hint == "" {
		t.Fatalf("expected _hint in append response, got: %#v", data)
	}
}

// ── Helpers ──

func docsUpdateTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()

	replacer := strings.NewReplacer("/", "-", " ", "-")
	suffix := replacer.Replace(strings.ToLower(t.Name()))
	return &core.CliConfig{
		AppID:     "test-docs-update-" + suffix,
		AppSecret: "secret-docs-update-" + suffix,
		Brand:     core.BrandFeishu,
	}
}

func registerDocsUpdateAPIStub(reg *httpmock.Registry, data map[string]interface{}) {
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/docs_ai/v1/documents/doxcnUpdateDryRun",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": data,
		},
	})
}

func runDocsUpdateShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()

	return mountAndRunDocs(t, DocsUpdate, args, f, stdout)
}
