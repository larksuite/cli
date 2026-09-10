// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

// countingTransport replays responders in order and counts requests. Running out
// of responders is a transport error, so a loop that issues one request too many
// fails loudly instead of quietly succeeding.
func countingTransport(responders ...func() (*http.Response, error)) (http.RoundTripper, *int) {
	i, n := 0, 0
	return roundTripFunc(func(*http.Request) (*http.Response, error) {
		n++
		if i >= len(responders) {
			return nil, errors.New("loop issued a request past the last responder")
		}
		r := responders[i]
		i++
		return r()
	}), &n
}

// codeShape is one spelling of the business code field. `zero` is whether the
// loop may paginate from it; whether it is a business error is not declared
// here but read from CheckResponse itself, so the table asserts parity with the
// classifier rather than a hardcoded answer.
type codeShape struct {
	name  string
	field string // the literal placed after `"code":`, or "" for no code field
	zero  bool
}

// The rows cover every boundary a float would blur: spellings of 0, integers at
// 2^53 and int64, fractions, underflow, out-of-range, and non-numbers.
var codeShapes = []codeShape{
	{"missing", "", false},
	{"0", "0", true},
	{"-0", "-0", true},
	{"0.0", "0.0", true},
	{"0e10", "0e10", true},
	{"230027", "230027", false},
	{"230027.0", "230027.0", false},
	{"230027.5", "230027.5", false},
	{"999999", "999999", false},
	{"2^53+1 as decimal", "9007199254740993.0", false},
	{"int64 max", "9223372036854775807", false},
	{"int64+1", "9223372036854775808", false},
	{"null", "null", false},
	{"string", `"230027"`, false},
	{"object", "{}", false},
	{"0.5", "0.5", false},
	{"1e-324", "1e-324", false},
	{"-1e-400", "-1e-400", false},
	{"1e20", "1e20", false},
	{"1e999", "1e999", false},
}

// pageVariants are the shapes a page's data can take, independent of its code.
// The two cursor variants pin both supported field names; following either is
// observable as an extra request. "terminal" is a list that ends the run;
// "bare" has no data at all — the shape a failed later page most often has, and
// the one a rule that only looks at cursors misses.
var pageVariants = []string{"cursor", "next cursor", "terminal", "bare"}

type shapeVariant struct {
	shape   codeShape
	variant string
}

// shapeVariants is the cross product of codeShapes and pageVariants.
func shapeVariants() []shapeVariant {
	var out []shapeVariant
	for _, v := range pageVariants {
		for _, sh := range codeShapes {
			out = append(out, shapeVariant{sh, v})
		}
	}
	return out
}

// body builds a page for a shape and a data variant.
func (s codeShape) body(variant string) string {
	var data string
	switch variant {
	case "cursor":
		data = `,"data":{"items":[{"id":"x"}],"has_more":true,"page_token":"next"}`
	case "next cursor":
		data = `,"data":{"items":[{"id":"x"}],"has_more":true,"next_page_token":"next"}`
	case "terminal":
		data = `,"data":{"items":[{"id":"x"}],"has_more":false}`
	case "bare":
		data = ``
	default:
		panic("unknown variant " + variant)
	}
	if s.field == "" {
		return `{"msg":"m"` + data + `}`
	}
	return `{"code":` + s.field + `,"msg":"m"` + data + `}`
}

