// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

const testPageXML = `<slide><style><fill type="solid" color="rgba(0,0,0,1)"/></style>` +
	`<data><shape id="bUn" type="text"><content fontFamily="思源黑体"><p>hi</p></content></shape></data></slide>`

func TestUpdateSlideDeclaredScopes(t *testing.T) {
	// Pre-flight stays at the two slides scopes: an XML-only page needs neither
	// wiki read nor media upload, and gating every call on them would force
	// unrelated consent.
	want := []string{"slides:presentation:update", "slides:presentation:write_only"}
	for _, identity := range []string{"user", "bot"} {
		if got := SlidesUpdateSlide.ScopesForIdentity(identity); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s preflight scopes = %#v, want %#v", identity, got, want)
		}
	}
	got := SlidesUpdateSlide.DeclaredScopesForIdentity("user")
	wantDeclared := []string{"slides:presentation:update", "slides:presentation:write_only", "wiki:node:read", "docs:document.media:upload"}
	if !reflect.DeepEqual(got, wantDeclared) {
		t.Fatalf("declared scopes = %#v, want %#v", got, wantDeclared)
	}
}

// TestUpdateSlideAliasMatchesCanonical guards the copy-derivation: the alias must
// not drift from +update-slide, and must stay out of --help.
func TestUpdateSlideAliasMatchesCanonical(t *testing.T) {
	if SlidesUpdate.Command != "+update" {
		t.Fatalf("alias command = %q", SlidesUpdate.Command)
	}
	if !SlidesUpdate.Hidden {
		t.Fatal("alias must be hidden so only the canonical name is advertised")
	}
	if SlidesUpdateSlide.Hidden {
		t.Fatal("canonical command must not be hidden")
	}
	if !reflect.DeepEqual(SlidesUpdate.Flags, SlidesUpdateSlide.Flags) {
		t.Fatal("alias flags drifted from canonical")
	}
	if !reflect.DeepEqual(SlidesUpdate.Scopes, SlidesUpdateSlide.Scopes) ||
		!reflect.DeepEqual(SlidesUpdate.AuthTypes, SlidesUpdateSlide.AuthTypes) {
		t.Fatal("alias scopes/identities drifted from canonical")
	}
}

// TestUpdateSlideSendsOnePagePart is the core of the design: whatever the page
// contains, exactly one block_replace part goes out and its block_id is the page
// id. That is what makes the backend swap the whole <slide>.
func TestUpdateSlideSendsOnePagePart(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide/replace",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 42}},
	}
	reg.Register(stub)

	if err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide",
		"--presentation", "pres_abc",
		"--slide-id", "pYw",
		"--content", testPageXML,
		"--as", "user",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body struct {
		Parts []struct {
			Action      string `json:"action"`
			BlockID     string `json:"block_id"`
			Replacement string `json:"replacement"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode body: %v raw=%s", err, stub.CapturedBody)
	}
	if len(body.Parts) != 1 {
		t.Fatalf("sent %d parts, want exactly 1: %s", len(body.Parts), stub.CapturedBody)
	}
	part := body.Parts[0]
	if part.Action != "block_replace" {
		t.Fatalf("action = %q, want block_replace", part.Action)
	}
	if part.BlockID != "pYw" {
		t.Fatalf("block_id = %q, want the page id pYw", part.BlockID)
	}
	// The caller's bytes travel through untouched apart from the injected root id,
	// so their formatting lands in the page.
	if !strings.HasPrefix(part.Replacement, `<slide id="pYw">`) {
		t.Fatalf("replacement root not stamped with the page id: %s", part.Replacement)
	}
	if !strings.Contains(part.Replacement, "思源黑体") || !strings.Contains(part.Replacement, `id="bUn"`) {
		t.Fatalf("replacement lost caller content: %s", part.Replacement)
	}

	data := decodeShortcutData(t, stdout)
	if data["slide_id"] != "pYw" || data["xml_presentation_id"] != "pres_abc" {
		t.Fatalf("result envelope = %#v", data)
	}
	if data["revision_id"] != float64(42) {
		t.Fatalf("revision_id = %#v, want 42", data["revision_id"])
	}
}

// TestUpdateSlideExistingRootIDIsKept covers the round trip: XML read back from
// the page already carries id="<slide_id>", and re-sending it must not be
// rewritten or rejected.
func TestUpdateSlideExistingRootIDIsKept(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide/replace",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 7}},
	}
	reg.Register(stub)

	if err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide",
		"--presentation", "pres_abc",
		"--slide-id", "pYw",
		"--content", `<slide id="pYw"><data/></slide>`,
		"--as", "user",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var body struct {
		Parts []struct{ Replacement string } `json:"parts"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode body: %v raw=%s", err, stub.CapturedBody)
	}
	if got := body.Parts[0].Replacement; got != `<slide id="pYw"><data/></slide>` {
		t.Fatalf("root id was rewritten: %s", got)
	}
}

