// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"io/fs"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/registry"
)

// TestNativeAffordanceHeadingsMatchMetadata checks the native command headings
// that are present in affordance Markdown. It deliberately does not require
// every metadata method to have a document; completeness is a manual warning.
func TestNativeAffordanceHeadingsMatchMetadata(t *testing.T) {
	if os.Getenv("LARKSUITE_CLI_CHECK_AFFORDANCE_CONSISTENCY") != "1" {
		t.Skip("enabled by the affordance-document CI gate")
	}
	methods := registry.EmbeddedCatalog().WalkMethods(nil)
	if len(methods) == 0 {
		t.Skip("embedded API metadata is unavailable in the bare-module registry")
	}

	known := make(map[string]map[string]bool)
	for _, method := range methods {
		service, methodID := method.ServiceName(), method.Method.ID
		if strings.HasPrefix(methodID, "+") {
			continue
		}
		if known[service] == nil {
			known[service] = make(map[string]bool)
		}
		known[service][methodID] = true
	}

	entries, err := fs.ReadDir(os.DirFS("../../affordance"), ".")
	if err != nil {
		t.Fatal(err)
	}
	var findings []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "README.md" || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		service := strings.TrimSuffix(entry.Name(), ".md")
		source, err := os.ReadFile("../../affordance/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		for methodID := range parseDomainMD(source, commandFormResolver(service)).methods {
			if strings.HasPrefix(methodID, "+") {
				continue
			}
			if !known[service][methodID] {
				findings = append(findings, service+"/"+methodID+": document heading has no matching native command")
			}
		}
	}
	sort.Strings(findings)
	if len(findings) > 0 {
		t.Fatalf("native affordance consistency failed:\n%s", strings.Join(findings, "\n"))
	}
}