// parsed decodes a terminal body the way ParseJSONResponse does.
func (s codeShape) parsed(t *testing.T) interface{} {
	t.Helper()
	var v interface{}
	dec := json.NewDecoder(strings.NewReader(s.body("terminal")))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// statusCategory is what httpStatusError assigns a status, read from the
// function rather than restated here.
func statusCategory(status int) errs.Category {
	return errs.CategoryOf(httpStatusError(status, nil))
}

// The success check's contract: a JSON number is zero only if every mantissa
// digit is 0. This is what separates "may paginate" from "may not", so it is
// pinned per literal.
func TestPageDeclaresSuccess(t *testing.T) {
	for _, sh := range codeShapes {
		t.Run(sh.name, func(t *testing.T) {
			if got := pageDeclaresSuccess(sh.parsed(t)); got != sh.zero {
				t.Errorf("pageDeclaresSuccess(code=%s) = %v, want %v", sh.field, got, sh.zero)
			}
		})
	}
	for _, tc := range []struct {
		name string
		v    interface{}
		want bool
	}{
		{"not an object", []interface{}{}, false},
		{"JSON null page", nil, false},
		{"Go int 0", map[string]interface{}{"code": 0}, true},
		{"Go int 7", map[string]interface{}{"code": 7}, false},
		{"Go float64 0", map[string]interface{}{"code": 0.0}, true},
		{"zero mantissa, absurd exponent", map[string]interface{}{"code": json.Number("0e1000000")}, true},
		{"nonzero mantissa, absurd exponent", map[string]interface{}{"code": json.Number("1e1000000")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := pageDeclaresSuccess(tc.v); got != tc.want {
				t.Errorf("pageDeclaresSuccess(%v) = %v, want %v", tc.v, got, tc.want)
			}
		})
	}
}

// The pagination rule as a decision table. Every cell's expected outcome is
// derived from three facts — what CheckResponse says about the body, the HTTP
// status, and whether the page is the first — never written per row, so the
// test states the rule once and checks the loop follows it everywhere:
//
//  1. CheckResponse classifies the body first, exactly as HandleResponse does
//     for plain `api`; a business error wins over the status.
//  2. Otherwise an HTTP status of 400 or more classifies the page.
//  3. Otherwise, whether the page is a step in the pagination is decided by
//     whether its code is exactly 0 — and the rule is asymmetric on purpose:
//     a later page that is not fails the run before it is emitted; a first
//     page that is not and carries a continuation token is refused before output;
//     a first page that is not and carries no continuation token is output on the
//     existing contract, because there is nothing to paginate.
//  4. A page that is a step is emitted, accumulated, and its cursor followed.
//  5. Page position decides only how a failure from 1 or 2 is reported: page 1
//     is handed back whole with a nil error for the command layer to classify;
//     a later page fails the run.
//
// Every shape runs with both cursor field names, as a terminal list, and with no
// data at all, so the table sees the failed-page-without-data case a cursor-only
// rule misses, and the request count proves rule 4 in both directions. Both
// entry points run the same rows.
func TestPagination_DecisionTable(t *testing.T) {
	req := RawApiRequest{Method: "POST", URL: "/open-apis/test", As: "bot"}
	opts := PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: "bot"}

	for _, mode := range []string{"PaginateAll", "StreamPages"} {
		for _, first := range []bool{true, false} {
			for _, status := range []int{200, 403, 502} {
				for _, sv := range shapeVariants() {
					sh, variant := sv.shape, sv.variant
					position := "page1"
					if !first {
						position = "page2"
					}
					t.Run(fmt.Sprintf("%s/%s/%d/%s/%s", mode, position, status, variant, sh.name), func(t *testing.T) {
						var responders []func() (*http.Response, error)
						if !first {
							responders = append(responders, firstPageHasMore)
						}
						responders = append(responders, ctResponse(status, "", sh.body(variant)))
						responders = append(responders, ctResponse(200, "application/json",
							`{"code":0,"data":{"items":[{"id":"after"}],"has_more":false}}`)) // reached only by following the cursor
						rt, reqs := countingTransport(responders...)
						ac, _ := newTestAPIClient(t, rt)

						// the classifier's own verdict on this body — the oracle for step 1
						bizErr := ac.CheckResponse(sh.parsed(t), "bot")

						callbacks := 0
						var result interface{}
						var err error
						if mode == "PaginateAll" {
							result, err = ac.PaginateAll(context.Background(), req, opts)
						} else {
							result, _, err = ac.StreamPages(context.Background(), req,
								func([]interface{}) error { callbacks++; return nil }, opts)
						}

						pageIndex := 1
						if !first {
							pageIndex = 2
						}
						hasCursor := variant == "cursor" || variant == "next cursor"
						hasItems := variant != "bare"
						classified := bizErr == nil && status < 400                // rules 1-2 passed
						refused := classified && !sh.zero && (!first || hasCursor) // rule 3: not a step, not output
						isOutput := classified && !refused                         // rules 3-4: output on the existing contract
						cursorFollowed := isOutput && sh.zero && hasCursor         // rule 4

						// ---- requests: only a zero page's cursor is followed ----
						wantReqs := pageIndex
						if cursorFollowed {
							wantReqs++
						}
						if *reqs != wantReqs {
							t.Errorf("requests = %d, want %d", *reqs, wantReqs)
						}

						// ---- callbacks: an output page with items is emitted; nothing else is ----
						if mode == "StreamPages" {
							wantCallbacks := pageIndex - 1 // the good first page, on page-2 rows
							if isOutput && hasItems {
								wantCallbacks++
							}
							if cursorFollowed {
								wantCallbacks++ // the page after the cursor
							}
							if callbacks != wantCallbacks {
								t.Errorf("callbacks = %d, want %d", callbacks, wantCallbacks)
							}
						}

						// ---- outcome, by the rule ----
						switch {
						case bizErr != nil: // step 1: business error wins
							if first {
								if err != nil {
									t.Fatalf("page 1 error = %v, want nil so the command layer can dump and classify it", err)
								}
								err = ac.CheckResponse(result, "bot")
							}
							wp, _ := errs.ProblemOf(bizErr)
							gp, ok := errs.ProblemOf(err)
							if !ok || gp.Category != wp.Category || gp.Code != wp.Code {
								t.Errorf("classified as %v, want the classifier's own %s/%d", err, wp.Category, wp.Code)
							}

						case status >= 400: // step 2: status
							if got, want := errs.CategoryOf(err), statusCategory(status); err == nil || got != want {
								t.Errorf("error = %v (category %q), want HTTP %d as %q", err, got, status, want)
							}

						case refused: // rule 3: not a step in the pagination
							p, ok := errs.ProblemOf(err)
							if !ok || p.Subtype != errs.SubtypeInvalidResponse {
								t.Fatalf("error = %v, want %s: a page that did not declare success is neither a continuation nor a page to paginate from", err, errs.SubtypeInvalidResponse)
							}
							// ERROR_CONTRACT: Message says what is wrong, Hint says what to do
							// next, and the two are not merged. A refused first page is the
							// one place the loop has a next step to offer.
							if first {
								if p.Hint == "" {
									t.Errorf("Hint = %q, want a safe recovery step", p.Hint)
								}
							} else if p.Hint != "" {
								t.Errorf("Hint = %q, want none for a failed later page", p.Hint)
							}
							if strings.Contains(strings.ToLower(p.Message), "run without") || strings.HasSuffix(p.Message, ".") {
								t.Errorf("Message = %q, want no recovery step and no trailing period in it", p.Message)
							}

						default: // rules 3-4: output on the existing contract; the run ends without error
							if err != nil {
								t.Fatalf("error = %v, want nil: the page passed classification and needs no pagination", err)
							}
							if err := ac.CheckResponse(result, "bot"); err != nil {
								t.Errorf("command-layer CheckResponse = %v, want nil: plain api accepts this body", err)
							}
						}
					})
				}
			}
		}
	}
}

