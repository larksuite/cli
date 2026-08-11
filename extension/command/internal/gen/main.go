// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Command gen regenerates the public existing-domain enumeration.
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts"
)

var serviceNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

func commandDir() (string, error) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot resolve source location")
	}
	return filepath.Join(filepath.Dir(sourceFile), "..", ".."), nil
}

//nolint:forbidigo // Standalone go-generate process reports to stderr and exits before any CLI command boundary exists.
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "command domain generator:", err)
		os.Exit(1)
	}
}

func run() error {
	seen := make(map[string]struct{})
	for _, shortcut := range shortcuts.AllShortcuts() {
		seen[shortcut.Service] = struct{}{}
	}
	if len(seen) == 0 {
		return fmt.Errorf("no shortcut domains found")
	}

	domains := make([]string, 0, len(seen))
	for domain := range seen {
		if !serviceNamePattern.MatchString(domain) {
			return fmt.Errorf("service %q cannot form a stable Go identifier", domain)
		}
		for _, lang := range []string{"en", "zh"} {
			if registry.GetServiceTitle(domain, lang) == "" {
				return fmt.Errorf("service %q has no %s title", domain, lang)
			}
			if registry.GetServiceDescription(domain, lang) == "" {
				return fmt.Errorf("service %q has no %s description", domain, lang)
			}
		}
		domains = append(domains, domain)
	}
	sort.Strings(domains)

	var output bytes.Buffer
	output.WriteString(`// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Code generated from shortcuts.AllShortcuts; DO NOT EDIT.

package command

const (
`)
	for _, domain := range domains {
		identifier := domainIdentifier(domain)
		fmt.Fprintf(&output, "\t// Domain%s 表示%s域。\n", identifier, registry.GetServiceTitle(domain, "zh"))
		fmt.Fprintf(&output, "\tDomain%s DomainName = %q\n", identifier, domain)
	}
	output.WriteString(")\n")

	formatted, err := format.Source(output.Bytes())
	if err != nil {
		return fmt.Errorf("format output: %w", err)
	}
	dir, err := commandDir()
	if err != nil {
		return err
	}
	target := filepath.Join(dir, "domains_gen.go")
	if err := vfs.WriteFile(target, formatted, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}

func domainIdentifier(domain string) string {
	switch domain {
	case "im":
		return "IM"
	case "okr":
		return "OKR"
	case "vc":
		return "VC"
	default:
		return strings.ToUpper(domain[:1]) + domain[1:]
	}
}
