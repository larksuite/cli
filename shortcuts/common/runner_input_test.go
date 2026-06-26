// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	_ "github.com/larksuite/cli/internal/vfs/localfileio"
	"github.com/spf13/cobra"
)

// newTestRuntimeWithStdin creates a RuntimeContext with string flags and a fake stdin.
func newTestRuntimeWithStdin(flags map[string]string, stdin string) *RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	for name := range flags {
		cmd.Flags().String(name, "", "")
	}
	cmd.ParseFlags(nil)
	for name, val := range flags {
		cmd.Flags().Set(name, val)
	}
	return &RuntimeContext{
		Cmd: cmd,
		Factory: &cmdutil.Factory{
			IOStreams: &cmdutil.IOStreams{
				In: strings.NewReader(stdin),
			},
		},
	}
}

func TestResolveInputFlags_DirectValue(t *testing.T) {
	rctx := newTestRuntimeWithStdin(map[string]string{"markdown": "hello world"}, "")
	flags := []Flag{{Name: "markdown", Input: []string{File, Stdin}}}

	if err := resolveInputFlags(rctx, flags); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rctx.Str("markdown"); got != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", got)
	}
}

func TestResolveInputFlags_Stdin(t *testing.T) {
	rctx := newTestRuntimeWithStdin(map[string]string{"markdown": "-"}, "content from stdin")
	flags := []Flag{{Name: "markdown", Input: []string{File, Stdin}}}

	if err := resolveInputFlags(rctx, flags); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rctx.Str("markdown"); got != "content from stdin" {
		t.Errorf("expected %q, got %q", "content from stdin", got)
	}
}

