// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/qualitygate/publiccontent"
	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

type eventPayload struct {
	Comment *struct {
		Body string `json:"body"`
	} `json:"comment"`
	Review *struct {
		Body string `json:"body"`
	} `json:"review"`
}

func main() {
	eventPath := flag.String("event", os.Getenv("GITHUB_EVENT_PATH"), "GitHub event payload path")
	kind := flag.String("kind", os.Getenv("GITHUB_EVENT_NAME"), "GitHub event kind")
	flag.Parse()

	if *eventPath == "" {
		fmt.Fprintln(os.Stderr, "comment-audit: --event or GITHUB_EVENT_PATH is required")
		os.Exit(2)
	}
	body, err := commentBody(*eventPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "comment-audit: %v\n", err)
		os.Exit(2)
	}
	diags := diagnostics(publiccontent.ScanComment(*kind, body))
	if len(diags) > 0 {
		fmt.Fprintln(os.Stderr, auditFailureSummary(len(diags)))
	}
	report.Print(os.Stderr, diags)
	os.Exit(report.ExitCode(diags))
}

func auditFailureSummary(count int) string {
	return fmt.Sprintf("post-publication audit found public content findings: %d", count)
}

func commentBody(path string) (string, error) {
	safePath, err := validate.SafeInputPath(path)
	if err != nil {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --event: %v", err).
			WithParam("--event").
			WithCause(err)
	}
	data, err := vfs.ReadFile(safePath)
	if err != nil {
		return "", err
	}
	var payload eventPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", err
	}
	switch {
	case payload.Comment != nil:
		return payload.Comment.Body, nil
	case payload.Review != nil:
		return payload.Review.Body, nil
	default:
		return "", nil
	}
}

func diagnostics(items []publiccontent.Finding) []report.Diagnostic {
	out := make([]report.Diagnostic, 0, len(items))
	for _, item := range items {
		out = append(out, report.Diagnostic{
			Rule:       item.Rule,
			Action:     item.Action,
			File:       item.File,
			Line:       item.Line,
			Message:    item.Message,
			Suggestion: item.Suggestion,
		})
	}
	return out
}
