// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestDeleteSlideDeclaredScopes(t *testing.T) {
	want := []string{"slides:presentation:update", "slides:presentation:write_only"}
	for _, identity := range []string{"user", "bot"} {
		if got := SlidesDeleteSlide.ScopesForIdentity(identity); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s preflight scopes = %#v, want %#v", identity, got, want)
		}
	}
	got := SlidesDeleteSlide.DeclaredScopesForIdentity("user")
	wantDeclared := []string{"slides:presentation:update", "slides:presentation:write_only", "wiki:node:read"}
	if !reflect.DeepEqual(got, wantDeclared) {
		t.Fatalf("declared scopes = %#v, want %#v", got, wantDeclared)
	}
}

// TestDeleteSlideRiskIsWriteNotHighRisk pins a deliberate divergence: the raw
// xml_presentation.slide.delete command is marked high-risk-write in its schema
// and demands --yes, while this shortcut does not. The rationale is that
// +replace-pages already deletes through the same endpoint at Risk "write", and
// a wrongly deleted page is recoverable via +history-revert. Flipping this to
// high-risk-write would silently start rejecting every existing caller that
// does not pass --yes, so make that a conscious edit rather than a drive-by.
func TestDeleteSlideRiskIsWriteNotHighRisk(t *testing.T) {
	if SlidesDeleteSlide.Risk != "write" {
		t.Fatalf("Risk = %q, want write (see the comment on this test before changing)", SlidesDeleteSlide.Risk)
	}
}

