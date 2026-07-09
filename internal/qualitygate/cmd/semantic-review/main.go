// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/larksuite/cli/internal/qualitygate/facts"
	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/qualitygate/semantic"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("semantic-review", flag.ContinueOnError)
	var repo, factsPath, reviewPath, waiversPath, decisionOut, markdownOut string
	var block bool
	fs.StringVar(&repo, "repo", ".", "repository root")
	fs.StringVar(&factsPath, "facts", "", "facts.json path")
	fs.StringVar(&reviewPath, "review-json", "", "optional precomputed review JSON")
	fs.StringVar(&waiversPath, "waivers-file", "", "optional semantic waiver TSV file")
	fs.StringVar(&decisionOut, "decision-out", "", "optional decision JSON output path")
	fs.StringVar(&markdownOut, "markdown-out", "", "optional markdown output path")
	fs.BoolVar(&block, "block", false, "exit 1 when gatekeeper finds blockers")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if factsPath == "" && fs.NArg() == 1 {
		factsPath = fs.Arg(0)
	}
	if factsPath == "" {
		fmt.Fprintln(os.Stderr, "semantic-review: --facts is required")
		return 2
	}

	f, err := facts.ReadFile(factsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "semantic-review: %v\n", err)
		return 2
	}
	policy, waivers, waiverDiags, modelConfig, err := loadSemanticConfig(repo, waiversPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "semantic-review: %v\n", err)
		decision := semantic.InfrastructureFailureDecision(err)
		decision.BlockMode = block
		_ = semantic.WriteDecision(decisionOut, decision)
		_ = semantic.WriteMarkdown(markdownOut, decision)
		return 0
	}
	if reviewPath == "" && !semantic.BuildInputView(f).HasReviewableFacts() {
		decision := finalizeDecision(block, waiverDiags, semantic.Decision{})
		if err := writeSemanticOutputs(decisionOut, markdownOut, decision); err != nil {
			fmt.Fprintf(os.Stderr, "semantic-review: %v\n", err)
			return 2
		}
		return decisionExitCode(decision)
	}
	review, err := semantic.LoadOrReviewWithConfig(context.Background(), f, reviewPath, modelConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "semantic-review: %v\n", err)
		decision := semantic.DegradedDecision(err)
		switch {
		case errors.Is(err, semantic.ErrReviewerUnavailable):
			decision = semantic.SkippedDecision(err)
		case errors.Is(err, semantic.ErrReviewerConfiguration):
			decision = semantic.InfrastructureFailureDecision(err)
		}
		decision.BlockMode = block
		_ = semantic.WriteDecision(decisionOut, decision)
		_ = semantic.WriteMarkdown(markdownOut, decision)
		return 0
	}
	decision := semantic.DecideWithWaivers(f, review, policy, waivers)
	decision = finalizeDecision(block, waiverDiags, decision)
	if err := writeSemanticOutputs(decisionOut, markdownOut, decision); err != nil {
		fmt.Fprintf(os.Stderr, "semantic-review: %v\n", err)
		return 2
	}
	return decisionExitCode(decision)
}

func finalizeDecision(block bool, waiverDiags []report.Diagnostic, decision semantic.Decision) semantic.Decision {
	decision.BlockMode = block
	if !block && len(decision.Blockers) > 0 {
		for i := range decision.Blockers {
			decision.Blockers[i].ReviewAction = semantic.ReviewActionObserve
		}
		decision.Warnings = append(decision.Warnings, decision.Blockers...)
		decision.Blockers = nil
	}
	decision.SystemWarnings = append(diagnosticSystemWarnings(waiverDiags), decision.SystemWarnings...)
	return decision
}

func writeSemanticOutputs(decisionOut, markdownOut string, decision semantic.Decision) error {
	if err := semantic.WriteDecision(decisionOut, decision); err != nil {
		return fmt.Errorf("write decision: %w", err)
	}
	if err := semantic.WriteMarkdown(markdownOut, decision); err != nil {
		return fmt.Errorf("write markdown: %w", err)
	}
	return nil
}

func decisionExitCode(decision semantic.Decision) int {
	if decision.BlockMode && len(decision.Blockers) > 0 {
		return 1
	}
	return 0
}

func loadSemanticConfig(repo, waiversPath string) (semantic.Policy, semantic.Waivers, []report.Diagnostic, semantic.ModelConfig, error) {
	policy, err := semantic.LoadPolicy(repo)
	if err != nil {
		return semantic.Policy{}, semantic.Waivers{}, nil, semantic.ModelConfig{}, fmt.Errorf("load policy: %w", err)
	}
	var (
		waivers     semantic.Waivers
		waiverDiags []report.Diagnostic
	)
	if waiversPath != "" {
		waivers, waiverDiags, err = semantic.LoadWaiversFile(waiversPath, now())
	} else {
		waivers, waiverDiags, err = semantic.LoadWaivers(repo, now())
	}
	if err != nil {
		return semantic.Policy{}, semantic.Waivers{}, nil, semantic.ModelConfig{}, fmt.Errorf("load waivers: %w", err)
	}
	modelConfig, err := semantic.LoadModelConfig(repo)
	if err != nil {
		return semantic.Policy{}, semantic.Waivers{}, nil, semantic.ModelConfig{}, fmt.Errorf("load model config: %w", err)
	}
	return policy, waivers, waiverDiags, modelConfig, nil
}

var now = func() time.Time {
	return time.Now()
}

func diagnosticSystemWarnings(diags []report.Diagnostic) []semantic.SystemWarning {
	if len(diags) == 0 {
		return nil
	}
	out := make([]semantic.SystemWarning, 0, len(diags))
	for _, diag := range diags {
		out = append(out, semantic.SystemWarning{
			Severity:        "minor",
			Message:         diag.Message,
			SuggestedAction: diag.Suggestion,
		})
	}
	return out
}
