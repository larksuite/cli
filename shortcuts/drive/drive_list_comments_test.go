// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestResolveDriveListCommentsInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		urlInput     string
		rawInput     string
		docType      string
		wantResource string
		wantType     string
		wantErr      string
		wantParam    string
	}{
		{
			name:         "url docx",
			urlInput:     "https://example.larksuite.com/docx/docxResource?from=wiki",
			wantResource: "docxResource",
			wantType:     "docx",
		},
		{
			name:         "token flag also accepts url",
			rawInput:     "https://example.larksuite.com/base/bitableResource",
			wantResource: "bitableResource",
			wantType:     "bitable",
		},
		{
			name:         "bare wiki token",
			rawInput:     "wikiResource",
			docType:      "wiki",
			wantResource: "wikiResource",
			wantType:     "wiki",
		},
		{
			name:      "url and token mutually exclusive",
			urlInput:  "https://example.larksuite.com/docx/docxResource",
			rawInput:  "docxResource",
			wantErr:   "mutually exclusive",
			wantParam: "--url",
		},
		{
			name:      "bare token needs type",
			rawInput:  "docxResource",
			wantErr:   "--type is required",
			wantParam: "--type",
		},
		{
			name:      "type conflicts with url",
			urlInput:  "https://example.larksuite.com/wiki/wikiResource",
			docType:   "docx",
			wantErr:   "conflicts",
			wantParam: "--type",
		},
		{
			name:      "unsupported url type",
			urlInput:  "https://example.larksuite.com/drive/folder/folderResource",
			wantErr:   "unsupported",
			wantParam: "--url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveDriveListCommentsInput(tt.urlInput, tt.rawInput, tt.docType)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				assertDriveListCommentsValidationError(t, err, tt.wantParam)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Token != tt.wantResource || got.Type != tt.wantType {
				t.Fatalf("got (%q, %q), want (%q, %q)", got.Token, got.Type, tt.wantResource, tt.wantType)
			}
		})
	}
}

func TestValidateDriveListCommentsSpec(t *testing.T) {
	t.Parallel()

	valid := driveListCommentsSpec{
		PageSize:     50,
		SolvedStatus: "false",
		CommentScope: "all",
	}

	tests := []struct {
		name      string
		mutate    func(*driveListCommentsSpec)
		wantParam string
	}{
		{
			name: "invalid page size",
			mutate: func(spec *driveListCommentsSpec) {
				spec.PageSize = 0
			},
			wantParam: "--page-size",
		},
		{
			name: "invalid solved status",
			mutate: func(spec *driveListCommentsSpec) {
				spec.SolvedStatus = "open"
			},
			wantParam: "--solved-status",
		},
		{
			name: "invalid comment scope",
			mutate: func(spec *driveListCommentsSpec) {
				spec.CommentScope = "inline"
			},
			wantParam: "--comment-scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec := valid
			tt.mutate(&spec)
			err := validateDriveListCommentsSpec(spec)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			assertDriveListCommentsValidationError(t, err, tt.wantParam)
		})
	}
}

func assertDriveListCommentsValidationError(t *testing.T, err error, wantParam string) {
	t.Helper()

	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if validationErr.Category != errs.CategoryValidation {
		t.Fatalf("category = %q, want %q", validationErr.Category, errs.CategoryValidation)
	}
	if validationErr.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", validationErr.Subtype, errs.SubtypeInvalidArgument)
	}
	if validationErr.Param != wantParam {
		t.Fatalf("param = %q, want %q", validationErr.Param, wantParam)
	}
	if cause := errors.Unwrap(err); cause != nil {
		t.Fatalf("unexpected cause on direct validation error: %v", cause)
	}

	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected errs.ProblemOf to recognize typed error: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("problem category = %q, want %q", problem.Category, errs.CategoryValidation)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem subtype = %q, want %q", problem.Subtype, errs.SubtypeInvalidArgument)
	}
}

