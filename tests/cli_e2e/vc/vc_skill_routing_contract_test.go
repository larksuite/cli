// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"errors"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
	"github.com/stretchr/testify/require"
)

func TestVCMeetingReferencesAreSharedByBothSkills(t *testing.T) {
	vcSkill := readVCContractFile(t, "skills", "lark-vc", "SKILL.md")
	agentSkill := readVCContractFile(t, "skills", "lark-vc-agent", "SKILL.md")

	require.Contains(t, vcSkill, "查询进行中的会议、会中事件或发送会中消息")
	require.Contains(t, agentSkill, `"会议现在还开着，谁刚加入了"`)
	require.Contains(t, agentSkill, `"会议里谁在发言"`)
	require.Contains(t, agentSkill, `"我/某个用户现在在哪个会里"`)

	references := []struct {
		sharedName string
		oldName    string
		command    string
	}{
		{
			sharedName: "lark-vc-meeting-list-active.md",
			oldName:    "lark-vc-agent-meeting-list-active.md",
			command:    "lark-cli vc +meeting-list-active",
		},
		{
			sharedName: "lark-vc-meeting-events.md",
			oldName:    "lark-vc-agent-meeting-events.md",
			command:    "lark-cli vc +meeting-events",
		},
		{
			sharedName: "lark-vc-meeting-message-send.md",
			oldName:    "lark-vc-agent-meeting-message-send.md",
			command:    "lark-cli vc +meeting-message-send",
		},
	}

	for _, reference := range references {
		t.Run(reference.sharedName, func(t *testing.T) {
			require.Contains(t, vcSkill, "references/"+reference.sharedName)
			require.Contains(t, agentSkill, "../lark-vc/references/"+reference.sharedName)

			content := readVCContractFile(t, "skills", "lark-vc", "references", reference.sharedName)
			require.Contains(t, content, reference.command)
			require.Contains(t, content, "--as user")
			require.Contains(t, content, "--as bot")

			oldPath := vcContractPath(t, "skills", "lark-vc-agent", "references", reference.oldName)
			_, err := vfs.Stat(oldPath)
			require.True(t, errors.Is(err, fs.ErrNotExist), "legacy reference still exists: %s", oldPath)
		})
	}
}

func TestVCSharedMeetingReferencesHaveValidMarkdownLinks(t *testing.T) {
	linkPattern := regexp.MustCompile(`\[[^\]]+\]\(([^)#]+\.md)\)`)
	references := []string{
		"lark-vc-meeting-list-active.md",
		"lark-vc-meeting-events.md",
		"lark-vc-meeting-message-send.md",
	}

	for _, reference := range references {
		t.Run(reference, func(t *testing.T) {
			path := vcContractPath(t, "skills", "lark-vc", "references", reference)
			content := readVCContractFile(t, "skills", "lark-vc", "references", reference)
			links := linkPattern.FindAllStringSubmatch(content, -1)
			require.NotEmpty(t, links, "expected local markdown links in %s", path)

			for _, link := range links {
				target := filepath.Clean(filepath.Join(filepath.Dir(path), link[1]))
				_, err := vfs.Stat(target)
				require.NoError(t, err, "broken markdown link %q in %s", link[1], path)
			}
		})
	}
}

func readVCContractFile(t *testing.T, pathElements ...string) string {
	t.Helper()

	path := vcContractPath(t, pathElements...)
	content, err := vfs.ReadFile(path)
	require.NoError(t, err, "read %s", path)
	return string(content)
}

func vcContractPath(t *testing.T, pathElements ...string) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	repoRoot := filepath.Join(filepath.Dir(currentFile), "..", "..", "..")
	return filepath.Join(append([]string{repoRoot}, pathElements...)...)
}
