// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package allowlist

import (
	"bufio"
	"io"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/qualitygate/report"
)

type LegacyCommand struct {
	Command string
	Owner   string
	Reason  string
	AddedAt time.Time
}

type LegacyFlag struct {
	Command string
	Flag    string
	Owner   string
	Reason  string
	AddedAt time.Time
}

func ParseLegacyCommands(r io.Reader) ([]LegacyCommand, []report.Diagnostic) {
	scanner := bufio.NewScanner(r)
	var items []LegacyCommand
	var diags []report.Diagnostic
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimRight(scanner.Text(), "\r")
		if skipAllowlistLine(text) {
			continue
		}
		parts := strings.Split(text, "\t")
		if len(parts) != 4 {
			diags = append(diags, malformedAllowlist("legacy_commands", line))
			continue
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		added, addErr := time.Parse(time.DateOnly, parts[3])
		if blank(parts[0], parts[1], parts[2]) || addErr != nil {
			diags = append(diags, malformedAllowlist("legacy_commands", line))
			continue
		}
		item := LegacyCommand{
			Command: parts[0],
			Owner:   parts[1],
			Reason:  parts[2],
			AddedAt: added,
		}
		items = append(items, item)
	}
	if err := scanner.Err(); err != nil {
		diags = append(diags, report.Diagnostic{
			Rule:    "allowlist_format",
			Action:  report.ActionReject,
			File:    "legacy_allowlist",
			Message: "failed to scan allowlist: " + err.Error(),
		})
	}
	return items, diags
}

func ParseLegacyFlags(r io.Reader) ([]LegacyFlag, []report.Diagnostic) {
	scanner := bufio.NewScanner(r)
	var items []LegacyFlag
	var diags []report.Diagnostic
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimRight(scanner.Text(), "\r")
		if skipAllowlistLine(text) {
			continue
		}
		parts := strings.Split(text, "\t")
		if len(parts) != 5 {
			diags = append(diags, malformedAllowlist("legacy_flags", line))
			continue
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		added, addErr := time.Parse(time.DateOnly, parts[4])
		if blank(parts[0], parts[1], parts[2], parts[3]) || addErr != nil {
			diags = append(diags, malformedAllowlist("legacy_flags", line))
			continue
		}
		item := LegacyFlag{
			Command: parts[0],
			Flag:    parts[1],
			Owner:   parts[2],
			Reason:  parts[3],
			AddedAt: added,
		}
		items = append(items, item)
	}
	if err := scanner.Err(); err != nil {
		diags = append(diags, report.Diagnostic{
			Rule:    "allowlist_format",
			Action:  report.ActionReject,
			File:    "legacy_allowlist",
			Message: "failed to scan allowlist: " + err.Error(),
		})
	}
	return items, diags
}

func skipAllowlistLine(text string) bool {
	trimmed := strings.TrimSpace(text)
	return trimmed == "" || strings.HasPrefix(trimmed, "#")
}

func blank(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}

func malformedAllowlist(kind string, line int) report.Diagnostic {
	return report.Diagnostic{
		Rule:       "allowlist_format",
		Action:     report.ActionReject,
		File:       kind,
		Line:       line,
		Message:    "legacy allowlist row must include owner, reason, and added_at",
		Suggestion: "use tab-separated fields with dates in YYYY-MM-DD format",
	}
}
