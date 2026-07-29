// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"os"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestDriveImportDryRunFolderTokenWikiProbe(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	workDir := t.TempDir()
	if err := os.WriteFile(workDir+"/notes.md", []byte("# dry run\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+import",
			"--file", "notes.md",
			"--type", "docx",
			"--folder-token", "fldcnImportDryRunTarget",
			"--dry-run",
		},
		WorkDir:   workDir,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "GET" {
		t.Fatalf("data.api.0.method = %q, want GET\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/wiki/v2/spaces/get_node" {
		t.Fatalf("data.api.0.url = %q, want wiki get_node\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.token").String(); got != "fldcnImportDryRunTarget" {
		t.Fatalf("data.api.0.params.token = %q, want fldcnImportDryRunTarget\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/drive/v1/medias/upload_all" {
		t.Fatalf("data.api.1.url = %q, want upload_all\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.2.method").String(); got != "POST" {
		t.Fatalf("data.api.2.method = %q, want POST\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.2.url").String(); got != "/open-apis/drive/v1/lark_cli_file_event/report" {
		t.Fatalf("data.api.2.url = %q, want report_file_event\nstdout:\n%s", got, out)
	}
	reportChecks := map[string]string{
		"api.2.body.file_scene":         "lark-cli",
		"api.2.body.scene":              "upload",
		"api.2.body.operation":          "upload",
		"api.2.body.tags.api_path":      "/open-apis/drive/v1/medias/upload_all",
		"api.2.body.tags.command":       "drive +import",
		"api.2.body.tags.upload_mode":   "singlepart",
		"api.2.body.tags.resource_type": "media",
		"api.2.body.tags.status":        "success",
		"api.2.body.tags.mount_point":   "ccm_import_open",
		"api.2.body.tags.file_token":    "<file_token from upload response>",
	}
	for path, want := range reportChecks {
		if got := clie2e.DryRunGet(out, path).String(); got != want {
			t.Fatalf("data.%s = %q, want %q\nstdout:\n%s", path, got, want, out)
		}
	}
	if got := clie2e.DryRunGet(out, "api.3.body.point.mount_key").String(); got != "fldcnImportDryRunTarget" {
		t.Fatalf("data.api.3.body.point.mount_key = %q, want fldcnImportDryRunTarget\nstdout:\n%s", got, out)
	}
}
