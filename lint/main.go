// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Command lintcheck runs the source-level errs/ contract guards (all four checks).
// The fifth contract rule (business path must use typed errors) lives in
// .golangci.yml as a forbidigo entry; the four checks here are AST-level
// guards that golangci-lint cannot express.
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

	"github.com/larksuite/cli/lint/errscontract"
	"github.com/larksuite/cli/lint/lintapi"
)

// scanner is the contract every lint domain implements. New domains drop in
// as sibling packages under lint/ (see README.md) and are added below.
type scanner struct {
	name string
	fn   func(root string) ([]lintapi.Violation, error)
}

var scanners = []scanner{
	{name: "errscontract", fn: errscontract.ScanRepo},
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"Usage: lintcheck [repo-root]\n"+
				"Runs every registered lint domain against repo-root (default: current directory).\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
		// `./...` is a common Go-toolchain idiom; map it to the working dir.
		if root == "./..." {
			root = "."
		}
	}

	var all []lintapi.Violation
	for _, s := range scanners {
		violations, err := s.fn(root)
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