func TestResolveInputFlags_File(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	content := "## Hello\n\nThis is **markdown** from a file.\n"
	if err := os.WriteFile("test.md", []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rctx := newTestRuntimeWithStdin(map[string]string{"markdown": "@test.md"}, "")
	flags := []Flag{{Name: "markdown", Input: []string{File, Stdin}}}

	if err := resolveInputFlags(rctx, flags); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rctx.Str("markdown"); got != content {
		t.Errorf("expected %q, got %q", content, got)
	}
}

func TestResolveInputFlags_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	if err := os.WriteFile("empty.md", nil, 0644); err != nil {
		t.Fatal(err)
	}

	rctx := newTestRuntimeWithStdin(map[string]string{"markdown": "@empty.md"}, "")
	flags := []Flag{{Name: "markdown", Input: []string{File, Stdin}}}

	if err := resolveInputFlags(rctx, flags); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rctx.Str("markdown"); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestResolveInputFlags_EmptyInput(t *testing.T) {
	rctx := newTestRuntimeWithStdin(map[string]string{"markdown": ""}, "")
	flags := []Flag{{Name: "markdown", Input: []string{File, Stdin}}}

	if err := resolveInputFlags(rctx, flags); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rctx.Str("markdown"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestResolveInputFlags_NoInputSpec(t *testing.T) {
	rctx := newTestRuntimeWithStdin(map[string]string{"token": "@something"}, "")
	flags := []Flag{{Name: "token"}} // no Input

	if err := resolveInputFlags(rctx, flags); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// value should be unchanged — no resolution
	if got := rctx.Str("token"); got != "@something" {
		t.Errorf("expected %q, got %q", "@something", got)
	}
}

func TestResolveInputFlags_StdinNotSupported(t *testing.T) {
	rctx := newTestRuntimeWithStdin(map[string]string{"data": "-"}, "stdin data")
	flags := []Flag{{Name: "data", Input: []string{File}}} // only file, no stdin

	err := resolveInputFlags(rctx, flags)
	if err == nil {
		t.Fatal("expected error for stdin not supported")
	}
	vErr := assertValidationParam(t, err, "--data")
	if !strings.Contains(vErr.Message, "does not support stdin") {
		t.Errorf("unexpected error message: %q", vErr.Message)
	}
}

func TestResolveInputFlags_FileNotSupported(t *testing.T) {
	rctx := newTestRuntimeWithStdin(map[string]string{"data": "@file.txt"}, "")
	flags := []Flag{{Name: "data", Input: []string{Stdin}}} // only stdin, no file

	err := resolveInputFlags(rctx, flags)
	if err == nil {
		t.Fatal("expected error for file not supported")
	}
	vErr := assertValidationParam(t, err, "--data")
	if !strings.Contains(vErr.Message, "does not support file input") {
		t.Errorf("unexpected error message: %q", vErr.Message)
	}
}

func TestResolveInputFlags_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	rctx := newTestRuntimeWithStdin(map[string]string{"markdown": "@nonexistent.md"}, "")
	flags := []Flag{{Name: "markdown", Input: []string{File, Stdin}}}

	err := resolveInputFlags(rctx, flags)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	vErr := assertValidationParam(t, err, "--markdown")
	if !strings.Contains(vErr.Message, "cannot read file") {
		t.Errorf("unexpected error message: %q", vErr.Message)
	}
}

func TestResolveInputFlags_EmptyFilePath(t *testing.T) {
	rctx := newTestRuntimeWithStdin(map[string]string{"markdown": "@ "}, "")
	flags := []Flag{{Name: "markdown", Input: []string{File, Stdin}}}

	err := resolveInputFlags(rctx, flags)
	if err == nil {
		t.Fatal("expected error for empty file path")
	}
	vErr := assertValidationParam(t, err, "--markdown")
	if !strings.Contains(vErr.Message, "file path cannot be empty after @") {
		t.Errorf("unexpected error message: %q", vErr.Message)
	}
}

func TestResolveInputFlags_EscapeAtSign(t *testing.T) {
	rctx := newTestRuntimeWithStdin(map[string]string{"text": "@@mention someone"}, "")
	flags := []Flag{{Name: "text", Input: []string{File, Stdin}}}

	if err := resolveInputFlags(rctx, flags); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rctx.Str("text"); got != "@mention someone" {
		t.Errorf("expected %q, got %q", "@mention someone", got)
	}
}

func TestResolveInputFlags_EscapeDoubleAt(t *testing.T) {
	rctx := newTestRuntimeWithStdin(map[string]string{"text": "@@@triple"}, "")
	flags := []Flag{{Name: "text", Input: []string{File, Stdin}}}

	if err := resolveInputFlags(rctx, flags); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// @@@ → strip first @, remaining is @@triple which is literal
	if got := rctx.Str("text"); got != "@@triple" {
		t.Errorf("expected %q, got %q", "@@triple", got)
	}
}

func TestResolveInputFlags_DuplicateStdin(t *testing.T) {
	rctx := newTestRuntimeWithStdin(map[string]string{"a": "-", "b": "-"}, "data")
	flags := []Flag{
		{Name: "a", Input: []string{Stdin}},
		{Name: "b", Input: []string{Stdin}},
	}

	err := resolveInputFlags(rctx, flags)
	if err == nil {
		t.Fatal("expected error for duplicate stdin usage")
	}
	vErr := assertValidationParam(t, err, "--b")
	if !strings.Contains(vErr.Message, "stdin (-) can only be used by one flag") {
		t.Errorf("unexpected error message: %q", vErr.Message)
	}
	// The hint must steer an AI agent to the fix (@file for the extra flags),
	// since `--a - <x --b - <y` is the exact misuse this guards against.
	if !strings.Contains(vErr.Hint, "@file") {
		t.Errorf("hint %q should mention @file as the fix", vErr.Hint)
	}
}

func TestStripUTF8BOM(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"leading BOM removed", "\uFEFFhello", "hello"},
		{"no BOM unchanged", "hello", "hello"},
		{"empty unchanged", "", ""},
		{"only BOM becomes empty", "\uFEFF", ""},
		{"interior BOM preserved", "a\uFEFFb", "a\uFEFFb"},
		{"only the first BOM removed", "\uFEFF\uFEFFx", "\uFEFFx"},
	}
	for _, c := range cases {
		if got := stripUTF8BOM(c.in); got != c.want {
			t.Errorf("%s: stripUTF8BOM(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestResolveInputFlags_StripBOMStdin(t *testing.T) {
	// A CSV piped via stdin with a leading BOM (e.g. from an upstream export)
	// must reach the shortcut without the BOM, so it can't corrupt the first cell.
	rctx := newTestRuntimeWithStdin(map[string]string{"csv": "-"}, "\uFEFFname,age\nzhang,8")
	flags := []Flag{{Name: "csv", Input: []string{File, Stdin}}}

	if err := resolveInputFlags(rctx, flags); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rctx.Str("csv"); got != "name,age\nzhang,8" {
		t.Errorf("leading BOM not stripped from stdin, got %q", got)
	}
}

func TestResolveInputFlags_StripBOMFile(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	// A JSON operations file saved with a BOM would otherwise fail json.Unmarshal
	// with "invalid character 'ï'".
	if err := os.WriteFile("ops.json", []byte("\uFEFF[{\"shortcut\":\"+cells-set\"}]"), 0644); err != nil {
		t.Fatal(err)
	}
	rctx := newTestRuntimeWithStdin(map[string]string{"operations": "@ops.json"}, "")
	flags := []Flag{{Name: "operations", Input: []string{File, Stdin}}}

	if err := resolveInputFlags(rctx, flags); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rctx.Str("operations"); got != "[{\"shortcut\":\"+cells-set\"}]" {
		t.Errorf("leading BOM not stripped from file, got %q", got)
	}
}
