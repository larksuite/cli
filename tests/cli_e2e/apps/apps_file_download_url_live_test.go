// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestAppsFileDownloadURLRoundTripLive pins the contract that a download_url the
// platform itself handed out is accepted anywhere a --path is taken.
//
// It exists because that round trip was broken: +file-sign, +file-get and
// +file-download all rejected their own download_url with 400000034
// "File not found or no access". An agent has no way to tell that apart from a
// genuinely missing file, so it reads the platform's own output as invalid and
// gives up — which is why this is worth a live case rather than a unit test. The
// resolution happens server-side (the CLI forwards --path verbatim), so only an
// end-to-end call can catch a regression here.
//
// Fixture-gated like the sibling upload workflow: it uploads into a dedicated app
// and deletes what it created.
func TestAppsFileDownloadURLRoundTripLive(t *testing.T) {
	if strings.TrimSpace(os.Getenv("LARKSUITE_CLI_CONFIG_DIR")) == "" {
		t.Skip("FIXTURE: Set LARKSUITE_CLI_CONFIG_DIR to an isolated live-test config")
	}
	appID := strings.TrimSpace(os.Getenv("LARK_CLI_E2E_APPS_FILE_APP_ID"))
	if appID == "" {
		t.Skip("FIXTURE: Set LARK_CLI_E2E_APPS_FILE_APP_ID to a dedicated app for upload/delete testing")
	}

	fileName := fmt.Sprintf("lark-cli-download-url-e2e-%d.txt", time.Now().UnixNano())
	localPath := filepath.Join(t.TempDir(), fileName)
	content := []byte("download-url-round-trip-live-e2e")
	require.NoError(t, os.WriteFile(localPath, content, 0o600))

	remotePath := ""
	t.Cleanup(func() {
		if remotePath == "" {
			return
		}
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"apps", "+file-delete", "--app-id", appID, "--path", remotePath},
			DefaultAs: "user",
			Yes:       true,
		})
		clie2e.ReportCleanupFailure(t, "delete uploaded file "+remotePath, deleteResult, deleteErr)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	uploadResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"apps", "+file-upload", "--app-id", appID, "--file", localPath},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	uploadResult.AssertExitCode(t, 0)
	uploadResult.AssertStdoutStatus(t, true)
	remotePath = gjson.Get(uploadResult.Stdout, "data.path").String()
	require.NotEmpty(t, remotePath, "stdout:\n%s", uploadResult.Stdout)

	// The download_url is read back from the platform rather than constructed
	// here: a hand-built URL would test this test's idea of the format instead of
	// the format actually being handed to callers.
	getResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"apps", "+file-get", "--app-id", appID, "--path", remotePath},
		DefaultAs: "user",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0 || gjson.Get(result.Stdout, "data.download_url").String() == ""
		},
	})
	require.NoError(t, err)
	getResult.AssertExitCode(t, 0)
	downloadURL := gjson.Get(getResult.Stdout, "data.download_url").String()
	require.NotEmpty(t, downloadURL, "stdout:\n%s", getResult.Stdout)
	require.NotEqual(t, remotePath, downloadURL,
		"download_url must differ from path, otherwise this case proves nothing:\n%s", getResult.Stdout)

	t.Run("file-get accepts download_url", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+file-get", "--app-id", appID, "--path", downloadURL},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		// Resolution, not a lucky pass: the server must report the real path.
		assert.Equal(t, remotePath, gjson.Get(result.Stdout, "data.path").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, int64(len(content)), gjson.Get(result.Stdout, "data.size_bytes").Int(), "stdout:\n%s", result.Stdout)
	})

	t.Run("file-sign accepts download_url", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+file-sign", "--app-id", appID, "--path", downloadURL},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, remotePath, gjson.Get(result.Stdout, "data.path").String(), "stdout:\n%s", result.Stdout)
		assert.NotEmpty(t, gjson.Get(result.Stdout, "data.signed_url").String(), "stdout:\n%s", result.Stdout)
	})

	t.Run("file-download accepts download_url", func(t *testing.T) {
		// --output is confined to the working directory by design, so the
		// destination is a relative name inside a per-subtest WorkDir rather than
		// an absolute temp path.
		workDir := t.TempDir()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+file-download", "--app-id", appID, "--path", downloadURL, "--output", "./roundtrip.txt"},
			DefaultAs: "user",
			WorkDir:   workDir,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		// Asserting on the bytes, not just the exit code: a download that writes
		// an error page or a truncated body would still exit 0.
		got, readErr := os.ReadFile(filepath.Join(workDir, "roundtrip.txt"))
		require.NoError(t, readErr)
		assert.Equal(t, content, got)
	})

	// The compatibility must not turn into "accept anything": a file that does not
	// exist has to keep failing, in both path shapes, or the round trip above would
	// pass even against a server that stopped looking the object up at all.
	t.Run("missing file still fails in both shapes", func(t *testing.T) {
		missingName := fmt.Sprintf("lark-cli-absent-%d.txt", time.Now().UnixNano())
		shapes := map[string]string{
			"plain path":   "/" + missingName,
			"download_url": strings.Replace(downloadURL, filepath.Base(downloadURL), missingName, 1),
		}
		for name, path := range shapes {
			t.Run(name, func(t *testing.T) {
				result, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args:      []string{"apps", "+file-sign", "--app-id", appID, "--path", path},
					DefaultAs: "user",
				})
				require.NoError(t, err)
				assert.NotEqual(t, 0, result.ExitCode, "absent file must not sign:\n%s", result.Stderr)
				// The failure envelope goes to stderr; asserting on the subtype
				// rather than the exit code alone keeps this from passing on an
				// unrelated failure such as a bad --app-id.
				assert.Equal(t, "not_found", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)
			})
		}
	})
}