// Refusing a first page that did not declare success is a property of the page,
// not of how many pages the caller allowed. Keep the admission check ahead of
// the page-limit stop so --page-limit 1 cannot turn a response carrying a
// continuation token into a successful run or expose a write request to replay
// later.
func TestPagination_FirstPageRefusalPrecedesPageLimit(t *testing.T) {
	const body = `{"code":0.5,"data":{"items":[{"id":"x"}],"has_more":true,"page_token":"next"}}`
	for _, mode := range []string{"PaginateAll", "StreamPages"} {
		t.Run(mode, func(t *testing.T) {
			rt, reqs := countingTransport(ctResponse(200, "", body))
			ac, _ := newTestAPIClient(t, rt)
			req := RawApiRequest{Method: "POST", URL: "/open-apis/test", As: "bot"}
			opts := PaginationOptions{PageLimit: 1, PageDelay: -1, Identity: "bot"}

			callbacks := 0
			var err error
			if mode == "PaginateAll" {
				_, err = ac.PaginateAll(context.Background(), req, opts)
			} else {
				_, _, err = ac.StreamPages(context.Background(), req, func([]interface{}) error {
					callbacks++
					return nil
				}, opts)
			}

			p, ok := errs.ProblemOf(err)
			if !ok || p.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("error = %v, want %s", err, errs.SubtypeInvalidResponse)
			}
			if *reqs != 1 {
				t.Errorf("requests = %d, want 1", *reqs)
			}
			if callbacks != 0 {
				t.Errorf("callbacks = %d, want 0", callbacks)
			}
		})
	}
}

