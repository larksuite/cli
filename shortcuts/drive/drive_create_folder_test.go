// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestDriveCreateFolder_Validation_MissingName(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("folder-token", "", "")
	_ = cmd.Flags().Set("folder-token", "fldcnabc123")

	runtime := common.TestNewRuntimeContext(cmd, driveTestConfig())
	err := DriveCreateFolder.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error for missing --name")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestDriveCreateFolder_Validation_MissingFolderToken(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("folder-token", "", "")
	_ = cmd.Flags().Set("name", "test-folder")

	runtime := common.TestNewRuntimeContext(cmd, driveTestConfig())
	err := DriveCreateFolder.Validate(context.Background(), runtime)
	// folder-token is optional (defaults to root), so validation should pass
	if err != nil {
		t.Fatalf("folder-token is optional, unexpected error: %v", err)
	}
}

func TestDriveCreateFolder_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveCreateFolder,
		[]string{"+create-folder", "--name", "周报", "--folder-token", "fldcnabc123", "--dry-run", "--as", "user"},
		f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "/open-apis/drive/v1/files/create_folder") {
		t.Errorf("dry-run should show API path, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "周报") {
		t.Errorf("dry-run should show folder name, got: %s", stdout.String())
	}
}
