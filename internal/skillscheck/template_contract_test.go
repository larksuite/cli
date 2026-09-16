// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestSuiteTemplateMatchesCropContract(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	raw, err := vfs.ReadFile(filepath.Join(repoRoot, "isolated-skills", "lark-suite", "SKILL.md"))
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
	if !strings.Contains(template, "references/<skill-name>/GUIDE.md") {
		t.Fatal("suite template must route nested guides through GUIDE.md")
	}
	if strings.Contains(template, "references/<skill-name>/SKILL.md") {
		t.Fatal("suite template must not route nested guides through SKILL.md")
	}
}

func TestSuiteKeywordKeysMatchOfficialSkillDirectories(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	raw, err := vfs.ReadFile(filepath.Join(repoRoot, "skill-template", "lark-suite-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Keywords map[string][]string `json:"keywords"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}

	entries, err := vfs.ReadDir(filepath.Join(repoRoot, "skills"))
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
	keys := make([]string, 0, len(config.Keywords))
	for name := range config.Keywords {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	assertStrings(t, keys, directories)
}
