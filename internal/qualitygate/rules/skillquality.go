// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/qualitygate/facts"
	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/vfs"
)

type SkillDoc struct {
	File        string
	Name        string
	Description string
	Body        string
}

func LoadSkillDocs(skillsDir string) ([]SkillDoc, error) {
	var out []SkillDoc
	if err := walkSkillDocs(skillsDir, func(path string) error {
		data, err := vfs.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, parseSkillDoc(path, string(data)))
		return nil
	}); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].File < out[j].File
	})
	return out, nil
}

func CheckSkillQuality(docs []SkillDoc) ([]report.Diagnostic, []facts.SkillQualityFact) {
	var diags []report.Diagnostic
	var out []facts.SkillQualityFact
	for _, doc := range docs {
		criticalCount := strings.Count(doc.Body, "CRITICAL")
		wordCount := len(strings.Fields(doc.Body))
		fact := facts.SkillQualityFact{
			SourceFile:         doc.File,
			WordCount:          wordCount,
			CriticalCount:      criticalCount,
			DescriptionLength:  len([]rune(doc.Description)),
			CriticalOverBudget: criticalCount > 3,
		}
		if fact.CriticalOverBudget {
			diags = append(diags, report.Diagnostic{
				Rule:       "skill_critical_noise",
				Action:     report.ActionWarning,
				File:       doc.File,
				Message:    fmt.Sprintf("skill has %d CRITICAL markers; keep hard instructions focused", criticalCount),
				Suggestion: "reduce CRITICAL markers to at most 3 and move procedural detail into references",
			})
		}
		if fact.DescriptionLength < 20 || fact.DescriptionLength > 500 {
			diags = append(diags, report.Diagnostic{
				Rule:       "skill_description_route_quality",
				Action:     report.ActionWarning,
				File:       doc.File,
				Message:    fmt.Sprintf("description length is %d runes; routing description may be too vague or too noisy", fact.DescriptionLength),
				Suggestion: "write a concise WHAT / WHEN / NOT description for skill routing",
			})
		}
		if wordCount > 2500 {
			diags = append(diags, report.Diagnostic{
				Rule:       "skill_size_budget",
				Action:     report.ActionWarning,
				File:       doc.File,
				Message:    fmt.Sprintf("skill body has %d words", wordCount),
				Suggestion: "move long procedural sections into references and keep SKILL.md focused on routing",
			})
		}
		out = append(out, fact)
	}
	return diags, out
}

func walkSkillDocs(root string, visit func(string) error) error {
	entries, err := vfs.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			if err := walkSkillDocs(path, visit); err != nil {
				return err
			}
			continue
		}
		if entry.Type()&fs.ModeType != 0 || entry.Name() != "SKILL.md" {
			continue
		}
		if err := visit(path); err != nil {
			return err
		}
	}
	return nil
}

func parseSkillDoc(path, raw string) SkillDoc {
	doc := SkillDoc{File: path, Body: raw}
	lines := strings.Split(raw, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return doc
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
		key, value, ok := strings.Cut(lines[i], ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "name":
			doc.Name = trimFrontmatterValue(value)
		case "description":
			doc.Description = trimFrontmatterValue(value)
		}
	}
	if end >= 0 && end+1 < len(lines) {
		doc.Body = strings.Join(lines[end+1:], "\n")
	}
	return doc
}

func trimFrontmatterValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return value
}
