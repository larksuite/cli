// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestNormalizeDriveFileSource(t *testing.T) {
	tests := []struct {
		name        string
		fileToken   string
		url         string
		wikiToken   string
		wantFile    string
		wantWiki    string
		wantParam   string
		wantErrSub  errs.Subtype
		wantErrPara string
	}{
		{
			name:      "bare file token",
			fileToken: "boxcnFileToken",
			wantFile:  "boxcnFileToken",
			wantParam: "--file-token",
		},
		{
			name:      "drive file url",
			url:       "https://example.feishu.cn/file/boxcnFileToken",
			wantFile:  "boxcnFileToken",
			wantParam: "--url",
		},
		{
			name:      "wiki url flagged for resolution",
			url:       "https://example.feishu.cn/wiki/wikiNodeToken",
			wantWiki:  "wikiNodeToken",
			wantParam: "--url",
		},
		{
			name:      "bare wiki token flagged for resolution",
			wikiToken: "wikiNodeToken",
			wantWiki:  "wikiNodeToken",
			wantParam: "--wiki-token",
		},
		{
			name:        "no input",
			wantErrSub:  errs.SubtypeInvalidArgument,
			wantErrPara: "--file-token",
		},
		{
			name:        "mutually exclusive file and wiki",
			fileToken:   "boxcnFileToken",
			wikiToken:   "wikiNodeToken",
			wantErrSub:  errs.SubtypeInvalidArgument,
			wantErrPara: "--file-token",
		},
		{
			name:        "mutually exclusive file and url",
			fileToken:   "boxcnFileToken",
			url:         "https://example.feishu.cn/file/boxcnFileToken",
			wantErrSub:  errs.SubtypeInvalidArgument,
			wantErrPara: "--file-token",
		},
		{
			name:        "mutually exclusive url and wiki",
			url:         "https://example.feishu.cn/wiki/wikiNodeToken",
			wikiToken:   "wikiNodeToken",
			wantErrSub:  errs.SubtypeInvalidArgument,
			wantErrPara: "--url",
		},
		{
			name:        "non-file document url rejected",
			url:         "https://example.feishu.cn/docx/docxToken",
			wantErrSub:  errs.SubtypeInvalidArgument,
			wantErrPara: "--url",
		},
		{
			name:        "file url path traversal rejected",
			url:         "https://example.feishu.cn/file/../admin",
			wantErrSub:  errs.SubtypeInvalidArgument,
			wantErrPara: "--url",
		},
		{
			name:        "file url percent-encoded traversal rejected",
			url:         "https://example.feishu.cn/file/%2e%2e/admin",
			wantErrSub:  errs.SubtypeInvalidArgument,
			wantErrPara: "--url",
		},
		{
			name:        "wiki url path traversal rejected",
			url:         "https://example.feishu.cn/wiki/../admin",
			wantErrSub:  errs.SubtypeInvalidArgument,
			wantErrPara: "--url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, err := normalizeDriveFileSource(tt.fileToken, tt.url, tt.wikiToken)
			if tt.wantErrSub != "" {
				if err == nil {
					t.Fatalf("expected error, got source %+v", source)
				}
				problem, ok := errs.ProblemOf(err)
				if !ok {
					t.Fatalf("expected typed error, got %v", err)
				}
				if problem.Category != errs.CategoryValidation {
					t.Fatalf("category=%q, want %q", problem.Category, errs.CategoryValidation)
				}
				if problem.Subtype != tt.wantErrSub {
					t.Fatalf("subtype=%q, want %q", problem.Subtype, tt.wantErrSub)
				}
				var verr *errs.ValidationError
				if !errors.As(err, &verr) {
					t.Fatalf("expected *errs.ValidationError, got %T", err)
				}
				if verr.Param != tt.wantErrPara {
					t.Fatalf("param=%q, want %q", verr.Param, tt.wantErrPara)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if source.FileToken != tt.wantFile {
				t.Fatalf("FileToken=%q, want %q", source.FileToken, tt.wantFile)
			}
			if source.WikiToken != tt.wantWiki {
				t.Fatalf("WikiToken=%q, want %q", source.WikiToken, tt.wantWiki)
			}
			if source.InputParam != tt.wantParam {
				t.Fatalf("InputParam=%q, want %q", source.InputParam, tt.wantParam)
			}
			if source.NeedsWikiResolution() != (tt.wantWiki != "") {
				t.Fatalf("NeedsWikiResolution=%v, want %v", source.NeedsWikiResolution(), tt.wantWiki != "")
			}
		})
	}
}

func TestAnnotateDriveFileWikiOutput(t *testing.T) {
	base := map[string]interface{}{"saved_path": "/tmp/x"}

	// Unresolved: passthrough untouched — the wiki keys must be absent, not
	// present-with-nil (which would serialize as "wiki_token": null).
	got := annotateDriveFileWikiOutput(base, driveFileWikiResolution{})
	if _, ok := got["wiki_token"]; ok {
		t.Fatalf("expected no wiki_token key when unresolved, got %+v", got)
	}
	if _, ok := got["wiki_node"]; ok {
		t.Fatalf("expected no wiki_node key when unresolved, got %+v", got)
	}

	out := annotateDriveFileWikiOutput(map[string]interface{}{"saved_path": "/tmp/x"}, driveFileWikiResolution{
		Resolved:  true,
		WikiToken: "wikiNodeToken",
		ObjToken:  "boxcnFileToken",
		ObjType:   "file",
	})
	if out["wiki_token"] != "wikiNodeToken" {
		t.Fatalf("wiki_token=%v, want wikiNodeToken", out["wiki_token"])
	}
	node, ok := out["wiki_node"].(map[string]interface{})
	if !ok {
		t.Fatalf("wiki_node missing or wrong type: %+v", out["wiki_node"])
	}
	if node["obj_token"] != "boxcnFileToken" || node["obj_type"] != "file" {
		t.Fatalf("wiki_node=%+v, want obj_token=boxcnFileToken obj_type=file", node)
	}
}
