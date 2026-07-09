// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscan

import (
	"errors"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/vfs"
)

type Example struct {
	Raw            string `json:"raw"`
	SourceFile     string `json:"source_file"`
	Line           int    `json:"line"`
	HasPlaceholder bool   `json:"has_placeholder"`
}

var (
	placeholderTokenPattern        = regexp.MustCompile(`\b[a-z]{2}_x+\b`)
	angleTokenPattern              = regexp.MustCompile(`<([^>\n]+)>`)
	lowerStructuredPlaceholderName = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)+$`)
	xmlTagNamePattern              = regexp.MustCompile(`^[a-z][a-z0-9:_-]*$`)
)

func Harvest(skillsDir string) ([]Example, error) {
	var out []Example
	if err := walkMarkdown(skillsDir, func(path string) error {
		examples, err := harvestFile(path)
		if err != nil {
			return err
		}
		out = append(out, examples...)
		return nil
	}); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SourceFile != out[j].SourceFile {
			return out[i].SourceFile < out[j].SourceFile
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

func walkMarkdown(root string, visit func(string) error) error {
	entries, err := vfs.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			if err := walkMarkdown(path, visit); err != nil {
				return err
			}
			continue
		}
		if entry.Type()&fs.ModeType != 0 || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if err := visit(path); err != nil {
			return err
		}
	}
	return nil
}

func harvestFile(path string) ([]Example, error) {
	data, err := vfs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	var out []Example
	inFence := false
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if !inFence || line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(line, "lark-cli ") {
			continue
		}

		startLine := i + 1
		raw := trimContinuation(line)
		for continues(line) && i+1 < len(lines) {
			i++
			line = strings.TrimSpace(lines[i])
			raw += " " + trimContinuation(line)
		}
		raw = strings.Join(strings.Fields(raw), " ")
		out = append(out, Example{
			Raw:            raw,
			SourceFile:     path,
			Line:           startLine,
			HasPlaceholder: HasPlaceholder(raw),
		})
	}
	return out, nil
}

func continues(line string) bool {
	return strings.HasSuffix(strings.TrimRight(line, " \t"), "\\")
}

func trimContinuation(line string) string {
	line = strings.TrimRight(line, " \t")
	line = strings.TrimSuffix(line, "\\")
	return strings.TrimSpace(line)
}

func HasPlaceholder(raw string) bool {
	return hasAnglePlaceholder(raw) ||
		strings.Contains(raw, "$") ||
		strings.Contains(raw, "...") ||
		placeholderTokenPattern.MatchString(raw)
}

func hasAnglePlaceholder(raw string) bool {
	for _, match := range angleTokenPattern.FindAllStringSubmatch(raw, -1) {
		if len(match) < 2 {
			continue
		}
		if isAnglePlaceholder(match[1], raw) {
			return true
		}
	}
	return false
}

func isAnglePlaceholder(inner, raw string) bool {
	inner = strings.TrimSpace(inner)
	if inner == "" || strings.HasPrefix(inner, "/") || strings.HasPrefix(inner, "!") || strings.HasPrefix(inner, "?") {
		return false
	}
	name := inner
	if cut := strings.IndexAny(name, " \t/"); cut >= 0 {
		name = name[:cut]
	}
	name = strings.TrimPrefix(name, "/")
	lower := strings.ToLower(name)
	if isMarkupLikeAngle(inner, lower, raw) {
		return false
	}
	if strings.ContainsAny(inner, "_| ") {
		return true
	}
	if strings.Contains(strings.ToLower(inner), "token") || strings.Contains(strings.ToLower(inner), " id") {
		return true
	}
	if genericAnglePlaceholders[lower] {
		return true
	}
	if lowerStructuredPlaceholderName.MatchString(lower) {
		return true
	}
	return inner == "id" || inner == "url" || hasUppercase(inner) || containsNonASCII(inner)
}

func isMarkupLikeAngle(inner, lowerName, raw string) bool {
	if markupTags[lowerName] {
		return true
	}
	if !xmlTagNamePattern.MatchString(lowerName) {
		return false
	}
	if strings.Contains(inner, "=") || strings.HasSuffix(strings.TrimSpace(inner), "/") {
		return true
	}
	return strings.Contains(strings.ToLower(raw), "</"+lowerName+">")
}

func hasUppercase(value string) bool {
	for _, r := range value {
		if 'A' <= r && r <= 'Z' {
			return true
		}
	}
	return false
}

func containsNonASCII(value string) bool {
	for _, r := range value {
		if r > 127 {
			return true
		}
	}
	return false
}

var markupTags = map[string]bool{
	"a": true, "b": true, "br": true, "code": true, "content": true, "div": true, "em": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"i": true, "img": true, "li": true, "ol": true, "p": true, "span": true,
	"strong": true, "table": true, "tbody": true, "td": true, "th": true, "thead": true,
	"title": true, "tr": true, "ul": true,
}

var genericAnglePlaceholders = map[string]bool{
	"action": true, "command": true, "field": true, "file": true, "method": true,
	"path": true, "query": true, "resource": true, "service": true, "shortcut": true, "value": true,
}

func FilterExamples(examples []Example, skills map[string]bool) []Example {
	if len(skills) == 0 {
		return nil
	}
	var out []Example
	for _, ex := range examples {
		name := skillNameFromPath(ex.SourceFile)
		if skills[name] {
			out = append(out, ex)
		}
	}
	return out
}

func skillNameFromPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, part := range parts {
		if part == "skills" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
