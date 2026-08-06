// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docsFrameworkFlags are injected by the common command runner into every
// command (not declared in each command's own Flags list), so they are valid in
// skill examples even though they never appear in v2FetchFlags.
var docsFrameworkFlags = map[string]bool{
	"as": true, "json": true, "dry-run": true, "format": true,
	"yes": true, "print-schema": true, "flag-name": true,
}

func TestSkillDocFetchExampleFlagsAreRegistered(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "skills", "lark-doc", "references", "lark-doc-fetch.md"))
	if err != nil {
		t.Skipf("skill doc not found: %v", err)
	}
	registered := map[string]bool{}
	for _, f := range DocsFetch.Flags {
		registered[f.Name] = true
	}
	// Join backslash-continued lines so a multi-line command example is scanned
	// as a single command.
	joined := strings.ReplaceAll(string(data), "\\\n", " ")
	flagRe := regexp.MustCompile(`--[a-z][a-z0-9-]*`)
	var bad []string
	seen := map[string]bool{}
	for _, line := range strings.Split(joined, "\n") {
		if !strings.Contains(line, "docs +fetch") {
			continue
		}
		for _, m := range flagRe.FindAllString(line, -1) {
			name := strings.TrimPrefix(m, "--")
			if registered[name] || docsFrameworkFlags[name] || seen[name] {
				continue
			}
			seen[name] = true
			bad = append(bad, name)
		}
	}
	if len(bad) > 0 {
		t.Errorf("lark-doc-fetch.md uses flags not registered on docs +fetch: %s\n"+
			"delete a removed flag from the skill doc, or if it is a new framework "+
			"flag add it to docsFrameworkFlags", strings.Join(bad, ", "))
	}
}
