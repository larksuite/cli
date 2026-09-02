// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
	"github.com/stretchr/testify/require"
)

func TestMeetingScreenshotSkillKeepsQuickActionFocused(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	skillDir := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "skills", "lark-meeting")
	skillContent, err := vfs.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	require.NoError(t, err)
	skill := string(skillContent)
	require.Contains(t, skill, "### 查询进行中的会议内容")
	require.NotContains(t, skill, "### 查询或截图进行中的会议内容")
	require.NotContains(t, skill, "## 视频会议截图")
	frontmatterParts := strings.SplitN(skill, "---", 3)
	require.Len(t, frontmatterParts, 3)
	require.NotContains(t, frontmatterParts[1], "获取视频会议截图")
	require.Contains(t, skill, "| `vc +meeting-screenshot` | 获取视频会议截图 |")
	require.Contains(t, skill, "查看发言/聊天/共享内容、按需读取当前会议画面")

	sceneContent, err := vfs.ReadFile(filepath.Join(skillDir, "scenes", "live-meeting-interact.md"))
	require.NoError(t, err)
	scene := string(sceneContent)
	require.Contains(t, scene, "## 读取当前会议画面")
	sectionParts := strings.SplitN(scene, "## 读取当前会议画面", 2)
	require.Len(t, sectionParts, 2)
	sectionParts = strings.SplitN(sectionParts[1], "## 发送会中文本或表情", 2)
	require.Len(t, sectionParts, 2)
	visualSection := sectionParts[0]
	require.Contains(t, visualSection, "必须读取当前会议合成画面中的视觉信息")
	require.Contains(t, visualSection, "事件、字幕、聊天或可直接读取的共享文档已经足够回答时，不要截图")
	require.Contains(t, visualSection, "lark-cli vc +meeting-screenshot --as <same_identity> --meeting-id <meeting_id>")
	require.NotContains(t, visualSection, "+meeting-events")
	require.NotContains(t, visualSection, "JPEG")
	require.NotContains(t, visualSection, "重新截图")

	referenceContent, err := vfs.ReadFile(filepath.Join(skillDir, "references", "lark-vc-meeting-screenshot.md"))
	require.NoError(t, err)
	reference := string(referenceContent)
	require.Contains(t, reference, "## 参数")
	for _, flag := range []string{"`--as <identity>`", "`--meeting-id <meeting_id>`", "`--output <relative-path>`", "`--overwrite`"} {
		require.Contains(t, reference, flag)
	}
	for _, genericFlag := range []string{"`--dry-run`", "`--format <format>`", "`--json`", "`-q, --jq <expression>`", "`-h, --help`"} {
		require.NotContains(t, reference, genericFlag)
	}
	require.Contains(t, reference, "--output ./meeting-screenshots/current.jpg")
	require.Contains(t, reference, "相对于执行命令时的当前工作目录")
	require.Contains(t, reference, "父目录会自动创建")
	require.Contains(t, reference, "绝对路径")
	parts := strings.Split(reference, "```")
	require.GreaterOrEqual(t, len(parts), 3)
	require.NotContains(t, parts[1], "--overwrite")
}
