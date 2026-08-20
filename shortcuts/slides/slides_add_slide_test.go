// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"encoding/json"
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

const testSlideXML = `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data><shape type="text" topLeftX="80" topLeftY="80" width="800" height="120"><content textType="title"><p>hi</p></content></shape></data></slide>`

func TestAddSlideDeclaredScopes(t *testing.T) {
	// Pre-flight must stay at the two slides scopes: an XML-only page needs
	// neither wiki read nor media upload, and gating every call on them would
	// force unrelated consent.
	want := []string{"slides:presentation:update", "slides:presentation:write_only"}
	for _, identity := range []string{"user", "bot"} {
		if got := SlidesAddSlide.ScopesForIdentity(identity); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s preflight scopes = %#v, want %#v", identity, got, want)
		}
	}
	got := SlidesAddSlide.DeclaredScopesForIdentity("user")
	wantDeclared := []string{"slides:presentation:update", "slides:presentation:write_only", "wiki:node:read", "docs:document.media:upload"}
	if !reflect.DeepEqual(got, wantDeclared) {
		t.Fatalf("declared scopes = %#v, want %#v", got, wantDeclared)
	}
}

// TestAddSlideOmitsBeforeSlideIDWhenUnset guards the append case: the backend
// appends only when before_slide_id is absent, so sending "" would turn a
// plain append into a lookup of a slide that does not exist.
func TestAddSlideOmitsBeforeSlideIDWhenUnset(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	var gotQuery url.Values
	stub := &httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body:    map[string]interface{}{"code": 0, "data": map[string]interface{}{"slide_id": "slide_new", "revision_id": 9}},
		OnMatch: func(req *http.Request) { gotQuery = req.URL.Query() },
	}
	reg.Register(stub)

	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "pres_abc",
		"--slide", testSlideXML,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := body["before_slide_id"]; ok {
		t.Fatalf("before_slide_id must be absent when --before-slide-id is unset, got body %s", stub.CapturedBody)
	}
	slide, _ := body["slide"].(map[string]interface{})
	if slide["content"] != testSlideXML {
		t.Fatalf("slide.content = %v, want the XML passed to --slide", slide["content"])
	}
	if gotQuery.Get("revision_id") != "-1" {
		t.Fatalf("revision_id query = %q, want -1", gotQuery.Get("revision_id"))
	}

	data := decodeShortcutData(t, stdout)
	if data["slide_id"] != "slide_new" {
		t.Fatalf("slide_id = %v, want slide_new", data["slide_id"])
	}
	if data["revision_id"] != float64(9) {
		t.Fatalf("revision_id = %v, want 9", data["revision_id"])
	}
	if data["xml_presentation_id"] != "pres_abc" {
		t.Fatalf("xml_presentation_id = %v, want pres_abc", data["xml_presentation_id"])
	}
	if _, ok := data["before_slide_id"]; ok {
		t.Fatalf("output should not report before_slide_id when unset: %#v", data)
	}
}

func TestAddSlideSendsBeforeSlideID(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	var gotQuery url.Values
	stub := &httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body:    map[string]interface{}{"code": 0, "data": map[string]interface{}{"slide_id": "slide_new", "revision_id": 3}},
		OnMatch: func(req *http.Request) { gotQuery = req.URL.Query() },
	}
	reg.Register(stub)

	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "pres_abc",
		"--slide", testSlideXML,
		"--before-slide-id", "slide_target",
		"--revision-id", "12",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["before_slide_id"] != "slide_target" {
		t.Fatalf("before_slide_id = %v, want slide_target", body["before_slide_id"])
	}
	if gotQuery.Get("revision_id") != "12" {
		t.Fatalf("revision_id query = %q, want 12", gotQuery.Get("revision_id"))
	}

	data := decodeShortcutData(t, stdout)
	if data["before_slide_id"] != "slide_target" {
		t.Fatalf("output before_slide_id = %v, want slide_target", data["before_slide_id"])
	}
}

