// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/vfs"
)

const (
	LayoutSeparate = "separate"
	LayoutHybrid   = "hybrid"

	suiteSkillName         = "lark-suite"
	sharedSkillName        = "lark-shared"
	suiteRoutesPlaceholder = "<!-- LARK_SUITE_ROUTES -->"
)

type GlobalSkillInfo struct {
	Name string
	Path string
}

func NormalizeLayout(layout string) (string, bool) {
	switch strings.TrimSpace(layout) {
	case "", LayoutSeparate:
		return LayoutSeparate, true
	case LayoutHybrid:
		return LayoutHybrid, true
	default:
		return "", false
	}
}

func ParseFlatSkills(value string) []string {
	seen := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		name := strings.TrimSpace(part)
		if name != "" {
			seen[name] = true
		}
	}
	return sortedKeys(seen)
}

func ParseGlobalSkillInfosJSON(text string) []GlobalSkillInfo {
	infos, _ := parseGlobalSkillInfosJSON(text)
	return infos
}

func parseGlobalSkillInfosJSON(text string) ([]GlobalSkillInfo, bool) {
	type globalSkill struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}

	var skills []globalSkill
	if err := json.Unmarshal([]byte(text), &skills); err != nil {
		return nil, false
	}

	seen := map[string]GlobalSkillInfo{}
	for _, skill := range skills {
		name := strings.TrimSpace(skill.Name)
		path := strings.TrimSpace(skill.Path)
		if name == "" || path == "" || !skillNamePattern.MatchString(name) {
			continue
		}
		seen[name] = GlobalSkillInfo{Name: name, Path: path}
	}

	out := make([]GlobalSkillInfo, 0, len(seen))
	for _, info := range seen {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, true
}

func installedSkillNamesFromInfos(infos []GlobalSkillInfo) []string {
	seen := map[string]bool{}
	for _, info := range infos {
		seen[info.Name] = true
		if info.Name == suiteSkillName {
			for _, subskill := range listSuiteSubskills(info.Path) {
				seen[subskill] = true
			}
		}
	}
	return sortedKeys(seen)
}

func listSuiteSubskills(suitePath string) []string {
	entries, err := vfs.ReadDir(filepath.Join(suitePath, "references", "subskills"))
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name != "" && skillNamePattern.MatchString(name) {
			seen[name] = true
		}
	}
	return sortedKeys(seen)
}

func normalOfficialSkills(skills []string) []string {
	out := []string{}
	for _, skill := range uniqueSorted(skills) {
		if skill != suiteSkillName {
			out = append(out, skill)
		}
	}
	return out
}

func deletedOfficialSkills(official, local []string, previous *SkillsState, stateReadable, force bool, layout string) []string {
	if force || !stateReadable || previous == nil {
		return []string{}
	}
	officialSet := toSet(official)
	localSet := toSet(local)
	deleted := map[string]bool{}
	for _, skill := range previous.OfficialSkills {
		if !officialSet[skill] || localSet[skill] {
			continue
		}
		if layout != LayoutSeparate && skill == sharedSkillName {
			continue
		}
		deleted[skill] = true
	}
	return sortedKeys(deleted)
}

func suiteEffectiveSkills(official []string, deleted map[string]bool) []string {
	out := []string{}
	for _, skill := range normalOfficialSkills(official) {
		if !deleted[skill] {
			out = append(out, skill)
		}
	}
	return uniqueSorted(out)
}

func resolveHybridSkillSets(layout string, requestedFlat, official []string, skippedDeleted []string) ([]string, []string, error) {
	if layout == LayoutSeparate {
		return []string{}, []string{}, nil
	}

	officialSet := toSet(official)
	deletedSet := toSet(skippedDeleted)
	configuredFlat := map[string]bool{}
	effectiveFlat := map[string]bool{}
	for _, skill := range uniqueSorted(requestedFlat) {
		if skill == sharedSkillName {
			continue
		}
		if !officialSet[skill] {
			continue
		}
		configuredFlat[skill] = true
		if !deletedSet[skill] {
			effectiveFlat[skill] = true
		}
	}

	collected := []string{}
	for _, skill := range normalOfficialSkills(official) {
		if skill == sharedSkillName {
			collected = append(collected, skill)
			continue
		}
		if deletedSet[skill] || effectiveFlat[skill] {
			continue
		}
		collected = append(collected, skill)
	}
	return sortedKeys(configuredFlat), uniqueSortedWithFirst(collected, sharedSkillName), nil
}

