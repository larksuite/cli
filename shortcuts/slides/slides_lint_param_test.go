// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// The lint switch is asserted on the wire rather than on the body-builder
// return value. What matters is what the backend receives, and only a real
// request proves the switch reached it: a builder test would still pass if a
// command stopped calling its own builder.
//
// Where it sits on the wire is asserted too, not just its value. The switch
// used to ride the query string, where the gateway dropped it silently because
// its api meta never declared the parameter — the request succeeded and the
// page went in unlinted. A test that only read the value would have been just
// as green then as it is now.

// captureLintBody records the decoded request body of a stub.
func captureLintBody(t *testing.T, into *map[string]interface{}) func(*http.Request) {
	t.Helper()
	return func(req *http.Request) {
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Errorf("decode request body %q: %v", raw, err)
			return
		}
		*into = decoded
	}
}

// assertLintSwitch checks the switch is in the body, carries the expected
// value, and left the query alone.
func assertLintSwitch(t *testing.T, body map[string]interface{}, query url.Values, want bool) {
	t.Helper()
	got, ok := body[lintXMLBodyKey]
	if !ok {
		t.Fatalf("body = %v, want a %s key", body, lintXMLBodyKey)
	}
	if got != want {
		t.Fatalf("body[%s] = %v, want %v", lintXMLBodyKey, got, want)
	}
	if q := query.Get(lintXMLBodyKey); q != "" {
		t.Fatalf("query carried %s = %q; the gateway does not bind it there, so a value in the query is a silent no-op",
			lintXMLBodyKey, q)
	}
}

// TestAddSlideLintXMLTravels pins both directions on +add-slide.
func TestAddSlideLintXMLTravels(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		extra []string
		want  bool
	}{
		{name: "default lints", want: true},
		{name: "--no-lint disables", extra: []string{"--no-lint"}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			var body map[string]interface{}
			var query url.Values
			capture := captureLintBody(t, &body)
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
				Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"slide_id": "slide_001"}},
				OnMatch: func(req *http.Request) {
					query = req.URL.Query()
					capture(req)
				},
			})

			args := append([]string{
				"+add-slide",
				"--presentation", "pres_abc",
				"--slide", testPageXML,
				"--as", "user",
			}, tc.extra...)
			if err := runSlidesShortcut(t, f, stdout, SlidesAddSlide, args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertLintSwitch(t, body, query, tc.want)
			// The switch is added to the body, not substituted for it.
			if _, ok := body["slide"]; !ok {
				t.Fatalf("body = %v, want the slide payload alongside the switch", body)
			}
		})
	}
}

// TestUpdateSlideLintXMLTravels pins both directions on +update-slide, and that
// the switch does not displace the parts payload.
func TestUpdateSlideLintXMLTravels(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		extra []string
		want  bool
	}{
		{name: "default lints", want: true},
		{name: "--no-lint disables", extra: []string{"--no-lint"}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			var body map[string]interface{}
			var query url.Values
			capture := captureLintBody(t, &body)
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide/replace",
				Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 9}},
				OnMatch: func(req *http.Request) {
					query = req.URL.Query()
					capture(req)
				},
			})

			args := append([]string{
				"+update-slide",
				"--presentation", "pres_abc",
				"--slide-id", "bUn",
				"--content", testPageXML,
				"--as", "user",
			}, tc.extra...)
			if err := runSlidesShortcut(t, f, stdout, SlidesUpdateSlide, args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertLintSwitch(t, body, query, tc.want)
			if _, ok := body["parts"]; !ok {
				t.Fatalf("body = %v, want the parts payload alongside the switch", body)
			}
			// slide_id genuinely is a query parameter here, and the gateway does
			// declare it. Moving the switch must not have disturbed it.
			if got := query.Get("slide_id"); got != "bUn" {
				t.Fatalf("slide_id = %q, want bUn", got)
			}
		})
	}
}

// TestCreateLintXMLTravels pins both directions on +create.
//
// +create makes the presentation first and then sends the pages one at a time,
// so the switch has to reach every page call — a deck where only the first page
// is linted is the failure this guards against. The create call itself sends the
// title-only shell, which has no page in it to lint, so it carries no switch at
// all.
func TestCreateLintXMLTravels(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		extra []string
		want  bool
	}{
		{name: "default lints", want: true},
		{name: "--no-lint disables", extra: []string{"--no-lint"}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			var createBody map[string]interface{}
			type slideCall struct {
				body  map[string]interface{}
				query url.Values
			}
			var slideCalls []slideCall
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/slides_ai/v1/xml_presentations",
				Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
					"xml_presentation_id": "pres_lint",
					"revision_id":         1,
				}},
				OnMatch: captureLintBody(t, &createBody),
			})
			for i := 0; i < 2; i++ {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_lint/slide",
					Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
						"slide_id":    "s_1",
						"revision_id": i + 2,
					}},
					OnMatch: func(req *http.Request) {
						call := slideCall{query: req.URL.Query()}
						captureLintBody(t, &call.body)(req)
						slideCalls = append(slideCalls, call)
					},
				})
			}

			args := append([]string{
				"+create",
				"--title", "Lint",
				"--slide", testPageXML,
				"--slide", testPageXML,
				"--as", "user",
			}, tc.extra...)
			if err := runSlidesCreateShortcut(t, f, stdout, args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// The shell holds the title and nothing else, so there is nothing for a
			// switch on it to decide.
			presentation, _ := createBody["xml_presentation"].(map[string]interface{})
			content, _ := presentation["content"].(string)
			if strings.Contains(content, "<slide") {
				t.Fatalf("create content = %q, want the empty shell", content)
			}
			if v, ok := createBody[lintXMLBodyKey]; ok {
				t.Fatalf("create body carried %s = %v, want it absent on a shell with no page in it", lintXMLBodyKey, v)
			}

			// Every page carries it, not just the first.
			if len(slideCalls) != 2 {
				t.Fatalf("slide calls = %d, want 2", len(slideCalls))
			}
			for _, call := range slideCalls {
				assertLintSwitch(t, call.body, call.query, tc.want)
			}
		})
	}
}

