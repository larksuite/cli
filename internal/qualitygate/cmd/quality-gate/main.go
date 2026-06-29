// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/qualitygate/rules"
	"github.com/larksuite/cli/internal/validate"
)

func main() {
	if len(os.Args) < 2 {
		usageAndExit(2)
	}

	switch os.Args[1] {
	case "check":
		os.Exit(runCheck(os.Args[2:]))
	default:
		usageAndExit(2)
	}
}

func runCheck(args []string) int {
	configureQualityGateEnvironment()

	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	opts := rules.Options{}
	var printLegacyCommandCandidates bool
	var printLegacyFlagCandidates bool
	fs.StringVar(&opts.Repo, "repo", ".", "repository root")
	fs.StringVar(&opts.CLIBin, "cli-bin", "./lark-cli", "lark-cli binary used for dry-run validation")
	fs.StringVar(&opts.ChangedFrom, "changed-from", "", "base revision for incremental checks")
	fs.StringVar(&opts.FactsOut, "facts-out", "", "write facts JSON to this path")
	fs.StringVar(&opts.ManifestPath, "manifest", "", "hand-authored command manifest JSON")
	fs.StringVar(&opts.CommandIndexPath, "command-index", "", "full command index JSON")
	fs.StringVar(&opts.PublicContentMetadataPath, "public-content-metadata", "", "PR title/body metadata JSON for public content checks")
	fs.BoolVar(&printLegacyCommandCandidates, "print-legacy-command-candidates", false, "print current non-kebab-case hand-authored command candidates")
	fs.BoolVar(&printLegacyFlagCandidates, "print-legacy-flag-candidates", false, "print current non-kebab-case flag candidates")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "quality-gate check: %v\n", err)
		return 2
	}

	if opts.PublicContentMetadataPath != "" {
		safePath, err := validate.SafeInputPath(opts.PublicContentMetadataPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "quality-gate check: --public-content-metadata: %v\n", err)
			return 2
		}
		opts.PublicContentMetadataPath = safePath
	}

	if opts.ManifestPath == "" || opts.CommandIndexPath == "" {
		fmt.Fprintln(os.Stderr, "quality-gate check: --manifest and --command-index are required")
		return 2
	}

	if printLegacyCommandCandidates || printLegacyFlagCandidates {
		m, err := manifest.ReadFile(opts.ManifestPath, manifest.KindCommandManifest)
		if err != nil {
			fmt.Fprintf(os.Stderr, "quality-gate check: --manifest: %v\n", err)
			return 2
		}
		if printLegacyCommandCandidates {
			for _, line := range rules.LegacyCommandCandidates(m) {
				fmt.Fprintln(os.Stdout, line)
			}
		}
		if printLegacyFlagCandidates {
			for _, line := range rules.LegacyFlagCandidates(m) {
				fmt.Fprintln(os.Stdout, line)
			}
		}
		return 0
	}

	diags, facts, err := rules.Run(context.Background(), opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "quality-gate check: %v\n", err)
		return 2
	}
	report.Print(os.Stderr, diags)
	if opts.FactsOut != "" {
		if err := facts.WriteFile(opts.FactsOut); err != nil {
			fmt.Fprintf(os.Stderr, "quality-gate check: write facts: %v\n", err)
			return 2
		}
	}
	return report.ExitCode(diags)
}

func configureQualityGateEnvironment() {
	_ = os.Setenv("LARKSUITE_CLI_REMOTE_META", "off")
	if os.Getenv("LARKSUITE_CLI_CONFIG_DIR") == "" {
		_ = os.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(os.TempDir(), "quality-gate-cli-config"))
	}
}

func usageAndExit(code int) {
	fmt.Fprintln(os.Stderr, "usage: quality-gate check")
	os.Exit(code)
}
