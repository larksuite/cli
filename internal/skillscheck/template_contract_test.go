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

func TestAttendanceSkillDescriptionIncludesTriggerAndBoundary(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	raw, err := vfs.ReadFile(filepath.Join(repoRoot, "skills", "lark-attendance", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	var descriptionLine string
	inFrontmatter := false
	for i, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if i == 0 && trimmed == "---" {
			inFrontmatter = true
			continue
		}
		if !inFrontmatter {
			break
		}
		if trimmed == "---" {
			break
		}
		if strings.HasPrefix(trimmed, "description:") {
			descriptionLine = trimmed
			break
		}
	}
	if descriptionLine == "" {
		t.Fatal("lark-attendance SKILL.md is missing frontmatter description")
	}
	for _, required := range []string{
		"当用户提到查询考勤打卡记录、上下班打卡情况、迟到早退或漏卡结果等个人考勤查询意图时使用",
		"仅用于个人考勤打卡记录查询，不负责考勤排班管理或请假审批",
	} {
		if !strings.Contains(descriptionLine, required) {
			t.Fatalf("lark-attendance description %q must contain %q", descriptionLine, required)
		}
	}
}