// TestAddSlideUploadsImagePlaceholder is the reason this shortcut exists for
// image-bearing pages: before it, adding one to an existing deck meant calling
// +media-upload and splicing the token in by hand.
func TestAddSlideUploadsImagePlaceholder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "chart.png"), []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"file_token": "tok_chart"}},
	}
	slideStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_img/slide",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"slide_id": "s_img", "revision_id": 4}},
	}
	reg.Register(uploadStub)
	reg.Register(slideStub)

	// The same image referenced twice must still upload once.
	slideXML := `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>` +
		`<img src="@./chart.png" topLeftX="10" topLeftY="10" width="100" height="100"/>` +
		`<img src="@./chart.png" topLeftX="200" topLeftY="10" width="100" height="100"/>` +
		`</data></slide>`

	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "pres_img",
		"--slide", slideXML,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(slideStub.CapturedBody, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	slide, _ := body["slide"].(map[string]interface{})
	content, _ := slide["content"].(string)
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

func TestAddSlideRejectsNonSlideRoot(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "pres_abc",
		"--slide", `<presentation xmlns="https://www.larkoffice.com/sml/2.0"><slide/></presentation>`,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for a <presentation> root")
	}
	// The flag tag is what lets an agent know which input to fix; the shared
	// XML validator alone reports only the structural problem, and it is kept as
	// the cause so the structural detail survives the re-tagging.
	assertValidationProblem(t, err, "--slide", errAnyCause)
	if !strings.Contains(err.Error(), "want <slide>") {
		t.Fatalf("message should name the expected root element: %v", err)
	}
}

func TestAddSlideRejectsEmptySlide(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "pres_abc",
		"--slide", "   ",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for a blank --slide")
	}
	// No cause: the flag value is rejected outright, nothing is wrapped.
	assertValidationProblem(t, err, "--slide", nil)
}

// TestAddSlideMissingImageFailsBeforeAnyCall proves the placeholder check runs
// in Validate: a bad path must not reach the API, or the caller is left
// guessing whether a page was created.
func TestAddSlideMissingImageFailsBeforeAnyCall(t *testing.T) {
	withSlidesTestWorkingDir(t, t.TempDir())

	// Register nothing on purpose: httpmock rejects any unstubbed request, so
	// reaching the API would surface as "no stub for POST .../slide" rather
	// than the ValidationError asserted below. That is the no-call assertion.
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "pres_abc",
		"--slide", `<slide xmlns="x"><data><img src="@./missing.png"/></data></slide>`,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for a missing image file")
	}
	// The Stat failure is wrapped, so a caller can still tell a missing file
	// apart from an unreadable one without parsing the message.
	assertValidationProblem(t, err, "--slide", fs.ErrNotExist)
	// The path came from an <img> inside the XML, not from an @file passed to
	// --slide. Naming the element keeps the caller from re-checking the flag
	// argument, which is what "--slide @./missing.png" used to imply.
	if !strings.Contains(err.Error(), `<img src="@./missing.png">`) {
		t.Fatalf("error should quote the <img> placeholder, got %v", err)
	}
	if !strings.HasSuffix(err.Error(), "file not found") {
		t.Fatalf("a missing file should be reported as such, got %v", err)
	}
}

func TestAddSlideResolvesWikiURL(t *testing.T) {
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
	slideStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_from_wiki/slide",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"slide_id": "s_wiki", "revision_id": 2}},
	}
	reg.Register(slideStub)

	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "https://example.feishu.cn/wiki/wikcnTOKEN",
		"--slide", testSlideXML,
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

// TestAddSlideDryRunPlansSinglePost covers the plain append plan: one POST, no
// upload step, and the same query/body builders Execute uses. Dry-run is what
// callers inspect before a write, so a plan that disagrees with the real
// request is worse than no plan.
func TestAddSlideDryRunPlansSinglePost(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "pres_abc",
		"--slide", testSlideXML,
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
	step := assertDryRunStep(t, steps, 0, "POST", "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide")

	params, _ := step["params"].(map[string]interface{})
	if params["revision_id"] != float64(-1) {
		t.Fatalf("params.revision_id = %v, want -1", params["revision_id"])
	}
	body, _ := step["body"].(map[string]interface{})
	slide, _ := body["slide"].(map[string]interface{})
	if slide["content"] != testSlideXML {
		t.Fatalf("body.slide.content = %v, want the --slide XML", slide["content"])
	}
	// Same rule as the live request: appending is expressed by omitting the key.
	if _, ok := body["before_slide_id"]; ok {
		t.Fatalf("before_slide_id must be absent when appending: %#v", body)
	}
}

