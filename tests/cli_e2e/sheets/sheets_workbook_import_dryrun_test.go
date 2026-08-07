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
)

// TestSheets_WorkbookImportDryRun pins the +workbook-import dry-run shape:
// upload the local file, best-effort report the upload, create an import task,
// and poll it with the doc type pinned to "sheet". The shortcut is distinct
// from generic drive +import because it hard-codes type=sheet and uses --name
// instead of --file-name.
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
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/drive/v1/medias/upload_all",
		clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
	require.Contains(t, clie2e.DryRunGet(out, "api.0.body.extra").String(), `"obj_type":"sheet"`,
		"upload extra should pin obj_type=sheet; stdout:\n%s", out)
	require.Equal(t, "ccm_import_open", clie2e.DryRunGet(out, "api.0.body.parent_type").String(),
		"stdout:\n%s", out)

	// api.1 — report the completed upload using the workbook-import command
	// identity while retaining the shared drive import mount point.
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.1.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/drive/v1/lark_cli_file_event/report",
		clie2e.DryRunGet(out, "api.1.url").String(), "stdout:\n%s", out)
	require.Equal(t, "lark-cli", clie2e.DryRunGet(out, "api.1.body.file_scene").String(), "stdout:\n%s", out)
	require.Equal(t, "sheets +workbook-import", clie2e.DryRunGet(out, "api.1.body.tags.command").String(),
		"stdout:\n%s", out)
	require.Equal(t, "ccm_import_open", clie2e.DryRunGet(out, "api.1.body.tags.mount_point").String(),
		"stdout:\n%s", out)

	// api.2 — create import task. type=sheet is the wrapper's whole reason for
	// existing (drive +import would require --doc-type sheet explicitly);
	// --name reaches the wire as file_name; file_extension is sniffed from
	// the local file (.csv).
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.2.method").String(), "stdout:\n%s", out)
	require.Equal(t, "/open-apis/drive/v1/import_tasks",
		clie2e.DryRunGet(out, "api.2.url").String(), "stdout:\n%s", out)
	require.Equal(t, "sheet", clie2e.DryRunGet(out, "api.2.body.type").String(),
		"workbook-import must hard-code type=sheet; stdout:\n%s", out)
	require.Equal(t, "imported", clie2e.DryRunGet(out, "api.2.body.file_name").String(),
		"--name should reach file_name; stdout:\n%s", out)
	require.Equal(t, "csv", clie2e.DryRunGet(out, "api.2.body.file_extension").String(),
		"file_extension sniffed from .csv; stdout:\n%s", out)
	// The import task must consume the upload step's result: the dry-run plan
	// carries the <file_token> placeholder standing in for api.0's returned
	// token — its absence would mean the two steps are not actually linked.
	require.Equal(t, "<file_token>", clie2e.DryRunGet(out, "api.2.body.file_token").String(),
		"upload file_token must feed the import task body; stdout:\n%s", out)
}
