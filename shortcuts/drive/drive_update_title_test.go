// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestResolveDriveUpdateTitleInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
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
			urlInput:  "https://example.larksuite.com/docx/docxRenameTarget?from=share",
			wantToken: "docxRenameTarget",
			wantType:  "docx",
		},
		{
			name:      "url folder",
			urlInput:  "https://example.larksuite.com/drive/folder/folderRenameTarget",
			wantToken: "folderRenameTarget",
			wantType:  "folder",
		},
		{
			name:      "url wiki keeps the node token",
			urlInput:  "https://example.larksuite.com/wiki/wikiRenameTarget",
			wantToken: "wikiRenameTarget",
			wantType:  "wiki",
		},
		{
			name:      "url base normalizes to bitable",
			urlInput:  "https://example.larksuite.com/base/bitableRenameTarget",
			wantToken: "bitableRenameTarget",
			wantType:  "bitable",
		},
		{
			name:      "token flag also accepts url",
			rawInput:  "https://example.larksuite.com/sheets/sheetRenameTarget",
			wantToken: "sheetRenameTarget",
			wantType:  "sheet",
		},
		{
			name:      "bare token with type",
			rawInput:  "fileRenameTarget",
			docType:   "file",
			wantToken: "fileRenameTarget",
			wantType:  "file",
		},
		{
			name:      "bare token with base alias",
			rawInput:  "bitableRenameTarget",
			docType:   "BASE",
			wantToken: "bitableRenameTarget",
			wantType:  "bitable",
		},
		{
			name:      "bare token with wiki type",
			rawInput:  "wikiRenameTarget",
			docType:   "wiki",
			wantToken: "wikiRenameTarget",
			wantType:  "wiki",
		},
		{
			name:      "url and token mutually exclusive",
			urlInput:  "https://example.larksuite.com/docx/docxRenameTarget",
			rawInput:  "docxRenameTarget",
			wantErr:   "mutually exclusive",
			wantParam: "--url",
		},
		{
			name:      "missing input",
			wantErr:   "specify --url or --token",
			wantParam: "--url",
		},
		{
			name:      "bare token needs type",
			rawInput:  "docxRenameTarget",
			wantErr:   "--type is required",
			wantParam: "--type",
		},
		{
			name:      "type conflicts with url",
			urlInput:  "https://example.larksuite.com/docx/docxRenameTarget",
			docType:   "sheet",
			wantErr:   "conflicts",
			wantParam: "--type",
		},
		{
			name:      "unrecognized url",
			urlInput:  "https://example.larksuite.com/unknown/path",
			wantErr:   "unsupported --url URL",
			wantParam: "--url",
		},
		{
			name:      "token with path fragments",
			rawInput:  "token/with/../slash",
			docType:   "docx",
			wantErr:   "path traversal",
			wantParam: "--token",
		},
		{
			name:      "url token with path traversal",
			urlInput:  "https://example.larksuite.com/docx/..",
			wantErr:   "path traversal",
			wantParam: "--url",
		},
		{
			// minutes is a real Drive resource kind, just not one this endpoint takes.
			name:      "invalid bare type",
			rawInput:  "someToken",
			docType:   "minutes",
			wantErr:   "invalid --type",
			wantParam: "--type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveDriveUpdateTitleInput(tt.urlInput, tt.rawInput, tt.docType)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				assertDriveUpdateTitleValidationError(t, err, tt.wantParam)
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

func TestResolveDriveUpdateTitleInputPreservesTokenValidationCause(t *testing.T) {
	t.Parallel()

	for _, input := range []struct {
		urlInput string
		rawInput string
		docType  string
	}{
		{urlInput: "https://example.larksuite.com/docx/.."},
		{rawInput: "token/../other", docType: "docx"},
	} {
		_, err := resolveDriveUpdateTitleInput(input.urlInput, input.rawInput, input.docType)
		if err == nil {
			t.Fatal("expected token validation error")
		}
		if errors.Unwrap(err) == nil {
			t.Fatalf("validation cause was not preserved: %v", err)
		}
	}
}

// Miaoda apps are the one Drive-shaped resource the endpoint refuses, so both
// spellings redirect to the domain that owns them instead of failing as an
// unrecognized URL or a bare enum rejection.
func TestResolveDriveUpdateTitleInputRedirectsApps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		urlInput  string
		rawInput  string
		docType   string
		wantParam string
	}{
		{name: "page url", urlInput: "https://example.feishu.cn/page/pageRenameTarget", wantParam: "--url"},
		{name: "page url with trailing slash via token flag", rawInput: "https://example.feishu.cn/page/pageRenameTarget/", wantParam: "--token"},
		{name: "bare token with apps type", rawInput: "pageRenameTarget", docType: "apps", wantParam: "--type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := resolveDriveUpdateTitleInput(tt.urlInput, tt.rawInput, tt.docType)
			if err == nil {
				t.Fatal("expected an apps redirect error, got nil")
			}
			assertDriveUpdateTitleValidationError(t, err, tt.wantParam)

			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected *errs.ValidationError, got %T", err)
			}
			if !strings.Contains(validationErr.Message, "does not support Miaoda apps") {
				t.Fatalf("message = %q, want the apps limitation named", validationErr.Message)
			}
			// The hint stops at the owning domain: apps-domain flags would drift.
			if !strings.Contains(validationErr.Hint, "apps domain") || !strings.Contains(validationErr.Hint, "lark-cli apps --help") {
				t.Fatalf("hint = %q, want the apps-domain redirect", validationErr.Hint)
			}
			// The rejected token is never echoed next to a runnable command.
			if strings.Contains(validationErr.Message, "pageRenameTarget") || strings.Contains(validationErr.Hint, "pageRenameTarget") {
				t.Fatalf("message %q / hint %q must not echo the caller's token", validationErr.Message, validationErr.Hint)
			}
		})
	}
}

