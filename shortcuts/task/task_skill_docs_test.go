// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskSkillDocumentsInlineImageLimit(t *testing.T) {
	root := taskSkillRepoRoot(t)
	files := []string{
		"skills/lark-task/SKILL.md",
		"skills/lark-task/references/lark-task-upload-attachment.md",
		"skills/lark-task/references/lark-task-comment.md",
	}

	for _, rel := range files {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		text := string(body)
		for _, want := range []string{
			"inline images",
			"[Image]",
			"not downloadable",
			"task_delivery",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s missing %q", rel, want)
			}
		}
	}
}

func taskSkillRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
