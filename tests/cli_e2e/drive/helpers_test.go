// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func createDriveFolder(t *testing.T, parentT *testing.T, ctx context.Context, name string, parentFolderToken string) string {
	t.Helper()
	folderToken := CreateDriveFolder(t, parentT, ctx, name, "bot", parentFolderToken)
	require.NotEmpty(t, folderToken)
	return folderToken
}
