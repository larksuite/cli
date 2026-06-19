// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/vfs"
)

func main() {
	os.Exit(runManifestExport(os.Args[1:], os.Stderr))
}

func runManifestExport(args []string, stderr io.Writer) int {
	configureManifestExportEnvironment()

	fs := flag.NewFlagSet("manifest-export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var manifestOut string
	var commandIndexOut string
	fs.StringVar(&manifestOut, "manifest-out", "", "write hand-authored command manifest JSON to this path")
	fs.StringVar(&commandIndexOut, "command-index-out", "", "write full command index JSON to this path")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "manifest-export: %v\n", err)
		return 2
	}
	if manifestOut == "" || commandIndexOut == "" {
		fmt.Fprintln(stderr, "manifest-export: --manifest-out and --command-index-out are required")
		return 2
	}

	ctx := context.Background()
	m, err := collectHandAuthored(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "manifest-export: collect command manifest: %v\n", err)
		return 2
	}
	idx, err := collectCommandIndex(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "manifest-export: collect command index: %v\n", err)
		return 2
	}
	if err := ensureParentDir(manifestOut); err != nil {
		fmt.Fprintf(stderr, "manifest-export: create manifest output directory: %v\n", err)
		return 2
	}
	if err := ensureParentDir(commandIndexOut); err != nil {
		fmt.Fprintf(stderr, "manifest-export: create command index output directory: %v\n", err)
		return 2
	}
	if err := manifest.WriteFile(manifestOut, manifest.KindCommandManifest, m); err != nil {
		fmt.Fprintf(stderr, "manifest-export: write command manifest: %v\n", err)
		return 2
	}
	if err := manifest.WriteFile(commandIndexOut, manifest.KindCommandIndex, idx); err != nil {
		fmt.Fprintf(stderr, "manifest-export: write command index: %v\n", err)
		return 2
	}
	return 0
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return vfs.MkdirAll(dir, 0o755)
}

func configureManifestExportEnvironment() {
	_ = os.Setenv("LARKSUITE_CLI_REMOTE_META", "off")
	if os.Getenv("LARKSUITE_CLI_CONFIG_DIR") == "" {
		_ = os.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(os.TempDir(), "quality-gate-cli-config"))
	}
}
