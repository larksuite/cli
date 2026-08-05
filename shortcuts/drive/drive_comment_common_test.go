// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func assertDriveCommentValidationError(t *testing.T, err error, wantParam string) {
	t.Helper()

	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if validationErr.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", validationErr.Subtype, errs.SubtypeInvalidArgument)
	}
	if validationErr.Param != wantParam {
		t.Fatalf("param = %q, want %q", validationErr.Param, wantParam)
	}
}

// assertDriveCommentAPIError asserts the error kept the typed API contract:
// CallAPITyped errors must reach the caller unchanged, message-only checks
// would still pass if a refactor wrapped them into untyped errors.
func assertDriveCommentAPIError(t *testing.T, err error, wantCode int) {
	t.Helper()

	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if problem.Category != errs.CategoryAPI {
		t.Fatalf("category = %q, want %q", problem.Category, errs.CategoryAPI)
	}
	if problem.Subtype == "" {
		t.Fatalf("subtype is empty, want populated")
	}
	if problem.Code != wantCode {
		t.Fatalf("code = %d, want %d", problem.Code, wantCode)
	}
}

func TestResolveDriveCommentInput(t *testing.T) {
	t.Parallel()

	op := driveCommentOp{Label: "comments batch query", Types: []string{"doc", "docx", "sheet", "file", "slides"}}
	docOnlyOp := driveCommentOp{Label: "comment reply", Types: []string{"doc", "docx"}}

	tests := []struct {
		name      string
		op        driveCommentOp
		urlInput  string
		rawInput  string
		docType   string
		wantToken string
		wantType  string
		wantErr   string
		wantParam string
	}{
		{
			name:      "url docx",
			op:        op,
			urlInput:  "https://example.larksuite.com/docx/docxResource?from=wiki",
			wantToken: "docxResource",
			wantType:  "docx",
		},
		{
			name:      "url wiki always accepted",
			op:        docOnlyOp,
			urlInput:  "https://example.larksuite.com/wiki/wikiResource",
			wantToken: "wikiResource",
			wantType:  "wiki",
		},
		{
			name:      "token flag also accepts url",
			op:        op,
			rawInput:  "https://example.larksuite.com/sheets/sheetResource",
			wantToken: "sheetResource",
			wantType:  "sheet",
		},
		{
			name:      "bare token with type",
			op:        op,
			rawInput:  "docxResource",
			docType:   "docx",
			wantToken: "docxResource",
			wantType:  "docx",
		},
		{
			name:      "bare wiki token",
			op:        docOnlyOp,
			rawInput:  "wikiResource",
			docType:   "wiki",
			wantToken: "wikiResource",
			wantType:  "wiki",
		},
		{
			name:      "url and token mutually exclusive",
			op:        op,
			urlInput:  "https://example.larksuite.com/docx/docxResource",
			rawInput:  "docxResource",
			wantErr:   "mutually exclusive",
			wantParam: "--url",
		},
		{
			name:      "missing input",
			op:        op,
			wantErr:   "specify --url or --token",
			wantParam: "--url",
		},
		{
			name:      "bare token needs type",
			op:        op,
			rawInput:  "docxResource",
			wantErr:   "--type is required",
			wantParam: "--type",
		},
		{
			name:      "type conflicts with url",
			op:        op,
			urlInput:  "https://example.larksuite.com/wiki/wikiResource",
			docType:   "docx",
			wantErr:   "conflicts",
			wantParam: "--type",
		},
		{
			name:      "unsupported url type",
			op:        op,
			urlInput:  "https://example.larksuite.com/drive/folder/folderResource",
			wantErr:   "unsupported --url resource type",
			wantParam: "--url",
		},
		{
			name:      "unsupported url type for doc-only op",
			op:        docOnlyOp,
			urlInput:  "https://example.larksuite.com/sheets/sheetResource",
			wantErr:   "comment reply supports doc, docx, wiki",
			wantParam: "--url",
		},
		{
			name:      "apps page url",
			op:        driveCommentOp{Label: "comments batch query", Types: []string{"doc", "docx", "apps"}},
			urlInput:  "https://example.feishu.cn/page/appsPageResource/",
			wantToken: "appsPageResource",
			wantType:  "apps",
		},
		{
			name:      "apps page url rejected by op without apps",
			op:        docOnlyOp,
			urlInput:  "https://example.feishu.cn/page/appsPageResource",
			wantErr:   `unsupported --url resource type "apps"`,
			wantParam: "--url",
		},
		{
			name:      "apps page url conflicts with explicit type",
			op:        driveCommentOp{Label: "comments batch query", Types: []string{"doc", "docx", "apps"}},
			urlInput:  "https://example.feishu.cn/page/appsPageResource",
			docType:   "docx",
			wantErr:   "conflicts",
			wantParam: "--type",
		},
		{
			name:      "base alias normalized in error",
			op:        op,
			urlInput:  "https://example.larksuite.com/base/baseResource",
			wantErr:   `unsupported --url resource type "bitable"`,
			wantParam: "--url",
		},
		{
			name:      "unrecognized url",
			op:        op,
			urlInput:  "https://example.com/unknown/path",
			wantErr:   "unsupported --url URL",
			wantParam: "--url",
		},
		{
			name:      "bare token with path fragments",
			op:        op,
			rawInput:  "abc/def",
			docType:   "docx",
			wantErr:   "invalid bare token",
			wantParam: "--token",
		},
		{
			name:      "invalid explicit type",
			op:        docOnlyOp,
			rawInput:  "sheetResource",
			docType:   "sheet",
			wantErr:   "invalid --type",
			wantParam: "--type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveDriveCommentInput(tt.op, tt.urlInput, tt.rawInput, tt.docType)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				assertDriveCommentValidationError(t, err, tt.wantParam)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Token != tt.wantToken || got.Type != tt.wantType {
				t.Fatalf("got (%q, %q), want (%q, %q)", got.Token, got.Type, tt.wantToken, tt.wantType)
			}
		})
	}
}

func TestValidateDriveCommentPathID(t *testing.T) {
	t.Parallel()

	if err := validateDriveCommentPathID("7457000000000000001", "--comment-id"); err != nil {
		t.Fatalf("unexpected error for valid ID: %v", err)
	}

	tests := []struct {
		name    string
		value   string
		wantErr string
	}{
		{name: "empty", value: "   ", wantErr: "must not be empty"},
		{name: "path traversal", value: "../admin", wantErr: "path traversal"},
		{name: "url metacharacters", value: "abc?x=1", wantErr: "invalid characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateDriveCommentPathID(tt.value, "--comment-id")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
			assertDriveCommentValidationError(t, err, "--comment-id")
		})
	}
}

func TestDriveCommentOpTypeHelpers(t *testing.T) {
	t.Parallel()

	op := driveCommentOp{Label: "comment reply", Types: []string{"doc", "docx"}}
	if got := op.inputTypeList(); got != "doc, docx, wiki" {
		t.Fatalf("inputTypeList() = %q, want %q", got, "doc, docx, wiki")
	}
	if got := op.targetTypeList(); got != "doc, docx" {
		t.Fatalf("targetTypeList() = %q, want %q", got, "doc, docx")
	}
	if got := op.flagEnum(); len(got) != 3 || got[2] != "wiki" {
		t.Fatalf("flagEnum() = %v, want types plus trailing wiki", got)
	}
	if !op.supports("docx") || op.supports("sheet") || op.supports("wiki") {
		t.Fatalf("supports() misclassified: docx=%v sheet=%v wiki=%v", op.supports("docx"), op.supports("sheet"), op.supports("wiki"))
	}
}
