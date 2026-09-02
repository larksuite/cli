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

func TestLegacyVCSkillsRouteToMeetingSkill(t *testing.T) {
	for _, skillName := range []string{"lark-vc", "lark-vc-agent"} {
		t.Run(skillName, func(t *testing.T) {
			skill := readVCContractFile(t, "skills", skillName, "SKILL.md")
			require.Contains(t, skill, "本技能只用于兼容旧名称，不直接处理业务。")
			require.Contains(t, skill, "../lark-meeting/SKILL.md")
			require.Contains(t, skill, `skills: ["lark-meeting"]`)
		})
	}
}

func TestMeetingSkillOwnsVCReferences(t *testing.T) {
	meetingSkill := readVCContractFile(t, "skills", "lark-meeting", "SKILL.md")

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
		{
			sharedName: "lark-vc-meeting-countdown.md",
			oldName:    "lark-vc-agent-meeting-countdown.md",
			command:    "lark-cli vc +meeting-countdown",
		},
	}

	for _, reference := range references {
		t.Run(reference.sharedName, func(t *testing.T) {
			require.Contains(t, meetingSkill, "references/"+reference.sharedName)

			content := readVCContractFile(t, "skills", "lark-meeting", "references", reference.sharedName)
			require.Contains(t, content, reference.command)
			require.Contains(t, content, "--as user")
			require.Contains(t, content, "--as bot")

			oldPaths := []string{
				vcContractPath(t, "skills", "lark-vc", "references", reference.sharedName),
				vcContractPath(t, "skills", "lark-vc-agent", "references", reference.oldName),
			}
			for _, oldPath := range oldPaths {
				_, err := vfs.Stat(oldPath)
				require.True(t, errors.Is(err, fs.ErrNotExist), "legacy reference still exists: %s", oldPath)
			}
		})
	}
}

func TestVCSharedMeetingReferencesHaveValidMarkdownLinks(t *testing.T) {
	linkPattern := regexp.MustCompile(`\[[^\]]+\]\(([^)#]+\.md)\)`)
	references := []string{
		"lark-vc-meeting-list-active.md",
		"lark-vc-meeting-events.md",
		"lark-vc-meeting-message-send.md",
		"lark-vc-meeting-countdown.md",
	}

	for _, reference := range references {
		t.Run(reference, func(t *testing.T) {
			path := vcContractPath(t, "skills", "lark-meeting", "references", reference)
			content := readVCContractFile(t, "skills", "lark-meeting", "references", reference)
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