// doc and mindnote are listed by the platform metadata but refused by the
// server, so every spelling is turned down locally with the reason.
func TestResolveDriveUpdateTitleInputRejectsServerRefusedTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		urlInput  string
		rawInput  string
		docType   string
		wantType  string
		wantParam string
	}{
		{name: "bare token with doc type", rawInput: "docRenameTarget", docType: "doc", wantType: "doc", wantParam: "--type"},
		{name: "bare token with mindnote type", rawInput: "mindnoteRenameTarget", docType: "mindnote", wantType: "mindnote", wantParam: "--type"},
		{name: "doc url", urlInput: "https://example.larksuite.com/doc/docRenameTarget", wantType: "doc", wantParam: "--url"},
		{name: "mindnote url", urlInput: "https://example.larksuite.com/mindnote/mindnoteRenameTarget", wantType: "mindnote", wantParam: "--url"},
		{name: "mindnote url via token flag", rawInput: "https://example.larksuite.com/mindnote/mindnoteRenameTarget", wantType: "mindnote", wantParam: "--token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := resolveDriveUpdateTitleInput(tt.urlInput, tt.rawInput, tt.docType)
			if err == nil {
				t.Fatalf("expected %s to be rejected, got nil", tt.wantType)
			}
			assertDriveUpdateTitleValidationError(t, err, tt.wantParam)

			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected *errs.ValidationError, got %T", err)
			}
			if !strings.Contains(validationErr.Message, "type="+tt.wantType) {
				t.Fatalf("message = %q, want the refused type named", validationErr.Message)
			}
			// The server code is part of the message so an agent can match what it
			// would have seen had the request gone out.
			if !strings.Contains(validationErr.Message, "981002") {
				t.Fatalf("message = %q, want the server code named", validationErr.Message)
			}
			if !strings.Contains(validationErr.Hint, "Supported types here: docx, sheet, bitable, slides, file, folder, wiki") {
				t.Fatalf("hint = %q, want the supported set listed", validationErr.Hint)
			}
		})
	}
}

// Tokens are caller-controlled, so no error text may hand back one inside
// something a human or an agent could paste into a shell.
func TestResolveDriveUpdateTitleInputNeverEchoesTokenIntoCommands(t *testing.T) {
	t.Parallel()

	hostile := []string{
		"tok;whoami",
		"tok with space",
		"tok`id`",
		"tok$(id)",
		"tok'squote",
		"tok\"dquote",
		"tok&&ls",
		"tok|cat",
	}

	for _, token := range hostile {
		t.Run(token, func(t *testing.T) {
			t.Parallel()

			// Every spelling that can fail with the token in hand: an unsupported
			// type, a server-refused type, and the apps redirect.
			for _, docType := range []string{"minutes", "doc", "mindnote", "apps"} {
				_, err := resolveDriveUpdateTitleInput("", token, docType)
				if err == nil {
					t.Fatalf("type %s: expected a rejection for %q", docType, token)
				}
				problem, ok := errs.ProblemOf(err)
				if !ok {
					t.Fatalf("type %s: expected a typed error, got %T", docType, err)
				}
				if strings.Contains(problem.Hint, token) {
					t.Fatalf("type %s: hint %q must not embed the caller token %q", docType, problem.Hint, token)
				}
				// Messages may quote the input, but never unquoted next to a command.
				if strings.Contains(problem.Message, "lark-cli") && strings.Contains(problem.Message, token) {
					t.Fatalf("type %s: message %q pastes the token into a command", docType, problem.Message)
				}
			}
		})
	}
}

func TestParseDriveUpdateTitleAppsURL(t *testing.T) {
	t.Parallel()

	if token, ok := parseDriveUpdateTitleAppsURL("https://example.feishu.cn/page/pageRenameTarget?from=share"); !ok || token != "pageRenameTarget" {
		t.Fatalf("parse = (%q, %v), want (pageRenameTarget, true)", token, ok)
	}
	for _, raw := range []string{
		"pageRenameTarget", // bare token
		"https://example.feishu.cn/docx/docxRenameTarget", // another resource kind
		"https://example.feishu.cn/page/",                 // no token
		"https://example.feishu.cn/page/nested/extra",     // not a single segment
		"ftp://example.feishu.cn/page/pageRenameTarget",   // unsupported scheme
		"/page/pageRenameTarget",                          // no host
	} {
		if token, ok := parseDriveUpdateTitleAppsURL(raw); ok {
			t.Fatalf("parse(%q) = (%q, true), want no match", raw, token)
		}
	}
}

