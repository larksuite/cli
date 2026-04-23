// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
)

// TestDrive_FilesCreateFolderWorkflow tests the files create_folder resource command.
func TestDrive_FilesCreateFolderWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	parentFolderName := "lark-cli-e2e-drive-parent-" + suffix
	folderName := "lark-cli-e2e-drive-folder-" + suffix
	parentFolderToken := createDriveFolder(t, parentT, ctx, parentFolderName, "")

	t.Run("create_folder as bot", func(t *testing.T) {
		folderToken := createDriveFolder(t, parentT, ctx, folderName, parentFolderToken)
		if folderToken == "" {
			t.Fatalf("folder token should be available")
		}
	})
}
