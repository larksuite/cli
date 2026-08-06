// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// driveFrameworkFlags are injected by the common command runner into every
// command (not declared in DriveFetch.Flags), so they are valid in skill
// examples even though they never appear in DriveFetch.Flags.
var driveFrameworkFlags = map[string]bool{
	"as": true, "json": true, "dry-run": true, "format": true, "jq": true,
	"yes": true, "print-schema": true, "flag-name": true,
}

func TestSkillDriveFetchExampleFlagsAreRegistered(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "skills", "lark-drive", "references", "lark-drive-fetch.md"))
	if err != nil {
		t.Fatalf("skill doc not found: %v", err)
	}
	registered := map[string]bool{}
	for _, f := range DriveFetch.Flags {
		registered[f.Name] = true
	}
	joined := strings.ReplaceAll(string(data), "\\\n", " ")
	flagRe := regexp.MustCompile(`--[a-z][a-z0-9-]*`)
	var bad []string
	seen := map[string]bool{}
	for _, line := range strings.Split(joined, "\n") {
		if !strings.Contains(line, "drive +fetch") {
			continue
		}
		for _, m := range flagRe.FindAllString(line, -1) {
			name := strings.TrimPrefix(m, "--")
			if registered[name] || driveFrameworkFlags[name] || seen[name] {
				continue
			}
			seen[name] = true
			bad = append(bad, name)
		}
	}
	if len(bad) > 0 {
		t.Errorf("lark-drive-fetch.md uses flags not registered on drive +fetch: %s\n"+
			"delete a removed flag from the skill doc, or if it is a new framework "+
			"flag add it to driveFrameworkFlags", strings.Join(bad, ", "))
	}
}
