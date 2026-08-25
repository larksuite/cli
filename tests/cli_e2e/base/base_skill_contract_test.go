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

func TestBaseSkillRoutesFileImportExportToDrive(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	skillPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "skills", "lark-base", "SKILL.md")
	content, err := vfs.ReadFile(skillPath)
	require.NoError(t, err)

	skill := string(content)
	require.Contains(t, skill, "文件导入/导出转 lark-drive")
	require.Contains(t, skill, "本地文件与 Base 之间的导入/导出转 `lark-drive`")
	require.Contains(t, skill, "在线复制走 `+base-copy`")
	require.Contains(t, skill, "查找模板中心模板")
	require.Contains(t, skill, "lark-base-template-center.md")
	require.Contains(t, skill, "`+template-categories`")
	require.Contains(t, skill, "`+template-list`")
	require.Contains(t, skill, "`+template-search`")
	referencePath := filepath.Join(filepath.Dir(skillPath), "references", "lark-base-template-center.md")
	reference, err := vfs.ReadFile(referencePath)
	require.NoError(t, err)
	referenceText := string(reference)
	require.Contains(t, referenceText, "`templates[].token`")
	require.Contains(t, referenceText, "`--category-key`")
	require.Contains(t, referenceText, "`--offset`")
	require.Contains(t, referenceText, "`+base-copy --base-token`")
	require.NotContains(t, skill, "--only-schema")
	require.NotContains(t, skill, "--output-dir")
	require.NotContains(t, skill, "/tmp/")
}
