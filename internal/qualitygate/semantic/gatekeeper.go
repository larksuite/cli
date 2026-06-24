// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/qualitygate/facts"
	"github.com/larksuite/cli/internal/qualitygate/report"
)

var evidencePattern = regexp.MustCompile(`^facts\.(commands|skills|errors|outputs|public_content)\[(\d+)\]$`)

func Decide(f facts.Facts, r Review, p Policy) Decision {
	return DecideWithWaivers(f, r, p, Waivers{})
}

func DecideWithWaivers(f facts.Facts, r Review, p Policy, waivers Waivers) Decision {
	var d Decision
	for _, finding := range r.Findings {
		if !validFinding(finding) {
			addDecisionFinding(&d, decisionFinding(f, finding, ReviewActionObserve))
			continue
		}
		if !categoryBlocks(p, finding.Category) || !reproducible(f, finding) {
			addDecisionFinding(&d, decisionFinding(f, finding, ReviewActionObserve))
			continue
		}
		scopes, ok := scopesForFinding(f, finding)
		if !ok {
			addDecisionFinding(&d, decisionFinding(f, finding, ReviewActionObserve))
			continue
		}
		groups := matchingRolloutGroups(p, finding, scopes)
		if len(groups) == 0 {
			addDecisionFinding(&d, decisionFinding(f, finding, ReviewActionObserve))
			continue
		}
		finding.RolloutGroups = groups
		if waiverID, waiverKeys, ok := waivers.MatchFinding(finding.Category, scopes); ok {
			finding.WaiverID = waiverID
			finding.WaiverKeys = waiverKeys
			addDecisionFinding(&d, decisionFinding(f, finding, ReviewActionConfirm))
			continue
		}
		addDecisionFinding(&d, decisionFinding(f, finding, ReviewActionMustFix))
	}
	return d
}

func addDecisionFinding(d *Decision, finding Finding) {
	if finding.Fingerprint == "" {
		return
	}
	switch finding.ReviewAction {
	case ReviewActionMustFix:
		if containsFindingFingerprint(d.Blockers, finding.Fingerprint) {
			return
		}
		d.Warnings = removeFindingFingerprint(d.Warnings, finding.Fingerprint)
		d.Blockers = append(d.Blockers, finding)
	case ReviewActionConfirm:
		if containsFindingFingerprint(d.Blockers, finding.Fingerprint) {
			return
		}
		if replaceWarningFinding(d.Warnings, finding, ReviewActionObserve) {
			return
		}
		if containsFindingFingerprint(d.Warnings, finding.Fingerprint) {
			return
		}
		d.Warnings = append(d.Warnings, finding)
	default:
		if containsFindingFingerprint(d.Blockers, finding.Fingerprint) || containsFindingFingerprint(d.Warnings, finding.Fingerprint) {
			return
		}
		d.Warnings = append(d.Warnings, finding)
	}
}

func containsFindingFingerprint(findings []Finding, fingerprint string) bool {
	for _, finding := range findings {
		if finding.Fingerprint == fingerprint {
			return true
		}
	}
	return false
}

func removeFindingFingerprint(findings []Finding, fingerprint string) []Finding {
	out := findings[:0]
	for _, finding := range findings {
		if finding.Fingerprint != fingerprint {
			out = append(out, finding)
		}
	}
	return out
}

func replaceWarningFinding(findings []Finding, replacement Finding, replaceAction string) bool {
	for i, finding := range findings {
		if finding.Fingerprint == replacement.Fingerprint && finding.ReviewAction == replaceAction {
			findings[i] = replacement
			return true
		}
	}
	return false
}

func decisionFinding(f facts.Facts, finding Finding, action string) Finding {
	finding.ReviewAction = action
	finding.Fingerprint = findingFingerprint(f, finding)
	return finding
}

func findingFingerprint(f facts.Facts, finding Finding) string {
	parts := make([]string, 0, len(finding.Evidence)+1)
	parts = append(parts, "category:"+finding.Category)
	evidence := make([]string, 0, len(finding.Evidence))
	for _, ev := range finding.Evidence {
		evidence = append(evidence, evidenceFingerprint(f, ev))
	}
	sort.Strings(evidence)
	parts = append(parts, evidence...)
	return strings.Join(parts, "|")
}

