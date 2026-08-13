// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docs

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// By default the live workflows create their own documents and comments. A
// read-only document fixture can be supplied when the environment does not
// permit comment creation (for example, a BOE document with a large corpus of
// real comments).
func TestDocsFetchCommentsWorkflowAsBot(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	testDocsFetchCommentsWorkflow(t, "bot")
}

func TestDocsFetchCommentsWorkflowAsUser(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)
	testDocsFetchCommentsWorkflow(t, "user")
}

// TestDocsFetchCommentsDeniedDocumentDoesNotLeakToBot creates a user-owned
// document that is not shared with the bot, then verifies that the bot cannot
// obtain either the body or its comments. This is intentionally a separate
// workflow from the positive fixture: permission denial must be proven with a
// real document ACL, not an empty/invalid --doc argument.
func TestDocsFetchCommentsDeniedDocumentDoesNotLeakToBot(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)
	clie2e.SkipWithoutTenantAccessToken(t)
	if os.Getenv("LARK_DOCS_FETCH_COMMENTS_E2E") == "" {
		t.Skip("set LARK_DOCS_FETCH_COMMENTS_E2E=1 to run the document comment fetch workflow")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	anchorText := "private document anchor " + suffix
	commentText := "private document comment " + suffix
	folderToken := drive.CreateDriveFolder(t, t, ctx, "lark-cli-e2e-fetch-comments-denied-"+suffix, "user", "")
	docToken := createDocWithRetry(t, t, ctx, folderToken, "fetch comments denied "+suffix, anchorText, "user")
	addDocComment(t, ctx, "user", docToken, commentText, "--full-comment")
	ownerResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"docs", "+fetch", "--doc", docToken, "--doc-format", "xml"},
		DefaultAs: "user",
	}, clie2e.RetryOptions{ShouldRetry: func(result *clie2e.Result) bool {
		return result == nil || result.ExitCode != 0 ||
			!docsFetchReferenceGroupContains(result.Stdout, "comments", commentText)
	}})
	require.NoError(t, err)
	ownerResult.AssertExitCode(t, 0)
	ownerResult.AssertStdoutStatus(t, true)
	require.Equal(t, "user", gjson.Get(ownerResult.Stdout, "identity").String())
	require.Contains(t, gjson.Get(ownerResult.Stdout, "data.document.content").String(), anchorText)
	require.True(t, docsFetchReferenceGroupContains(ownerResult.Stdout, "comments", commentText), "owner fetch must prove the private fixture and its comment exist")

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"docs", "+fetch", "--doc", docToken, "--doc-format", "xml"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	require.NotZero(t, result.ExitCode, "bot unexpectedly read a user-private document")

	combined := result.Stdout + "\n" + result.Stderr
	require.NotContains(t, combined, anchorText, "permission error leaked document body")
	require.NotContains(t, combined, commentText, "permission error leaked comment content")
	require.False(t, gjson.Get(result.Stdout, "data.document").Exists(), "permission error returned a document payload")
	require.Equal(t, "bot", gjson.Get(result.Stderr, "identity").String())
	require.Equal(t, "api", gjson.Get(result.Stderr, "error.type").String(), "denial must be a structured upstream API failure")
	require.EqualValues(t, 3380004, gjson.Get(result.Stderr, "error.code").Int(), "denial must be the ai_edit document-ACL error")
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "No permission to operate on this document")
	require.False(t, gjson.Get(result.Stderr, "error.retryable").Bool(), "document ACL denial must not be retryable")
}