// The flag enum accepts apps so the redirect fires; the request path never does.
func TestDriveUpdateTitleFlagTypesCarryAppsButAPITypesDoNot(t *testing.T) {
	t.Parallel()

	var flagHasApps bool
	for _, docType := range driveUpdateTitleFlagTypes {
		if docType == driveUpdateTitleAppsType {
			flagHasApps = true
		}
	}
	if !flagHasApps {
		t.Fatalf("driveUpdateTitleFlagTypes = %v, want apps accepted at parse time", driveUpdateTitleFlagTypes)
	}
	if driveUpdateTitleTypeSupported(driveUpdateTitleAppsType) {
		t.Fatal("apps must not be treated as a supported request type")
	}

	// Same split for the types the server refuses: accepted by the flag so the
	// caller gets the reason, never sent as a request type.
	flagTypes := strings.Join(driveUpdateTitleFlagTypes, ",")
	for _, docType := range driveUpdateTitleRejectedTypes {
		if !strings.Contains(flagTypes, docType) {
			t.Fatalf("driveUpdateTitleFlagTypes = %v, want %q accepted at parse time", driveUpdateTitleFlagTypes, docType)
		}
		if driveUpdateTitleTypeSupported(docType) {
			t.Fatalf("%q must not be treated as a supported request type", docType)
		}
		if !driveUpdateTitleTypeRejected(docType) {
			t.Fatalf("%q must be recognized as server-refused", docType)
		}
	}
	if driveUpdateTitleTypeRejected("docx") {
		t.Fatal("docx must not be marked server-refused")
	}
}

func TestValidateDriveUpdateTitleSpec(t *testing.T) {
	t.Parallel()

	valid := driveUpdateTitleSpec{
		Ref:       driveUpdateTitleRef{Token: "docxRenameTarget", Type: "docx", SourceFlag: "--url"},
		Title:     "New title",
		ExtPolicy: driveUpdateTitleExtKeep,
	}
	if err := validateDriveUpdateTitleSpec(valid, false); err != nil {
		t.Fatalf("unexpected error for valid spec: %v", err)
	}

	empty := valid
	empty.Title = ""
	err := validateDriveUpdateTitleSpec(empty, false)
	if err == nil || !strings.Contains(err.Error(), "--title must not be empty") {
		t.Fatalf("expected empty-title error, got %v", err)
	}
	assertDriveUpdateTitleValidationError(t, err, "--title")

	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	if !strings.Contains(validationErr.Hint, "clears the resource title") {
		t.Fatalf("hint should explain why an empty title is refused locally, got %q", validationErr.Hint)
	}

	// The extension policy has nothing to guard outside --type file, so passing
	// it there is rejected instead of silently ignored.
	err = validateDriveUpdateTitleSpec(valid, true)
	if err == nil || !strings.Contains(err.Error(), "only applies to --type file") {
		t.Fatalf("expected extension-policy scope error, got %v", err)
	}
	assertDriveUpdateTitleValidationError(t, err, "--on-extension-mismatch")

	fileSpec := driveUpdateTitleSpec{
		Ref:       driveUpdateTitleRef{Token: "fileRenameTarget", Type: "file", SourceFlag: "--token"},
		Title:     "report.xlsx",
		ExtPolicy: driveUpdateTitleExtAllow,
	}
	if err := validateDriveUpdateTitleSpec(fileSpec, true); err != nil {
		t.Fatalf("unexpected error for --type file with an explicit policy: %v", err)
	}

	removedPolicy := fileSpec
	removedPolicy.ExtPolicy = "error"
	err = validateDriveUpdateTitleSpec(removedPolicy, true)
	if err == nil {
		t.Fatal("expected the removed error policy to be rejected")
	}
	assertDriveUpdateTitleValidationError(t, err, "--on-extension-mismatch")
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %#v, want validation/invalid_argument", problem)
	}
}

func TestDriveUpdateTitleDeclaresOnlySharedDriveScope(t *testing.T) {
	t.Parallel()

	got := DriveUpdateTitle.ConditionalScopes
	if len(got) != 1 || got[0] != "drive:file:upload" {
		t.Fatalf("ConditionalScopes = %v, want only drive:file:upload", got)
	}
}

