// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"encoding/json"
	"fmt"
	"io"
)

const maxModelResponseBytes = 1 << 20

type Review struct {
	Verdict  string    `json:"verdict"`
	Findings []Finding `json:"findings"`
}

type Finding struct {
	Category        string   `json:"category"`
	Severity        string   `json:"severity"`
	Evidence        []string `json:"evidence"`
	Message         string   `json:"message"`
	SuggestedAction string   `json:"suggested_action"`
	ReviewAction    string   `json:"review_action,omitempty"`
	Fingerprint     string   `json:"fingerprint,omitempty"`
	RolloutGroups   []string `json:"rollout_groups,omitempty"`
	WaiverID        string   `json:"waiver_id,omitempty"`
	WaiverKeys      []string `json:"waiver_keys,omitempty"`
}

const (
	ReviewActionMustFix = "must_fix"
	ReviewActionConfirm = "confirm"
	ReviewActionObserve = "observe"
)

type SystemWarning struct {
	Severity        string `json:"severity"`
	Message         string `json:"message"`
	SuggestedAction string `json:"suggested_action,omitempty"`
}

type Policy struct {
	SchemaVersion      int            `json:"schema_version"`
	DefaultEnforcement string         `json:"default_enforcement"`
	BlockCategories    []string       `json:"block_categories"`
	RolloutGroups      []RolloutGroup `json:"rollout_groups,omitempty"`
}

type RolloutGroup struct {
	ID          string        `json:"id"`
	Enforcement string        `json:"enforcement"`
	Scope       ScopeSelector `json:"scope,omitempty"`
	Categories  []string      `json:"categories"`
	Owner       string        `json:"owner"`
	Reason      string        `json:"reason"`
}

type ScopeSelector struct {
	ChangedOnly bool     `json:"changed_only,omitempty"`
	Domains     []string `json:"domains,omitempty"`
	FactKinds   []string `json:"fact_kinds,omitempty"`
	Sources     []string `json:"sources,omitempty"`
}

type Decision struct {
	BlockMode             bool            `json:"block_mode"`
	Skipped               bool            `json:"skipped,omitempty"`
	Degraded              bool            `json:"degraded,omitempty"`
	InfrastructureFailure bool            `json:"infrastructure_failure,omitempty"`
	Blockers              []Finding       `json:"blockers,omitempty"`
	Warnings              []Finding       `json:"warnings,omitempty"`
	SystemWarnings        []SystemWarning `json:"system_warnings,omitempty"`
}

func DefaultPolicy() Policy {
	return Policy{
		SchemaVersion:      1,
		DefaultEnforcement: "observe",
		BlockCategories:    []string{"error_hint", "default_output", "naming", "skill_quality", "public_content_leakage"},
		RolloutGroups: []RolloutGroup{{
			ID:          "all",
			Enforcement: "blocking",
			Categories:  []string{"error_hint", "default_output", "naming", "skill_quality", "public_content_leakage"},
			Owner:       "test",
			Reason:      "default in-memory policy",
		}},
	}
}

func DecodeReview(r io.Reader) (Review, error) {
	return decodeReview(r, true)
}

// Model responses are normalized through modelReview for compatibility; the
// local gatekeeper still recomputes blocking evidence from facts.
func DecodeModelReview(r io.Reader) (Review, error) {
	dec := json.NewDecoder(io.LimitReader(r, maxModelResponseBytes))
	var raw modelReview
	if err := dec.Decode(&raw); err != nil {
		return Review{}, err
	}
	review := Review{
		Verdict:  raw.Verdict,
		Findings: make([]Finding, 0, len(raw.Findings)),
	}
	for _, finding := range raw.Findings {
		review.Findings = append(review.Findings, Finding{
			Category:        finding.Category,
			Severity:        finding.Severity,
			Evidence:        []string(finding.Evidence),
			Message:         finding.Message,
			SuggestedAction: finding.SuggestedAction,
			RolloutGroups:   finding.RolloutGroups,
			WaiverID:        finding.WaiverID,
			WaiverKeys:      finding.WaiverKeys,
		})
	}
	if err := validateReview(review); err != nil {
		return Review{}, err
	}
	return review, nil
}

func decodeReview(r io.Reader, strict bool) (Review, error) {
	dec := json.NewDecoder(io.LimitReader(r, maxModelResponseBytes))
	if strict {
		dec.DisallowUnknownFields()
	}
	var review Review
	if err := dec.Decode(&review); err != nil {
		return Review{}, err
	}
	if err := validateReview(review); err != nil {
		return Review{}, err
	}
	return review, nil
}

type modelReview struct {
	Verdict  string         `json:"verdict"`
	Findings []modelFinding `json:"findings"`
}

type modelFinding struct {
	Category        string        `json:"category"`
	Severity        string        `json:"severity"`
	Evidence        modelEvidence `json:"evidence"`
	Message         string        `json:"message"`
	SuggestedAction string        `json:"suggested_action"`
	RolloutGroups   []string      `json:"rollout_groups,omitempty"`
	WaiverID        string        `json:"waiver_id,omitempty"`
	WaiverKeys      []string      `json:"waiver_keys,omitempty"`
}

type modelEvidence []string

func (e *modelEvidence) UnmarshalJSON(data []byte) error {
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*e = list
		return nil
	}
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*e = []string{one}
		return nil
	}
	return fmt.Errorf("evidence must be a string or string array")
}

func validateReview(review Review) error {
	if len(review.Findings) > 20 {
		return fmt.Errorf("too many findings: %d", len(review.Findings))
	}
	for _, finding := range review.Findings {
		if len(finding.Message) > 500 || len(finding.SuggestedAction) > 500 {
			return fmt.Errorf("finding text exceeds limit")
		}
	}
	return nil
}
