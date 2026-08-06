// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func newMinutesFetchRuntime(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
	return newMinutesFetchRuntimeWithScopes(t, "")
}

type minutesFetchTokenResolver struct {
	scopes string
}

func (r *minutesFetchTokenResolver) ResolveToken(context.Context, credential.TokenSpec) (*credential.TokenResult, error) {
	return &credential.TokenResult{Token: "test-token", Scopes: r.scopes}, nil
}

func newMinutesFetchRuntimeWithScopes(t *testing.T, scopes string) (*common.RuntimeContext, *httpmock.Registry) {
	t.Helper()
	cfg := defaultConfig()
	factory, _, _, registry := cmdutil.TestFactory(t, cfg)
	factory.Credential = credential.NewCredentialProvider(nil, nil, &minutesFetchTokenResolver{scopes: scopes}, factory.HttpClient)
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+fetch"}, cfg, factory, core.AsUser)
	return runtime, registry
}

func registerMinutesFetchCore(registry *httpmock.Registry, token, noteID, transcript string) {
	registry.Register(detailMinuteGetStub(token, noteID, "Test Meeting"))
	registry.Register(detailArtifactsStub(token, transcript))
}

func TestFetchMinutesMarkdownUsesArtifactTranscript(t *testing.T) {
	runtime, registry := newMinutesFetchRuntime(t)
	registerMinutesFetchCore(registry, "toktranscript", "", "Speaker: hello")

	result, err := FetchMinutesMarkdown(context.Background(), runtime, "toktranscript", map[string]bool{"transcript": true})
	if err != nil {
		t.Fatalf("FetchMinutesMarkdown() error = %v", err)
	}
	if !strings.Contains(result.Content, "## 逐字稿\n\nSpeaker: hello") {
		t.Fatalf("Content does not include artifact transcript:\n%s", result.Content)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("Warnings = %v, want none", result.Warnings)
	}
}

func TestFetchMinutesMarkdownReturnsStructuredNoteDocuments(t *testing.T) {
	runtime, registry := newMinutesFetchRuntime(t)
	registerMinutesFetchCore(registry, "toknote", "note123", "")
	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/notes/note123",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"note": map[string]interface{}{
					"artifacts": []interface{}{
						map[string]interface{}{"artifact_type": 1, "doc_token": "doc-main"},
						map[string]interface{}{"artifact_type": 2, "doc_token": "doc-verbatim"},
					},
				},
			},
		},
	})

	result, err := FetchMinutesMarkdown(context.Background(), runtime, "toknote", map[string]bool{"note-doc": true})
	if err != nil {
		t.Fatalf("FetchMinutesMarkdown() error = %v", err)
	}
	if result.NoteID != "note123" || result.NoteDocToken != "doc-main" || result.VerbatimDocToken != "doc-verbatim" {
		t.Fatalf("note fields = %#v", result)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("Warnings = %v, want none", result.Warnings)
	}
}

func TestFetchMinutesMarkdownNoteFailureDegradesToWarning(t *testing.T) {
	runtime, registry := newMinutesFetchRuntime(t)
	registerMinutesFetchCore(registry, "toknotefail", "note404", "")
	registry.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/vc/v1/notes/note404", Status: 500})

	result, err := FetchMinutesMarkdown(context.Background(), runtime, "toknotefail", map[string]bool{"note-doc": true})
	if err != nil {
		t.Fatalf("FetchMinutesMarkdown() error = %v", err)
	}
	if result.NoteID != "note404" || result.NoteDocToken != "" || result.VerbatimDocToken != "" {
		t.Fatalf("note fields = %#v", result)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "note documents omitted") {
		t.Fatalf("Warnings = %v", result.Warnings)
	}
}

func TestFetchMinutesMarkdownMissingNoteScopeKeepsCoreContent(t *testing.T) {
	runtime, registry := newMinutesFetchRuntimeWithScopes(t,
		"minutes:minutes.basic:read minutes:minutes.artifacts:read")
	registerMinutesFetchCore(registry, "toknotescope", "note123", "")

	result, err := FetchMinutesMarkdown(context.Background(), runtime, "toknotescope", map[string]bool{"note-doc": true})
	if err != nil {
		t.Fatalf("FetchMinutesMarkdown() error = %v", err)
	}
	if !strings.Contains(result.Content, "Test Meeting") {
		t.Fatalf("core content was lost: %q", result.Content)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "vc:note:read") ||
		!strings.Contains(result.Warnings[0], "auth login") {
		t.Fatalf("Warnings = %v, want actionable missing-scope warning", result.Warnings)
	}
	if result.NoteID != "note123" || result.NoteDocToken != "" || result.VerbatimDocToken != "" {
		t.Fatalf("note fields = %#v", result)
	}
}

// TestRenderChapters_PreservesAPIOrderWhenTimestampMissing guards the sort fix:
// a timestamp-less chapter must not jump to the front of the meeting (the
// 0-fallback bug). When any chapter lacks a timestamp, the API order is kept.
func TestRenderChapters_PreservesAPIOrderWhenTimestampMissing(t *testing.T) {
	t.Parallel()
	chapters := []interface{}{
		map[string]interface{}{"title": "First", "start_ms": "5000"},
		map[string]interface{}{"title": "Untimed"}, // no timestamp
		map[string]interface{}{"title": "Third", "start_ms": "10000"},
	}
	got := renderChapters(chapters)

	pos := func(name string) int { return strings.Index(got, name) }
	if !(pos("First") < pos("Untimed") && pos("Untimed") < pos("Third")) {
		t.Fatalf("API order not preserved (First=%d Untimed=%d Third=%d):\n%s",
			pos("First"), pos("Untimed"), pos("Third"), got)
	}
}

// TestRenderChapters_SortsWhenAllTimed confirms chapters sort by start time
// when every chapter carries a timestamp (the API may deliver out of order).
func TestRenderChapters_SortsWhenAllTimed(t *testing.T) {
	t.Parallel()
	chapters := []interface{}{
		map[string]interface{}{"title": "Late", "start_ms": "20000"},
		map[string]interface{}{"title": "Early", "start_ms": "3000"},
		map[string]interface{}{"title": "Mid", "start_ms": "10000"},
	}
	got := renderChapters(chapters)

	pos := func(name string) int { return strings.Index(got, name) }
	if !(pos("Early") < pos("Mid") && pos("Mid") < pos("Late")) {
		t.Fatalf("timed chapters not sorted by start (Early=%d Mid=%d Late=%d):\n%s",
			pos("Early"), pos("Mid"), pos("Late"), got)
	}
}
