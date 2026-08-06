// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"crypto/md5"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// The download always starts with a bounded Range request. This fixture keeps
// the live round trip cheap; unit tests cover continuation across multiple
// parts because doing so here would require uploading more than 8 MiB.
//
// What this test can prove is that a real upload/download round trip returns the
// exact bytes. It cannot prove which path ran: nothing in the command output
// says whether the endpoint answered 206 or ignored the Range and answered 200,
// so a green run is not evidence that the Content-Range and validator checks
// executed. Those are pinned by the unit tests in shortcuts/im.
const resourceDownloadFixtureSize = 320 * 1024

// TestIM_MessageResourceDownloadWorkflowAsBot uploads a file through
// `im +messages-send`, reads its file_key back off the message, downloads it
// with `im +messages-resources-download`, and compares the bytes. It is the only
// coverage that runs the shortcut against the real endpoint rather than a fake
// server told what to send.
func TestIM_MessageResourceDownloadWorkflowAsBot(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	chatID := createChat(t, parentT, ctx, "im-resource-download-"+suffix)

	workDir := t.TempDir()
	fixtureRelPath := filepath.Join("fixture", "resource.bin")
	payload := make([]byte, resourceDownloadFixtureSize)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, "fixture"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, fixtureRelPath), payload, 0o600))

	messageID := sendFileMessageOrSkipPermission(t, ctx, chatID, workDir, fixtureRelPath)
	recallMessageOnCleanup(parentT, messageID)
	fileKey := fileKeyOfMessage(t, ctx, messageID)

	t.Run("download file resource through the bounded first request", func(t *testing.T) {
		downloadDir := t.TempDir()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"im", "+messages-resources-download",
				"--message-id", messageID,
				"--file-key", fileKey,
				"--type", "file",
				"--output", "./downloaded.bin",
			},
			WorkDir:   downloadDir,
			DefaultAs: "bot",
		})
		requireCLISuccess(t, "resource download", result, err)

		require.Equal(t, int64(len(payload)), gjson.Get(result.Stdout, "data.size_bytes").Int(),
			"downloaded size should match the fixture")
		require.NotEmpty(t, gjson.Get(result.Stdout, "data.saved_path").String(),
			"result should report where the resource was saved")

		got, readErr := os.ReadFile(filepath.Join(downloadDir, "downloaded.bin"))
		require.NoError(t, readErr)
		require.Equal(t, md5.Sum(payload), md5.Sum(got),
			"downloaded bytes must match the uploaded fixture byte for byte")
	})
}

// sendFileMessageOrSkipPermission sends workDir-relative relPath as a file
// message and returns the message id, skipping when the test account cannot
// upload IM resources.
func sendFileMessageOrSkipPermission(t *testing.T, ctx context.Context, chatID, workDir, relPath string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"im", "+messages-send",
			"--chat-id", chatID,
			"--file", "./" + filepath.ToSlash(relPath),
		},
		WorkDir:   workDir,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 {
		combined := strings.ToLower(result.Stdout + "\n" + result.Stderr)
		if strings.Contains(combined, "app scope not enabled") ||
			strings.Contains(combined, "im:resource") ||
			strings.Contains(combined, "99991672") {
			t.Skipf("skip IM resource download workflow due to missing bot scope (exit %d, error.subtype=%q)",
				result.ExitCode, gjson.Get(result.Stderr, "error.subtype").String())
		}
	}
	requireCLISuccess(t, "send file message", result, nil)

	messageID := gjson.Get(result.Stdout, "data.message_id").String()
	require.NotEmpty(t, messageID, "message_id should not be empty")
	return messageID
}

