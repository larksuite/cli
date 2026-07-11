// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Command lintcheck runs repository source-contract guards that golangci-lint
// cannot express directly. It currently covers typed-error contracts and the
// resolver-owned endpoint contract.
//
// lintcheck lives in its own Go module under lint/ so its build-time
// dependency on golang.org/x/tools/go/packages does not leak into the
// shipped lark-cli binary's module graph.
//
// Usage (from repo root):
//
//	go run -C lint . .                # scan the lark-cli repo
//	go run -C lint . /path/to/repo    # scan another path
//
// Exit codes:
//
//	0  no REJECT violations (LABEL and WARNING diagnostics are advisory)
//	1  one or more REJECT violations
//
// WARNING and LABEL diagnostics are still printed so a CI workflow can grep
// for the prefixes — LABEL emits `[needs-taxonomy-decision]` for an
// auto-labeler — but neither severity fails CI. Only REJECT does.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/larksuite/cli/lint/domaincontract"
	"github.com/larksuite/cli/lint/errscontract"
	"github.com/larksuite/cli/lint/lintapi"
)

// scanner is the contract every lint domain implements. New domains drop in
// as sibling packages under lint/ (see README.md) and are added below.
type scanner struct {
	name string
	fn   func(root string, opts errscontract.ScanOptions) ([]lintapi.Violation, error)
}

var scanners = []scanner{
	{name: "errscontract", fn: errscontract.ScanRepoWithOptions},
	{name: "domaincontract", fn: func(root string, _ errscontract.ScanOptions) ([]lintapi.Violation, error) {
		return domaincontract.ScanRepo(root)
	}},
}

func main() {
	var changedFrom string
	var printLegacyCommandErrorCandidates bool
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"Usage: lintcheck [repo-root]\n"+
				"Runs every registered lint domain against repo-root (default: current directory).\n")
		flag.PrintDefaults()
	}
	flag.StringVar(&changedFrom, "changed-from", "", "base revision for incremental boundary-error checks")
	flag.BoolVar(&printLegacyCommandErrorCandidates, "print-legacy-command-error-candidates", false, "print existing command boundary bare errors as allowlist candidates")
	flag.Parse()

	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
		// `./...` is a common Go-toolchain idiom; map it to the working dir.
		if root == "./..." {
			root = "."
		}
	}
	if printLegacyCommandErrorCandidates {
		lines, err := errscontract.LegacyCommandErrorCandidatesForRepo(root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lintcheck errscontract: %v\n", err)
			os.Exit(2)
		}
		for _, line := range lines {
			fmt.Fprintln(os.Stdout, line)
		}
		return
	}

	opts := errscontract.ScanOptions{ChangedFrom: changedFrom}
	var all []lintapi.Violation
	for _, s := range scanners {
		violations, err := s.fn(root, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lintcheck %s: %v\n", s.name, err)
			os.Exit(2)
		}
		all = append(all, violations...)
	}

	exitCode := 0
	for _, v := range all {
		fmt.Fprintf(os.Stderr, "%s:%d: [%s/%s] %s\n", v.File, v.Line, v.Action, v.Rule, v.Message)
		if v.Suggestion != "" {
			fmt.Fprintf(os.Stderr, "    hint: %s\n", v.Suggestion)
		}
		if v.Action == lintapi.ActionReject {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}
