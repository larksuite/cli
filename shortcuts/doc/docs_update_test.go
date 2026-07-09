// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
package doc

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
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

func TestDocsUpdateDryRunIgnoresAPIVersionCompatFlag(t *testing.T) {
	for _, apiVersion := range []string{"v1", "v2", "legacy"} {
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

func TestBuildUpdateBodyWithHTML5ReferenceMapReportsPathError(t *testing.T) {
	t.Parallel()

	runtime := newUpdateShortcutTestRuntime(t, "", map[string]string{
		"content": `<html5-block path="@missing.html"></html5-block>`,
	})

	_, err := buildUpdateBodyWithHTML5ReferenceMap(runtime)
	if err == nil {
		t.Fatal("buildUpdateBodyWithHTML5ReferenceMap() succeeded, want error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf() ok = false for %T (%v)", err, err)
	}
	if p.Category != errs.CategoryValidation {
		t.Fatalf("category = %q, want %q", p.Category, errs.CategoryValidation)
	}
	if p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidArgument)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *errs.ValidationError", err)
	}
	if validationErr.Param != "path" {
		t.Fatalf("param = %q, want path", validationErr.Param)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error should preserve os.ErrNotExist cause, got: %v", err)
	}
}

func TestDocsUpdateV2ReferenceMapFlagIsPublicFileInput(t *testing.T) {
	t.Parallel()

	var flag common.Flag
	for _, candidate := range v2UpdateFlags() {
		if candidate.Name == "reference-map" {
			flag = candidate
			break
		}
	}
	if flag.Name == "" {
		t.Fatal("reference-map flag not found")
	}
	if flag.Hidden {
		t.Fatal("reference-map flag should be public")
	}
	if flag.Type != "" {
		t.Fatalf("reference-map flag Type = %q, want default string", flag.Type)
	}
	if !hasUpdateTestInput(flag, common.File) || !hasUpdateTestInput(flag, common.Stdin) {
		t.Fatalf("reference-map Input = %#v, want file and stdin", flag.Input)
	}
	if flag.Desc != docsUpdateReferenceMapFlagDesc {
		t.Fatalf("reference-map help = %q, want %q", flag.Desc, docsUpdateReferenceMapFlagDesc)
	}
}

func TestBuildUpdateBodyIncludesReferenceMap(t *testing.T) {
	t.Parallel()

	runtime := newUpdateShortcutTestRuntime(t, "", map[string]string{
		"command":       "append",
		"content":       `<p><widget data-ref="r1"></widget></p>`,
		"reference-map": `{"widget":{"r1":{"label":"widget-ref-value"}}}`,
	})
	body := buildUpdateBody(runtime)

	refMap, ok := body["reference_map"].(map[string]interface{})
	if !ok {
		t.Fatalf("reference_map = %#v, want object", body["reference_map"])
	}
	widget, _ := refMap["widget"].(map[string]interface{})
	r1, _ := widget["r1"].(map[string]interface{})
	if got := r1["label"]; got != "widget-ref-value" {
		t.Fatalf("reference_map.widget.r1.label = %#v, want widget-ref-value; body=%#v", got, body)
	}
	if got, want := body["command"], "block_insert_after"; got != want {
		t.Fatalf("command = %#v, want %q", got, want)
	}
	if got, want := body["block_id"], "-1"; got != want {
		t.Fatalf("block_id = %#v, want %q", got, want)
	}
}

func TestValidateUpdateV2RejectsInvalidReferenceMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setFlags  map[string]string
		wantCause bool
	}{
		{
			name: "invalid json",
			setFlags: map[string]string{
				"reference-map": "{",
			},
			wantCause: true,
		},
		{
			name: "empty",
			setFlags: map[string]string{
				"reference-map": "",
			},
		},
		{
			name: "without content",
			setFlags: map[string]string{
				"content":       "",
				"reference-map": `{"widget":{"r1":{"label":"widget-ref-value"}}}`,
			},
		},
		{
			name: "unsupported command",
			setFlags: map[string]string{
				"command":       "block_move_after",
				"block-id":      "blk_anchor",
				"src-block-ids": "blk_src",
				"reference-map": `{"widget":{"r1":{"label":"widget-ref-value"}}}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtime := newUpdateShortcutTestRuntime(t, "", tt.setFlags)
			err := validateUpdateV2(context.Background(), runtime)
			if err == nil {
				t.Fatal("validateUpdateV2() succeeded, want error")
			}
			assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--reference-map")
			if tt.wantCause && errors.Unwrap(err) == nil {
				t.Fatal("validateUpdateV2() error lost underlying JSON cause")
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

func hasUpdateTestInput(flag common.Flag, input string) bool {
	for _, candidate := range flag.Input {
		if candidate == input {
			return true
		}
	}
	return false
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
	cmd.Flags().String("reference-map", "", "")
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
