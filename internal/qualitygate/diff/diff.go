// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package diff

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

var ErrFileAtRevisionMissing = errors.New("file missing at revision")

type Scope struct {
	Global    bool
	AllSkills map[string]bool
	Files     map[string]bool
}

func ChangedFiles(ctx context.Context, repo, from string) ([]string, error) {
	if from == "" {
		return nil, nil
	}
	return gitChangedFiles(ctx, repo, "diff", "--name-only", "-z", "--diff-filter=ACMRD", from+"...HEAD")
}

func ChangedFilesIncludingWorktree(ctx context.Context, repo, from string) ([]string, error) {
	var all []string
	if from != "" {
		committed, err := ChangedFiles(ctx, repo, from)
		if err != nil {
			return nil, err
		}
		all = append(all, committed...)
	}
	for _, args := range [][]string{
		{"diff", "--name-only", "-z", "--diff-filter=ACMRD"},
		{"diff", "--cached", "--name-only", "-z", "--diff-filter=ACMRD"},
		{"ls-files", "--others", "--exclude-standard", "-z"},
	} {
		files, err := gitChangedFiles(ctx, repo, args...)
		if err != nil {
			return nil, err
		}
		all = append(all, files...)
	}
	return uniqueSorted(all), nil
}

func gitChangedFiles(ctx context.Context, repo string, args ...string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s changed files: %w", strings.Join(args, " "), err)
	}
	// Output is NUL-delimited (-z) so paths containing whitespace stay intact.
	var lines []string
	for _, name := range strings.Split(string(out), "\x00") {
		if name != "" {
			lines = append(lines, name)
		}
	}
	sort.Strings(lines)
	return lines, nil
}

func uniqueSorted(files []string) []string {
	if len(files) == 0 {
		return nil
	}
	sort.Strings(files)
	out := files[:0]
	var last string
	for i, file := range files {
		if i > 0 && file == last {
			continue
		}
		out = append(out, file)
		last = file
	}
	return out
}

func FileAtRevision(ctx context.Context, repo, rev, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "show", rev+":"+path)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		if isFileAtRevisionMissing(string(out)) {
			return nil, ErrFileAtRevisionMissing
		}
		return nil, fmt.Errorf("git show %s:%s: %w", rev, path, err)
	}
	return out, nil
}

func isFileAtRevisionMissing(stderr string) bool {
	return strings.Contains(stderr, "exists on disk, but not in") ||
		strings.Contains(stderr, "does not exist in")
}

func FromChangedFiles(files []string) Scope {
	scope := Scope{AllSkills: map[string]bool{}, Files: map[string]bool{}}
	for _, file := range files {
		scope.Files[file] = true
		parts := strings.Split(file, "/")
		if len(parts) >= 2 && parts[0] == "skills" {
			scope.AllSkills[parts[1]] = true
			continue
		}
		if strings.HasPrefix(file, "cmd/") ||
			strings.HasPrefix(file, "shortcuts/") ||
			strings.HasPrefix(file, "internal/output/") ||
			strings.HasPrefix(file, "internal/errclass/") ||
			strings.HasPrefix(file, "errs/") {
			scope.Global = true
		}
	}
	return scope
}

func ChangedUnder(files map[string]bool, prefix string) bool {
	for file := range files {
		if strings.HasPrefix(file, prefix) {
			return true
		}
	}
	return false
}