func TestDeleteSlideSendsSlideIDAndRevision(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	var gotQuery url.Values
	reg.Register(&httpmock.Stub{
		Method:  "DELETE",
		URL:     "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body:    map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 17}},
		OnMatch: func(req *http.Request) { gotQuery = req.URL.Query() },
	})

	err := runSlidesShortcut(t, f, stdout, SlidesDeleteSlide, []string{
		"+delete-slide",
		"--presentation", "pres_abc",
		"--slide-id", "slide_gone",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotQuery.Get("slide_id") != "slide_gone" {
		t.Fatalf("slide_id query = %q, want slide_gone", gotQuery.Get("slide_id"))
	}
	if gotQuery.Get("revision_id") != "-1" {
		t.Fatalf("revision_id query = %q, want -1", gotQuery.Get("revision_id"))
	}

	data := decodeShortcutData(t, stdout)
	if data["deleted"] != true {
		t.Fatalf("deleted = %v, want true", data["deleted"])
	}
	if data["slide_id"] != "slide_gone" {
		t.Fatalf("slide_id = %v, want slide_gone", data["slide_id"])
	}
	if data["xml_presentation_id"] != "pres_abc" {
		t.Fatalf("xml_presentation_id = %v, want pres_abc", data["xml_presentation_id"])
	}
	if data["revision_id"] != float64(17) {
		t.Fatalf("revision_id = %v, want 17", data["revision_id"])
	}
}

func TestDeleteSlideSendsExplicitRevision(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	var gotQuery url.Values
	reg.Register(&httpmock.Stub{
		Method:  "DELETE",
		URL:     "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body:    map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 2}},
		OnMatch: func(req *http.Request) { gotQuery = req.URL.Query() },
	})

	err := runSlidesShortcut(t, f, stdout, SlidesDeleteSlide, []string{
		"+delete-slide",
		"--presentation", "pres_abc",
		"--slide-id", "slide_gone",
		"--revision-id", "8",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotQuery.Get("revision_id") != "8" {
		t.Fatalf("revision_id query = %q, want 8", gotQuery.Get("revision_id"))
	}
}

// TestDeleteSlideRejectsBlankSlideID keeps a whitespace-only id from becoming a
// pointless round trip that the backend answers with an opaque 3350001.
func TestDeleteSlideRejectsBlankSlideID(t *testing.T) {
	t.Parallel()

	// Register nothing on purpose: httpmock rejects any unstubbed request, so
	// issuing the DELETE would surface as "no stub for DELETE .../slide"
	// rather than the ValidationError asserted below.
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	err := runSlidesShortcut(t, f, stdout, SlidesDeleteSlide, []string{
		"+delete-slide",
		"--presentation", "pres_abc",
		"--slide-id", "   ",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for a blank --slide-id")
	}
	// No cause: the flag value is rejected outright, nothing is wrapped.
	assertValidationProblem(t, err, "--slide-id", nil)
}

func TestDeleteSlideResolvesWikiURL(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{"obj_type": "slides", "obj_token": "pres_from_wiki"},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_from_wiki/slide",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 5}},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesDeleteSlide, []string{
		"+delete-slide",
		"--presentation", "https://example.feishu.cn/wiki/wikcnTOKEN",
		"--slide-id", "slide_gone",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutData(t, stdout)
	if data["xml_presentation_id"] != "pres_from_wiki" {
		t.Fatalf("xml_presentation_id = %v, want the wiki-resolved token", data["xml_presentation_id"])
	}
}

// TestDeleteSlideRejectsNonSlidesWiki guards the cross-type case: a wiki node
// pointing at a docx must fail before the DELETE, not delete something else.
func TestDeleteSlideRejectsNonSlidesWiki(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{"obj_type": "docx", "obj_token": "doc_abc"},
			},
		},
	})
	// Only the wiki stub is registered: an unstubbed DELETE fails with
	// "no stub for DELETE .../slide", so asserting the error names the
	// resolved obj_type also proves the DELETE never went out.
	err := runSlidesShortcut(t, f, stdout, SlidesDeleteSlide, []string{
		"+delete-slide",
		"--presentation", "https://example.feishu.cn/wiki/wikcnTOKEN",
		"--slide-id", "slide_gone",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected an error when the wiki node is not a slides deck")
	}
	// Tagged --presentation, not --slide-id: the wrong input is the URL. No
	// cause — the mismatch is read off a successful get_node response.
	assertValidationProblem(t, err, "--presentation", nil)
	if !strings.Contains(err.Error(), "docx") {
		t.Fatalf("err = %v, want mention of the resolved obj_type", err)
	}
}

// TestDeleteSlideDryRunNamesTheTarget covers the plan callers are told to run
// before a destructive command: it must state which presentation and which page
// will be hit, since that is the whole point of dry-running a delete.
func TestDeleteSlideDryRunNamesTheTarget(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesDeleteSlide, []string{
		"+delete-slide",
		"--presentation", "pres_abc",
		"--slide-id", "slide_gone",
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	if data["slide_id"] != "slide_gone" {
		t.Fatalf("slide_id = %v, want slide_gone", data["slide_id"])
	}
	steps := decodeShortcutDryRunAPI(t, stdout)
	if len(steps) != 1 {
		t.Fatalf("planned %d calls, want a single DELETE: %#v", len(steps), steps)
	}
	step := assertDryRunStep(t, steps, 0, "DELETE", "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide")

	params, _ := step["params"].(map[string]interface{})
	if params["slide_id"] != "slide_gone" {
		t.Fatalf("params.slide_id = %v, want slide_gone", params["slide_id"])
	}
	if params["revision_id"] != float64(-1) {
		t.Fatalf("params.revision_id = %v, want -1", params["revision_id"])
	}
}

// TestDeleteSlideDryRunPlansWikiResolution pins that a wiki ref costs an extra
// declared step. Sending the wiki token straight to the slides API would delete
// from nothing — or, worse, from something else that happens to match.
func TestDeleteSlideDryRunPlansWikiResolution(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesDeleteSlide, []string{
		"+delete-slide",
		"--presentation", "https://example.feishu.cn/wiki/wikcnTOKEN",
		"--slide-id", "slide_gone",
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	steps := decodeShortcutDryRunAPI(t, stdout)
	if len(steps) != 2 {
		t.Fatalf("planned %d calls, want resolve then delete: %#v", len(steps), steps)
	}
	resolve := assertDryRunStep(t, steps, 0, "GET", "/open-apis/wiki/v2/spaces/get_node")
	resolveParams, _ := resolve["params"].(map[string]interface{})
	if resolveParams["token"] != "wikcnTOKEN" {
		t.Fatalf("resolve token = %v, want wikcnTOKEN", resolveParams["token"])
	}
	del := assertDryRunStep(t, steps, 1, "DELETE", "/open-apis/slides_ai/v1/xml_presentations/%3Cresolved_slides_token%3E/slide")
	delParams, _ := del["params"].(map[string]interface{})
	if delParams["slide_id"] != "slide_gone" {
		t.Fatalf("params.slide_id = %v, want slide_gone on the resolved step", delParams["slide_id"])
	}
}

// TestDeleteSlideOmitsRevisionWhenAbsent keeps a missing upstream revision from
// being reported as revision 0, which a caller could then pass back as an
// optimistic lock and have rejected — or, worse, matched.
func TestDeleteSlideOmitsRevisionWhenAbsent(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesDeleteSlide, []string{
		"+delete-slide",
		"--presentation", "pres_abc",
		"--slide-id", "slide_gone",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	if data["deleted"] != true {
		t.Fatalf("deleted = %v, want true", data["deleted"])
	}
	if _, ok := data["revision_id"]; ok {
		t.Fatalf("revision_id should be omitted when absent upstream: %#v", data)
	}
}

// TestDeleteSlideSurfacesBackendFailure pins that a backend rejection is not
// reported as a successful delete. "deleted": true is written unconditionally
// once the call returns, so the error path has to stop before it.
func TestDeleteSlideSurfacesBackendFailure(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body:   map[string]interface{}{"code": 3350001, "msg": "slide not found"},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesDeleteSlide, []string{
		"+delete-slide",
		"--presentation", "pres_abc",
		"--slide-id", "slide_missing",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected the backend rejection to surface")
	}
	if strings.Contains(stdout.String(), `"deleted"`) {
		t.Fatalf("a failed delete must not print a deleted envelope: %s", stdout.String())
	}
}

// TestDeleteSlideRejectsUnsupportedPresentation keeps a docx URL from reaching
// the slides API as if it were a presentation token.
func TestDeleteSlideRejectsUnsupportedPresentation(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesDeleteSlide, []string{
		"+delete-slide",
		"--presentation", "https://example.feishu.cn/docx/doccnTOKEN",
		"--slide-id", "slide_gone",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for a docx URL")
	}
	assertValidationProblem(t, err, "--presentation", nil)
}
