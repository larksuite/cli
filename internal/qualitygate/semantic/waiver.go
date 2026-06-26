// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"bufio"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/vfs"
)

const waiverPath = "internal/qualitygate/config/semantic/waivers.txt"

type Waivers struct {
	Items []Waiver
}

type Waiver struct {
	ID          string
	Category    string
	FactKind    string
	SourceFile  string
	Line        int
	CommandPath string
	Owner       string
	Reason      string
	AddedAt     time.Time
	ExpiresAt   time.Time
}

func LoadWaivers(repo string, now time.Time) (Waivers, []report.Diagnostic, error) {
	data, err := vfs.ReadFile(filepath.Join(repo, filepath.FromSlash(waiverPath)))
	if err != nil {
		if missingFile(err) {
			return Waivers{}, nil, nil
		}
		return Waivers{}, nil, err
	}
	return ParseWaivers(strings.NewReader(string(data)), now)
}

func LoadWaiversFile(file string, now time.Time) (Waivers, []report.Diagnostic, error) {
	data, err := vfs.ReadFile(file)
	if err != nil {
		return Waivers{}, nil, err
	}
	return ParseWaivers(strings.NewReader(string(data)), now)
}

func ParseWaivers(r *strings.Reader, now time.Time) (Waivers, []report.Diagnostic, error) {
	scanner := bufio.NewScanner(r)
	var waivers Waivers
	var diags []report.Diagnostic
	for lineNo := 1; scanner.Scan(); lineNo++ {
		text := strings.TrimRight(scanner.Text(), "\r")
		if skipTSVLine(text) {
			continue
		}
		parts := strings.Split(text, "\t")
		if len(parts) != 10 {
			return Waivers{}, diags, fmt.Errorf("%s:%d: expected 10 TSV columns", waiverPath, lineNo)
		}
		item, err := parseWaiver(parts, lineNo)
		if err != nil {
			return Waivers{}, diags, err
		}
		if waiverExpired(item.ExpiresAt, now) {
			diags = append(diags, report.Diagnostic{
				Rule:    "semantic_waiver_expired",
				Action:  report.ActionWarning,
				File:    waiverPath,
				Line:    lineNo,
				Message: fmt.Sprintf("semantic waiver %s expired on %s", item.ID, item.ExpiresAt.Format(time.DateOnly)),
			})
			continue
		}
		waivers.Items = append(waivers.Items, item)
	}
	if err := scanner.Err(); err != nil {
		return Waivers{}, diags, err
	}
	return waivers, diags, nil
}

func waiverExpired(expiresAt, now time.Time) bool {
	expiryDate := time.Date(expiresAt.Year(), expiresAt.Month(), expiresAt.Day(), 0, 0, 0, 0, time.UTC)
	currentDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return currentDate.After(expiryDate)
}

func parseWaiver(parts []string, lineNo int) (Waiver, error) {
	if !rolloutIDPattern.MatchString(parts[0]) {
		return Waiver{}, fmt.Errorf("%s:%d: invalid waiver_id", waiverPath, lineNo)
	}
	if !allowedCategory(parts[1]) {
		return Waiver{}, fmt.Errorf("%s:%d: invalid category", waiverPath, lineNo)
	}
	if !allowedFactKind(parts[2]) {
		return Waiver{}, fmt.Errorf("%s:%d: invalid fact_kind", waiverPath, lineNo)
	}
	sourceFile, err := normalizeRepoPath(parts[3])
	if err != nil {
		return Waiver{}, fmt.Errorf("%s:%d: invalid source_file: %w", waiverPath, lineNo, err)
	}
	line, err := parseOptionalPositiveInt(parts[4])
	if err != nil {
		return Waiver{}, fmt.Errorf("%s:%d: invalid line", waiverPath, lineNo)
	}
	item := Waiver{
		ID:          parts[0],
		Category:    parts[1],
		FactKind:    parts[2],
		SourceFile:  sourceFile,
		Line:        line,
		CommandPath: strings.TrimSpace(parts[5]),
		Owner:       strings.TrimSpace(parts[6]),
		Reason:      strings.TrimSpace(parts[7]),
	}
	addedAt, addErr := time.Parse(time.DateOnly, parts[8])
	expiresAt, expErr := time.Parse(time.DateOnly, parts[9])
	if addErr != nil || expErr != nil {
		return Waiver{}, fmt.Errorf("%s:%d: invalid date", waiverPath, lineNo)
	}
	item.AddedAt = addedAt
	item.ExpiresAt = expiresAt
	if item.Owner == "" || item.Reason == "" {
		return Waiver{}, fmt.Errorf("%s:%d: owner and reason are required", waiverPath, lineNo)
	}
	switch item.FactKind {
	case "skill", "error":
		if item.SourceFile == "" || item.Line == 0 {
			return Waiver{}, fmt.Errorf("%s:%d: %s waiver requires source_file and line", waiverPath, lineNo, item.FactKind)
		}
	case "public_content":
		if item.SourceFile == "" || item.Line == 0 || item.CommandPath != "" {
			return Waiver{}, fmt.Errorf("%s:%d: public_content waiver requires source_file and line only", waiverPath, lineNo)
		}
	case "command", "output":
		if item.CommandPath == "" {
			return Waiver{}, fmt.Errorf("%s:%d: %s waiver requires command_path", waiverPath, lineNo, item.FactKind)
		}
	}
	if item.SourceFile == "" && item.CommandPath == "" {
		return Waiver{}, fmt.Errorf("%s:%d: waiver requires a selector", waiverPath, lineNo)
	}
	return item, nil
}

func normalizeRepoPath(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if strings.Contains(raw, "\\") || strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("path must be repo-relative POSIX")
	}
	clean := path.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path escapes repository")
	}
	return clean, nil
}

func parseOptionalPositiveInt(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("line must be positive")
	}
	return value, nil
}

func skipTSVLine(text string) bool {
	trimmed := strings.TrimSpace(text)
	return trimmed == "" || strings.HasPrefix(trimmed, "#")
}