func TestDriveUpdateTitleExtensionHelpers(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"report.xlsx":     ".xlsx",
		"report.XLSX":     ".XLSX",
		"  report.md  ":   ".md",
		"report":          "",
		"report v1.2":     ".2",
		"archive.tar.gz":  ".gz",
		"trailing.":       ".",
		".hidden":         "", // leading dot only is a name, not an extension
		"dir/report.xlsx": ".xlsx",
	}
	for title, want := range tests {
		if got := driveUpdateTitleExtension(title); got != want {
			t.Fatalf("driveUpdateTitleExtension(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestDriveUpdateTitleSpecNeedsCurrentTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		docType string
		policy  string
		want    bool
	}{
		{docType: "file", policy: driveUpdateTitleExtKeep, want: true},
		{docType: "file", policy: driveUpdateTitleExtAllow, want: false},
		{docType: "docx", policy: driveUpdateTitleExtKeep, want: false},
		{docType: "folder", policy: driveUpdateTitleExtKeep, want: false},
	}
	for _, tt := range tests {
		spec := driveUpdateTitleSpec{
			Ref:       driveUpdateTitleRef{Token: "tok", Type: tt.docType},
			Title:     "t",
			ExtPolicy: tt.policy,
		}
		if got := spec.NeedsCurrentTitle(); got != tt.want {
			t.Fatalf("type=%s policy=%s NeedsCurrentTitle() = %v, want %v", tt.docType, tt.policy, got, tt.want)
		}
	}
}

func TestBuildDriveUpdateTitleRequest(t *testing.T) {
	t.Parallel()

	spec := driveUpdateTitleSpec{
		Ref:   driveUpdateTitleRef{Token: "wikiRenameTarget", Type: "wiki", SourceFlag: "--url"},
		Title: "Renamed node",
	}
	if got := buildDriveUpdateTitleParams(spec)["type"]; got != "wiki" {
		t.Fatalf("params type = %#v, want wiki", got)
	}
	if got := buildDriveUpdateTitleBody(spec)["new_title"]; got != "Renamed node" {
		t.Fatalf("body new_title = %#v, want Renamed node", got)
	}
	if got := driveUpdateTitlePath("folder/token"); got != "/open-apis/drive/v1/files/folder%2Ftoken" {
		t.Fatalf("path = %q, want the token percent-encoded as a single segment", got)
	}
}

func TestDriveUpdateTitleErrorGuidance(t *testing.T) {
	t.Parallel()

	wikiSpec := driveUpdateTitleSpec{Ref: driveUpdateTitleRef{Token: "wikiRenameTarget", Type: "wiki"}, Title: "t"}
	if got := driveUpdateTitleErrorGuidance(981003, wikiSpec); !strings.Contains(got, "wiki node token") {
		t.Fatalf("wiki 981003 guidance = %q, want wiki node token guidance", got)
	}
	docxSpec := driveUpdateTitleSpec{Ref: driveUpdateTitleRef{Token: "docxRenameTarget", Type: "docx"}, Title: "t"}
	if got := driveUpdateTitleErrorGuidance(981003, docxSpec); !strings.Contains(got, "--type docx") {
		t.Fatalf("docx 981003 guidance = %q, want the declared type echoed", got)
	}
	if got := driveUpdateTitleErrorGuidance(981004, docxSpec); !strings.Contains(got, "edit permission") {
		t.Fatalf("981004 guidance = %q, want edit permission guidance", got)
	}
	// A document-permission failure must not send the caller hunting for a scope.
	if got := driveUpdateTitleErrorGuidance(981004, docxSpec); !strings.Contains(got, "not a missing scope") {
		t.Fatalf("981004 guidance = %q, want it separated from the scope codes", got)
	}
	if got := driveUpdateTitleErrorGuidance(1061003, docxSpec); got != "" {
		t.Fatalf("unmapped code guidance = %q, want empty", got)
	}

	// Rate limit: the platform answers 99991400 for this endpoint, and the fix is
	// serializing, not retrying harder.
	if got := driveUpdateTitleErrorGuidance(99991400, docxSpec); !strings.Contains(got, "rate limited") ||
		!strings.Contains(got, "serialize") || !strings.Contains(got, "backoff") {
		t.Fatalf("99991400 guidance = %q, want serialize-and-backoff guidance", got)
	}

	// Scope: the endpoint's scope set is an any-of keyed by --type, so the
	// guidance has to name the one that matches the target.
	scopeByType := map[string]string{
		"docx":    "docx:document:write_only",
		"doc":     "docx:document:write_only",
		"sheet":   "sheets:spreadsheet:write_only",
		"bitable": "base:app:update",
		"file":    "drive:file:upload",
		"folder":  "drive:file:upload",
	}
	for docType, wantScope := range scopeByType {
		spec := driveUpdateTitleSpec{
			Ref:       driveUpdateTitleRef{Token: "tok", Type: docType},
			Title:     "t.xlsx",
			ExtPolicy: driveUpdateTitleExtAllow, // keeps the guard note out of the way
		}
		for _, code := range []int{99991672, 99991679} {
			got := driveUpdateTitleErrorGuidance(code, spec)
			if !strings.Contains(got, wantScope) {
				t.Fatalf("%d guidance for --type %s = %q, want %s named", code, docType, got, wantScope)
			}
		}
	}

	// Types the platform metadata does not map to a single scope get the whole
	// any-of set instead of a guess.
	wikiScopeSpec := driveUpdateTitleSpec{Ref: driveUpdateTitleRef{Token: "tok", Type: "wiki"}, Title: "t"}
	if got := driveUpdateTitleErrorGuidance(99991672, wikiScopeSpec); !strings.Contains(got, "one of docx:document:write_only") {
		t.Fatalf("wiki scope guidance = %q, want the any-of set", got)
	}

	// When the extension guard is on, its own read scope is part of the fix.
	guardedSpec := driveUpdateTitleSpec{
		Ref:       driveUpdateTitleRef{Token: "tok", Type: "file"},
		Title:     "t",
		ExtPolicy: driveUpdateTitleExtKeep,
	}
	got := driveUpdateTitleErrorGuidance(99991679, guardedSpec)
	if !strings.Contains(got, "drive:drive.metadata:readonly") || !strings.Contains(got, "--on-extension-mismatch=allow") {
		t.Fatalf("guarded scope guidance = %q, want the metadata scope and the allow escape", got)
	}

	if err := decorateDriveUpdateTitleError(nil, docxSpec); err != nil {
		t.Fatalf("decorating nil = %v, want nil", err)
	}
	plain := errors.New("not typed")
	if got := decorateDriveUpdateTitleError(plain, docxSpec); got != plain {
		t.Fatalf("untyped error should pass through unchanged, got %v", got)
	}

	// A hint the lower layer already produced is kept, with command guidance
	// appended once rather than replacing it.
	existing := errs.NewAPIError(errs.SubtypeNotFound, "not found.").WithCode(981003).WithHint("server hint")
	decorated := decorateDriveUpdateTitleError(existing, docxSpec)
	problem, ok := errs.ProblemOf(decorated)
	if !ok {
		t.Fatalf("expected typed error, got %T", decorated)
	}
	if !strings.HasPrefix(problem.Hint, "server hint; ") || !strings.Contains(problem.Hint, "--type docx") {
		t.Fatalf("hint = %q, want the server hint kept and command guidance appended", problem.Hint)
	}
	before := problem.Hint
	if _, ok := errs.ProblemOf(decorateDriveUpdateTitleError(existing, docxSpec)); !ok {
		t.Fatal("second decoration should stay typed")
	}
	if problem.Hint != before {
		t.Fatalf("hint = %q, want unchanged on a second decoration", problem.Hint)
	}

	// An unmapped code leaves the error untouched.
	other := errs.NewAPIError(errs.SubtypeServerError, "boom").WithCode(1061003)
	if got := decorateDriveUpdateTitleError(other, docxSpec); got != error(other) {
		t.Fatalf("unmapped code should pass through unchanged, got %v", got)
	}
	if hint := other.Hint; hint != "" {
		t.Fatalf("hint = %q, want empty for an unmapped code", hint)
	}
}

func TestDriveUpdateTitleExecuteDocxURL(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())

	var capturedQuery string
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v1/files/docxRenameTarget",
		OnMatch: func(req *http.Request) {
			capturedQuery = req.URL.RawQuery
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveUpdateTitle, []string{
		"+update-title",
		"--url", "https://example.larksuite.com/docx/docxRenameTarget",
		"--title", "  Q3 plan  ",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(capturedQuery, "type=docx") {
		t.Fatalf("query = %q, want type=docx", capturedQuery)
	}

	var requestBody map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &requestBody); err != nil {
		t.Fatalf("failed to decode captured body: %v\nbody:\n%s", err, string(stub.CapturedBody))
	}
	// Surrounding spaces are stripped before the title is sent.
	if got := requestBody["new_title"]; got != "Q3 plan" {
		t.Fatalf("body new_title = %#v, want the trimmed title", got)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := data["updated"]; got != true {
		t.Fatalf("updated = %#v, want true", got)
	}
	if got := mustStringField(t, data, "file_token", "data.file_token"); got != "docxRenameTarget" {
		t.Fatalf("file_token = %q, want docxRenameTarget", got)
	}
	if got := mustStringField(t, data, "type", "data.type"); got != "docx" {
		t.Fatalf("type = %q, want docx", got)
	}
	if got := mustStringField(t, data, "title", "data.title"); got != "Q3 plan" {
		t.Fatalf("title = %q, want the trimmed title", got)
	}
	if got := mustStringField(t, data, "url", "data.url"); got != "https://www.feishu.cn/docx/docxRenameTarget" {
		t.Fatalf("url = %q, want the built docx URL", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no update progress", stderr.String())
	}
}

func TestDriveUpdateTitleExecuteWikiNodeToken(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	var capturedQuery string
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v1/files/wikiRenameTarget",
		OnMatch: func(req *http.Request) {
			capturedQuery = req.URL.RawQuery
		},
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})

	err := mountAndRunDrive(t, DriveUpdateTitle, []string{
		"+update-title",
		"--token", "wikiRenameTarget",
		"--type", "wiki",
		"--new-title", "Renamed node",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The wiki node token is patched as-is: no get_node unwrapping step, because
	// the endpoint resolves wiki tokens itself and rejects the document token.
	if !strings.Contains(capturedQuery, "type=wiki") {
		t.Fatalf("query = %q, want type=wiki", capturedQuery)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "url", "data.url"); got != "https://www.feishu.cn/wiki/wikiRenameTarget" {
		t.Fatalf("url = %q, want the built wiki URL", got)
	}
	if got := mustStringField(t, data, "title", "data.title"); got != "Renamed node" {
		t.Fatalf("title = %q, want the --new-title alias value", got)
	}
}

func TestDriveUpdateTitleExecuteAPIErrorCarriesGuidance(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v1/files/sheetRenameTarget",
		Body: map[string]interface{}{
			"code": 981003,
			"msg":  "not found.",
		},
	})

	err := mountAndRunDrive(t, DriveUpdateTitle, []string{
		"+update-title",
		"--token", "sheetRenameTarget",
		"--type", "sheet",
		"--title", "Renamed sheet",
		"--as", "user",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if problem.Code != 981003 {
		t.Fatalf("problem code = %d, want 981003", problem.Code)
	}
	if !strings.Contains(problem.Hint, "--type sheet") {
		t.Fatalf("hint = %q, want type/token mismatch guidance", problem.Hint)
	}
}

func TestDriveUpdateTitleMountedDryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	err := mountAndRunDrive(t, DriveUpdateTitle, []string{
		"+update-title",
		"--url", "https://example.larksuite.com/drive/folder/folderRenameTarget",
		"--title", "Archive 2026",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	api, ok := data["api"].([]interface{})
	if !ok || len(api) != 1 {
		t.Fatalf("api = %#v, want a single planned request", data["api"])
	}
	call := mustMapValue(t, api[0], "api.0")
	if got := call["method"]; got != "PATCH" {
		t.Fatalf("api.0.method = %#v, want PATCH", got)
	}
	if got := call["url"]; got != "/open-apis/drive/v1/files/folderRenameTarget" {
		t.Fatalf("api.0.url = %#v, want the files patch endpoint with :file_token resolved", got)
	}
	params := mustMapValue(t, call["params"], "api.0.params")
	if got := params["type"]; got != "folder" {
		t.Fatalf("api.0.params.type = %#v, want folder", got)
	}
	body := mustMapValue(t, call["body"], "api.0.body")
	if got := body["new_title"]; got != "Archive 2026" {
		t.Fatalf("api.0.body.new_title = %#v, want Archive 2026", got)
	}
	if got := mustStringField(t, data, "file_token", "data.file_token"); got != "folderRenameTarget" {
		t.Fatalf("file_token = %q, want the token parsed from the folder URL", got)
	}
}

// Validation runs before the dry-run preview, so a bad target is rejected with a
// typed error rather than previewed — including under --dry-run.
func TestDriveUpdateTitleDryRunRejectsBareTokenWithoutType(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	err := mountAndRunDrive(t, DriveUpdateTitle, []string{
		"+update-title",
		"--token", "docxRenameTarget",
		"--title", "New title",
		"--dry-run",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected a validation error, got nil")
	}
	if !strings.Contains(err.Error(), "--type is required") {
		t.Fatalf("error = %v, want the missing --type validation message", err)
	}
	assertDriveUpdateTitleValidationError(t, err, "--type")
}

// driveUpdateTitleMetaStub answers the extension guard's current-name lookup.
func driveUpdateTitleMetaStub(token, title string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"metas": []interface{}{
					map[string]interface{}{
						"doc_token": token,
						"doc_type":  "file",
						"title":     title,
					},
				},
			},
		},
	}
}

func TestDriveUpdateTitleFileKeepsExtension(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(driveUpdateTitleMetaStub("fileRenameTarget", "quarterly.xlsx"))
	patch := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v1/files/fileRenameTarget",
		Body:   map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	}
	reg.Register(patch)

	err := mountAndRunDrive(t, DriveUpdateTitle, []string{
		"+update-title",
		"--token", "fileRenameTarget",
		"--type", "file",
		"--title", "2026 Q3 report",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var requestBody map[string]interface{}
	if err := json.Unmarshal(patch.CapturedBody, &requestBody); err != nil {
		t.Fatalf("failed to decode captured body: %v", err)
	}
	if got := requestBody["new_title"]; got != "2026 Q3 report.xlsx" {
		t.Fatalf("body new_title = %#v, want the current extension appended", got)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "title", "data.title"); got != "2026 Q3 report.xlsx" {
		t.Fatalf("title = %q, want the applied title with the extension", got)
	}
	if got := mustStringField(t, data, "extension_appended", "data.extension_appended"); got != ".xlsx" {
		t.Fatalf("extension_appended = %q, want .xlsx", got)
	}
	if got := mustStringField(t, data, "previous_title", "data.previous_title"); got != "quarterly.xlsx" {
		t.Fatalf("previous_title = %q, want the name read before the rename", got)
	}
	if progress := stderr.String(); !strings.Contains(progress, "keeping .xlsx") {
		t.Fatalf("stderr = %q, want the appended extension reported", progress)
	}
}

func TestDriveUpdateTitleFileKeepsExtensionCase(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(driveUpdateTitleMetaStub("fileRenameTarget", "quarterly.XLSX"))
	patch := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v1/files/fileRenameTarget",
		Body:   map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	}
	reg.Register(patch)

	err := mountAndRunDrive(t, DriveUpdateTitle, []string{
		"+update-title",
		"--token", "fileRenameTarget",
		"--type", "file",
		"--title", "2026 Q3 report",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var requestBody map[string]interface{}
	if err := json.Unmarshal(patch.CapturedBody, &requestBody); err != nil {
		t.Fatalf("failed to decode captured body: %v", err)
	}
	if got := requestBody["new_title"]; got != "2026 Q3 report.XLSX" {
		t.Fatalf("body new_title = %#v, want the current extension with its original case", got)
	}

	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if got := mustStringField(t, data, "extension_appended", "data.extension_appended"); got != ".XLSX" {
		t.Fatalf("extension_appended = %q, want the original extension spelling", got)
	}
}

// A padded title is trimmed first, so the extension lands at the end of the
// real name instead of after the padding.
func TestDriveUpdateTitleFileKeepsExtensionOnPaddedTitle(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(driveUpdateTitleMetaStub("fileRenameTarget", "notes.md"))
	patch := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v1/files/fileRenameTarget",
		Body:   map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	}
	reg.Register(patch)

	err := mountAndRunDrive(t, DriveUpdateTitle, []string{
		"+update-title",
		"--token", "fileRenameTarget",
		"--type", "file",
		"--title", " notes v2 ",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var requestBody map[string]interface{}
	if err := json.Unmarshal(patch.CapturedBody, &requestBody); err != nil {
		t.Fatalf("failed to decode captured body: %v", err)
	}
	if got := requestBody["new_title"]; got != "notes v2.md" {
		t.Fatalf("body new_title = %#v, want the trimmed title with the extension appended", got)
	}
}

// A padded title that already ends in the current extension is not given a
// second one.
func TestDriveUpdateTitleFilePaddedTitleWithExtensionIsNotDoubled(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(driveUpdateTitleMetaStub("fileRenameTarget", "notes.md"))
	patch := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v1/files/fileRenameTarget",
		Body:   map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	}
	reg.Register(patch)

	err := mountAndRunDrive(t, DriveUpdateTitle, []string{
		"+update-title",
		"--token", "fileRenameTarget",
		"--type", "file",
		"--title", " notes v2.md ",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var requestBody map[string]interface{}
	if err := json.Unmarshal(patch.CapturedBody, &requestBody); err != nil {
		t.Fatalf("failed to decode captured body: %v", err)
	}
	if got := requestBody["new_title"]; got != "notes v2.md" {
		t.Fatalf("body new_title = %#v, want the trimmed title", got)
	}
}

func TestDriveUpdateTitleFileKeepsMatchingExtensionVerbatim(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(driveUpdateTitleMetaStub("fileRenameTarget", "quarterly.XLSX"))
	patch := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v1/files/fileRenameTarget",
		Body:   map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	}
	reg.Register(patch)

	err := mountAndRunDrive(t, DriveUpdateTitle, []string{
		"+update-title",
		"--token", "fileRenameTarget",
		"--type", "file",
		"--title", "2026 Q3 report.xlsx",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var requestBody map[string]interface{}
	if err := json.Unmarshal(patch.CapturedBody, &requestBody); err != nil {
		t.Fatalf("failed to decode captured body: %v", err)
	}
	// Case-insensitive match: the title already carries the extension, so it is
	// submitted verbatim instead of gaining a second one.
	if got := requestBody["new_title"]; got != "2026 Q3 report.xlsx" {
		t.Fatalf("body new_title = %#v, want the title verbatim", got)
	}
	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if _, ok := data["extension_appended"]; ok {
		t.Fatalf("extension_appended should be absent when nothing was appended: %#v", data)
	}
}

func TestDriveUpdateTitleFileRejectsExtensionChange(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(driveUpdateTitleMetaStub("fileRenameTarget", "notes.md"))
	reg.Register(&httpmock.Stub{
		Method:   "PATCH",
		URL:      "/open-apis/drive/v1/files/fileRenameTarget",
		Optional: true,
		OnMatch: func(*http.Request) {
			t.Error("the rename must not be sent when the extension would change")
		},
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})

	err := mountAndRunDrive(t, DriveUpdateTitle, []string{
		"+update-title",
		"--token", "fileRenameTarget",
		"--type", "file",
		"--title", "notes.txt",
		"--as", "user",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected an extension-change rejection, got nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if problem.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("subtype = %q, want %q", problem.Subtype, errs.SubtypeFailedPrecondition)
	}
	if !strings.Contains(problem.Message, ".md") || !strings.Contains(problem.Message, ".txt") {
		t.Fatalf("message = %q, want both extensions named", problem.Message)
	}
	if !strings.Contains(problem.Hint, "--on-extension-mismatch=allow") {
		t.Fatalf("hint = %q, want the allow escape hatch", problem.Hint)
	}
}

func TestDriveUpdateTitleFileAllowPolicySkipsCurrentTitleRead(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/drive/v1/metas/batch_query",
		Optional: true,
		OnMatch: func(*http.Request) {
			t.Error("allow must not read the current title")
		},
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})
	patch := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v1/files/fileRenameTarget",
		Body:   map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	}
	reg.Register(patch)

	err := mountAndRunDrive(t, DriveUpdateTitle, []string{
		"+update-title",
		"--token", "fileRenameTarget",
		"--type", "file",
		"--title", "no-extension-on-purpose",
		"--on-extension-mismatch", "allow",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var requestBody map[string]interface{}
	if err := json.Unmarshal(patch.CapturedBody, &requestBody); err != nil {
		t.Fatalf("failed to decode captured body: %v", err)
	}
	if got := requestBody["new_title"]; got != "no-extension-on-purpose" {
		t.Fatalf("body new_title = %#v, want the title verbatim", got)
	}
	out := decodeJSONMap(t, stdout.String())
	data := mustMapValue(t, out["data"], "data")
	if _, ok := data["previous_title"]; ok {
		t.Fatalf("previous_title should be absent when the guard did not read it: %#v", data)
	}
}

// A dotfile has no extension to protect, so the guard leaves the title alone.
func TestDriveUpdateTitleFileDotfileNeedsNoExtension(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(driveUpdateTitleMetaStub("fileRenameTarget", ".gitignore"))
	patch := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v1/files/fileRenameTarget",
		Body:   map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	}
	reg.Register(patch)

	err := mountAndRunDrive(t, DriveUpdateTitle, []string{
		"+update-title",
		"--token", "fileRenameTarget",
		"--type", "file",
		"--title", "ignore-rules",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var requestBody map[string]interface{}
	if err := json.Unmarshal(patch.CapturedBody, &requestBody); err != nil {
		t.Fatalf("failed to decode captured body: %v", err)
	}
	if got := requestBody["new_title"]; got != "ignore-rules" {
		t.Fatalf("body new_title = %#v, want the title verbatim", got)
	}
}

func TestDriveUpdateTitleFileCurrentTitleReadFailureCarriesEscape(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body:   map[string]interface{}{"code": 1061004, "msg": "forbidden"},
	})
	reg.Register(&httpmock.Stub{
		Method:   "PATCH",
		URL:      "/open-apis/drive/v1/files/fileRenameTarget",
		Optional: true,
		OnMatch: func(*http.Request) {
			t.Error("the rename must not be sent when the guard could not run")
		},
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})

	err := mountAndRunDrive(t, DriveUpdateTitle, []string{
		"+update-title",
		"--token", "fileRenameTarget",
		"--type", "file",
		"--title", "renamed.xlsx",
		"--as", "user",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected the current-title read failure to surface, got nil")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if problem.Code != 1061004 {
		t.Fatalf("problem code = %d, want the metadata read code 1061004", problem.Code)
	}
	if !strings.Contains(problem.Hint, "--on-extension-mismatch=allow") {
		t.Fatalf("hint = %q, want the allow escape hatch", problem.Hint)
	}
}

func TestDriveUpdateTitleFileRejectsMissingCurrentTitle(t *testing.T) {
	for _, test := range []struct {
		name  string
		metas interface{}
	}{
		{name: "empty metas", metas: []interface{}{}},
		{name: "missing title", metas: []interface{}{map[string]interface{}{"doc_token": "fileRenameTarget", "doc_type": "file"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/drive/v1/metas/batch_query",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"metas": test.metas},
				},
			})
			reg.Register(&httpmock.Stub{
				Method:   "PATCH",
				URL:      "/open-apis/drive/v1/files/fileRenameTarget",
				Optional: true,
				OnMatch: func(*http.Request) {
					t.Error("the rename must not be sent without a current title")
				},
				Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
			})

			err := mountAndRunDrive(t, DriveUpdateTitle, []string{
				"+update-title",
				"--token", "fileRenameTarget",
				"--type", "file",
				"--title", "renamed",
				"--as", "user",
			}, f, stdout)
			if err == nil {
				t.Fatal("expected missing current title to fail")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("problem = %#v, want internal/invalid_response", problem)
			}
		})
	}
}

func TestDriveUpdateTitleDryRunPlansExtensionGuardRead(t *testing.T) {
	t.Parallel()

	planCalls := func(spec driveUpdateTitleSpec) []map[string]interface{} {
		t.Helper()
		raw, err := json.Marshal(buildDriveUpdateTitleDryRun(spec))
		if err != nil {
			t.Fatalf("marshal dry-run plan: %v", err)
		}
		plan := decodeJSONMap(t, string(raw))
		calls, ok := plan["api"].([]interface{})
		if !ok {
			t.Fatalf("api = %#v, want a call list", plan["api"])
		}
		out := make([]map[string]interface{}, 0, len(calls))
		for i, call := range calls {
			out = append(out, mustMapValue(t, call, fmt.Sprintf("api.%d", i)))
		}
		return out
	}

	spec := driveUpdateTitleSpec{
		Ref:       driveUpdateTitleRef{Token: "fileRenameTarget", Type: "file", SourceFlag: "--token"},
		Title:     "renamed",
		ExtPolicy: driveUpdateTitleExtKeep,
	}
	calls := planCalls(spec)
	if len(calls) != 2 {
		t.Fatalf("plan has %d calls, want the metadata read plus the rename", len(calls))
	}
	if got := calls[0]["url"]; got != "/open-apis/drive/v1/metas/batch_query" {
		t.Fatalf("api.0.url = %#v, want the metadata read", got)
	}
	if got := calls[1]["url"]; got != "/open-apis/drive/v1/files/fileRenameTarget" {
		t.Fatalf("api.1.url = %#v, want the files patch endpoint", got)
	}
	body := mustMapValue(t, calls[1]["body"], "api.1.body")
	if got := body["new_title"]; got != "<resolved from --title and current title in step 1>" {
		t.Fatalf("api.1.body.new_title = %#v, want response-derived placeholder", got)
	}

	// allow renames without reading the current name, so the plan is one request.
	spec.ExtPolicy = driveUpdateTitleExtAllow
	if calls := planCalls(spec); len(calls) != 1 {
		t.Fatalf("plan has %d calls under the allow policy, want 1", len(calls))
	}
}

func TestDriveUpdateTitleMountedRejectsWhitespaceTitle(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())

	err := mountAndRunDrive(t, DriveUpdateTitle, []string{
		"+update-title",
		"--url", "https://example.larksuite.com/docx/docxRenameTarget",
		"--title", "   ",
		"--as", "user",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected a validation error, got nil")
	}
	assertDriveUpdateTitleValidationError(t, err, "--title")
}

func assertDriveUpdateTitleValidationError(t *testing.T, err error, wantParam string) {
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

	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected errs.ProblemOf to recognize typed error: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("problem category = %q, want %q", problem.Category, errs.CategoryValidation)
	}
}