// TestUpdateSlideFailedReasonIsAnError pins that a rejected write is reported as
// one. The single part carries the whole page, so a non-empty failed_reason means
// nothing was written — emitting an ok:true envelope would tell the caller their
// edit landed when it did not.
func TestUpdateSlideFailedReasonIsAnError(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide/replace",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"failed_reason":     "block with id 'pYw' not found",
			"failed_part_index": 0,
		}},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide",
		"--presentation", "pres_abc",
		"--slide-id", "pYw",
		"--content", testPageXML,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("a non-empty failed_reason must surface as an error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error must carry the backend reason, got %v", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("failed_reason must surface as a typed error, got %T", err)
	}
	if p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeInvalidParameters {
		t.Fatalf("category/subtype = %q/%q, want %q/%q",
			p.Category, p.Subtype, errs.CategoryAPI, errs.SubtypeInvalidParameters)
	}
	if p.Hint != updateSlideNotFoundHint {
		t.Fatalf("hint = %q, want %q", p.Hint, updateSlideNotFoundHint)
	}
	for _, want := range []string{"--presentation", "--slide-id", "+xml-get"} {
		if !strings.Contains(p.Hint, want) {
			t.Fatalf("not-found hint = %q, want it to mention %q", p.Hint, want)
		}
	}
	if strings.Contains(p.Hint, "--content") {
		t.Fatalf("not-found hint must not prescribe XML repair: %q", p.Hint)
	}
	if strings.Contains(stdout.String(), `"ok": true`) || strings.Contains(stdout.String(), `"ok":true`) {
		t.Fatalf("no success envelope may be printed, got %s", stdout.String())
	}
}

func TestUpdateSlideContentValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		slideID   string
		content   string
		wantErr   string
		wantCause bool
	}{
		{
			name:    "element_root_is_refused",
			slideID: "pYw",
			content: `<shape type="text"><content><p>x</p></content></shape>`,
			wantErr: "root must be <slide>",
		},
		{
			name:    "mismatched_root_id_is_refused",
			slideID: "pYw",
			content: `<slide id="pOther"><data/></slide>`,
			wantErr: "pass the page you mean to replace",
		},
		{
			name:    "second_root_is_refused",
			slideID: "pYw",
			content: `<slide><data/></slide><slide><data/></slide>`,
			wantErr: "found a second root",
		},
		{
			name:    "trailing_text_is_refused",
			slideID: "pYw",
			content: `<slide><data/></slide>oops`,
			wantErr: "text outside the <slide> element",
		},
		{
			name:      "malformed_xml_is_refused",
			slideID:   "pYw",
			content:   `<slide><data></slide>`,
			wantErr:   "not well-formed XML",
			wantCause: true,
		},
		{
			name:    "empty_content_is_refused",
			slideID: "pYw",
			content: "   ",
			wantErr: "--content cannot be empty",
		},
		{
			name:    "comment_before_root_is_allowed",
			slideID: "pYw",
			content: `<!-- generated --><slide><data/></slide>`,
			wantErr: "",
		},
		{
			name:    "whitespace_around_root_is_allowed",
			slideID: "pYw",
			content: "\n  <slide><data/></slide>\n",
			wantErr: "",
		},
		{
			name:    "namespaced_root_is_allowed",
			slideID: "pYw",
			content: `<slide xmlns="http://www.larkoffice.com/sml/2.0"><data/></slide>`,
			wantErr: "",
		},
		{
			// The page id has to be stamped onto the root, and the shared stamper
			// cannot rewrite a prefixed tag. Its own message is "no root element
			// found in XML fragment", which says nothing useful here.
			name:      "prefixed_root_is_refused_with_a_useful_message",
			slideID:   "pYw",
			content:   `<sml:slide xmlns:sml="http://www.larkoffice.com/sml/2.0"><data/></sml:slide>`,
			wantErr:   "namespace prefix such as <sml:slide>",
			wantCause: true,
		},
		{
			// Emptying the page is a legitimate thing to ask for, and follows from
			// "anything omitted is removed" — it must not be special-cased away.
			name:    "self_closing_root_empties_the_page",
			slideID: "pYw",
			content: `<slide/>`,
			wantErr: "",
		},
		{
			name:    "xml_declaration_is_allowed",
			slideID: "pYw",
			content: `<?xml version="1.0" encoding="UTF-8"?><slide><data/></slide>`,
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			// Optional because a refused input must not reach the API at all;
			// OnMatch turns "it was called anyway" into a failure. A page that
			// replaces everything is the wrong thing to send on a guess.
			called := false
			reg.Register(&httpmock.Stub{
				Method:   "POST",
				URL:      "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide/replace",
				Body:     map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 1}},
				Optional: true,
				OnMatch:  func(*http.Request) { called = true },
			})

			err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
				"+update-slide",
				"--presentation", "pres_abc",
				"--slide-id", tt.slideID,
				"--content", tt.content,
				"--as", "user",
			})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tt.wantErr)
			}
			if called {
				t.Fatal("a refused --content must not produce a request")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
			// Every refusal on this path goes through contentError, so the typed
			// shape is part of the contract, not something to check only when it
			// happens to be present.
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("refusal must be a *errs.ValidationError, got %T: %v", err, err)
			}
			if ve.Param != "--content" {
				t.Fatalf("validation error should blame --content, got %q", ve.Param)
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("refusal must carry a problem, got %T", err)
			}
			if p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("category/subtype = %q/%q, want %q/%q",
					p.Category, p.Subtype, errs.CategoryValidation, errs.SubtypeInvalidArgument)
			}
			if tt.wantCause && errors.Unwrap(err) == nil {
				t.Fatalf("error must preserve its underlying cause: %v", err)
			}
		})
	}
}

// TestUpdateSlideRejectsEmptySlideID keeps the page id from being omitted: an
// empty block_id would make the backend reject the part rather than replace the
// page.
func TestUpdateSlideRejectsEmptySlideID(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide",
		"--presentation", "pres_abc",
		"--slide-id", "   ",
		"--content", testPageXML,
		"--as", "user",
	})
	if err == nil || !strings.Contains(err.Error(), "--slide-id cannot be empty") {
		t.Fatalf("want empty slide-id error, got %v", err)
	}
}

// TestUpdateSlideRevisionAndTIDTravel pins the two pass-through query params.
func TestUpdateSlideRevisionAndTIDTravel(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	var query url.Values
	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide/replace",
		Body:    map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 9}},
		OnMatch: func(req *http.Request) { query = req.URL.Query() },
	})

	if err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide",
		"--presentation", "pres_abc",
		"--slide-id", "pYw",
		"--content", testPageXML,
		"--revision-id", "5",
		"--tid", "tid-123",
		"--as", "user",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if query.Get("revision_id") != "5" {
		t.Fatalf("revision_id = %q, want 5", query.Get("revision_id"))
	}
	if query.Get("tid") != "tid-123" {
		t.Fatalf("tid = %q, want tid-123", query.Get("tid"))
	}
	if query.Get("slide_id") != "pYw" {
		t.Fatalf("slide_id = %q", query.Get("slide_id"))
	}
}

// TestUpdateSlideOmitsEmptyTID keeps an unset --tid out of the query rather than
// sending tid="".
func TestUpdateSlideOmitsEmptyTID(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	var query url.Values
	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide/replace",
		Body:    map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 1}},
		OnMatch: func(req *http.Request) { query = req.URL.Query() },
	})

	if err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide",
		"--presentation", "pres_abc",
		"--slide-id", "pYw",
		"--content", testPageXML,
		"--as", "user",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := query["tid"]; ok {
		t.Fatalf("tid must be omitted when unset, query=%v", query)
	}
}