func evidenceFingerprint(f facts.Facts, ev string) string {
	kind, idx, ok := parseEvidence(ev)
	if !ok || !evidenceExists(f, kind, idx) {
		return "ref:" + ev
	}
	switch kind {
	case "commands":
		cmd := f.Commands[idx]
		return strings.Join([]string{
			"commands",
			"path:" + cmd.Path,
			"name_conflicts_existing:" + strconv.FormatBool(cmd.NameConflictsExisting),
			"flag_alias_conflict:" + strconv.FormatBool(cmd.FlagAliasConflict),
		}, ":")
	case "skills":
		skill := f.Skills[idx]
		return strings.Join([]string{
			"skills",
			"source_file:" + skill.SourceFile,
			"line:" + strconv.Itoa(skill.Line),
			"command_path:" + skill.CommandPath,
			"references_invalid_command:" + strconv.FormatBool(skill.ReferencesInvalidCommand),
		}, ":")
	case "errors":
		errFact := f.Errors[idx]
		return strings.Join([]string{
			"errors",
			"file:" + errFact.File,
			"line:" + strconv.Itoa(errFact.Line),
			"command_path:" + errFact.CommandPath,
			"code:" + errFact.Code,
			"boundary:" + strconv.FormatBool(errFact.Boundary),
			"required_hint:" + strconv.FormatBool(errFact.RequiredHint),
			"hint_action_count:" + strconv.Itoa(errFact.HintActionCount),
		}, ":")
	case "outputs":
		out := f.Outputs[idx]
		return strings.Join([]string{
			"outputs",
			"command:" + out.Command,
			"is_list:" + strconv.FormatBool(out.IsList),
			"has_default_limit:" + strconv.FormatBool(out.HasDefaultLimit),
			"has_decision_field:" + strconv.FormatBool(out.HasDecisionField),
		}, ":")
	case "public_content":
		item := f.PublicContent[idx]
		return strings.Join([]string{
			"public_content",
			"rule:" + item.Rule,
			"action:" + string(item.Action),
			"file:" + item.File,
			"line:" + strconv.Itoa(item.Line),
			"source:" + item.Source,
		}, ":")
	default:
		return "ref:" + ev
	}
}

func categoryBlocks(p Policy, category string) bool {
	return containsString(p.BlockCategories, category)
}

func validFinding(f Finding) bool {
	if !allowedCategory(f.Category) {
		return false
	}
	if strings.TrimSpace(f.Severity) == "" ||
		strings.TrimSpace(f.Message) == "" ||
		strings.TrimSpace(f.SuggestedAction) == "" {
		return false
	}
	if len(f.Message) > 500 || len(f.SuggestedAction) > 500 {
		return false
	}
	if len(f.Evidence) == 0 || len(f.Evidence) > 20 {
		return false
	}
	return true
}

func allowedCategory(category string) bool {
	switch category {
	case "error_hint", "default_output", "naming", "skill_quality", "public_content_leakage":
		return true
	default:
		return false
	}
}

func reproducible(f facts.Facts, finding Finding) bool {
	for _, ev := range finding.Evidence {
		kind, idx, ok := parseEvidence(ev)
		if !ok || !evidenceExists(f, kind, idx) {
			return false
		}
		if !reproducibleEvidence(f, finding.Category, kind, idx) {
			return false
		}
	}
	return true
}

func reproducibleEvidence(f facts.Facts, category, kind string, idx int) bool {
	switch category {
	case "error_hint":
		if kind != "errors" {
			return false
		}
		errFact := f.Errors[idx]
		return errFact.Boundary && errFact.RequiredHint && errFact.HintActionCount == 0
	case "default_output":
		if kind != "outputs" {
			return false
		}
		out := f.Outputs[idx]
		return out.IsList && (!out.HasDefaultLimit || !out.HasDecisionField)
	case "naming":
		if kind != "commands" {
			return false
		}
		cmd := f.Commands[idx]
		return cmd.NameConflictsExisting || cmd.FlagAliasConflict
	case "skill_quality":
		if kind != "skills" {
			return false
		}
		skill := f.Skills[idx]
		return skill.ReferencesInvalidCommand
	case "public_content_leakage":
		if kind != "public_content" {
			return false
		}
		item := f.PublicContent[idx]
		return item.Action == report.ActionReject || item.Rule == "public_content_semantic_candidate"
	default:
		return false
	}
}

func parseEvidence(ev string) (string, int, bool) {
	matches := evidencePattern.FindStringSubmatch(ev)
	if len(matches) != 3 {
		return "", 0, false
	}
	idx, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", 0, false
	}
	return matches[1], idx, true
}

func evidenceExists(f facts.Facts, kind string, idx int) bool {
	if idx < 0 {
		return false
	}
	switch kind {
	case "commands":
		return idx < len(f.Commands)
	case "skills":
		return idx < len(f.Skills)
	case "errors":
		return idx < len(f.Errors)
	case "outputs":
		return idx < len(f.Outputs)
	case "public_content":
		return idx < len(f.PublicContent)
	default:
		return false
	}
}
