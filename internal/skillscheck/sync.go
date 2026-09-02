// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/selfupdate"
	"github.com/larksuite/cli/internal/vfs"
)

var (
	skillNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_:-]*(@[^\s]+)?$`)
	digestPattern    = regexp.MustCompile(`^sha256:[0-9a-fA-F]{64}$`)
	ansiPattern      = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
)

const githubSkillsSource = "larksuite/cli"

type SyncInput struct {
	Version        string
	OfficialSkills []string
	LocalSkills    []string
	PreviousState  *SkillsState
	StateReadable  bool
	Force          bool
}

type SyncPlan struct {
	Version         string
	OfficialSkills  []string
	CleanupOfficial []string
	ToUpdate        []string
	Added           []string
	SkippedDeleted  []string
}

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func ParseSkillsList(text string) []string {
	text = stripANSI(text)
	if strings.Contains(text, "Global Skills") {
		return parseGlobalSkillsList(strings.Split(text, "\n"))
	}
	return nil
}

type installedSkill struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func parseInstalledSkillsJSON(text string) ([]installedSkill, error) {
	type globalSkill struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}

	var skills []globalSkill
	if err := json.Unmarshal([]byte(text), &skills); err != nil {
		return nil, err
	}

	seen := map[string]installedSkill{}
	for _, skill := range skills {
		candidate := strings.TrimSpace(skill.Name)
		if candidate == "" || !skillNamePattern.MatchString(candidate) {
			continue
		}
		seen[candidate] = installedSkill{Name: candidate, Path: strings.TrimSpace(skill.Path)}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]installedSkill, 0, len(names))
	for _, name := range names {
		entries = append(entries, seen[name])
	}
	return entries, nil
}

func ParseOfficialSkillsIndexJSON(text string) ([]string, error) {
	type officialSkill struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		URL    string `json:"url"`
		Digest string `json:"digest"`
	}
	type officialIndex struct {
		Skills []officialSkill `json:"skills"`
	}

	var index officialIndex
	if err := json.Unmarshal([]byte(text), &index); err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	for _, skill := range index.Skills {
		candidate := strings.TrimSpace(skill.Name)
		if !skillNamePattern.MatchString(candidate) {
			return nil, fmt.Errorf("invalid skill name %q", candidate)
		}
		if skill.Type != "archive" || strings.TrimSpace(skill.URL) == "" || !digestPattern.MatchString(skill.Digest) {
			return nil, fmt.Errorf("skill %s is not a complete v0.2 archive entry", candidate)
		}
		if seen[candidate] {
			return nil, fmt.Errorf("duplicate skill %s", candidate)
		}
		seen[candidate] = true
	}

	return sortedKeys(seen), nil
}

// parseGlobalSkillsList parses the output of "npx -y skills ls -g"
func parseGlobalSkillsList(lines []string) []string {
	seen := map[string]bool{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip header
		if strings.HasPrefix(trimmed, "Global Skills") {
			continue
		}

		// Skip empty lines
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "Tip:") {
			continue
		}

		if strings.HasPrefix(trimmed, "Agents:") {
			continue
		}

		if isGlobalSkillsSectionHeader(trimmed) {
			continue
		}

		// Extract skill name, format is typically "skill-name /path/to/skill"
		parts := strings.Fields(trimmed)
		if len(parts) == 0 {
			continue
		}

		candidate := parts[0]

		// Validate and add
		if candidate == "" || !skillNamePattern.MatchString(candidate) {
			continue
		}
		seen[candidate] = true
	}

	return sortedKeys(seen)
}

func isGlobalSkillsSectionHeader(line string) bool {
	switch line {
	case "General", "Project", "Local":
		return true
	default:
		return false
	}
}

func PlanSync(input SyncInput) SyncPlan {
	official := uniqueSorted(input.OfficialSkills)
	previousOfficial := []string{}
	if input.StateReadable && input.PreviousState != nil {
		previousOfficial = input.PreviousState.OfficialSkills
	}
	cleanupOfficial := uniqueSorted(append(append([]string{}, official...), previousOfficial...))
	if input.Force {
		return SyncPlan{
			Version:         input.Version,
			OfficialSkills:  official,
			CleanupOfficial: cleanupOfficial,
			ToUpdate:        official,
			Added:           []string{},
			SkippedDeleted:  []string{},
		}
	}

	officialSet := toSet(official)
	installedOfficial := intersection(input.LocalSkills, officialSet)

	previousSet := toSet(previousOfficial)

	newAddedOfficial := []string{}
	for _, skill := range official {
		if !previousSet[skill] {
			newAddedOfficial = append(newAddedOfficial, skill)
		}
	}

	updateSet := toSet(installedOfficial)
	if len(installedOfficial) == 0 {
		updateSet = toSet(official)
	}
	for _, skill := range newAddedOfficial {
		updateSet[skill] = true
	}
	toUpdate := sortedKeys(updateSet)
	updateSet = toSet(toUpdate)

	skipped := []string{}
	for _, skill := range official {
		if !updateSet[skill] {
			skipped = append(skipped, skill)
		}
	}

	return SyncPlan{
		Version:         input.Version,
		OfficialSkills:  official,
		CleanupOfficial: cleanupOfficial,
		ToUpdate:        toUpdate,
		Added:           uniqueSorted(newAddedOfficial),
		SkippedDeleted:  skipped,
	}
}

type SkillsRunner interface {
	SkillsSources() []string
	FetchSkillsIndex(source string) *selfupdate.NpmResult
	ListGlobalSkillsJSON() *selfupdate.NpmResult
	ListGlobalSkills() *selfupdate.NpmResult
	InstallSkills(source string, nameList []string) *selfupdate.NpmResult
	InstallAllSkills(source string) *selfupdate.NpmResult
	StageSuite(source, dir string) *selfupdate.NpmResult
	InstallLocalSuite(path string) *selfupdate.NpmResult
	RemoveGlobalSkills(names []string) *selfupdate.NpmResult
}

type SyncOptions struct {
	Version string
	Layout  Layout
	Force   bool
	Runner  SkillsRunner
	Now     func() time.Time
}

type SyncResult struct {
	Action          string
	Official        []string
	OfficialUnknown bool
	Updated         []string
	Added           []string
	SkippedDeleted  []string
	Failed          []string
	Err             error
	Detail          string
	Warning         string
	Layout          Layout
	Force           bool
}

func SyncSkills(opts SyncOptions) *SyncResult {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Runner == nil {
		return &SyncResult{Action: "failed", Err: fmt.Errorf("skills runner is nil")}
	}

	previous, readable, err := ReadState()
	if err != nil {
		readable = false
		previous = nil
	}
	targetLayout, err := ResolveLayout(opts.Layout, previous, readable)
	if err != nil {
		return &SyncResult{Action: "failed", Err: err}
	}
	installed, err := listInstalledSkills(opts.Runner)
	if err != nil {
		return &SyncResult{Action: "failed", Layout: targetLayout, Err: err}
	}
	localOfficial, err := localOfficialSkills(installed, previous, readable)
	if err != nil {
		// A suite whose installed path or references cannot be read is treated as
		// absent. Planning from the old state would otherwise produce no updates
		// and leave the damaged installation in place.
		localOfficial = nil
		readable = false
		previous = nil
	} else if readable && previous != nil && previous.OfficialSkillsUnknown {
		// A cold GitHub fallback installed content without a trustworthy official
		// list. Retry the official sources as a cold sync, even at the same version.
		readable = false
		previous = nil
	}

	reasons := []string{}
	var fallbackPlan *SyncPlan
	for _, source := range opts.Runner.SkillsSources() {
		official, fetchErr := fetchOfficialSkills(opts.Runner, source)
		if fetchErr != nil {
			reasons = append(reasons, source+": "+fetchErr.Error())
			continue
		}
		plan := PlanSync(SyncInput{
			Version:        opts.Version,
			OfficialSkills: official,
			LocalSkills:    localOfficial,
			PreviousState:  previous,
			StateReadable:  readable,
			Force:          opts.Force,
		})
		fallbackPlan = &plan

		if syncErr := syncLayout(opts.Runner, source, targetLayout, plan, installed); syncErr != nil {
			reasons = append(reasons, source+": "+syncErr.Error())
			continue
		}
		return finishSync(opts, targetLayout, plan, "", "", false)
	}

	if targetLayout == LayoutSuite {
		return &SyncResult{
			Action: "failed",
			Layout: targetLayout,
			Err:    fmt.Errorf("suite skills sync failed: %s", strings.Join(reasons, "; ")),
			Detail: strings.Join(reasons, "\n"),
			Force:  opts.Force,
		}
	}

	return fallbackSeparate(opts, previous, readable, localOfficial, installed, fallbackPlan, reasons)
}

func fetchOfficialSkills(runner SkillsRunner, source string) ([]string, error) {
	result := runner.FetchSkillsIndex(source)
	if result == nil || result.Err != nil {
		return nil, fmt.Errorf("index request failed: %s", resultDetail(result))
	}
	official, err := ParseOfficialSkillsIndexJSON(result.Stdout.String())
	if err != nil {
		return nil, fmt.Errorf("invalid v0.2 index: %w", err)
	}
	if len(official) == 0 {
		return nil, fmt.Errorf("v0.2 index contains no skills")
	}
	return official, nil
}

func listInstalledSkills(runner SkillsRunner) ([]installedSkill, error) {
	jsonResult := runner.ListGlobalSkillsJSON()
	if jsonResult != nil && jsonResult.Err == nil {
		if installed, err := parseInstalledSkillsJSON(jsonResult.Stdout.String()); err == nil {
			return installed, nil
		}
	}

	textResult := runner.ListGlobalSkills()
	if textResult != nil && textResult.Err == nil {
		names := ParseSkillsList(textResult.Stdout.String())
		if names != nil {
			installed := make([]installedSkill, 0, len(names))
			for _, name := range names {
				installed = append(installed, installedSkill{Name: name})
			}
			return installed, nil
		}
	}
	return nil, fmt.Errorf("local skills list failed")
}

func localOfficialSkills(installed []installedSkill, previous *SkillsState, readable bool) ([]string, error) {
	if !readable || previous == nil || EffectiveLayout(previous) == LayoutSeparate {
		names := make([]string, 0, len(installed))
		for _, skill := range installed {
			names = append(names, skill.Name)
		}
		return names, nil
	}

	for _, skill := range installed {
		if skill.Name != "lark-suite" {
			continue
		}
		if skill.Path == "" {
			return nil, fmt.Errorf("cannot inspect installed lark-suite: global skills JSON did not include its path")
		}
		return listDirectSubdirs(filepath.Join(skill.Path, "references"))
	}
	return nil, fmt.Errorf("cannot inspect installed lark-suite: skill is not installed")
}

func listDirectSubdirs(root string) ([]string, error) {
	entries, err := vfs.ReadDir(root)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func syncLayout(runner SkillsRunner, source string, layout Layout, plan SyncPlan, installed []installedSkill) error {
	if layout == LayoutSuite {
		return syncSuite(runner, source, plan, installed)
	}
	if len(plan.ToUpdate) > 0 {
		result := runner.InstallSkills(source, plan.ToUpdate)
		if result == nil || result.Err != nil {
			return fmt.Errorf("archive install failed: %s", resultDetail(result))
		}
	}
	if hasInstalledSkill(installed, "lark-suite") {
		if result := runner.RemoveGlobalSkills([]string{"lark-suite"}); result == nil || result.Err != nil {
			return fmt.Errorf("remove lark-suite failed: %s", resultDetail(result))
		}
	}
	return nil
}

func fallbackSeparate(opts SyncOptions, previous *SkillsState, readable bool, local []string, installed []installedSkill, plan *SyncPlan, reasons []string) *SyncResult {
	if plan == nil && readable && previous != nil && len(previous.OfficialSkills) > 0 {
		fallback := PlanSync(SyncInput{
			Version:        opts.Version,
			OfficialSkills: previous.OfficialSkills,
			LocalSkills:    local,
			PreviousState:  previous,
			StateReadable:  true,
			Force:          opts.Force,
		})
		plan = &fallback
	}

	var installResult *selfupdate.NpmResult
	officialUnknown := plan == nil
	if plan == nil {
		installResult = opts.Runner.InstallAllSkills(githubSkillsSource)
	} else if len(plan.ToUpdate) > 0 {
		installResult = opts.Runner.InstallSkills(githubSkillsSource, plan.ToUpdate)
	}
	if installResult != nil && installResult.Err != nil {
		reasons = append(reasons, githubSkillsSource+": "+resultDetail(installResult))
		return &SyncResult{
			Action: "failed",
			Layout: LayoutSeparate,
			Err:    fmt.Errorf("separate skills sync failed: %s", strings.Join(reasons, "; ")),
			Detail: strings.Join(reasons, "\n"),
			Force:  opts.Force,
		}
	}
	if hasInstalledSkill(installed, "lark-suite") {
		if result := opts.Runner.RemoveGlobalSkills([]string{"lark-suite"}); result == nil || result.Err != nil {
			return &SyncResult{Action: "failed", Layout: LayoutSeparate, Err: fmt.Errorf("remove lark-suite failed: %s", resultDetail(result)), Force: opts.Force}
		}
	}
	if plan == nil {
		empty := SyncPlan{Version: opts.Version}
		plan = &empty
	}
	warning := ""
	if installResult != nil {
		warning = "used the GitHub legacy fallback; installed Skill content may be incomplete because the legacy protocol can ignore individual file download failures"
	}
	return finishSync(opts, LayoutSeparate, *plan, "fallback_synced", warning, officialUnknown)
}

func finishSync(opts SyncOptions, layout Layout, plan SyncPlan, action, warning string, officialUnknown bool) *SyncResult {
	if action == "" {
		action = "synced"
	}
	result := &SyncResult{
		Action:          action,
		Official:        plan.OfficialSkills,
		OfficialUnknown: officialUnknown,
		Updated:         plan.ToUpdate,
		Added:           plan.Added,
		SkippedDeleted:  plan.SkippedDeleted,
		Warning:         warning,
		Layout:          layout,
		Force:           opts.Force,
	}
	state := SkillsState{
		Version:               opts.Version,
		Layout:                layout,
		OfficialSkills:        plan.OfficialSkills,
		OfficialSkillsUnknown: officialUnknown,
		UpdatedSkills:         plan.ToUpdate,
		AddedOfficialSkills:   plan.Added,
		SkippedDeletedSkills:  plan.SkippedDeleted,
		UpdatedAt:             opts.Now().UTC().Format(time.RFC3339),
	}
	if err := WriteState(state); err != nil {
		result.Action = "failed"
		result.Err = fmt.Errorf("skills synced but state not written: %w", err)
	}
	return result
}

func hasInstalledSkill(installed []installedSkill, name string) bool {
	for _, skill := range installed {
		if skill.Name == name {
			return true
		}
	}
	return false
}

func resultDetail(result *selfupdate.NpmResult) string {
	if result == nil {
		return ""
	}
	parts := []string{}
	if output := strings.TrimSpace(result.CombinedOutput()); output != "" {
		parts = append(parts, output)
	}
	if result.Err != nil {
		parts = append(parts, result.Err.Error())
	}
	return strings.Join(parts, "\n")
}

func uniqueSorted(values []string) []string {
	return sortedKeys(toSet(values))
}

func toSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = true
		}
	}
	return out
}

// result = { x | x ∈ values ∧ x ∈ allowed }
func intersection(values []string, allowed map[string]bool) []string {
	out := map[string]bool{}
	for _, value := range values {
		if allowed[value] {
			out[value] = true
		}
	}
	return sortedKeys(out)
}

func sortedKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