// TestAddSlideDryRunPlansUploadsBeforePost proves the plan states the upload
// cost up front. The uploads are the irreversible half of the command — they
// land in the deck's media store even if the page later fails — so a caller who
// dry-runs first must be able to see them coming.
func TestAddSlideDryRunPlansUploadsBeforePost(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "chart.png"), []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	slideXML := `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>` +
		`<img src="@./chart.png" topLeftX="10" topLeftY="10" width="100" height="100"/>` +
		`</data></slide>`

	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "pres_img",
		"--slide", slideXML,
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
		t.Fatalf("planned %d calls, want upload then add: %#v", len(steps), steps)
	}
	upload := assertDryRunStep(t, steps, 0, "POST", "/open-apis/drive/v1/medias/upload_all")
	uploadBody, _ := upload["body"].(map[string]interface{})
	if uploadBody["parent_type"] != slideFileParentType {
		t.Fatalf("upload parent_type = %v, want %q", uploadBody["parent_type"], slideFileParentType)
	}
	// parent_node is the presentation itself, not a placeholder: without a wiki
	// hop the token is already known at plan time.
	if uploadBody["parent_node"] != "pres_img" {
		t.Fatalf("upload parent_node = %v, want pres_img", uploadBody["parent_node"])
	}
	assertDryRunStep(t, steps, 1, "POST", "/open-apis/slides_ai/v1/xml_presentations/pres_img/slide")
}

// TestAddSlideDryRunPlansWikiResolution pins that a wiki ref is declared as its
// own first step. The presentation token is unknown until that call returns, so
// the plan must show a placeholder rather than sending the wiki token to the
// slides API.
func TestAddSlideDryRunPlansWikiResolution(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "https://example.feishu.cn/wiki/wikcnTOKEN",
		"--slide", testSlideXML,
		"--before-slide-id", "slide_target",
		"--dry-run",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	steps := decodeShortcutDryRunAPI(t, stdout)
	if len(steps) != 2 {
		t.Fatalf("planned %d calls, want resolve then add: %#v", len(steps), steps)
	}
	resolve := assertDryRunStep(t, steps, 0, "GET", "/open-apis/wiki/v2/spaces/get_node")
	resolveParams, _ := resolve["params"].(map[string]interface{})
	if resolveParams["token"] != "wikcnTOKEN" {
		t.Fatalf("resolve token = %v, want wikcnTOKEN", resolveParams["token"])
	}
	add := assertDryRunStep(t, steps, 1, "POST", "/open-apis/slides_ai/v1/xml_presentations/%3Cresolved_slides_token%3E/slide")

	addParams, _ := add["params"].(map[string]interface{})
	if addParams["revision_id"] != float64(-1) {
		t.Fatalf("params.revision_id = %v, want -1 on the resolved step", addParams["revision_id"])
	}
	addBody, _ := add["body"].(map[string]interface{})
	if addBody["before_slide_id"] != "slide_target" {
		t.Fatalf("body.before_slide_id = %v, want slide_target", addBody["before_slide_id"])
	}
}

// TestAddSlideRejectsResponseWithoutSlideID guards against reporting success
// for a page nobody can address afterwards: without a slide_id the caller
// cannot delete, replace or position relative to what was just created.
func TestAddSlideRejectsResponseWithoutSlideID(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 4}},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "pres_abc",
		"--slide", testSlideXML,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected an error when the response carries no slide_id")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("problem = %#v, ok = %v, want CategoryInternal/SubtypeInvalidResponse", problem, ok)
	}
}