func TestFirstCommentRefAnchor(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    docsFetchLocalAnchor
		wantErr bool
	}{
		{
			name:    "nested inline content",
			content: `<doc><p comment-refs="c1"> alpha <b comment-refs="c2">beta</b> gamma </p></doc>`,
			want:    docsFetchLocalAnchor{keyword: "alpha beta gamma", refs: []string{"c1", "c2"}},
		},
		{
			name:    "skip empty referenced block",
			content: `<doc><p comment-refs="c1"><img src="token"/></p><p comment-refs="c2">usable anchor</p></doc>`,
			want:    docsFetchLocalAnchor{keyword: "usable anchor", refs: []string{"c2"}},
		},
		{
			name:    "rune safe limit",
			content: `<doc><p comment-refs="c1">` + strings.Repeat("评", 81) + `</p></doc>`,
			want:    docsFetchLocalAnchor{keyword: strings.Repeat("评", 80), refs: []string{"c1"}},
		},
		{
			name:    "skip non unique keyword",
			content: `<doc><p comment-refs="c1">same</p><p>same</p><p comment-refs="c2">unique</p></doc>`,
			want:    docsFetchLocalAnchor{keyword: "unique", refs: []string{"c2"}},
		},
		{
			name:    "missing comment refs",
			content: `<doc><p>plain body</p></doc>`,
			wantErr: true,
		},
		{
			name:    "malformed document XML",
			content: `<doc><p comment-refs="c1">broken</doc>`,
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := firstCommentRefAnchor(test.content)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestAssertCommentXMLAcceptsDocumentAnchorContract(t *testing.T) {
	for _, test := range []struct {
		name      string
		data      string
		commentID string
		isWhole   bool
	}{
		{
			name:      "single block",
			data:      `<comment comment-id="101" block-id="blk1"><quote>text</quote><msg user="Reviewer">review</msg></comment>`,
			commentID: "101",
		},
		{
			name:      "cross block",
			data:      `<comment comment-id="102" start-block-id="blk1" end-block-id="blk2"><quote>text</quote><msg user="Reviewer">review</msg></comment>`,
			commentID: "102",
		},
		{
			name:      "whole document",
			data:      `<comment comment-id="103" is_whole="true"><msg user="Reviewer">review</msg></comment>`,
			commentID: "103",
			isWhole:   true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			shape := assertCommentXML(t, test.data)
			require.Equal(t, test.commentID, shape.commentID)
			require.Equal(t, test.isWhole, shape.isWhole)
		})
	}
}

func testDocsFetchCommentsWorkflow(t *testing.T, defaultAs string) {
	if os.Getenv("LARK_DOCS_FETCH_COMMENTS_E2E") == "" {
		t.Skip("set LARK_DOCS_FETCH_COMMENTS_E2E=1 to run the document comment fetch workflow")
	}
	if docToken := strings.TrimSpace(os.Getenv("LARK_DOCS_FETCH_COMMENTS_E2E_DOC")); docToken != "" {
		testDocsFetchCommentsReadOnlyFixture(t, defaultAs, docToken)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	parentT := t
	suffix := clie2e.GenerateSuffix()
	anchorText := "comment anchor " + suffix
	secondaryText := "secondary block " + suffix
	localText := "local review " + suffix
	secondaryLocalText := "secondary review " + suffix
	wholeText := "whole document review " + suffix

	// Both creation helpers register parentT cleanup immediately. Cleanup is
	// LIFO, so the document is deleted before its containing folder.
	folderToken := drive.CreateDriveFolder(t, parentT, ctx, "lark-cli-e2e-fetch-comments-"+suffix, defaultAs, "")
	docToken := createDocWithRetry(t, parentT, ctx, folderToken, "fetch comments "+suffix, anchorText+"\n\n"+secondaryText, defaultAs)

	localCommentID := addDocComment(t, ctx, defaultAs, docToken, localText, "--selection-with-ellipsis", anchorText)
	secondaryCommentID := addDocComment(t, ctx, defaultAs, docToken, secondaryLocalText, "--selection-with-ellipsis", secondaryText)
	wholeCommentID := addDocComment(t, ctx, defaultAs, docToken, wholeText, "--full-comment")

	var fetched *clie2e.Result
	t.Run("xml full", func(t *testing.T) {
		var err error
		fetched, err = clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args:      []string{"docs", "+fetch", "--doc", docToken, "--doc-format", "xml"},
			DefaultAs: defaultAs,
		}, clie2e.RetryOptions{ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0 ||
				!strings.Contains(gjson.Get(result.Stdout, "data.document.content").String(), "comment-refs=") ||
				!docsFetchReferenceGroupContains(result.Stdout, "comments", localText) ||
				!docsFetchReferenceGroupContains(result.Stdout, "comments", secondaryLocalText) ||
				!docsFetchReferenceGroupContains(result.Stdout, "comments", wholeText)
		}})
		require.NoError(t, err)
		fetched.AssertExitCode(t, 0)
		fetched.AssertStdoutStatus(t, true)
		require.Equal(t, defaultAs, gjson.Get(fetched.Stdout, "identity").String())
		if !strings.Contains(gjson.Get(fetched.Stdout, "data.document.content").String(), "comment-refs=") {
			t.Fatalf("local comment marker missing:\n%s", fetched.Stdout)
		}
		if !docsFetchReferenceGroupContains(fetched.Stdout, "comments", localText) ||
			!docsFetchReferenceGroupContains(fetched.Stdout, "comments", secondaryLocalText) ||
			!docsFetchReferenceGroupContains(fetched.Stdout, "comments", wholeText) {
			t.Fatalf("comment reference groups missing:\n%s", fetched.Stdout)
		}
		assertDocsFetchCommentContract(t, fetched.Stdout, []docsFetchExpectedComment{
			{text: localText, commentID: localCommentID, local: true},
			{text: secondaryLocalText, commentID: secondaryCommentID, local: true},
			{text: wholeText, commentID: wholeCommentID},
		})
	})

	t.Run("Markdown protocols return sidecars without inline anchors", func(t *testing.T) {
		for _, docFormat := range []string{"markdown", "im-markdown"} {
			t.Run(docFormat, func(t *testing.T) {
				result, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args:      []string{"docs", "+fetch", "--doc", docToken, "--doc-format", docFormat},
					DefaultAs: defaultAs,
				})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				require.Equal(t, defaultAs, gjson.Get(result.Stdout, "identity").String())
				assertDocsFetchMarkdownCommentContract(t, result.Stdout, []docsFetchExpectedComment{
					{text: localText, commentID: localCommentID, local: true},
					{text: secondaryLocalText, commentID: secondaryCommentID, local: true},
					{text: wholeText, commentID: wholeCommentID},
				})
			})
		}
	})

	t.Run("partial returns only intersecting local comment", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+fetch", "--doc", docToken, "--doc-format", "xml",
				"--scope", "keyword", "--keyword", anchorText,
			},
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Equal(t, defaultAs, gjson.Get(result.Stdout, "identity").String())
		if !docsFetchReferenceGroupContains(result.Stdout, "comments", localText) ||
			docsFetchReferenceGroupContains(result.Stdout, "comments", secondaryLocalText) ||
			docsFetchReferenceGroupContains(result.Stdout, "comments", wholeText) {
			t.Fatalf("partial comment filtering mismatch:\n%s", result.Stdout)
		}
		assertDocsFetchCommentContract(t, result.Stdout, []docsFetchExpectedComment{
			{text: localText, commentID: localCommentID, local: true},
		})
	})

	t.Run("pretty remains body only", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"docs", "+fetch", "--doc", docToken, "--doc-format", "xml", "--format", "pretty"},
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, anchorText)
		require.NotContains(t, result.Stdout, `"reference_map"`)
		require.NotContains(t, result.Stdout, localText)
		require.NotContains(t, result.Stdout, wholeText)
	})
}

