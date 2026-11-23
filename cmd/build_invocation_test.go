// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// TestBuildWithExplicitEmptyInvocationIgnoresAmbientArgs pins the contract of
// WithInvocationArgs([]string{}): it is a bare invocation. Cobra reads os.Args
// when SetArgs receives nil, so the copy handed to Cobra must stay non-nil or a
// wrapper's own process arguments would be executed instead of root help.
func TestBuildWithExplicitEmptyInvocationIgnoresAmbientArgs(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"lark-cli", "drive", "files", "list", "--help"}
	t.Cleanup(func() { os.Args = oldArgs })

	loader := newRecordingLoader(t)
	opens := 0
	var stdout bytes.Buffer
	root := Build(
		context.Background(),
		cmdutil.InvocationContext{},
		WithInvocationArgs([]string{}),
		WithIO(strings.NewReader(""), &stdout, io.Discard),
		WithoutPlugins(),
		WithoutStrictMode(),
		withRecordingCatalog(loader, &opens),
	)
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := stdout.String()
	if strings.Contains(out, "lark-cli drive files list [flags]") {
		t.Fatalf("an explicit empty invocation executed the host's ambient arguments:\n%s", out)
	}
	if !strings.Contains(out, "Lark domains:") || !strings.Contains(out, "\n  drive ") {
		t.Fatalf("an explicit empty invocation did not render the full root help:\n%s", out)
	}
}

// TestIsVersionOnlyInvocationRequiresTrueValue covers the boolean spellings
// Cobra accepts for the version flag. Only a true value prints the version and
// may skip the Catalog; `--version=false` falls through to root help.
func TestIsVersionOnlyInvocationRequiresTrueValue(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli", Version: "test"}
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"long flag", []string{"--version"}, true},
		{"short flag", []string{"-v"}, true},
		{"explicit true", []string{"--version=true"}, true},
		{"explicit false", []string{"--version=false"}, false},
		{"short false", []string{"-v=false"}, false},
		{"with help", []string{"--version", "--help"}, false},
		{"with command", []string{"--version", "drive"}, false},
		{"bare", nil, false},
	}
	for _, tc := range cases {
		if got := isVersionOnlyInvocation(root, tc.args); got != tc.want {
			t.Errorf("%s %v: isVersionOnlyInvocation = %v, want %v", tc.name, tc.args, got, tc.want)
		}
	}
}

// TestBuildWithFalseVersionFlagAssemblesFullTree is the end-to-end half: a
// spelled-out false version flag must reach the complete tree so the root help
// Cobra prints for it lists every domain.
func TestBuildWithFalseVersionFlagAssemblesFullTree(t *testing.T) {
	loader := newRecordingLoader(t)
	opens := 0
	var stdout bytes.Buffer
	root := Build(
		context.Background(),
		cmdutil.InvocationContext{},
		WithInvocationArgs([]string{"--version=false"}),
		WithIO(strings.NewReader(""), &stdout, io.Discard),
		WithoutPlugins(),
		WithoutStrictMode(),
		withRecordingCatalog(loader, &opens),
	)
	if findCommand(root, "drive files list") == nil {
		t.Fatal("--version=false skipped the Catalog: drive files list is missing")
	}
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Lark domains:") || !strings.Contains(out, "\n  drive ") {
		t.Fatalf("--version=false root help lost the business domains:\n%s", out)
	}
	if strings.Contains(out, "lark-cli version") {
		t.Fatalf("--version=false printed the version:\n%s", out)
	}
}
