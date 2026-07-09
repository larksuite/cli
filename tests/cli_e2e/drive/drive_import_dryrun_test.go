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
	if got := clie2e.DryRunGet(out, "api.2.body.point.mount_key").String(); got != "fldcnImportDryRunTarget" {
		t.Fatalf("data.api.2.body.point.mount_key = %q, want fldcnImportDryRunTarget\nstdout:\n%s", got, out)
	}
}