func testDocsFetchCommentsReadOnlyFixture(t *testing.T, defaultAs, docToken string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	var fetched *clie2e.Result
	t.Run("xml full read only", func(t *testing.T) {
		var err error
		fetched, err = clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args:      []string{"docs", "+fetch", "--doc", docToken, "--doc-format", "xml"},
			DefaultAs: defaultAs,
		}, clie2e.RetryOptions{ShouldRetry: func(result *clie2e.Result) bool {
			return result == nil || result.ExitCode != 0 ||
				!docsFetchReferenceGroupExists(result.Stdout, "comments")
		}})
		require.NoError(t, err)
		fetched.AssertExitCode(t, 0)
		fetched.AssertStdoutStatus(t, true)
		require.Equal(t, defaultAs, gjson.Get(fetched.Stdout, "identity").String())
		summary := assertDocsFetchReadOnlyContract(t, fetched.Stdout, true)
		require.Positive(t, summary.localCount, "the shared fixture must contain local comments")
		require.Equal(t, 200, summary.wholeCount, "the large shared fixture must exercise the whole-comment cap")
		require.True(t, summary.truncated, "the large shared fixture must emit the truncation tip")
	})

	t.Run("Markdown protocols return sidecars without inline anchors", func(t *testing.T) {
		for _, docFormat := range []string{"markdown", "im-markdown"} {
			t.Run(docFormat, func(t *testing.T) {
				result, err := clie2e.RunCmd(ctx, clie2e.Request{
					Args:      []string{"docs", "+fetch", "--doc", docToken, "--doc-format", docFormat},
					DefaultAs: defaultAs,
				})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				result.AssertStdoutStatus(t, true)
				require.Equal(t, defaultAs, gjson.Get(result.Stdout, "identity").String())
				summary := assertDocsFetchMarkdownReadOnlyContract(t, result.Stdout, true)
				require.Positive(t, summary.localCount, "the shared fixture must contain local comments")
				require.Equal(t, 200, summary.wholeCount, "the large shared fixture must exercise the whole-comment cap")
				require.True(t, summary.truncated, "the large shared fixture must emit the truncation tip")
			})
		}
	})

	t.Run("partial returns only intersecting local comments", func(t *testing.T) {
		require.NotNil(t, fetched)
		anchor := firstLocalCommentAnchor(t, fetched.Stdout)
		expectedRootIDs := rootCommentIDsForRefs(t, fetched.Stdout, anchor.refs)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"docs", "+fetch", "--doc", docToken, "--doc-format", "xml",
				"--scope", "keyword", "--keyword", anchor.keyword,
			},
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, defaultAs, gjson.Get(result.Stdout, "identity").String())
		summary := assertDocsFetchReadOnlyContract(t, result.Stdout, false)
		require.Positive(t, summary.localCount, "keyword fetch must retain at least one intersecting local comment")
		require.Zero(t, summary.wholeCount, "keyword fetch must omit whole-document comments")
		require.False(t, summary.truncated, "keyword fetch must not emit full-document truncation tips")
		require.Equal(t, expectedRootIDs, localRootCommentIDs(t, result.Stdout), "keyword fetch must return exactly the comments attached to the selected unique body block")
	})

	t.Run("outline remains comment free", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"docs", "+fetch", "--doc", docToken, "--doc-format", "xml", "--scope", "outline", "--max-depth", "3"},
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, defaultAs, gjson.Get(result.Stdout, "identity").String())
		require.False(t, docsFetchReferenceGroupExists(result.Stdout, "comments"))
		require.NotContains(t, gjson.Get(result.Stdout, "data.document.content").String(), "comment-refs=")
	})

	t.Run("pretty remains body only", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"docs", "+fetch", "--doc", docToken, "--doc-format", "xml", "--format", "pretty"},
			DefaultAs: defaultAs,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.NotEmpty(t, strings.TrimSpace(result.Stdout))
		require.NotContains(t, result.Stdout, `"reference_map"`)
	})
}