func uniqueSortedWithFirst(values []string, first string) []string {
	seen := toSet(values)
	if !seen[first] {
		return sortedKeys(seen)
	}
	delete(seen, first)
	return append([]string{first}, sortedKeys(seen)...)
}

func assembleSuiteLayout(layout string, collected []string, keepSharedTopLevel bool, infos []GlobalSkillInfo) error {
	if layout == LayoutSeparate {
		return nil
	}

	infoByName := map[string]GlobalSkillInfo{}
	for _, info := range infos {
		infoByName[info.Name] = info
	}
	suiteInfo, ok := infoByName[suiteSkillName]
	if !ok {
		return fmt.Errorf("%s was not installed from isolated skills source", suiteSkillName)
	}

	subskillsDir := filepath.Join(suiteInfo.Path, "references", "subskills")
	if err := vfs.RemoveAll(subskillsDir); err != nil {
		return err
	}
	if err := vfs.MkdirAll(subskillsDir, 0o755); err != nil {
		return err
	}

	for _, skill := range collected {
		info, ok := infoByName[skill]
		if !ok {
			return fmt.Errorf("suite subskill %q was not installed", skill)
		}
		dst := filepath.Join(subskillsDir, skill)
		if keepSharedTopLevel && skill == sharedSkillName {
			if err := copyDir(info.Path, dst); err != nil {
				return err
			}
			continue
		}
		if err := moveDir(info.Path, dst); err != nil {
			return err
		}
	}
	return renderSuiteRoutes(suiteInfo.Path, collected)
}

func renderSuiteRoutes(suitePath string, collected []string) error {
	skillPath := filepath.Join(suitePath, "SKILL.md")
	data, err := vfs.ReadFile(skillPath)
	if err != nil {
		return err
	}
	text := normalizeSuiteTemplateText(string(data))
	routes := []string{}
	for _, skill := range collected {
		desc := skillDescription(filepath.Join(suitePath, "references", "subskills", skill, "SKILL.md"))
		if desc == "" {
			desc = skill
		}
		routes = append(routes, fmt.Sprintf("- %s: %s", skill, desc))
	}
	if !strings.Contains(text, suiteRoutesPlaceholder) {
		return fmt.Errorf("%s route placeholder not found", suiteSkillName)
	}
	text = strings.Replace(text, suiteRoutesPlaceholder, strings.Join(routes, "\n"), 1)
	return vfs.WriteFile(skillPath, []byte(text), 0o644)
}

func normalizeSuiteTemplateText(text string) string {
	text = strings.ReplaceAll(text, "--collected-skills", "--flat-skills")
	oldShared := "`lark-shared` 是共享基础能力，不作为 `--flat-skills` 的可选项。为了保证 suite 内子能力可用，hybrid 布局会同时保留顶层 `lark-shared`，并在 `lark-suite/references/subskills/lark-shared/SKILL.md` 中维护一份副本。"
	newShared := "`lark-shared` 是共享基础能力，不作为 `--flat-skills` 的可选项。为了保证 suite 内子能力可用，它始终会进入 `lark-suite/references/subskills/lark-shared/SKILL.md`；只有 hybrid 布局存在平铺 skill 时，顶层才会额外保留一份 `lark-shared`。"
	return strings.ReplaceAll(text, oldShared, newShared)
}

func skillDescription(path string) string {
	data, err := vfs.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for i, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			return ""
		}
		if strings.HasPrefix(trimmed, "description:") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
			if value == ">" || value == "|" {
				return foldedYAMLScalar(lines[i+2:])
			}
			return strings.Trim(value, `"'`)
		}
	}
	return ""
}

func foldedYAMLScalar(lines []string) string {
	parts := []string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if trimmed == "---" || !isIndentedYAMLLine(line) {
			break
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, " ")
}

func isIndentedYAMLLine(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

func moveDir(src, dst string) error {
	if err := vfs.RemoveAll(dst); err != nil {
		return err
	}
	if err := vfs.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := vfs.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyDir(src, dst); err != nil {
		return err
	}
	return vfs.RemoveAll(src)
}

func copyDir(src, dst string) error {
	info, err := vfs.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}
	if err := vfs.RemoveAll(dst); err != nil {
		return err
	}
	return copyDirEntries(src, dst)
}

func copyDirEntries(src, dst string) error {
	if err := vfs.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := vfs.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDirEntries(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := vfs.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := vfs.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := vfs.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
