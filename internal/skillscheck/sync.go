// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/selfupdate"
)

var (
	skillNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_:-]*(@[^\s]+)?$`)
	ansiPattern      = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
)

type SyncInput struct {
	Version        string
	Layout         string
	OfficialSkills []string
	LocalSkills    []string
	PreviousState  *SkillsState
	StateReadable  bool
	Force          bool
}

type SyncPlan struct {
	Version        string
	OfficialSkills []string
	ToUpdate       []string
	Added          []string
	SkippedDeleted []string
}

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func ParseSkillsList(text string) []string {
	text = stripANSI(text)
	lines := strings.Split(text, "\n")

	// Detect format type
	hasGlobalSkills := strings.Contains(text, "Global Skills")
	hasAvailableSkills := strings.Contains(text, "Available Skills")

	if hasGlobalSkills {
		// Format 1: locally installed skills list from "npx -y skills ls -g"
		return parseGlobalSkillsList(lines)
	} else if hasAvailableSkills {
		// Format 2: official skills list from "npx -y skills add ... --list"
		return parseOfficialSkillsList(lines)
	}
	return nil
}

func ParseGlobalSkillsJSON(text string) []string {
	type globalSkill struct {
		Name string `json:"name"`
	}

	var skills []globalSkill
	if err := json.Unmarshal([]byte(text), &skills); err != nil {
		return nil
	}

	seen := map[string]bool{}
	for _, skill := range skills {
		candidate := strings.TrimSpace(skill.Name)
		if candidate == "" || !skillNamePattern.MatchString(candidate) {
			continue
		}
		seen[candidate] = true
	}

	return sortedKeys(seen)
}

func ParseOfficialSkillsIndexJSON(text string) ([]string, error) {
	type officialSkill struct {
		Name string `json:"name"`
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
		if skillNamePattern.MatchString(candidate) {
			seen[candidate] = true
		}
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

// parseOfficialSkillsList parses the output of "npx -y skills add ... --list"
func parseOfficialSkillsList(lines []string) []string {
	seen := map[string]bool{}
	inAvailableSection := false

	for _, line := range lines {
		// Check if we've reached the "Available Skills" section
		if strings.Contains(line, "Available Skills") {
			inAvailableSection = true
			continue
		}

		if !inAvailableSection {
			continue
		}

		// Process lines containing "│", e.g. " │    lark-approval "
		if strings.Contains(line, "│") {
			// Remove all "│" characters and spaces, extract the first valid token in order
			parts := strings.FieldsFunc(line, func(r rune) bool {
				return r == '│' || r == ' '
			})

			if len(parts) > 0 {
				candidate := parts[0]
				if skillNamePattern.MatchString(candidate) {
					seen[candidate] = true
				}
			}
		}
	}

	return sortedKeys(seen)
}

func PlanSync(input SyncInput) SyncPlan {
	official := normalOfficialSkills(input.OfficialSkills)
	layout, _ := NormalizeLayout(input.Layout)
	skippedDeleted := deletedOfficialSkills(official, input.LocalSkills, input.PreviousState, input.StateReadable, input.Force, layout)
	if layout != LayoutSeparate {
		return SyncPlan{
			Version:        input.Version,
			OfficialSkills: official,
			ToUpdate:       suiteEffectiveSkills(official, toSet(skippedDeleted)),
			Added:          newlyOfficialSkills(official, input.PreviousState, input.StateReadable),
			SkippedDeleted: skippedDeleted,
		}
	}
	if input.Force {
		return SyncPlan{
			Version:        input.Version,
			OfficialSkills: official,
			ToUpdate:       official,
			Added:          []string{},
			SkippedDeleted: []string{},
		}
	}

	officialSet := toSet(official)
	installedOfficial := intersection(input.LocalSkills, officialSet)
	newAddedOfficial := newlyOfficialSkills(official, input.PreviousState, input.StateReadable)

	updateSet := toSet(installedOfficial)
	for _, skill := range newAddedOfficial {
		updateSet[skill] = true
	}
	toUpdate := sortedKeys(updateSet)
	updateSet = toSet(toUpdate)

	return SyncPlan{
		Version:        input.Version,
		OfficialSkills: official,
		ToUpdate:       toUpdate,
		Added:          newAddedOfficial,
		SkippedDeleted: skippedDeleted,
	}
}

type SkillsRunner interface {
	ListOfficialSkillsIndex() *selfupdate.NpmResult
	ListOfficialSkills() *selfupdate.NpmResult
	ListGlobalSkillsJSON() *selfupdate.NpmResult
	ListGlobalSkills() *selfupdate.NpmResult
	InstallSkill(nameList []string) *selfupdate.NpmResult
	InstallAllSkills() *selfupdate.NpmResult
	InstallSuiteSkill() *selfupdate.NpmResult
}

type SyncOptions struct {
	Version    string
	Layout     string
	FlatSkills []string
	Force      bool
	Runner     SkillsRunner
	Now        func() time.Time
}

type SyncResult struct {
	Action         string
	Official       []string
	Updated        []string
	Added          []string
	SkippedDeleted []string
	Failed         []string
	Err            error
	Detail         string
	Force          bool
	Layout         string
	Flat           []string
	Collected      []string
}

func SyncSkills(opts SyncOptions) *SyncResult {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Runner == nil {
		return &SyncResult{Action: "failed", Err: fmt.Errorf("skills runner is nil")}
	}
	layout, ok := NormalizeLayout(opts.Layout)
	if !ok {
		return &SyncResult{Action: "failed", Err: fmt.Errorf("unsupported skills layout %q", opts.Layout)}
	}

	// --- Step 1: List official skills ---
	official, reason, ok := listOfficialSkills(opts.Runner)
	if !ok {
		if layout != LayoutSeparate {
			return failedSync(layout, opts.Force, fmt.Errorf("failed to discover official skills for %s layout: %s", layout, reason), reason)
		}
		return fallbackFullInstall(opts, reason, nil)
	}

	// --- Step 2: List local (installed) skills ---
	local, ok := listLocalSkills(opts.Runner)
	if !ok {
		if layout != LayoutSeparate {
			return failedSync(layout, opts.Force, fmt.Errorf("failed to list local skills for %s layout", layout), "local skills list failed or parsed as empty")
		}
		return fallbackFullInstall(opts, "local skills list failed or parsed as empty", official)
	}

	// --- Step 3: Read previous state ---
	previous, readable, err := ReadState()
	if err != nil {
		readable = false
		previous = nil
	}

	plan := PlanSync(SyncInput{
		Version:        opts.Version,
		Layout:         layout,
		OfficialSkills: official,
		LocalSkills:    local,
		PreviousState:  previous,
		StateReadable:  readable,
		Force:          opts.Force,
	})
	flat, collected, err := resolveHybridSkillSets(layout, opts.FlatSkills, plan.OfficialSkills, plan.SkippedDeleted)
	if err != nil {
		return &SyncResult{Action: "failed", Err: err, Official: plan.OfficialSkills, Force: opts.Force, Layout: layout}
	}

	result := &SyncResult{
		Action:         "synced",
		Official:       plan.OfficialSkills,
		Updated:        plan.ToUpdate,
		Added:          plan.Added,
		SkippedDeleted: plan.SkippedDeleted,
		Force:          opts.Force,
		Layout:         layout,
		Flat:           flat,
		Collected:      collected,
	}

	if len(plan.ToUpdate) == 0 {
		if layout != LayoutSeparate {
			return failedSync(layout, opts.Force, fmt.Errorf("no target skills to assemble %s layout", layout), "toUpdate skills empty")
		}
		return fallbackFullInstall(opts, "toUpdate skills empty fallback", official)
	}

	if len(plan.ToUpdate) > 0 {
		installResult := opts.Runner.InstallSkill(plan.ToUpdate)
		if installResult == nil || installResult.Err != nil {
			if layout != LayoutSeparate {
				return failedSync(layout, opts.Force, fmt.Errorf("failed to install skills for %s layout: %s", layout, resultDetail(installResult)), resultDetail(installResult))
			}
			return fallbackFullInstall(opts, resultDetail(installResult), official)
		}
	}
	if layout != LayoutSeparate {
		installSuiteResult := opts.Runner.InstallSuiteSkill()
		if installSuiteResult == nil || installSuiteResult.Err != nil {
			result.Action = "failed"
			result.Err = fmt.Errorf("failed to install %s from isolated skills source: %s", suiteSkillName, resultDetail(installSuiteResult))
			result.Detail = resultDetail(installSuiteResult)
			return result
		}
		infosResult := opts.Runner.ListGlobalSkillsJSON()
		if infosResult == nil || infosResult.Err != nil {
			result.Action = "failed"
			result.Err = fmt.Errorf("failed to list installed skills for %s assembly: %s", suiteSkillName, resultDetail(infosResult))
			result.Detail = resultDetail(infosResult)
			return result
		}
		infos := ParseGlobalSkillInfosJSON(infosResult.Stdout.String())
		keepSharedTopLevel := layout == LayoutHybrid && len(flat) > 0
		if err := assembleSuiteLayout(layout, collected, keepSharedTopLevel, infos); err != nil {
			result.Action = "failed"
			result.Err = fmt.Errorf("failed to assemble %s layout: %w", layout, err)
			return result
		}
	}

	state := SkillsState{
		Version:              opts.Version,
		Layout:               layout,
		OfficialSkills:       plan.OfficialSkills,
		UpdatedSkills:        plan.ToUpdate,
		AddedOfficialSkills:  plan.Added,
		SkippedDeletedSkills: plan.SkippedDeleted,
		FlatSkills:           stateFlatSkills(layout, flat),
		UpdatedAt:            opts.Now().UTC().Format(time.RFC3339),
	}
	if err := WriteState(state); err != nil {
		result.Action = "failed"
		result.Err = fmt.Errorf("skills synced but state not written: %w", err)
		return result
	}

	return result
}

func failedSync(layout string, force bool, err error, detail string) *SyncResult {
	return &SyncResult{
		Action: "failed",
		Err:    err,
		Detail: detail,
		Force:  force,
		Layout: layout,
	}
}

func listOfficialSkills(runner SkillsRunner) ([]string, string, bool) {
	reasons := []string{}

	indexResult := runner.ListOfficialSkillsIndex()
	if indexResult == nil || indexResult.Err != nil {
		reasons = append(reasons, "official skills index failed: "+resultDetail(indexResult))
	} else {
		official, err := ParseOfficialSkillsIndexJSON(indexResult.Stdout.String())
		if err != nil {
			reasons = append(reasons, "official skills index JSON invalid: "+err.Error())
		} else if len(official) > 0 {
			return official, "", true
		} else {
			reasons = append(reasons, "official skills index contains no skills")
		}
	}

	officialResult := runner.ListOfficialSkills()
	if officialResult == nil || officialResult.Err != nil {
		reasons = append(reasons, "official skills list failed: "+resultDetail(officialResult))
		return nil, strings.Join(reasons, "; "), false
	}
	official := ParseSkillsList(officialResult.Stdout.String())
	if len(official) > 0 {
		return official, "", true
	}
	if strings.TrimSpace(officialResult.Stdout.String()) != "" {
		reasons = append(reasons, "official skills list parsed as empty despite non-empty stdout")
	} else {
		reasons = append(reasons, "official skills list returned no skills")
	}
	return nil, strings.Join(reasons, "; "), false
}

func listLocalSkills(runner SkillsRunner) ([]string, bool) {
	jsonResult := runner.ListGlobalSkillsJSON()
	if jsonResult != nil && jsonResult.Err == nil {
		infos, valid := parseGlobalSkillInfosJSON(jsonResult.Stdout.String())
		if valid {
			return installedSkillNamesFromInfos(infos), true
		}
	}

	textResult := runner.ListGlobalSkills()
	if textResult != nil && textResult.Err == nil {
		if local := ParseSkillsList(textResult.Stdout.String()); len(local) > 0 {
			return local, true
		}
	}

	return nil, false
}

// fallbackFullInstall performs a full skills install (npx -y skills add <source> -g -y)
// when incremental sync is not possible. On success it writes a state file so that
// subsequent syncs can use incremental mode. When official is non-nil the state
// records the full official list; otherwise a minimal state (version only) is
// written to break the fallback loop.
func fallbackFullInstall(opts SyncOptions, reason string, official []string) *SyncResult {
	installResult := opts.Runner.InstallAllSkills()
	if installResult == nil {
		return &SyncResult{
			Action: "fallback_failed",
			Err:    fmt.Errorf("full skills install failed: empty result (reason: %s)", reason),
			Detail: reason,
			Force:  opts.Force,
			Layout: LayoutSeparate,
		}
	}
	if installResult.Err != nil {
		return &SyncResult{
			Action: "fallback_failed",
			Err:    fmt.Errorf("full skills install failed: %w (reason: %s)", installResult.Err, reason),
			Detail: reason + "\n" + resultDetail(installResult),
			Force:  opts.Force,
			Layout: LayoutSeparate,
		}
	}

	state := SkillsState{
		Version:              opts.Version,
		Layout:               LayoutSeparate,
		OfficialSkills:       official,
		UpdatedSkills:        official,
		AddedOfficialSkills:  official,
		SkippedDeletedSkills: []string{},
		UpdatedAt:            opts.Now().UTC().Format(time.RFC3339),
	}
	if writeErr := WriteState(state); writeErr != nil {
		return &SyncResult{
			Action:         "fallback_synced",
			Official:       official,
			Updated:        official,
			Added:          official,
			SkippedDeleted: []string{},
			Detail:         reason + "\nstate write failed: " + writeErr.Error(),
			Force:          opts.Force,
			Layout:         LayoutSeparate,
		}
	}

	return &SyncResult{
		Action:         "fallback_synced",
		Official:       official,
		Updated:        official,
		Added:          official,
		SkippedDeleted: []string{},
		Detail:         reason,
		Force:          opts.Force,
		Layout:         LayoutSeparate,
	}
}

func stateFlatSkills(layout string, requested []string) []string {
	if layout != LayoutHybrid {
		return []string{}
	}
	out := []string{}
	for _, skill := range uniqueSorted(requested) {
		if skill != sharedSkillName {
			out = append(out, skill)
		}
	}
	return out
}

func newlyOfficialSkills(official []string, previous *SkillsState, stateReadable bool) []string {
	previousOfficial := []string{}
	if stateReadable && previous != nil {
		previousOfficial = previous.OfficialSkills
	}
	previousSet := toSet(previousOfficial)
	added := []string{}
	for _, skill := range official {
		if !previousSet[skill] {
			added = append(added, skill)
		}
	}
	return uniqueSorted(added)
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