func TestCheckSlideRootReturnsRootID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "no_id", content: `<slide><data/></slide>`, want: ""},
		{name: "with_id", content: `<slide id="pYw"><data/></slide>`, want: "pYw"},
		{name: "id_after_other_attrs", content: `<slide xmlns="urn:x" id="pYw"><data/></slide>`, want: "pYw"},
		{
			// A nested element's id must not be mistaken for the page's.
			name:    "nested_id_ignored",
			content: `<slide><data><shape id="bUn"/></data></slide>`,
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := checkSlideRoot(tt.content)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("root id = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSlideReplaceAPIPathIsShared pins that both write commands address the same
// endpoint through the same helper, so they cannot drift apart.
func TestSlideReplaceAPIPathIsShared(t *testing.T) {
	if got := slideReplaceAPIPath("pres_abc"); got != "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide/replace" {
		t.Fatalf("path = %q", got)
	}
	if got := slideReplaceAPIPath("a/b"); strings.Contains(got, "a/b") {
		t.Fatalf("path segment must be encoded, got %q", got)
	}
}

// TestUpdateSlideInvalidParamHintIsCommandSpecific pins the hint away from the
// generic +replace-slide checklist. A generic invalid-parameter response does
// not establish that the page is missing, so this hint stays focused on the
// caller-controlled XML that can repair malformed-content failures.
func TestUpdateSlideInvalidParamHintIsCommandSpecific(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide/replace",
		Body:   map[string]interface{}{"code": 3350001, "msg": "invalid param"},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide",
		"--presentation", "pres_abc",
		"--slide-id", "pYw",
		"--content", testPageXML,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("3350001 must surface as an error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error carries no Problem: %v", err)
	}
	if p.Hint != updateSlideInvalidParamHint {
		t.Fatalf("hint = %q, want %q", p.Hint, updateSlideInvalidParamHint)
	}
	if !strings.Contains(p.Hint, "--content") {
		t.Fatalf("malformed-content hint must prescribe XML repair: %q", p.Hint)
	}
	for _, unwanted := range []string{"--presentation", "--slide-id", "+xml-get"} {
		if strings.Contains(p.Hint, unwanted) {
			t.Fatalf("malformed-content hint = %q, must not mention %q", p.Hint, unwanted)
		}
	}
}

func TestUpdateSlideNotFoundAPIErrorUsesPageRecoveryHint(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide/replace",
		Body:   map[string]interface{}{"code": 3350001, "msg": "block with id 'pYw' not found"},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide", "--presentation", "pres_abc", "--slide-id", "pYw",
		"--content", testPageXML, "--as", "user",
	})
	if err == nil {
		t.Fatal("3350001 not-found response must surface as an error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error carries no Problem: %v", err)
	}
	if p.Hint != updateSlideNotFoundHint {
		t.Fatalf("hint = %q, want %q", p.Hint, updateSlideNotFoundHint)
	}
}

// TestUpdateSlideKeepsUpstreamHint: a more specific hint from the backend or an
// earlier wrapper must win over the generic one.
func TestUpdateSlideKeepsUpstreamHint(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide/replace",
		Body:   map[string]interface{}{"code": 3350001, "msg": "invalid param"},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide", "--presentation", "pres_abc", "--slide-id", "pYw",
		"--content", testPageXML, "--as", "user",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	p, _ := errs.ProblemOf(err)
	before := p.Hint
	// Re-enriching must be idempotent: the hint already set stays put.
	if got := enrichUpdateSlideError(err); got != err {
		t.Fatal("enricher must return the same error value")
	}
	if p.Hint != before {
		t.Fatalf("hint was clobbered on a second pass: %q -> %q", before, p.Hint)
	}
}

// testPageImageXML references the same local image twice so the dedupe path is
// exercised: two <img> placeholders, one upload, both rewritten.
const testPageImageXML = `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>` +
	`<img src="@./chart.png" topLeftX="10" topLeftY="10" width="100" height="100"/>` +
	`<img src="@./chart.png" topLeftX="200" topLeftY="10" width="100" height="100"/>` +
	`</data></slide>`