type docsFetchCommentEnvelope struct {
	Data struct {
		Document struct {
			Content      string                                        `json:"content"`
			ReferenceMap map[string]map[string]docsFetchReferenceEntry `json:"reference_map"`
		} `json:"document"`
	} `json:"data"`
}

type docsFetchReferenceEntry struct {
	Data string `json:"data"`
}

type docsFetchExpectedComment struct {
	text      string
	commentID string
	local     bool
}

type docsFetchReadOnlySummary struct {
	localCount int
	wholeCount int
	truncated  bool
}

type docsFetchLocalAnchor struct {
	keyword string
	refs    []string
}

func assertDocsFetchReadOnlyContract(t *testing.T, stdout string, allowWhole bool) docsFetchReadOnlySummary {
	return assertDocsFetchReadOnlyContractForOutput(t, stdout, allowWhole, true)
}

func assertDocsFetchMarkdownReadOnlyContract(t *testing.T, stdout string, allowWhole bool) docsFetchReadOnlySummary {
	return assertDocsFetchReadOnlyContractForOutput(t, stdout, allowWhole, false)
}

func assertDocsFetchReadOnlyContractForOutput(t *testing.T, stdout string, allowWhole, expectInlineAnchors bool) docsFetchReadOnlySummary {
	t.Helper()

	var envelope docsFetchCommentEnvelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	document := envelope.Data.Document
	entries := document.ReferenceMap["comments"]
	require.NotEmpty(t, entries, "comments sidecar must not be empty")
	require.False(t, docsFetchReferenceGroupExists(stdout, "comment"), "legacy local-comment group must be absent")
	require.False(t, docsFetchReferenceGroupExists(stdout, "document-comment"), "legacy whole-comment group must be absent")

	localKeys := make(map[string]struct{})
	wholeKeys := make(map[string]struct{})
	truncated := false
	for key, entry := range entries {
		if key == "tips" {
			require.Equal(t, "Comments are truncated. Use the comment API to fetch complete content.", entry.Data)
			truncated = true
			continue
		}
		if !docsFetchCommentRefPattern.MatchString(key) {
			t.Fatalf("comment key %q is not an opaque cN surrogate", key)
		}
		shape := assertCommentXML(t, entry.Data)
		require.NotContains(t, entry.Data, "A-1(", "reaction users must expose compact names, never name(display)")
		if shape.isWhole {
			require.True(t, allowWhole, "partial fetch must not return whole-document comments")
			require.False(t, shape.hasQuote, "whole-document comments must not carry a quote")
			wholeKeys[key] = struct{}{}
			continue
		}
		require.True(t, shape.hasQuote, "local comments must carry a quote")
		localKeys[key] = struct{}{}
	}

	if strings.Contains(document.Content, "comment-ids=") {
		t.Fatal("raw Engine comment IDs must never reach the public document content")
	}
	if strings.Contains(document.Content, "<comment-ref") {
		t.Fatal("DocxXML output must use comment-refs attributes, not Markdown shells")
	}
	if !expectInlineAnchors {
		require.NotContains(t, document.Content, "comment-refs=", "Markdown body must not expose XML comment anchors")
		return docsFetchReadOnlySummary{localCount: len(localKeys), wholeCount: len(wholeKeys), truncated: truncated}
	}
	bodyRefs := collectCommentRefsFromXML(t, document.Content)
	require.Equal(t, localKeys, bodyRefs, "body refs and local sidecar keys must form an exact closure")
	for ref := range wholeKeys {
		if _, exists := bodyRefs[ref]; exists {
			t.Fatalf("whole-document comment %q must not be attached to a body block", ref)
		}
	}
	require.LessOrEqual(t, len(localKeys), 1000, "local comment cap must be enforced")
	require.LessOrEqual(t, len(wholeKeys), 200, "whole-document comment cap must be enforced")
	return docsFetchReadOnlySummary{localCount: len(localKeys), wholeCount: len(wholeKeys), truncated: truncated}
}