func TestFirstPageRecoveryHint(t *testing.T) {
	tests := []struct {
		method string
		want   string
	}{
		{http.MethodGet, "remove `--page-all` and `--jq`; use `--output <path>` to save the raw response"},
		{"get", "remove `--page-all` and `--jq`; use `--output <path>` to save the raw response"},
		{http.MethodHead, "remove `--page-all` and `--jq`; use `--output <path>` to save the raw response"},
		{http.MethodPost, "verify whether the first request changed remote state before retrying it"},
		{http.MethodPut, "verify whether the first request changed remote state before retrying it"},
		{http.MethodPatch, "verify whether the first request changed remote state before retrying it"},
		{http.MethodDelete, "verify whether the first request changed remote state before retrying it"},
		{"CUSTOM", "verify whether the first request changed remote state before retrying it"},
		{"", "verify whether the first request changed remote state before retrying it"},
	}
	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			if got := firstPageRecoveryHint(tc.method); got != tc.want {
				t.Errorf("firstPageRecoveryHint(%q) = %q, want %q", tc.method, got, tc.want)
			}
		})
	}
}

func TestNextPageTokenReturnsAlternateCursor(t *testing.T) {
	result := map[string]interface{}{
		"data": map[string]interface{}{
			"has_more":        true,
			"next_page_token": "next",
		},
	}
	if got := nextPageToken(result); got != "next" {
		t.Errorf("nextPageToken() = %q, want %q", got, "next")
	}
}

// A body that declares a non-JSON Content-Type is not a page, whatever its
// bytes say, because that is the rule HandleResponse applies: plain `api` saves
// such a body as a download and never reads a code out of it. The loop must not
// read one either — not to classify, and not to find a cursor and re-issue a
// write for it.
func TestPaginateAll_DeclaredNonJSONBodyIsNotAPage(t *testing.T) {
	for _, body := range []string{
		`{"code":0,"data":{"items":[{"id":"x"}],"has_more":true,"page_token":"next"}}`,
		`{"code":230027,"msg":"user not authorized"}`,
	} {
		t.Run(body, func(t *testing.T) {
			rt, reqs := countingTransport(ctResponse(200, "text/plain", body))
			ac, _ := newTestAPIClient(t, rt)
			_, err := ac.PaginateAll(context.Background(), RawApiRequest{
				Method: "POST", URL: "/open-apis/test", As: "bot",
			}, PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: "bot"})
			p, ok := errs.ProblemOf(err)
			if !ok || p.Subtype != errs.SubtypeInvalidResponse {
				t.Errorf("error = %v, want %s", err, errs.SubtypeInvalidResponse)
			}
			if *reqs != 1 {
				t.Errorf("requests = %d, want 1: a non-JSON body must not yield a cursor", *reqs)
			}
		})
	}
}