// TestUpdateSlideUploadsImagePlaceholder is why +update-slide gained @path
// support: before it, replacing a page with a fresh local image meant calling
// +media-upload and splicing the token into the XML by hand. The same image
// referenced twice must still upload once.
func TestUpdateSlideUploadsImagePlaceholder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "chart.png"), []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"file_token": "tok_chart"}},
	})
	replaceStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_img/slide/replace",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 11}},
	}
	reg.Register(replaceStub)

	err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide",
		"--presentation", "pres_img",
		"--slide-id", "pYw",
		"--content", testPageImageXML,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body struct {
		Parts []struct{ Replacement string } `json:"parts"`
	}
	if err := json.Unmarshal(replaceStub.CapturedBody, &body); err != nil {
		t.Fatalf("decode body: %v raw=%s", err, replaceStub.CapturedBody)
	}
	if len(body.Parts) != 1 {
		t.Fatalf("sent %d parts, want exactly 1: %s", len(body.Parts), replaceStub.CapturedBody)
	}
	content := body.Parts[0].Replacement
	if strings.Contains(content, "@./chart.png") {
		t.Fatalf("placeholder was not rewritten: %s", content)
	}
	if strings.Count(content, `src="tok_chart"`) != 2 {
		t.Fatalf("both references should carry the uploaded token, got: %s", content)
	}

	data := decodeShortcutData(t, stdout)
	if data["images_uploaded"] != float64(1) {
		t.Fatalf("images_uploaded = %v, want 1 (deduped by path)", data["images_uploaded"])
	}
}

// TestUpdateSlideMissingImageFailsBeforeAnyCall proves the placeholder check
// runs in Validate: a bad path must not reach the API, or the caller is left
// guessing whether the page was overwritten.
func TestUpdateSlideMissingImageFailsBeforeAnyCall(t *testing.T) {
	withSlidesTestWorkingDir(t, t.TempDir())

	// Register nothing on purpose: httpmock rejects any unstubbed request, so
	// reaching the API surfaces as "no stub for POST ..." rather than the
	// ValidationError asserted below. That is the no-call assertion.
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide",
		"--presentation", "pres_abc",
		"--slide-id", "pYw",
		"--content", `<slide><data><img src="@./missing.png"/></data></slide>`,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for a missing image file")
	}
	// The Stat failure is wrapped, so a caller can still tell a missing file
	// apart from an unreadable one without parsing the message.
	assertValidationProblem(t, err, "--content", fs.ErrNotExist)
	// The path came from an <img> inside the XML, not from an @file passed to
	// --content. Naming the element keeps the caller from re-checking the flag
	// argument.
	if !strings.Contains(err.Error(), `<img src="@./missing.png">`) {
		t.Fatalf("error should quote the <img> placeholder, got %v", err)
	}
}

// TestUpdateSlideRejectsDirectoryImagePlaceholder covers the placeholder that
// exists but is not a file: Stat succeeds, so only the mode check stops a
// directory from being streamed at the upload endpoint.
func TestUpdateSlideRejectsDirectoryImagePlaceholder(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "chart.png"), 0o750); err != nil {
		t.Fatalf("make fixture dir: %v", err)
	}
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide",
		"--presentation", "pres_abc",
		"--slide-id", "pYw",
		"--content", `<slide><data><img src="@./chart.png"/></data></slide>`,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for a directory placeholder")
	}
	// No cause: Stat succeeded, so there is no underlying error to wrap.
	assertValidationProblem(t, err, "--content", nil)
	if !strings.Contains(err.Error(), "must be a regular file") {
		t.Fatalf("err = %v, want the regular-file diagnosis", err)
	}
}

// TestUpdateSlideDryRunPlansUploadsBeforeReplace proves the plan states the
// upload cost up front. The uploads are the irreversible half of the command —
// they land in the deck's media store even if the replace later fails — so a
// caller who dry-runs first must be able to see them coming.
func TestUpdateSlideDryRunPlansUploadsBeforeReplace(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "chart.png"), []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	slideXML := `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>` +
		`<img src="@./chart.png" topLeftX="10" topLeftY="10" width="100" height="100"/>` +
		`</data></slide>`

	err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide",
		"--presentation", "pres_img",
		"--slide-id", "pYw",
		"--content", slideXML,
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	if data["images_to_upload"] != float64(1) {
		t.Fatalf("images_to_upload = %v, want 1", data["images_to_upload"])
	}
	steps := decodeShortcutDryRunAPI(t, stdout)
	if len(steps) != 2 {
		t.Fatalf("planned %d calls, want upload then replace: %#v", len(steps), steps)
	}
	upload := assertDryRunStep(t, steps, 0, "POST", "/open-apis/drive/v1/medias/upload_all")
	uploadBody, _ := upload["body"].(map[string]interface{})
	if uploadBody["parent_type"] != slidesMediaParentType {
		t.Fatalf("upload parent_type = %v, want %q", uploadBody["parent_type"], slidesMediaParentType)
	}
	if uploadBody["parent_node"] != "pres_img" {
		t.Fatalf("upload parent_node = %v, want pres_img", uploadBody["parent_node"])
	}
	assertDryRunStep(t, steps, 1, "POST", "/open-apis/slides_ai/v1/xml_presentations/pres_img/slide/replace")
}