// TestAddSlidePassesThroughIssues keeps backend schema warnings visible. The
// page is accepted but stripped of the unsupported markup in this case, so
// swallowing issues would leave the caller believing the deck says what they
// wrote. The stub mirrors the wire format: a single string, not a list.
func TestAddSlidePassesThroughIssues(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"slide_id": "slide_new",
			"issues":   "[issue=unsupported_attr tag=<strong> attr=style]",
		}},
	})

	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "pres_abc",
		"--slide", testSlideXML,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutData(t, stdout)
	if issues, _ := data["issues"].(string); issues != "[issue=unsupported_attr tag=<strong> attr=style]" {
		t.Fatalf("issues = %#v, want the backend warning passed through verbatim", data["issues"])
	}
	// No revision_id in the response: the output must omit the key rather than
	// report a zero revision the caller could pass back for optimistic locking.
	if _, ok := data["revision_id"]; ok {
		t.Fatalf("revision_id should be omitted when absent upstream: %#v", data)
	}
}

// TestAddSlideReportsUploadedImagesWhenPageFails covers the partial-failure
// hint. The images are already in the deck's media store at that point, so a
// blind retry silently uploads a second copy of every file; the hint is the
// only thing that tells the caller to reuse the tokens instead.
func TestAddSlideReportsUploadedImagesWhenPageFails(t *testing.T) {
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
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_img/slide",
		Body:   map[string]interface{}{"code": 3350001, "msg": "invalid slide xml"},
	})

	slideXML := `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>` +
		`<img src="@./chart.png" topLeftX="10" topLeftY="10" width="100" height="100"/>` +
		`</data></slide>`

	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "pres_img",
		"--slide", slideXML,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected the backend rejection to surface")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %v, want typed problem metadata", err)
	}
	// appendSlidesProgressHint attaches the hint on both of its branches, so the
	// hint alone cannot tell a preserved backend error from the Internal/Unknown
	// wrap the fallback produces. Only the category separates them. The subtype
	// stays unasserted here because it is the "unknown" catch-all, which errs
	// expects producers to narrow later.
	if problem.Category != errs.CategoryAPI {
		t.Fatalf("Category = %q, want %q: the backend rejection must survive the hint",
			problem.Category, errs.CategoryAPI)
	}
	if !strings.Contains(problem.Hint, "1 image(s) were uploaded before the page failed") {
		t.Fatalf("Hint = %q, want the already-uploaded count", problem.Hint)
	}
}

// TestAddSlideReportsProgressWhenUploadFails covers the other half of the
// partial-failure story: the page was never attempted, so the hint must say so
// rather than leave the caller checking whether a page appeared.
func TestAddSlideReportsProgressWhenUploadFails(t *testing.T) {
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
	// The slide endpoint is deliberately unstubbed: reaching it would fail as
	// "no stub for POST .../slide", which is the no-page-created assertion.

	slideXML := `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>` +
		`<img src="@./chart.png" topLeftX="10" topLeftY="10" width="100" height="100"/>` +
		`</data></slide>`

	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "pres_img",
		"--slide", slideXML,
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
	if !strings.Contains(problem.Hint, "no page was added") {
		t.Fatalf("Hint = %q, want it to state no page was added", problem.Hint)
	}
}

// TestAddSlideRejectsUnsupportedPresentation keeps a docx URL from being sent
// to the slides API as if it were a presentation token.
func TestAddSlideRejectsUnsupportedPresentation(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "https://example.feishu.cn/docx/doccnTOKEN",
		"--slide", testSlideXML,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for a docx URL")
	}
	assertValidationProblem(t, err, "--presentation", nil)
}

// TestAddSlideRejectsDirectoryImagePlaceholder covers the placeholder that
// exists but is not a file: Stat succeeds, so only the mode check stops a
// directory from being streamed at the upload endpoint.
func TestAddSlideRejectsDirectoryImagePlaceholder(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "chart.png"), 0o750); err != nil {
		t.Fatalf("make fixture dir: %v", err)
	}
	withSlidesTestWorkingDir(t, dir)

	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, []string{
		"+add-slide",
		"--presentation", "pres_abc",
		"--slide", `<slide xmlns="x"><data><img src="@./chart.png"/></data></slide>`,
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected a validation error for a directory placeholder")
	}
	// No cause: Stat succeeded, so there is no underlying error to wrap.
	assertValidationProblem(t, err, "--slide", nil)
	if !strings.Contains(err.Error(), "must be a regular file") {
		t.Fatalf("err = %v, want the regular-file diagnosis", err)
	}
}
