// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/vfs"
)

type Layout string

const (
	LayoutSeparate         Layout = "separate"
	LayoutSuite            Layout = "suite"
	suiteDescriptionPrefix        = "description: 飞书/Lark 聚合能力入口：管理飞书/Lark 产品能力（"
	suiteDescriptionSuffix        = "等）。"
)

func ParseLayout(value string) (Layout, error) {
	layout := Layout(strings.TrimSpace(value))
	switch layout {
	case "", LayoutSeparate, LayoutSuite:
		return layout, nil
	default:
		return "", fmt.Errorf("skills layout must be separate or suite")
	}
}

func EffectiveLayout(state *SkillsState) Layout {
	if state != nil && state.Layout == LayoutSuite {
		return LayoutSuite
	}
	return LayoutSeparate
}

func ResolveLayout(requested Layout, state *SkillsState, readable bool) (Layout, error) {
	if requested != "" {
		return ParseLayout(string(requested))
	}
	if readable {
		return EffectiveLayout(state), nil
	}
	return LayoutSeparate, nil
}

func syncSuite(runner SkillsRunner, source string, plan SyncPlan, installed []installedSkill) error {
	installedSeparate := installedOfficialNames(installed, plan.CleanupOfficial)
	if len(plan.ToUpdate) == 0 {
		remove := append(installedSeparate, installedNameIfPresent(installed, "lark-suite")...)
		return removeSkills(runner, remove)
	}

	stagingRoot, err := vfs.MkdirTemp("", "lark-cli-suite-")
	if err != nil {
		return fmt.Errorf("create suite staging directory: %w", err)
	}
	defer vfs.RemoveAll(stagingRoot)

	stageResult := runner.StageSuite(source, stagingRoot)
	if stageResult == nil || stageResult.Err != nil {
		return fmt.Errorf("suite archive install failed: %s", resultDetail(stageResult))
	}

	suitePath := filepath.Join(stagingRoot, ".agents", "skills", "lark-suite")
	if err := normalizeSuiteGuides(suitePath); err != nil {
		return err
	}
	if err := prepareSuite(suitePath, plan.OfficialSkills, plan.ToUpdate); err != nil {
		return err
	}
	installResult := runner.InstallLocalSuite(suitePath)
	if installResult == nil || installResult.Err != nil {
		return fmt.Errorf("install cropped lark-suite failed: %s", resultDetail(installResult))
	}
	if err := removeSkills(runner, installedSeparate); err != nil {
		return err
	}
	return nil
}

