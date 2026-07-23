// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestWhiteboardExportPreview_JPEGLiveWorkflow(t *testing.T) {
	token := os.Getenv("LARK_WHITEBOARD_E2E_TOKEN")
	if token == "" {
		t.Skip("skipped: LARK_WHITEBOARD_E2E_TOKEN not set")
	}
	clie2e.SkipWithoutUserToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)

	workDir := t.TempDir()
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"whiteboard", "+export",
			"--whiteboard-token", token,
			"--output-type", "preview",
			"--output", "preview",
			"--overwrite",
		},
		DefaultAs: "user",
		Format:    "json",
		WorkDir:   workDir,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	saved := filepath.Join(workDir, "preview.jpg")
	data, err := os.ReadFile(saved)
	require.NoError(t, err, "expected JPEG preview at %s\nstdout:\n%s\nstderr:\n%s", saved, result.Stdout, result.Stderr)
	require.True(t, isJPEG(data), "expected JPEG data in %s", saved)

	mismatch, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"whiteboard", "+export",
			"--whiteboard-token", token,
			"--output-type", "preview",
			"--output", "preview.png",
			"--overwrite",
		},
		DefaultAs: "user",
		Format:    "json",
		WorkDir:   workDir,
	})
	require.NoError(t, err)
	mismatch.AssertExitCode(t, 2)
	if !strings.Contains(mismatch.Stdout+"\n"+mismatch.Stderr, "failed_precondition") {
		t.Fatalf("expected failed_precondition for mismatched extension\nstdout:\n%s\nstderr:\n%s", mismatch.Stdout, mismatch.Stderr)
	}
}

func isJPEG(data []byte) bool {
	return len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff
}
