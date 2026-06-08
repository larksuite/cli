// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDriveListCommentsWorkflow(t *testing.T) {
	if os.Getenv("LARK_DRIVE_LIST_COMMENTS_E2E") == "" {
		t.Skip("set LARK_DRIVE_LIST_COMMENTS_E2E=1 to run the docx list-comments live workflow")
	}
	clie2e.SkipWithoutUserToken(t)

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	anchorText := "lark-cli-e2e-list-comments-anchor-" + suffix
	commentText := "please review " + suffix

	createResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"docs", "+create",
			"--api-version", "v2",
			"--doc-format", "xml",
			"--content", fmt.Sprintf("<paragraph>%s</paragraph>", anchorText),
		},
		DefaultAs: "user",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	createResult.AssertExitCode(t, 0)

	docToken := firstGJSON(createResult.Stdout,
		"data.document.document_id",
		"document.document_id",
		"data.document_id",
	)
	require.NotEmpty(t, docToken, "stdout:\n%s", createResult.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{
				"drive", "+delete",
				"--file-token", docToken,
				"--type", "docx",
			},
			DefaultAs: "user",
			Yes:       true,
		})
		clie2e.ReportCleanupFailure(parentT, "delete list-comments docx "+docToken, deleteResult, deleteErr)
	})

	fetchResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"docs", "+fetch",
			"--api-version", "v2",
			"--doc", docToken,
			"--doc-format", "xml",
			"--detail", "with-ids",
		},
		DefaultAs: "user",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	fetchResult.AssertExitCode(t, 0)

	docXML := gjson.Get(fetchResult.Stdout, "data.document.content").String()
	require.NotEmpty(t, docXML, "stdout:\n%s", fetchResult.Stdout)
	blockID := firstCommentableBlockID(t, docXML, docToken)
	require.NotEmpty(t, blockID, "doc XML:\n%s", docXML)

	commentContent := mustCommentContentJSON(t, commentText)
	commentResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"drive", "+add-comment",
			"--doc", docToken,
			"--type", "docx",
			"--block-id", blockID,
			"--content", commentContent,
		},
		DefaultAs: "user",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	commentResult.AssertExitCode(t, 0)
	commentResult.AssertStdoutStatus(t, true)

	commentID := gjson.Get(commentResult.Stdout, "data.comment_id").String()
	require.NotEmpty(t, commentID, "stdout:\n%s", commentResult.Stdout)

	listResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"drive", "+list-comments",
			"--doc", docToken,
			"--type", "docx",
			"--include-resolved",
			"--include-orphaned",
		},
		DefaultAs: "user",
	}, clie2e.RetryOptions{
		ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0 || !strings.Contains(result.Stdout, commentID)
		},
	})
	require.NoError(t, err)
	listResult.AssertExitCode(t, 0)

	item := findCommentItem(t, listResult.Stdout, commentID)
	require.Equal(t, "valid", item.Get("anchor_state").String(), "stdout:\n%s", listResult.Stdout)
	require.Equal(t, "relation_exact", item.Get("location_accuracy").String(), "stdout:\n%s", listResult.Stdout)
	require.Equal(t, blockID, item.Get("anchor_block_id").String(), "stdout:\n%s", listResult.Stdout)
	require.False(t, item.Get("content_deleted").Bool(), "stdout:\n%s", listResult.Stdout)
	require.True(t, item.Get("relation.relation").Exists(), "stdout:\n%s", listResult.Stdout)
}

func firstGJSON(raw string, paths ...string) string {
	for _, path := range paths {
		if value := strings.TrimSpace(gjson.Get(raw, path).String()); value != "" {
			return value
		}
	}
	return ""
}

func firstCommentableBlockID(t *testing.T, content string, docToken string) string {
	t.Helper()

	decoder := xml.NewDecoder(strings.NewReader(content))
	var fallback string
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		blockID := firstXMLAttrValue(start, "id", "block_id")
		if blockID == "" {
			continue
		}
		if fallback == "" {
			fallback = blockID
		}
		if blockID != docToken {
			return blockID
		}
	}
	return fallback
}

func firstXMLAttrValue(start xml.StartElement, names ...string) string {
	for _, name := range names {
		for _, attr := range start.Attr {
			if attr.Name.Local == name {
				return attr.Value
			}
		}
	}
	return ""
}

func mustCommentContentJSON(t *testing.T, text string) string {
	t.Helper()

	payload := []map[string]string{{"type": "text", "text": text}}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(raw)
}

func findCommentItem(t *testing.T, stdout string, commentID string) gjson.Result {
	t.Helper()

	items := gjson.Get(stdout, "data.items").Array()
	require.NotEmpty(t, items, "stdout:\n%s", stdout)
	for _, item := range items {
		if item.Get("comment_id").String() == commentID {
			return item
		}
	}
	t.Fatalf("comment %s not found in list output:\n%s", commentID, stdout)
	return gjson.Result{}
}
