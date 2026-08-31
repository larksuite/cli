// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDocs_CreateBatchWorkflow(t *testing.T) {
	if os.Getenv("LARK_DOC_CREATE_BATCH_E2E") != "1" {
		t.Skip("set LARK_DOC_CREATE_BATCH_E2E=1 to run the large document create workflow")
	}
	defaultAs := strings.TrimSpace(os.Getenv("LARK_DOC_CREATE_BATCH_E2E_AS"))
	if defaultAs == "" {
		defaultAs = "user"
	}
	if defaultAs != "user" && defaultAs != "bot" {
		t.Fatalf("LARK_DOC_CREATE_BATCH_E2E_AS = %q, want user or bot", defaultAs)
	}
	if defaultAs == "user" {
		clie2e.SkipWithoutUserToken(t)
	}
	docFormat := strings.TrimSpace(os.Getenv("LARK_DOC_CREATE_BATCH_E2E_FORMAT"))
	if docFormat == "" {
		docFormat = "markdown"
	}
	if docFormat != "markdown" && docFormat != "xml" {
		t.Fatalf("LARK_DOC_CREATE_BATCH_E2E_FORMAT = %q, want markdown or xml", docFormat)
	}
	bodyBlocks := 5_000
	if raw := strings.TrimSpace(os.Getenv("LARK_DOC_CREATE_BATCH_E2E_BODY_BLOCKS")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		require.NoError(t, err, "parse LARK_DOC_CREATE_BATCH_E2E_BODY_BLOCKS")
		require.GreaterOrEqual(t, parsed, 2_001, "LARK_DOC_CREATE_BATCH_E2E_BODY_BLOCKS")
		bodyBlocks = parsed
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	suffix := clie2e.GenerateSuffix()
	folderToken := drive.CreateDriveFolder(t, t, ctx, "lark-cli-e2e-create-batch-"+suffix, defaultAs, "")
	title := "lark-cli create batch " + suffix
	body := "first-batch-marker\n\n" + strings.Repeat("middle paragraph\n\n", bodyBlocks-2) + "last-batch-marker"
	args := []string{
		"docs", "+create",
		"--parent-token", folderToken,
		"--title", title,
		"--doc-format", docFormat,
		"--content", "-",
	}
	if docFormat == "xml" {
		body = "<title>" + title + "</title>\n<p>first-batch-marker</p>\n" +
			strings.Repeat("<p>middle paragraph</p>\n", bodyBlocks-2) +
			"<p>last-batch-marker</p>"
		args = []string{
			"docs", "+create",
			"--parent-token", folderToken,
			"--doc-format", docFormat,
			"--content", "-",
		}
	}

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      args,
		DefaultAs: defaultAs,
		Stdin:     []byte(body),
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	docToken := gjson.Get(result.Stdout, "data.document.document_id").String()
	require.NotEmpty(t, docToken, "stdout:\n%s", result.Stdout)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		deleteResult, deleteErr := drive.DeleteDriveResourceAndVerify(cleanupCtx, docToken, "docx", defaultAs)
		clie2e.ReportCleanupFailure(t, "delete doc "+docToken, deleteResult, deleteErr)
	})
	fetched, err := fetchDocsContent(ctx, docToken, "markdown", "full", defaultAs)
	require.NoError(t, err)
	require.Contains(t, fetched, "first-batch-marker")
	require.Contains(t, fetched, "last-batch-marker")
}