// The shared parser accepts one JSON value and nothing else. Trailing
// whitespace is fine; a second value or garbage after the first is not, because
// accepting the first value would let a truncated or concatenated body pass for
// a complete page — cursor included.
func TestParseJSONResponse_AcceptsExactlyOneValue(t *testing.T) {
	const page = `{"code":0,"data":{"items":[],"has_more":true,"page_token":"next"}}`
	for _, tc := range []struct {
		name string
		body string
		ok   bool
	}{
		{"one value", page, true},
		{"trailing newline", page + "\n", true},
		{"trailing whitespace", page + " \t\r\n", true},
		{"trailing garbage", page + "garbage", false},
		{"second value", page + ` {"code":230027}`, false},
		{"second value, no space", page + `{"code":230027}`, false},
		{"truncated", `{"code":0,`, false},
		{"empty", ``, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseJSONResponse(&larkcore.ApiResp{StatusCode: 200, Header: http.Header{}, RawBody: []byte(tc.body)})
			if (err == nil) != tc.ok {
				t.Errorf("ParseJSONResponse(%q) error = %v, want ok=%v", tc.body, err, tc.ok)
			}
		})
	}
}

// The exit criterion stated directly: one response classifies the same way
// through plain `api` and through `--page-all`. HandleResponse is the plain
// path; a first page is the paginated one, followed by the CheckResponse call
// apiPaginate makes on it. Success, category and error.code must all agree.
//
// The fixtures carry no Content-Type so that the SDK does not decode them inside
// DoAPI (which would fail identically on both paths and prove nothing) and so
// that callPage's non-JSON guard stays out of the way; the Content-Type
// dimension has its own test.
func TestPagination_FirstPageMatchesPlainAPI(t *testing.T) {
	bodies := []string{
		`null`, `[]`, `<html>Bad Gateway</html>`, `{"code":0,`,
		`{"code":0,"data":{"items":[],"has_more":true,"page_token":"next"}}garbage`,
		`{"code":0,"data":{"items":[],"has_more":true,"page_token":"next"}} {"code":230027}`,
		"{\"code\":0,\"data\":{\"items\":[],\"has_more\":false}}\n",
	}
	for _, sh := range codeShapes {
		// Parity is about one response, so the terminal and bare variants are
		// compared: a page with a cursor is the decision table's business.
		bodies = append(bodies, sh.body("terminal"), sh.body("bare"))
	}
	for _, status := range []int{200, 403, 502} {
		for _, body := range bodies {
			t.Run(fmt.Sprintf("%d/%s", status, body), func(t *testing.T) {
				var out, errOut bytes.Buffer
				plainErr := HandleResponse(&larkcore.ApiResp{
					StatusCode: status, Header: http.Header{}, RawBody: []byte(body),
				}, ResponseOptions{Identity: core.AsBot, Out: &out, ErrOut: &errOut})

				ac, _ := newTestAPIClient(t, pageSeqTransport(ctResponse(status, "", body)))
				result, pageErr := ac.PaginateAll(context.Background(), RawApiRequest{
					Method: "GET", URL: "/open-apis/test", As: "bot",
				}, PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: "bot"})
				if pageErr == nil {
					pageErr = ac.CheckResponse(result, core.AsBot)
				}

				if (plainErr == nil) != (pageErr == nil) {
					t.Fatalf("plain api error = %v, --page-all error = %v; one must not succeed where the other fails", plainErr, pageErr)
				}
				if plainErr == nil {
					return
				}
				pp, pok := errs.ProblemOf(plainErr)
				gp, gok := errs.ProblemOf(pageErr)
				if !pok || !gok {
					if plainErr.Error() != pageErr.Error() {
						t.Errorf("plain api = %q, --page-all = %q", plainErr, pageErr)
					}
					return
				}
				if pp.Category != gp.Category || pp.Code != gp.Code {
					t.Errorf("plain api = %s/%d, --page-all = %s/%d", pp.Category, pp.Code, gp.Category, gp.Code)
				}
			})
		}
	}
}