// TestReplaceSlideLintXMLTravels pins both directions on +replace-slide.
//
// This one is worth having even though it looks like the others: the backend
// lints the page the parts assemble into, not the parts, so the switch decides
// whether a fragment gets checked against the neighbours it lands next to.
// Dropping it here would leave the one path where "valid in isolation" and
// "valid on the page" differ as the only unchecked path.
func TestReplaceSlideLintXMLTravels(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		extra []string
		want  bool
	}{
		{name: "default lints", want: true},
		{name: "--no-lint disables", extra: []string{"--no-lint"}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			var body map[string]interface{}
			var query url.Values
			capture := captureLintBody(t, &body)
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide/replace",
				Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"revision_id": 11}},
				OnMatch: func(req *http.Request) {
					query = req.URL.Query()
					capture(req)
				},
			})

			args := append([]string{
				"+replace-slide",
				"--presentation", "pres_abc",
				"--slide-id", "bUn",
				"--parts", `[{"action":"block_insert","insertion":"<shape type=\"rect\" width=\"100\" height=\"100\"/>"}]`,
				"--as", "user",
			}, tc.extra...)
			if err := runSlidesShortcut(t, f, stdout, SlidesReplaceSlide, args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertLintSwitch(t, body, query, tc.want)
			if _, ok := body["parts"]; !ok {
				t.Fatalf("body = %v, want the parts payload alongside the switch", body)
			}
		})
	}
}

// TestReplacePagesLintXMLTravels pins both directions on +replace-pages.
//
// This path deletes the old page once the new one lands, so an unlinted create
// here is the only write that can destroy existing content on the strength of a
// page nobody checked. The switch has to reach every item, not just the first,
// for the same reason as on +create.
func TestReplacePagesLintXMLTravels(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		extra []string
		want  bool
	}{
		{name: "default lints", want: true},
		{name: "--no-lint disables", extra: []string{"--no-lint"}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			type slideCall struct {
				body  map[string]interface{}
				query url.Values
			}
			var createCalls []slideCall
			for i := 0; i < 2; i++ {
				reg.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
					Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
						"slide_id":    "new_1",
						"revision_id": i*2 + 11,
					}},
					OnMatch: func(req *http.Request) {
						call := slideCall{query: req.URL.Query()}
						captureLintBody(t, &call.body)(req)
						createCalls = append(createCalls, call)
					},
				})
				reg.Register(&httpmock.Stub{
					Method: "DELETE",
					URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide",
					Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
						"revision_id": i*2 + 12,
					}},
				})
			}

			pages := `[{"slide_id":"old1","content":` + strconv.Quote(testPageXML) +
				`},{"slide_id":"old2","content":` + strconv.Quote(testPageXML) + `}]`
			args := append([]string{
				"+replace-pages",
				"--presentation", "pres_abc",
				"--pages", pages,
				"--as", "user",
			}, tc.extra...)
			if err := runSlidesShortcut(t, f, stdout, SlidesReplacePages, args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(createCalls) != 2 {
				t.Fatalf("create calls = %d, want 2", len(createCalls))
			}
			for _, call := range createCalls {
				assertLintSwitch(t, call.body, call.query, tc.want)
				// The switch is added to the body, not substituted for it.
				if _, ok := call.body["before_slide_id"]; !ok {
					t.Fatalf("body = %v, want before_slide_id alongside the switch", call.body)
				}
			}
		})
	}
}

// TestLintXMLFlagIsOnEverySlideContentWriter guards the set. A new writer that
// forgets the flag ships pages the backend never checked, and the omission is
// invisible until something renders wrong.
func TestLintXMLFlagIsOnEverySlideContentWriter(t *testing.T) {
	t.Parallel()

	for _, sc := range []struct {
		name string
		want bool
		have []string
	}{
		{name: "+create", want: true, have: lintFlagNames(SlidesCreate.Flags)},
		{name: "+add-slide", want: true, have: lintFlagNames(SlidesAddSlide.Flags)},
		{name: "+update-slide", want: true, have: lintFlagNames(SlidesUpdateSlide.Flags)},
		// +replace-slide sends fragments rather than a whole <slide>, which is
		// exactly why it needs the flag: the lint's subject is the page they
		// assemble into, so this is the path where a fragment that is correct
		// on its own can still break the page.
		{name: "+replace-slide", want: true, have: lintFlagNames(SlidesReplaceSlide.Flags)},
		// +replace-pages deletes the old page once the new one lands, so it is
		// the one writer where skipping the lint can cost content that already
		// existed.
		{name: "+replace-pages", want: true, have: lintFlagNames(SlidesReplacePages.Flags)},
	} {
		if got := flagListHas(sc.have, noLintFlagName); got != sc.want {
			t.Fatalf("%s has --%s = %v, want %v", sc.name, noLintFlagName, got, sc.want)
		}
	}
}

func lintFlagNames(flags []common.Flag) []string {
	names := make([]string, 0, len(flags))
	for _, f := range flags {
		names = append(names, f.Name)
	}
	return names
}

func flagListHas(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
