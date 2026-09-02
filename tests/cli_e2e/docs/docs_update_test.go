// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDocs_UpdateWorkflow tests the create, update, and verify lifecycle.
func TestDocs_UpdateWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	folderName := "lark-cli-e2e-update-folder-" + suffix
	originalTitle := "lark-cli-e2e-update-" + suffix
	updatedTitle := "lark-cli-e2e-update-updated-" + suffix
	originalContent := "# Original\n\nThis is the original content."
	updatedContent := "# Updated\n\nThis is the updated content."
	const defaultAs = "bot"

	folderToken := drive.CreateDriveFolder(t, parentT, ctx, folderName, defaultAs, "")
	var docToken string

	t.Run("create as bot", func(t *testing.T) {
		docToken = createDocWithRetry(t, parentT, ctx, folderToken, originalTitle, originalContent, defaultAs)
	})

	t.Run("update-title-and-content as bot", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before update")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+update",
				"--doc", docToken,
				"--command", "overwrite",
				"--doc-format", "markdown",
				"--content", "# " + updatedTitle + "\n\n" + updatedContent,
			},
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("verify as bot", func(t *testing.T) {
		require.NotEmpty(t, docToken, "document token should be created before verify")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+fetch",
				"--doc", docToken,
				"--doc-format", "markdown",
			},
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		content := gjson.Get(result.Stdout, "data.document.content").String()
		assert.Contains(t, content, updatedTitle)
		assert.Contains(t, content, "This is the updated content.")
	})
}

// TestDocs_BlockMutationRangeWorkflow proves inclusive range replace and delete
// against the real docs service with bot credentials.
func TestDocs_BlockMutationRangeWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	const defaultAs = "bot"
	replaceTargets := []string{
		"range-replace-a-" + suffix,
		"range-replace-b-" + suffix,
		"range-replace-c-" + suffix,
	}
	deleteTargets := []string{
		"range-delete-a-" + suffix,
		"range-delete-b-" + suffix,
		"range-delete-c-" + suffix,
	}
	replacement := "range-replaced-" + suffix

	folderToken := drive.CreateDriveFolder(t, parentT, ctx, "lark-cli-e2e-range-folder-"+suffix, defaultAs, "")
	docToken := createDocWithRetry(t, parentT, ctx, folderToken, "lark-cli-e2e-range-"+suffix,
		strings.Join(append(append([]string{}, replaceTargets...), deleteTargets...), "\n\n"), defaultAs)

	initialXML, err := fetchDocsContent(ctx, docToken, "xml", "with-ids", defaultAs)
	require.NoError(t, err)
	replaceStart, err := docBlockIDByExactText(initialXML, replaceTargets[0])
	require.NoError(t, err)
	replaceEnd, err := docBlockIDByExactText(initialXML, replaceTargets[len(replaceTargets)-1])
	require.NoError(t, err)

	replaceResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+update",
			"--doc", docToken,
			"--command", "block_replace",
			"--start-block-id", replaceStart,
			"--end-block-id", replaceEnd,
			"--content", "<p>" + replacement + "</p>",
		},
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	replaceResult.AssertExitCode(t, 0)
	replaceResult.AssertStdoutStatus(t, true)

	var afterReplace string
	err = clie2e.WaitForCondition(ctx, clie2e.WaitOptions{Timeout: 30 * time.Second, Interval: time.Second}, func() (bool, error) {
		afterReplace, err = fetchDocsContent(ctx, docToken, "markdown", "", defaultAs)
		if err != nil {
			return false, err
		}
		return strings.Contains(afterReplace, replacement) && !containsAny(afterReplace, replaceTargets), nil
	})
	require.NoError(t, err, "range replacement did not become visible; content:\n%s", afterReplace)

	latestXML, err := fetchDocsContent(ctx, docToken, "xml", "with-ids", defaultAs)
	require.NoError(t, err)
	deleteStart, err := docBlockIDByExactText(latestXML, deleteTargets[0])
	require.NoError(t, err)
	deleteEnd, err := docBlockIDByExactText(latestXML, deleteTargets[len(deleteTargets)-1])
	require.NoError(t, err)

	deleteResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"docs", "+update",
			"--doc", docToken,
			"--command", "block_delete",
			"--start-block-id", deleteStart,
			"--end-block-id", deleteEnd,
		},
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	deleteResult.AssertExitCode(t, 0)
	deleteResult.AssertStdoutStatus(t, true)

	var afterDelete string
	err = clie2e.WaitForCondition(ctx, clie2e.WaitOptions{Timeout: 30 * time.Second, Interval: time.Second}, func() (bool, error) {
		afterDelete, err = fetchDocsContent(ctx, docToken, "markdown", "", defaultAs)
		if err != nil {
			return false, err
		}
		return strings.Contains(afterDelete, replacement) && !containsAny(afterDelete, deleteTargets), nil
	})
	require.NoError(t, err, "range deletion did not become visible; content:\n%s", afterDelete)
}

func fetchDocsContent(ctx context.Context, docToken, format, detail, defaultAs string) (string, error) {
	args := []string{"docs", "+fetch", "--doc", docToken, "--doc-format", format}
	if detail != "" {
		args = append(args, "--detail", detail)
	}
	result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: defaultAs})
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("docs fetch failed: exit=%d stdout=%s stderr=%s", result.ExitCode, result.Stdout, result.Stderr)
	}
	content := gjson.Get(result.Stdout, "data.document.content").String()
	if content == "" {
		return "", fmt.Errorf("docs fetch returned empty content: %s", result.Stdout)
	}
	return content, nil
}

func docBlockIDByExactText(content, target string) (string, error) {
	type elementFrame struct {
		id   string
		text strings.Builder
	}

	decoder := xml.NewDecoder(strings.NewReader(content))
	stack := make([]*elementFrame, 0, 8)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse fetched DocxXML: %w", err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			frame := &elementFrame{}
			for _, attr := range token.Attr {
				if attr.Name.Local == "id" {
					frame.id = attr.Value
					break
				}
			}
			stack = append(stack, frame)
		case xml.CharData:
			for _, frame := range stack {
				_, _ = frame.text.Write(token)
			}
		case xml.EndElement:
			if len(stack) == 0 {
				continue
			}
			frame := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if frame.id != "" && strings.TrimSpace(frame.text.String()) == target {
				return frame.id, nil
			}
		}
	}
	return "", fmt.Errorf("no block with exact text %q in fetched DocxXML", target)
}

func containsAny(content string, values []string) bool {
	for _, value := range values {
		if strings.Contains(content, value) {
			return true
		}
	}
	return false
}
