// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package domaincontract

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type addedLineRange struct {
	Start int
	End   int
}

type changedGoPath struct {
	Old string
	New string
}

var unifiedHunkRE = regexp.MustCompile(`^@@ -[0-9]+(?:,[0-9]+)? \+([0-9]+)(?:,([0-9]+))? @@`)

func changedGoLineRanges(root, from string) (map[string][]addedLineRange, error) {
	if from == "" {
		return nil, nil
	}
	names, err := gitCommandOutput(
		root,
		"diff",
		"--name-status",
		"-z",
		"--find-renames",
		"--diff-filter=ACMR",
		from+"...HEAD",
		"--",
	)
	if err != nil {
		return nil, fmt.Errorf("list changed Go files: %w", err)
	}
	paths, err := parseChangedGoPaths(names)
	if err != nil {
		return nil, fmt.Errorf("parse changed Go files: %w", err)
	}

	out := map[string][]addedLineRange{}
	for _, path := range paths {
		args := []string{
			"diff",
			"--unified=0",
			"--no-color",
			"--no-ext-diff",
			"--find-renames",
			"--diff-filter=ACMR",
			from + "...HEAD",
			"--",
		}
		if path.Old != path.New {
			args = append(args, path.Old)
		}
		args = append(args, path.New)
		patch, err := gitCommandOutput(root, args...)
		if err != nil {
			return nil, fmt.Errorf("read diff for %s: %w", path.New, err)
		}
		ranges, err := parseAddedLineRanges(patch)
		if err != nil {
			return nil, fmt.Errorf("parse diff for %s: %w", path.New, err)
		}
		out[path.New] = ranges
	}
	return out, nil
}

func parseChangedGoPaths(raw []byte) ([]changedGoPath, error) {
	fields := bytes.Split(raw, []byte{0})
	var out []changedGoPath
	for i := 0; i < len(fields); {
		status := string(fields[i])
		i++
		if status == "" {
			break
		}
		if i >= len(fields) || len(fields[i]) == 0 {
			return nil, fmt.Errorf("truncated name-status record")
		}
		oldPath := filepath.ToSlash(string(fields[i]))
		i++
		newPath := oldPath
		if status[0] == 'R' || status[0] == 'C' {
			if i >= len(fields) || len(fields[i]) == 0 {
				return nil, fmt.Errorf("truncated rename/copy record for %q", oldPath)
			}
			newPath = filepath.ToSlash(string(fields[i]))
			i++
			if status[0] == 'C' {
				// A copy introduces every destination line. Diff only the new
				// path so Git presents it as an added file rather than a
				// metadata-only copy with no added-line ranges.
				oldPath = newPath
			}
		}
		if !strings.HasSuffix(newPath, ".go") {
			continue
		}
		out = append(out, changedGoPath{Old: oldPath, New: newPath})
	}
	return out, nil
}

func parseAddedLineRanges(patch []byte) ([]addedLineRange, error) {
	var out []addedLineRange
	for _, raw := range bytes.Split(patch, []byte{'\n'}) {
		line := string(raw)
		if !strings.HasPrefix(line, "@@") {
			continue
		}
		match := unifiedHunkRE.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("unsupported unified hunk header %q", line)
		}
		start, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("parse added start line in %q: %w", line, err)
		}
		count := 1
		if match[2] != "" {
			count, err = strconv.Atoi(match[2])
			if err != nil {
				return nil, fmt.Errorf("parse added line count in %q: %w", line, err)
			}
		}
		if count == 0 {
			continue
		}
		out = append(out, addedLineRange{Start: start, End: start + count - 1})
	}
	return out, nil
}

func firstAddedLineInSpan(ranges []addedLineRange, start, end int) (int, bool) {
	for _, r := range ranges {
		if start <= r.End && end >= r.Start {
			if start > r.Start {
				return start, true
			}
			return r.Start, true
		}
	}
	return 0, false
}

func gitCommandOutput(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		stderr := strings.TrimSpace(string(exitErr.Stderr))
		if stderr != "" {
			return nil, fmt.Errorf("%w: %s", err, stderr)
		}
	}
	return nil, err
}