// TestUpdateSlideDryRunPlansSingleReplaceWithoutImages keeps the plain case
// honest: no @path means one POST and images_to_upload=0.
func TestUpdateSlideDryRunPlansSingleReplaceWithoutImages(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide",
		"--presentation", "pres_abc",
		"--slide-id", "pYw",
		"--content", testPageXML,
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	if data["images_to_upload"] != float64(0) {
		t.Fatalf("images_to_upload = %v, want 0", data["images_to_upload"])
	}
	steps := decodeShortcutDryRunAPI(t, stdout)
	if len(steps) != 1 {
		t.Fatalf("planned %d calls, want a single POST: %#v", len(steps), steps)
	}
	assertDryRunStep(t, steps, 0, "POST", "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide/replace")
}

// TestUpdateSlideReportsUploadedImagesWhenReplaceFails covers the partial-
// failure hint. The images are already in the deck's media store at that point,
// so a blind retry silently uploads a second copy; the hint is the only thing
// telling the caller they already landed.
func TestUpdateSlideReportsUploadedImagesWhenReplaceFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "chart.png"), []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"file_token": "tok_chart"}},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_img/slide/replace",
		Body:   map[string]interface{}{"code": 3350001, "msg": "invalid slide xml"},
	})

	slideXML := `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>` +
		`<img src="@./chart.png" topLeftX="10" topLeftY="10" width="100" height="100"/>` +
		`</data></slide>`

	err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide",
		"--presentation", "pres_img",
		"--slide-id", "pYw",
		"--content", slideXML,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected the backend rejection to surface")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %v, want typed problem metadata", err)
	}
	// The backend rejection must survive the appended hint: only the category
	// separates a preserved API error from the Internal/Unknown fallback wrap.
	if problem.Category != errs.CategoryAPI {
		t.Fatalf("Category = %q, want %q: the backend rejection must survive the hint",
			problem.Category, errs.CategoryAPI)
	}
	if !strings.Contains(problem.Hint, "1 image(s) were uploaded before the slide failed") {
		t.Fatalf("Hint = %q, want the already-uploaded count", problem.Hint)
	}
}

// TestUpdateSlideReportsProgressWhenUploadFails covers the other half of the
// partial-failure story: the replace was never attempted, so the hint must say
// the slide was not updated rather than leave the caller checking.
func TestUpdateSlideReportsProgressWhenUploadFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "chart.png"), []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body:   map[string]interface{}{"code": 1061002, "msg": "params error"},
	})
	// The replace endpoint is deliberately unstubbed: reaching it would fail as
	// "no stub for POST .../slide/replace", which is the no-write assertion.

	slideXML := `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>` +
		`<img src="@./chart.png" topLeftX="10" topLeftY="10" width="100" height="100"/>` +
		`</data></slide>`

	err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, []string{
		"+update-slide",
		"--presentation", "pres_img",
		"--slide-id", "pYw",
		"--content", slideXML,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected the upload failure to surface")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %v, want typed problem metadata", err)
	}
	if problem.Category != errs.CategoryAPI || problem.Subtype != errs.SubtypeInvalidParameters {
		t.Fatalf("problem = %#v, want %s/%s: the upload rejection must survive the hint",
			problem, errs.CategoryAPI, errs.SubtypeInvalidParameters)
	}
	if !strings.Contains(problem.Hint, "slide was not updated") {
		t.Fatalf("Hint = %q, want it to state the slide was not updated", problem.Hint)
	}
}
