// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestSuiteTemplateMatchesCropContract(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(repoRoot, "isolated-skills", "lark-suite", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	template := string(raw)
	if !strings.Contains(template, suiteDescriptionPrefix+"<!-- LARK_SUITE_KEYS -->"+suiteDescriptionSuffix) {
		t.Fatal("suite template description no longer matches the client crop contract")
	}
	if strings.Count(template, "<!-- LARK_SUITE_ROUTES -->") != 1 {
		t.Fatal("suite template must contain exactly one route placeholder")
	}
}

func TestSuiteKeywordKeysMatchOfficialSkillDirectories(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(repoRoot, "skill-template", "lark-suite-business-info.json"))
	if err != nil {
		t.Fatal(err)
	}
	var keywords map[string][]string
	if err := json.Unmarshal(raw, &keywords); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(filepath.Join(repoRoot, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	directories := []string{}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "lark-") {
			directories = append(directories, entry.Name())
		}
	}
	sort.Strings(directories)
	keys := make([]string, 0, len(keywords))
	for name := range keywords {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	assertStrings(t, keys, directories)
}
