// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
	"github.com/stretchr/testify/require"
)

func readBaseSkillFile(t *testing.T, path ...string) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	skillDir := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "skills", "lark-base")
	skillPath := filepath.Join(append([]string{skillDir}, path...)...)
	content, err := vfs.ReadFile(skillPath)
	require.NoError(t, err)
	return string(content)
}

func TestBaseSkillRoutesFileImportExportToDrive(t *testing.T) {
	skill := readBaseSkillFile(t, "SKILL.md")
	require.Contains(t, skill, "文件导入/导出转 lark-drive")
	require.Contains(t, skill, "本地文件与 Base 之间的导入/导出转 `lark-drive`")
	require.Contains(t, skill, "在线复制走 `+base-copy`")
	require.NotContains(t, skill, "--only-schema")
	require.NotContains(t, skill, "--output-dir")
	require.NotContains(t, skill, "/tmp/")
}

func TestBaseSkillRequiresFreshViewCreateEvidence(t *testing.T) {
	skill := readBaseSkillFile(t, "SKILL.md")
	require.Contains(t, skill, "已有同名视图不能替代本次创建")
	require.Contains(t, skill, "只使用本次响应中的 `views[].id`")
	require.Contains(t, skill, "批量创建部分失败时保留 `views` 中已成功的项")
	require.Contains(t, skill, "仅当用户表达“确保存在/若不存在则创建”时")
}
