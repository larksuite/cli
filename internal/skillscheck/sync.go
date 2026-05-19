// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/selfupdate"
)

var skillNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_:-]*(@[^\s]+)?$`)

type SyncInput struct {
	Version        string
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

func ParseSkillsList(text string) []string {
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		token := strings.TrimSpace(line)
		token = strings.TrimPrefix(token, "-")
		token = strings.TrimSpace(token)
		if token == "" || strings.Contains(token, " ") || strings.HasSuffix(token, ":") {
			continue
		}
		if !skillNamePattern.MatchString(token) {
			continue
		}
		if at := strings.Index(token, "@"); at > 0 {
			token = token[:at]
		}
		seen[token] = true
	}
	return sortedKeys(seen)
}

func PlanSync(input SyncInput) SyncPlan {
	official := uniqueSorted(input.OfficialSkills)
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
	localOfficial := intersection(input.LocalSkills, officialSet)

	previousOfficial := []string{}
	if input.StateReadable && input.PreviousState != nil {
		previousOfficial = input.PreviousState.OfficialSkills
	}
	previousSet := toSet(previousOfficial)

	newOfficial := []string{}
	for _, skill := range official {
		if !previousSet[skill] {
			newOfficial = append(newOfficial, skill)
		}
	}

	updateSet := toSet(localOfficial)
	for _, skill := range newOfficial {
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
		Version:        input.Version,
		OfficialSkills: official,
		ToUpdate:       toUpdate,
		Added:          uniqueSorted(newOfficial),
		SkippedDeleted: skipped,
	}
}

type SkillsRunner interface {
	ListOfficialSkills() *selfupdate.NpmResult
	ListGlobalSkills() *selfupdate.NpmResult
	InstallSkill(name string) *selfupdate.NpmResult
}

type SyncOptions struct {
	Version string
	Force   bool
	Runner  SkillsRunner
	Now     func() time.Time
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
}

func SyncSkills(opts SyncOptions) *SyncResult {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Runner == nil {
		return &SyncResult{Action: "failed", Err: fmt.Errorf("skills runner is nil")}
	}

	officialResult := opts.Runner.ListOfficialSkills()
	if officialResult == nil {
		return &SyncResult{Action: "failed", Err: fmt.Errorf("failed to list official skills: empty result")}
	}
	if officialResult.Err != nil {
		return &SyncResult{Action: "failed", Err: fmt.Errorf("failed to list official skills: %w", officialResult.Err), Detail: resultDetail(officialResult)}
	}
	official := ParseSkillsList(officialResult.Stdout.String())

	localResult := opts.Runner.ListGlobalSkills()
	if localResult == nil {
		return &SyncResult{Action: "failed", Official: official, Err: fmt.Errorf("failed to list installed skills: empty result")}
	}
	if localResult.Err != nil {
		return &SyncResult{Action: "failed", Official: official, Err: fmt.Errorf("failed to list installed skills: %w", localResult.Err), Detail: resultDetail(localResult)}
	}
	local := ParseSkillsList(localResult.Stdout.String())

	previous, readable, err := ReadState()
	if err != nil {
		return &SyncResult{Action: "failed", Official: official, Err: fmt.Errorf("failed to read skills state: %w", err)}
	}

	plan := PlanSync(SyncInput{
		Version:        opts.Version,
		OfficialSkills: official,
		LocalSkills:    local,
		PreviousState:  previous,
		StateReadable:  readable,
		Force:          opts.Force,
	})

	result := &SyncResult{
		Action:         "synced",
		Official:       plan.OfficialSkills,
		Updated:        plan.ToUpdate,
		Added:          plan.Added,
		SkippedDeleted: plan.SkippedDeleted,
		Force:          opts.Force,
	}

	failed := []string{}
	var details []string
	for _, skill := range plan.ToUpdate {
		installResult := opts.Runner.InstallSkill(skill)
		if installResult == nil {
			failed = append(failed, skill)
			details = append(details, skill+": empty result")
			continue
		}
		if installResult.Err != nil {
			failed = append(failed, skill)
			details = append(details, skill+": "+resultDetail(installResult))
		}
	}
	if len(failed) > 0 {
		result.Action = "failed"
		result.Failed = failed
		result.Err = fmt.Errorf("%d skill(s) failed", len(failed))
		result.Detail = strings.Join(details, "\n")
		return result
	}

	state := SkillsState{
		Version:              opts.Version,
		OfficialSkills:       plan.OfficialSkills,
		UpdatedSkills:        plan.ToUpdate,
		AddedSkills:          plan.Added,
		SkippedDeletedSkills: plan.SkippedDeleted,
		UpdatedAt:            opts.Now().UTC().Format(time.RFC3339),
	}
	if err := WriteState(state); err != nil {
		result.Action = "failed"
		result.Err = fmt.Errorf("skills synced but state not written: %w", err)
		return result
	}

	return result
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
