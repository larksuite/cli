// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"os"
	"strings"
	"testing"
)

func TestTaskQueryHelpDistinguishesAssignedAndRelatedScopes(t *testing.T) {
	if !strings.Contains(GetMyTasks.Description, "assigned to me only") {
		t.Fatalf("+get-my-tasks description must state its assigned-only scope: %q", GetMyTasks.Description)
	}
	if !strings.Contains(GetRelatedTasks.Description, "created") || !strings.Contains(GetRelatedTasks.Description, "followed") {
		t.Fatalf("+get-related-tasks description must distinguish other related roles: %q", GetRelatedTasks.Description)
	}
	if len(GetMyTasks.Tips) != 1 || !strings.Contains(GetMyTasks.Tips[0], "+get-related-tasks") || !strings.Contains(GetMyTasks.Tips[0], "+search --query") {
		t.Fatalf("+get-my-tasks help must redirect unscoped requests: %q", GetMyTasks.Tips)
	}
	if len(GetRelatedTasks.Tips) != 1 || !strings.Contains(GetRelatedTasks.Tips[0], "without an explicit assigned-to-me scope") {
		t.Fatalf("+get-related-tasks help must identify its default scope: %q", GetRelatedTasks.Tips)
	}
}

func TestTaskSkillRoutesUnscopedListsAndNamesWithoutNarrowing(t *testing.T) {
	skill := readTaskQueryGuidance(t, "../../skills/lark-task/SKILL.md")
	for _, required := range []string{
		"未指定任务关系范围",
		"`+get-related-tasks`",
		"明确“我负责/分配给我”",
		"`+get-my-tasks`",
		"`+search --query`",
	} {
		if !strings.Contains(skill, required) {
			t.Errorf("Task scope routing guidance missing %q", required)
		}
	}

	assigned := readTaskQueryGuidance(t, "../../skills/lark-task/references/lark-task-get-my-tasks.md")
	if strings.Contains(assigned, "If the user query only specifies a task name") {
		t.Fatal("assigned-only reference must not route a name-only request to +get-my-tasks")
	}
	if !strings.Contains(assigned, "+search --query") {
		t.Fatal("assigned-only reference must route unscoped name lookup to +search --query")
	}
}

func TestRelatedTaskDueGuidanceKeepsDueSeparateFromUpdateCursor(t *testing.T) {
	reference := readTaskQueryGuidance(t, "../../skills/lark-task/references/lark-task-get-related-tasks.md")
	if strings.Contains(reference, "If the request contains a start/end time boundary") {
		t.Fatal("related-task reference must not map every time boundary to updated_at page_token")
	}
	for _, required := range []string{
		"due.timestamp",
		"--include-complete=false --page-all --format json",
		"Asia/Shanghai",
		"has_more=false",
	} {
		if !strings.Contains(reference, required) {
			t.Errorf("related-task due guidance missing %q", required)
		}
	}
}

func readTaskQueryGuidance(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