func firstLocalCommentAnchor(t *testing.T, stdout string) docsFetchLocalAnchor {
	t.Helper()

	var envelope docsFetchCommentEnvelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	anchor, err := firstCommentRefAnchor(envelope.Data.Document.Content)
	require.NoError(t, err)
	return anchor
}

type commentRefAnchorCapture struct {
	depth int
	refs  []string
	text  strings.Builder
}

func firstCommentRefAnchor(content string) (docsFetchLocalAnchor, error) {
	decoder := xml.NewDecoder(strings.NewReader(content))
	depth := 0
	var documentText strings.Builder
	var captures []*commentRefAnchorCapture
	var active []*commentRefAnchorCapture
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return docsFetchLocalAnchor{}, fmt.Errorf("parse document XML: %w", err)
		}
		switch typed := token.(type) {
		case xml.StartElement:
			depth++
			var refs []string
			for _, attr := range typed.Attr {
				if attr.Name.Local == "comment-refs" && strings.TrimSpace(attr.Value) != "" {
					refs = strings.Fields(attr.Value)
					break
				}
			}
			if len(refs) > 0 {
				for _, parent := range active {
					parent.refs = append(parent.refs, refs...)
				}
				capture := &commentRefAnchorCapture{depth: depth, refs: append([]string(nil), refs...)}
				captures = append(captures, capture)
				active = append(active, capture)
			}
		case xml.CharData:
			documentText.Write([]byte(typed))
			for _, capture := range active {
				capture.text.Write([]byte(typed))
			}
		case xml.EndElement:
			for i := len(active) - 1; i >= 0; i-- {
				if active[i].depth == depth {
					active = append(active[:i], active[i+1:]...)
				}
			}
			depth--
		}
	}

	normalizedDocumentText := strings.Join(strings.Fields(documentText.String()), " ")
	for _, capture := range captures {
		keyword := strings.Join(strings.Fields(capture.text.String()), " ")
		if keyword == "" {
			continue
		}
		runes := []rune(keyword)
		if len(runes) > 80 {
			keyword = string(runes[:80])
		}
		if strings.Count(normalizedDocumentText, keyword) != 1 {
			continue
		}
		return docsFetchLocalAnchor{keyword: keyword, refs: dedupeStrings(capture.refs)}, nil
	}
	return docsFetchLocalAnchor{}, fmt.Errorf("document has no unique non-empty block with comment-refs")
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func rootCommentIDsForRefs(t *testing.T, stdout string, refs []string) map[string]struct{} {
	t.Helper()
	var envelope docsFetchCommentEnvelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	entries := envelope.Data.Document.ReferenceMap["comments"]
	rootIDs := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		entry, exists := entries[ref]
		require.True(t, exists, "body comment ref %q must resolve in the full sidecar", ref)
		shape := assertCommentXML(t, entry.Data)
		require.False(t, shape.isWhole, "body comment ref %q must resolve to a local comment", ref)
		rootIDs[shape.commentID] = struct{}{}
	}
	return rootIDs
}

