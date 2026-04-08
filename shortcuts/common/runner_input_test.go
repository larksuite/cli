// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"os"
	"path/filepath"
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
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	content := "## Hello\n\nThis is **markdown** from a file.\n"
	fpath := filepath.Join(dir, "test.md")
	os.WriteFile(fpath, []byte(content), 0644)

	rctx := newTestRuntimeWithStdin(map[string]string{"markdown": "@test.md"}, "")
	flags := []Flag{{Name: "markdown", Input: []string{File, Stdin}}}

	if err := resolveInputFlags(rctx, flags); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := rctx.Str("markdown"); got != content {
		t.Errorf("expected %q, got %q", content, got)
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
	if !strings.Contains(err.Error(), "does not support stdin") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveInputFlags_FileNotSupported(t *testing.T) {
	rctx := newTestRuntimeWithStdin(map[string]string{"data": "@file.txt"}, "")
	flags := []Flag{{Name: "data", Input: []string{Stdin}}} // only stdin, no file

	err := resolveInputFlags(rctx, flags)
	if err == nil {
		t.Fatal("expected error for file not supported")
	}
	if !strings.Contains(err.Error(), "does not support file input") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveInputFlags_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	rctx := newTestRuntimeWithStdin(map[string]string{"markdown": "@nonexistent.md"}, "")
	flags := []Flag{{Name: "markdown", Input: []string{File, Stdin}}}

	err := resolveInputFlags(rctx, flags)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "cannot read file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveInputFlags_EmptyFilePath(t *testing.T) {
	rctx := newTestRuntimeWithStdin(map[string]string{"markdown": "@ "}, "")
	flags := []Flag{{Name: "markdown", Input: []string{File, Stdin}}}

	err := resolveInputFlags(rctx, flags)
	if err == nil {
		t.Fatal("expected error for empty file path")
	}
	if !strings.Contains(err.Error(), "file path cannot be empty") {
		t.Errorf("unexpected error: %v", err)
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
	if !strings.Contains(err.Error(), "stdin (-) can only be used by one flag") {
		t.Errorf("unexpected error: %v", err)
	}
}