// normalizeSuiteGuides keeps nested domain guides from being discovered as
// standalone skills by recursive skill catalogues.
func normalizeSuiteGuides(suitePath string) error {
	root := filepath.Join(suitePath, "references")
	type guideRename struct{ oldPath, newPath, dir string }
	var renames []guideRename
	markdownFiles := []string{filepath.Join(suitePath, "SKILL.md")}
	var visit func(string) error
	visit = func(dir string) error {
		entries, err := vfs.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				if err := visit(path); err != nil {
					return err
				}
				continue
			}
			if entry.Name() == "SKILL.md" {
				renames = append(renames, guideRename{oldPath: path, newPath: filepath.Join(dir, "GUIDE.md"), dir: dir})
			}
			if strings.HasSuffix(entry.Name(), ".md") && entry.Name() != "SKILL.md" {
				markdownFiles = append(markdownFiles, path)
			}
		}
		return nil
	}
	if err := visit(root); err != nil {
		return fmt.Errorf("inspect suite guides: %w", err)
	}
	for _, rename := range renames {
		if _, err := vfs.Stat(rename.newPath); err == nil {
			return fmt.Errorf("rename nested suite guide: destination already exists: %s", filepath.Base(rename.newPath))
		}
		if err := vfs.Rename(rename.oldPath, rename.newPath); err != nil {
			return fmt.Errorf("rename nested suite guide: %w", err)
		}
		markdownFiles = append(markdownFiles, rename.newPath)
	}
	for _, path := range markdownFiles {
		content, err := vfs.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read suite guide %s: %w", filepath.Base(path), err)
		}
		updated := string(content)
		for _, rename := range renames {
			oldRel := filepath.ToSlash(relativePath(filepath.Dir(path), rename.oldPath))
			newRel := filepath.ToSlash(relativePath(filepath.Dir(path), rename.newPath))
			if oldRel == "SKILL.md" {
				updated = replaceCurrentSuiteGuideToken(updated, "./SKILL.md", "./GUIDE.md")
				updated = replaceCurrentSuiteGuideToken(updated, `.\SKILL.md`, `.\GUIDE.md`)
			} else {
				oldBackslash := strings.ReplaceAll(oldRel, "/", `\`)
				newBackslash := strings.ReplaceAll(newRel, "/", `\`)
				updated = replaceSuitePathToken(updated, "./"+oldRel, "./"+newRel)
				updated = replaceSuitePathToken(updated, `.\`+oldBackslash, `.\`+newBackslash)
				updated = replaceSuitePathToken(updated, oldRel, newRel)
				updated = replaceSuitePathToken(updated, oldBackslash, newBackslash)
			}
		}
		if path == filepath.Join(suitePath, "SKILL.md") {
			updated = strings.ReplaceAll(updated, "references/<skill-name>/SKILL.md", "references/<skill-name>/GUIDE.md")
			updated = strings.ReplaceAll(updated, `references\<skill-name>\SKILL.md`, `references\<skill-name>\GUIDE.md`)
		}
		if updated != string(content) {
			if err := vfs.WriteFile(path, []byte(updated), 0o644); err != nil {
				return fmt.Errorf("rewrite suite guide %s: %w", filepath.Base(path), err)
			}
		}
	}
	return nil
}

func replaceCurrentSuiteGuideToken(content, oldPath, newPath string) string {
	for cursor := 0; ; {
		rel := strings.Index(content[cursor:], oldPath)
		if rel < 0 {
			break
		}
		start := cursor + rel
		end := start + len(oldPath)
		beforeOK := start == 0 || !suitePathChar(content[start-1])
		afterOK := end == len(content) || !currentGuideContinuation(content, end)
		if beforeOK && afterOK {
			content = content[:start] + newPath + content[end:]
			cursor = start + len(newPath)
		} else {
			cursor = start + 1
		}
	}
	return content
}

func currentGuideContinuation(content string, index int) bool {
	if content[index] != '.' {
		return suitePathContinuation(content[index])
	}
	for index < len(content) && content[index] == '.' {
		index++
	}
	return index < len(content) && suitePathContinuation(content[index])
}

func replaceSuitePathToken(content, oldPath, newPath string) string {
	for cursor := 0; ; {
		rel := strings.Index(content[cursor:], oldPath)
		if rel < 0 {
			break
		}
		start := cursor + rel
		end := start + len(oldPath)
		beforeOK := start == 0 || !suitePathChar(content[start-1])
		afterOK := end == len(content) || !suitePathContinuation(content[end])
		if beforeOK && afterOK {
			content = content[:start] + newPath + content[end:]
			cursor = start + len(newPath)
		} else {
			cursor = start + 1
		}
	}
	return content
}

func suitePathChar(value byte) bool {
	return value == '.' || value == '/' || value == '\\' || value == '-' || value == '_' ||
		(value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') || (value >= '0' && value <= '9')
}

func suitePathContinuation(value byte) bool {
	return value == '/' || value == '\\' || value == '-' || value == '_' ||
		(value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') || (value >= '0' && value <= '9')
}

func relativePath(root, target string) string {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return target
	}
	return rel
}

func prepareSuite(suitePath string, official, target []string) error {
	referencesPath := filepath.Join(suitePath, "references")
	archived, err := listDirectSubdirs(referencesPath)
	if err != nil {
		return fmt.Errorf("inspect suite archive: %w", err)
	}
	if err := assertSameSkillNames(archived, official, "suite archive"); err != nil {
		return err
	}

	targetSet := toSet(target)
	removed := []string{}
	for _, name := range official {
		if targetSet[name] {
			continue
		}
		if err := vfs.RemoveAll(filepath.Join(referencesPath, name)); err != nil {
			return fmt.Errorf("remove suite child %s: %w", name, err)
		}
		removed = append(removed, name)
	}

	skillPath := filepath.Join(suitePath, "SKILL.md")
	raw, err := vfs.ReadFile(skillPath)
	if err != nil {
		return fmt.Errorf("read suite SKILL.md: %w", err)
	}
	rendered, err := cropSuiteRoutes(string(raw), removed, target)
	if err != nil {
		return err
	}
	if err := vfs.WriteFile(skillPath, []byte(rendered), 0o644); err != nil {
		return fmt.Errorf("write cropped suite SKILL.md: %w", err)
	}

	kept, err := listDirectSubdirs(referencesPath)
	if err != nil {
		return fmt.Errorf("verify cropped suite: %w", err)
	}
	return assertSameSkillNames(kept, target, "cropped suite")
}

func cropSuiteRoutes(content string, removed, target []string) (string, error) {
	routeLine := func(name string) *regexp.Regexp {
		return regexp.MustCompile(`(?m)^- ` + regexp.QuoteMeta(name) + `(?:（[^）\n]*）)?:[^\n]*(?:\n|$)`)
	}

	for _, name := range removed {
		line := routeLine(name)
		if len(line.FindAllStringIndex(content, -1)) != 1 {
			return "", fmt.Errorf("suite route for %s is missing or duplicated", name)
		}
		content = line.ReplaceAllString(content, "")
	}

	for _, name := range target {
		if len(routeLine(name).FindAllStringIndex(content, -1)) != 1 {
			return "", fmt.Errorf("cropped suite route for %s is missing or duplicated", name)
		}
	}

	keywords := suiteKeywords(content)
	start := strings.Index(content, suiteDescriptionPrefix)
	if start < 0 {
		return "", fmt.Errorf("suite description prefix is missing")
	}
	valueStart := start + len(suiteDescriptionPrefix)
	valueEndOffset := strings.Index(content[valueStart:], suiteDescriptionSuffix)
	if valueEndOffset < 0 {
		return "", fmt.Errorf("suite description keyword suffix is missing")
	}
	valueEnd := valueStart + valueEndOffset
	content = content[:valueStart] + strings.Join(keywords, "、") + content[valueEnd:]
	return content, nil
}

func suiteKeywords(content string) []string {
	routeLine := regexp.MustCompile(`(?m)^- [^\n（]+(?:（([^）\n]*)）)?:`)
	seen := map[string]bool{}
	keywords := []string{}
	for _, match := range routeLine.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 || match[1] == "" {
			continue
		}
		for _, keyword := range strings.Split(match[1], "、") {
			keyword = strings.TrimSpace(keyword)
			if keyword != "" && !seen[keyword] {
				seen[keyword] = true
				keywords = append(keywords, keyword)
			}
		}
	}
	return keywords
}

func installedOfficialNames(installed []installedSkill, official []string) []string {
	officialSet := toSet(official)
	names := []string{}
	for _, skill := range installed {
		if officialSet[skill.Name] {
			names = append(names, skill.Name)
		}
	}
	sort.Strings(names)
	return names
}

func installedNameIfPresent(installed []installedSkill, name string) []string {
	if hasInstalledSkill(installed, name) {
		return []string{name}
	}
	return nil
}

func removeSkills(runner SkillsRunner, names []string) error {
	if len(names) == 0 {
		return nil
	}
	result := runner.RemoveGlobalSkills(uniqueSorted(names))
	if result == nil || result.Err != nil {
		return fmt.Errorf("remove stale official skills failed: %s", resultDetail(result))
	}
	return nil
}

func assertSameSkillNames(got, want []string, label string) error {
	got = uniqueSorted(got)
	want = uniqueSorted(want)
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		return fmt.Errorf("%s child Skill list mismatch: got %v, want %v", label, got, want)
	}
	return nil
}