func localRootCommentIDs(t *testing.T, stdout string) map[string]struct{} {
	t.Helper()
	var envelope docsFetchCommentEnvelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	rootIDs := make(map[string]struct{})
	for key, entry := range envelope.Data.Document.ReferenceMap["comments"] {
		if key == "tips" {
			continue
		}
		shape := assertCommentXML(t, entry.Data)
		require.False(t, shape.isWhole, "keyword sidecar must contain only local comments")
		rootIDs[shape.commentID] = struct{}{}
	}
	return rootIDs
}

func assertDocsFetchCommentContract(t *testing.T, stdout string, expected []docsFetchExpectedComment) {
	assertDocsFetchCommentContractForOutput(t, stdout, expected, true)
}

func assertDocsFetchMarkdownCommentContract(t *testing.T, stdout string, expected []docsFetchExpectedComment) {
	assertDocsFetchCommentContractForOutput(t, stdout, expected, false)
}

func assertDocsFetchCommentContractForOutput(t *testing.T, stdout string, expected []docsFetchExpectedComment, expectInlineAnchors bool) {
	t.Helper()

	var envelope docsFetchCommentEnvelope
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	document := envelope.Data.Document
	entries := document.ReferenceMap["comments"]
	require.Len(t, entries, len(expected), "comments sidecar must contain only the fixture comments relevant to this fetch")
	require.False(t, docsFetchReferenceGroupExists(stdout, "comment"), "legacy local-comment group must be absent")
	require.False(t, docsFetchReferenceGroupExists(stdout, "document-comment"), "legacy whole-comment group must be absent")

	localKeys := make(map[string]struct{}, len(expected))
	wholeKeys := make(map[string]struct{}, len(expected))
	found := make(map[string]struct{}, len(expected))
	for key, entry := range entries {
		if !docsFetchCommentRefPattern.MatchString(key) {
			t.Fatalf("comment key %q is not an opaque cN surrogate", key)
		}
		shape := assertCommentXML(t, entry.Data)
		matched := false
		for _, want := range expected {
			if !strings.Contains(entry.Data, want.text) {
				continue
			}
			require.Equal(t, want.commentID, shape.commentID, "comment must expose the root ID returned by comment creation")
			require.Equal(t, want.local, shape.hasQuote, "only local comments carry a quote")
			require.Equal(t, !want.local, shape.isWhole, "only whole-document comments carry is_whole=true")
			found[want.text] = struct{}{}
			if want.local {
				localKeys[key] = struct{}{}
			} else {
				wholeKeys[key] = struct{}{}
			}
			matched = true
			break
		}
		if !matched {
			t.Fatalf("unexpected comment %q in comments sidecar: %s", key, entry.Data)
		}
	}
	for _, want := range expected {
		if _, ok := found[want.text]; !ok {
			t.Fatalf("expected comment containing %q was not returned", want.text)
		}
	}

	if strings.Contains(document.Content, "comment-ids=") {
		t.Fatal("raw Engine comment IDs must never reach the public document content")
	}
	if strings.Contains(document.Content, "<comment-ref") {
		t.Fatal("DocxXML output must use comment-refs attributes, not Markdown shells")
	}
	if !expectInlineAnchors {
		require.NotContains(t, document.Content, "comment-refs=", "Markdown body must not expose XML comment anchors")
		return
	}
	bodyRefs := collectCommentRefsFromXML(t, document.Content)
	require.Equal(t, localKeys, bodyRefs, "body refs and local sidecar keys must form an exact closure")
	for ref := range wholeKeys {
		if _, exists := bodyRefs[ref]; exists {
			t.Fatalf("whole-document comment %q must not be attached to a body block", ref)
		}
	}
}

