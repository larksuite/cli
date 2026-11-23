// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
	"github.com/stretchr/testify/require"
)

func TestMeetingScreenshotSkillIsNotDiscoverable(t *testing.T) {
	// Scan every agent-facing document instead of a fixed file list, so a new
	// scene or reference cannot quietly reintroduce the command.
	documents := collectMarkdownFiles(t, vcContractPath(t, "skills", "lark-meeting"))
	documents = append(documents, vcContractPath(t, "affordance", "vc.md"))

	for _, path := range documents {
		t.Run(filepath.Base(path), func(t *testing.T) {
			content, err := vfs.ReadFile(path)
			require.NoError(t, err, "read %s", path)
			for _, forbidden := range []string{"+meeting-screenshot", "lark-vc-meeting-screenshot.md"} {
				require.NotContains(t, string(content), forbidden, "found in %s", path)
			}
		})
	}

	_, err := vfs.Stat(vcContractPath(t, "skills", "lark-meeting", "references", "lark-vc-meeting-screenshot.md"))
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func collectMarkdownFiles(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := vfs.ReadDir(dir)
	require.NoError(t, err, "read dir %s", dir)

	var files []string
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			files = append(files, collectMarkdownFiles(t, path)...)
			continue
		}
		if strings.HasSuffix(entry.Name(), ".md") {
			files = append(files, path)
		}
	}
	return files
}