func TestBuildDriveListCommentsParams(t *testing.T) {
	t.Parallel()

	defaultSpec := driveListCommentsSpec{
		PageSize:     50,
		SolvedStatus: "false",
		CommentScope: "all",
	}
	defaultParams := buildDriveListCommentsParams(defaultSpec, "docx")
	if got := defaultParams["is_solved"]; got != false {
		t.Fatalf("default is_solved = %#v, want false", got)
	}
	if _, ok := defaultParams["is_whole"]; ok {
		t.Fatalf("default params should omit is_whole: %#v", defaultParams)
	}
	if _, ok := defaultParams["user_id_type"]; ok {
		t.Fatalf("default params should omit user_id_type: %#v", defaultParams)
	}

	allPartialSpec := driveListCommentsSpec{
		PageSize:     100,
		PageToken:    "next",
		SolvedStatus: "all",
		CommentScope: "partial",
		NeedReaction: true,
		NeedRelation: true,
	}
	allPartialParams := buildDriveListCommentsParams(allPartialSpec, "docx")
	if _, ok := allPartialParams["is_solved"]; ok {
		t.Fatalf("solved-status all should omit is_solved: %#v", allPartialParams)
	}
	if got := allPartialParams["is_whole"]; got != false {
		t.Fatalf("comment-scope partial is_whole = %#v, want false", got)
	}
	if got := allPartialParams["need_reaction"]; got != true {
		t.Fatalf("need_reaction = %#v, want true", got)
	}
	if got := allPartialParams["need_relation"]; got != true {
		t.Fatalf("need_relation = %#v, want true for docx", got)
	}
	if got := allPartialParams["page_token"]; got != "next" {
		t.Fatalf("page_token = %#v, want next", got)
	}

	sheetParams := buildDriveListCommentsParams(allPartialSpec, "sheet")
	if _, ok := sheetParams["need_relation"]; ok {
		t.Fatalf("need_relation should be ignored for non-docx: %#v", sheetParams)
	}
}

func TestDriveListCommentsExecuteDocx(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files/docxResource/comments",
		OnMatch: func(req *http.Request) {
			query := req.URL.Query()
			if got := query.Get("file_type"); got != "docx" {
				t.Errorf("file_type = %q, want docx", got)
			}
			if got := query.Get("is_solved"); got != "false" {
				t.Errorf("is_solved = %q, want false", got)
			}
			if got := query.Get("is_whole"); got != "" {
				t.Errorf("is_whole = %q, want omitted", got)
			}
			if got := query.Get("user_id_type"); got != "" {
				t.Errorf("user_id_type = %q, want omitted", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{"comment_id": "comment_1", "is_solved": false},
				},
				"has_more":   true,
				"page_token": "next",
			},
		},
	})

	err := mountAndRunDrive(t, DriveListComments, []string{
		"+list-comments",
		"--url", "https://example.larksuite.com/docx/docxResource",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "file_token", "data.file_token"); got != "docxResource" {
		t.Fatalf("file_token = %q, want docxResource", got)
	}
	if got := mustStringField(t, data, "file_type", "data.file_type"); got != "docx" {
		t.Fatalf("file_type = %q, want docx", got)
	}
	if got := data["count"]; got != float64(1) {
		t.Fatalf("count = %#v, want 1", got)
	}
}

func TestDriveListCommentsExecuteWikiResolvesToDocx(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		OnMatch: func(req *http.Request) {
			if got := req.URL.Query().Get("token"); got != "wikiResource" {
				t.Errorf("wiki token = %q, want wikiResource", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "docx",
					"obj_token": "docxFromWikiResource",
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files/docxFromWikiResource/comments",
		OnMatch: func(req *http.Request) {
			query := req.URL.Query()
			if got := query.Get("is_solved"); got != "" {
				t.Errorf("is_solved = %q, want omitted for solved-status all", got)
			}
			if got := query.Get("is_whole"); got != "true" {
				t.Errorf("is_whole = %q, want true", got)
			}
			if got := query.Get("need_relation"); got != "true" {
				t.Errorf("need_relation = %q, want true for resolved docx", got)
			}
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"items":    []map[string]interface{}{},
				"has_more": false,
			},
		},
	})

	err := mountAndRunDrive(t, DriveListComments, []string{
		"+list-comments",
		"--token", "wikiResource",
		"--type", "wiki",
		"--solved-status", "all",
		"--comment-scope", "whole",
		"--need-relation",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "file_token", "data.file_token"); got != "docxFromWikiResource" {
		t.Fatalf("file_token = %q, want docxFromWikiResource", got)
	}
	if got := mustStringField(t, data, "file_type", "data.file_type"); got != "docx" {
		t.Fatalf("file_type = %q, want docx", got)
	}
}
