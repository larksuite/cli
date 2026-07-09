// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/larksuite/cli/internal/qualitygate/facts"
	"github.com/larksuite/cli/internal/vfs"
)

var (
	ErrReviewerUnavailable   = errors.New("semantic reviewer is not configured")
	ErrReviewerConfiguration = errors.New("semantic reviewer configuration is invalid")
)

func LoadOrReviewWithConfig(ctx context.Context, f facts.Facts, reviewPath string, cfg ModelConfig) (Review, error) {
	if reviewPath == "" {
		client, ok, err := FromEnvWithConfig(cfg)
		if err != nil {
			return Review{}, err
		}
		if !ok {
			return Review{}, ErrReviewerUnavailable
		}
		return client.Review(ctx, f)
	}
	data, err := vfs.ReadFile(reviewPath)
	if err != nil {
		return Review{}, err
	}
	return DecodeReview(strings.NewReader(string(data)))
}

func SkippedDecision(err error) Decision {
	return Decision{
		Skipped: true,
		SystemWarnings: []SystemWarning{{
			Severity:        "minor",
			Message:         fmt.Sprintf("semantic review skipped: %v", err),
			SuggestedAction: "configure semantic review credentials and model when enabling model-based review",
		}},
	}
}

func DegradedDecision(err error) Decision {
	return Decision{
		Degraded: true,
		SystemWarnings: []SystemWarning{{
			Severity:        "minor",
			Message:         fmt.Sprintf("semantic review degraded: %v", err),
			SuggestedAction: "inspect deterministic quality-gate diagnostics",
		}},
	}
}

func InfrastructureFailureDecision(err error) Decision {
	return Decision{
		Degraded:              true,
		InfrastructureFailure: true,
		SystemWarnings: []SystemWarning{{
			Severity:        "critical",
			Message:         fmt.Sprintf("semantic review infrastructure failure: %v", err),
			SuggestedAction: "inspect semantic-review workflow logs and quality-gate configuration",
		}},
	}
}

func WriteDecision(path string, decision Decision) error {
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(decision, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return vfs.WriteFile(path, data, 0o644)
}

func WriteMarkdown(path string, decision Decision) error {
	if path == "" {
		return nil
	}
	body := Markdown(decision)
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return vfs.WriteFile(path, []byte(body), 0o644)
}

func Markdown(decision Decision) string {
	var b strings.Builder
	b.WriteString("## Semantic Review\n\n")
	if decision.Skipped {
		b.WriteString("Semantic review skipped; deterministic quality-gate results remain authoritative.\n\n")
	}
	if decision.Degraded {
		b.WriteString("Semantic review degraded; deterministic quality-gate results remain authoritative.\n\n")
	}
	if len(decision.Blockers) == 0 {
		b.WriteString("No semantic blockers.\n\n")
	} else {
		b.WriteString("### Blockers\n\n")
		for _, finding := range decision.Blockers {
			b.WriteString("- ")
			b.WriteString(markdownFindingText(finding.Message))
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	if len(decision.Warnings) > 0 {
		b.WriteString("### Warnings\n\n")
		for _, finding := range decision.Warnings {
			b.WriteString("- ")
			b.WriteString(markdownFindingText(finding.Message))
			b.WriteByte('\n')
		}
	}
	if len(decision.SystemWarnings) > 0 {
		b.WriteString("\n### System Warnings\n\n")
		for _, warning := range decision.SystemWarnings {
			b.WriteString("- ")
			b.WriteString(markdownFindingText(warning.Message))
			if warning.SuggestedAction != "" {
				b.WriteString(" ")
				b.WriteString(markdownFindingText(warning.SuggestedAction))
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func markdownFindingText(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteByte(' ')
		case unicode.IsControl(r):
			continue
		case r == '@':
			b.WriteString("@\u200b")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case strings.ContainsRune("\\`*_{}[]()#+-|!", r):
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	text := strings.Join(strings.Fields(b.String()), " ")
	text = strings.ReplaceAll(text, "https://", "https[:]//")
	text = strings.ReplaceAll(text, "http://", "http[:]//")
	return text
}