// fileKeyOfMessage reads the resource key out of a file message, proving the key
// the download uses came from the platform rather than from the test.
//
// `im +messages-mget` does not hand back the platform's raw JSON content: it
// converts each message to the CLI's display form, so a file message arrives as
// `<file key="file_v3_..." name="resource.bin"/>` rather than
// `{"file_key":"..."}`. The key is read out of that attribute.
func fileKeyOfMessage(t *testing.T, ctx context.Context, messageID string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"im", "+messages-mget", "--message-ids", messageID},
		DefaultAs: "bot",
	})
	requireCLISuccess(t, "messages-mget", result, err)

	msgType := gjson.Get(result.Stdout, "data.messages.0.msg_type").String()
	require.Equal(t, "file", msgType, "message should be a file message")

	content := gjson.Get(result.Stdout, "data.messages.0.content").String()
	fileKey, ok := fileKeyFromMessageContent(content)
	// Deliberately not echoing content or stdout: this job's logs are public and
	// the payload carries live chat, message, sender and tenant identifiers.
	require.True(t, ok, "file message content should carry a key attribute (content length %d)", len(content))
	require.NotEmpty(t, fileKey, "file key should not be empty")
	return fileKey
}

// fileKeyFromMessageContent extracts the key attribute from a converted file
// message such as `<file key="file_v3_x" name="resource.bin"/>`.
func fileKeyFromMessageContent(content string) (string, bool) {
	const marker = `key="`
	start := strings.Index(content, marker)
	if start < 0 {
		return "", false
	}
	rest := content[start+len(marker):]
	end := strings.Index(rest, `"`)
	if end <= 0 {
		return "", false
	}
	return rest[:end], true
}

// recallMessageOnCleanup recalls the fixture message once the test is done, so a
// live run does not leave a file message sitting in the test account on every
// CI cycle.
//
// The chat itself stays: lark-cli exposes no chat-delete command, which is why
// createChatAs registers an empty cleanup. Recalling the message is the part of
// create -> use -> cleanup this suite can actually honour; a cleanup failure is
// reported rather than failing the run, matching the other suites.
func recallMessageOnCleanup(parentT *testing.T, messageID string) {
	parentT.Cleanup(func() {
		cleanupCtx, cancel := clie2e.CleanupContext()
		defer cancel()

		result, err := clie2e.RunCmdWithRetry(cleanupCtx, clie2e.Request{
			Args:      []string{"im", "messages", "delete"},
			Params:    map[string]any{"message_id": messageID},
			DefaultAs: "bot",
			Yes:       true,
		}, clie2e.RetryOptions{})
		// clie2e.ReportCleanupFailure would print the whole envelope; see
		// requireCLISuccess for why that is not an option in this suite.
		if err != nil {
			parentT.Errorf("recall IM file message: runner error: %v", err)
			return
		}
		if result == nil {
			parentT.Errorf("recall IM file message: nil result")
			return
		}
		if result.ExitCode != 0 {
			parentT.Errorf("recall IM file message failed: exit=%d error.type=%q error.subtype=%q",
				result.ExitCode,
				gjson.Get(result.Stderr, "error.type").String(),
				gjson.Get(result.Stderr, "error.subtype").String())
		}
	})
}

// requireCLISuccess asserts a command succeeded without echoing what it returned.
//
// The shared clie2e.Result assertions print the full stdout and stderr on
// failure, and this suite's envelopes carry live chat ids, message ids, resource
// keys, app links, sender profiles and tenant identifiers. e2e-live runs on a
// public repository, so a failing assertion would publish all of it; only the
// exit code and the error classification are logged here.
func requireCLISuccess(t *testing.T, what string, result *clie2e.Result, err error) {
	t.Helper()
	require.NoError(t, err, "%s: runner error", what)
	require.NotNil(t, result, "%s: nil result", what)
	require.Equal(t, 0, result.ExitCode, "%s failed: exit=%d error.type=%q error.subtype=%q",
		what, result.ExitCode,
		gjson.Get(result.Stderr, "error.type").String(),
		gjson.Get(result.Stderr, "error.subtype").String())
	ok := gjson.Get(result.Stdout, "ok")
	require.True(t, ok.Exists() && ok.Bool(), "%s returned an unsuccessful envelope", what)
}
