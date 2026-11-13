// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDocs_InlineAttachmentsWorkflowAsUser verifies the server keeps text and
// both inline files, including the existing local-file upload/binding workflow.
func TestDocs_InlineAttachmentsWorkflowAsUser(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	skipWithoutInlineAttachmentCleanupScope(t, ctx)
	workDir := t.TempDir()
	writeLocalResourceFixture(t, workDir, "inline-a.txt", []byte("inline attachment A\n"))
	writeLocalResourceFixture(t, workDir, "inline-b.txt", []byte("inline attachment B\n"))
	created, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+create", "--title", "lark-cli inline attachments " + clie2e.GenerateSuffix(),
			"--content", `<p>created before <source path="@inline-a.txt"/> created between <source path="@inline-b.txt"/> created after</p>`,
		},
		DefaultAs: "user", WorkDir: workDir,
	})
	require.NoError(t, err)
	docToken := gjson.Get(created.Stdout, "data.document.document_id").String()
	if docToken != "" {
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := clie2e.CleanupContext()
			defer cleanupCancel()
			result, cleanupErr := drive.DeleteDriveResourceAndVerify(cleanupCtx, docToken, "docx", "user")
			clie2e.ReportCleanupFailure(t, "delete inline attachment test document", result, cleanupErr)
		})
	}
	created.AssertExitCode(t, 0)
	require.NotEmpty(t, docToken)
	assertBoundLocalResourceBlocks(t, created.Stdout, 0, 2)
	assertInlineAttachmentParagraph(t, ctx, docToken, "created")
	var tokens []string
	for _, block := range gjson.Get(created.Stdout, "data.document.new_blocks").Array() {
		if block.Get("block_type").String() == "file" {
			tokens = append(tokens, block.Get("block_token").String())
		}
	}
	require.Len(t, tokens, 2)
	updated, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+update", "--doc", docToken, "--command", "append",
			"--content", fmt.Sprintf(`<p>updated before <source token="%s"/> updated between <source token="%s"/> updated after</p>`, tokens[0], tokens[1]),
		},
		DefaultAs: "user", WorkDir: workDir,
	})
	require.NoError(t, err)
	updated.AssertExitCode(t, 0)
	assertInlineAttachmentParagraph(t, ctx, docToken, "updated")
}

// This workflow must be able to delete its document even when an assertion
// fails. An environment token without known grants is not a cleanable fixture.
func skipWithoutInlineAttachmentCleanupScope(t *testing.T, ctx context.Context) {
	t.Helper()
	status, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"auth", "status", "--json"}, DefaultAs: "user",
	})
	require.NoError(t, err)
	status.AssertExitCode(t, 0)
	for _, scope := range strings.Fields(gjson.Get(status.Stdout, "identities.user.scope").String()) {
		if scope == "drive:drive" || scope == "space:document:delete" {
			return
		}
	}
	t.Skip("inline attachment workflow requires a user profile with known drive:drive or space:document:delete grants for cleanup")
}

// assertInlineAttachmentParagraph checks source placement within one paragraph,
// rather than accepting matching text from unrelated blocks elsewhere in a doc.
func assertInlineAttachmentParagraph(t *testing.T, ctx context.Context, docToken, prefix string) {
	t.Helper()
	paragraphs := regexp.MustCompile(`(?s)<p\b[^>]*>.*?</p>`)
	var content string
	require.Eventually(t, func() bool {
		var err error
		content, err = fetchDocsContent(ctx, docToken, "xml", "full", "user")
		if err != nil {
			return false
		}
		for _, paragraph := range paragraphs.FindAllString(content, -1) {
			before := strings.Index(paragraph, prefix+" before")
			middle := strings.Index(paragraph, prefix+" between")
			after := strings.Index(paragraph, prefix+" after")
			if before < 0 || middle <= before || after <= middle || strings.Count(paragraph, "<source ") != 2 {
				continue
			}
			return strings.Contains(paragraph[before:middle], "<source ") && strings.Contains(paragraph[middle:after], "<source ")
		}
		return false
	}, 20*time.Second, 500*time.Millisecond, "inline attachment paragraph was not preserved")
}