var (
	docsFetchCommentRefPattern = regexp.MustCompile(`^c[1-9][0-9]*$`)
	docsFetchCommentIDPattern  = regexp.MustCompile(`^[1-9][0-9]*$`)
)

func collectCommentRefsFromXML(t *testing.T, fragment string) map[string]struct{} {
	t.Helper()

	refs := make(map[string]struct{})
	decoder := xml.NewDecoder(strings.NewReader("<root>" + fragment + "</root>"))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "comment marker XML must be well-formed")
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		for _, attr := range start.Attr {
			if attr.Name.Local != "comment-refs" {
				continue
			}
			for _, ref := range strings.Fields(attr.Value) {
				if !docsFetchCommentRefPattern.MatchString(ref) {
					t.Fatalf("body comment ref %q is not an opaque cN surrogate", ref)
				}
				refs[ref] = struct{}{}
			}
		}
	}
	return refs
}

type docsFetchCommentShape struct {
	hasQuote  bool
	commentID string
	isWhole   bool
}

func assertCommentXML(t *testing.T, data string) docsFetchCommentShape {
	t.Helper()

	allowedAttrOrder := map[string][]string{
		"comment":  {"comment-id", "block-id", "start-block-id", "end-block-id", "is_whole"},
		"quote":    {},
		"msg":      {"user"},
		"img":      {"src"},
		"cite":     {"type", "doc-id"},
		"reaction": {"key", "users", "count", "partial"},
	}
	decoder := xml.NewDecoder(strings.NewReader(data))
	stack := make([]string, 0, 3)
	sawRoot := false
	sawQuote := false
	messageCount := 0
	commentID := ""
	isWhole := false
	blockID := ""
	startBlockID := ""
	endBlockID := ""
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "comment sidecar must be well-formed XML")
		switch typed := token.(type) {
		case xml.StartElement:
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			switch typed.Name.Local {
			case "comment":
				require.Empty(t, parent, "comment must be the single root")
				require.False(t, sawRoot, "comment sidecar must have one root")
				sawRoot = true
			case "quote", "msg":
				require.Equal(t, "comment", parent, "%s must be a direct comment child", typed.Name.Local)
			case "img", "cite", "reaction":
				require.Equal(t, "msg", parent, "%s must belong to one msg", typed.Name.Local)
			default:
				t.Fatalf("unexpected comment element <%s>", typed.Name.Local)
			}

			allowed := allowedAttrOrder[typed.Name.Local]
			lastPosition := -1
			for _, attr := range typed.Attr {
				position := -1
				for i, name := range allowed {
					if attr.Name.Local == name {
						position = i
						break
					}
				}
				if position < 0 {
					t.Fatalf("unexpected %s attribute %q", typed.Name.Local, attr.Name.Local)
				}
				if position <= lastPosition {
					t.Fatalf("%s attributes are out of contract order: %#v", typed.Name.Local, typed.Attr)
				}
				lastPosition = position
				if typed.Name.Local == "comment" {
					switch attr.Name.Local {
					case "comment-id":
						commentID = attr.Value
					case "block-id":
						blockID = attr.Value
					case "start-block-id":
						startBlockID = attr.Value
					case "end-block-id":
						endBlockID = attr.Value
					case "is_whole":
						require.Equal(t, "true", attr.Value, "is_whole must use the true literal when present")
						isWhole = true
					}
				}
				if typed.Name.Local == "cite" && attr.Name.Local == "type" {
					require.Equal(t, "doc", attr.Value, "cite type must be doc")
				}
			}
			if typed.Name.Local == "comment" {
				require.NotEmpty(t, typed.Attr, "comment must include comment-id")
				require.Equal(t, "comment-id", typed.Attr[0].Name.Local, "comment-id must be the first attribute")
			}
			if typed.Name.Local == "quote" {
				require.False(t, sawQuote, "comment may contain at most one quote")
				sawQuote = true
			}
			if typed.Name.Local == "msg" {
				require.Len(t, typed.Attr, 1, "msg must include only user")
				require.Equal(t, "user", typed.Attr[0].Name.Local, "msg must include user even when the name is empty")
				messageCount++
			}
			if typed.Name.Local == "img" {
				require.Len(t, typed.Attr, 1, "img must include only src")
				require.Equal(t, "src", typed.Attr[0].Name.Local)
				require.NotEmpty(t, typed.Attr[0].Value)
			}
			if typed.Name.Local == "cite" {
				require.Len(t, typed.Attr, 2, "cite must include type and doc-id")
				require.Equal(t, "doc-id", typed.Attr[1].Name.Local)
				require.NotEmpty(t, typed.Attr[1].Value)
			}
			if typed.Name.Local == "reaction" {
				require.NotEmpty(t, typed.Attr, "reaction must include key")
				require.Equal(t, "key", typed.Attr[0].Name.Local, "reaction key must be the first attribute")
				require.NotEmpty(t, strings.TrimSpace(typed.Attr[0].Value))
			}
			stack = append(stack, typed.Name.Local)
		case xml.EndElement:
			require.NotEmpty(t, stack, "unexpected closing element </%s>", typed.Name.Local)
			require.Equal(t, stack[len(stack)-1], typed.Name.Local, "comment elements must close in order")
			stack = stack[:len(stack)-1]
		}
	}
	require.True(t, sawRoot)
	require.Empty(t, stack)
	require.True(t, docsFetchCommentIDPattern.MatchString(commentID), "comment must expose a positive numeric comment-id")
	require.False(t, blockID != "" && (startBlockID != "" || endBlockID != ""), "single-block and range anchors must not be combined")
	require.Equal(t, startBlockID == "", endBlockID == "", "start-block-id and end-block-id must appear together")
	if blockID != "" {
		require.NotEmpty(t, strings.TrimSpace(blockID), "block-id must not be blank")
	}
	if startBlockID != "" {
		require.NotEmpty(t, strings.TrimSpace(startBlockID), "start-block-id must not be blank")
		require.NotEmpty(t, strings.TrimSpace(endBlockID), "end-block-id must not be blank")
	}
	if isWhole {
		require.Empty(t, blockID, "whole-document comments must not have block-id")
		require.Empty(t, startBlockID, "whole-document comments must not have start-block-id")
		require.Empty(t, endBlockID, "whole-document comments must not have end-block-id")
	} else {
		require.True(t, blockID != "" || startBlockID != "",
			"local comments must carry block-id or the start-block-id/end-block-id pair")
	}
	require.Positive(t, messageCount, "comment must contain at least one msg")
	return docsFetchCommentShape{hasQuote: sawQuote, commentID: commentID, isWhole: isWhole}
}

func addDocComment(t *testing.T, ctx context.Context, defaultAs, docToken, text string, locationArgs ...string) string {
	t.Helper()
	content, err := json.Marshal([]map[string]string{{"type": "text", "text": text}})
	require.NoError(t, err)
	args := []string{
		"drive", "+add-comment",
		"--doc", docToken,
		"--type", "docx",
		"--content", string(content),
	}
	args = append(args, locationArgs...)
	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{Args: args, DefaultAs: defaultAs}, clie2e.RetryOptions{})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	commentID := gjson.Get(result.Stdout, "data.comment_id").String()
	require.NotEmpty(t, commentID, "comment creation must return data.comment_id: %s", result.Stdout)
	return commentID
}

func docsFetchReferenceGroupExists(stdout, group string) bool {
	return gjson.Get(stdout, "data.document.reference_map").Get(group).Exists()
}

func docsFetchReferenceGroupContains(stdout, group, text string) bool {
	for _, entry := range gjson.Get(stdout, "data.document.reference_map").Get(group).Map() {
		if strings.Contains(entry.Get("data").String(), text) {
			return true
		}
	}
	return false
}
