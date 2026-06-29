// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSheets_WorkbookImportDryRun pins the +workbook-import dry-run shape: a
// two-step plan that uploads the local file (drive media upload) and creates
// an import task with the doc type pinned to "sheet". This is the new shortcut
// added in this branch — distinct from generic drive +import because it
// hard-codes type=sheet and uses --name instead of --file-name. AGENTS.md
// requires a dry-run E2E to lock the request shape before a live run.
func TestSheets_WorkbookImportDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	// CLI sandbox only accepts relative file paths under cwd; write the CSV
	// into a TempDir and hand RunCmd that as WorkDir so --file resolves.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.csv"), []byte("a,b\n1,2\n"), 0o644))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"sheets", "+workbook-import",
			"--file", "data.csv",
			"--name", "imported",
			"--dry-run",
		},
		DefaultAs: "user",
		WorkDir:   dir,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout

	// api.0 — upload file to obtain the file_token; the wrapper sets
	// obj_type=sheet in extra so the upload is scoped for sheet import.
	require.Equal(t, "POST", gjson.Get(out, "api.0.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/drive/v1/medias/upload_all",
		gjson.Get(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Contains(t, gjson.Get(out, "api.0.body.extra").String(), `"obj_type":"sheet"`,
		"upload extra should pin obj_type=sheet; stdout:\n%s", out)
	require.Equal(t, "ccm_import_open", gjson.Get(out, "api.0.body.parent_type").String(),
		"stdout:\n%s", out)

	// api.1 — create import task. type=sheet is the wrapper's whole reason for
	// existing (drive +import would require --doc-type sheet explicitly);
	// --name reaches the wire as file_name; file_extension is sniffed from
	// the local file (.csv).
	require.Equal(t, "POST", gjson.Get(out, "api.1.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/drive/v1/import_tasks",
		gjson.Get(out, "api.1.url").String(), "stdout:\n%s", out)
	require.Equal(t, "sheet", gjson.Get(out, "api.1.body.type").String(),
		"workbook-import must hard-code type=sheet; stdout:\n%s", out)
	require.Equal(t, "imported", gjson.Get(out, "api.1.body.file_name").String(),
		"--name should reach file_name; stdout:\n%s", out)
	require.Equal(t, "csv", gjson.Get(out, "api.1.body.file_extension").String(),
		"file_extension sniffed from .csv; stdout:\n%s", out)
}
