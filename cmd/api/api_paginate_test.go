// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

type apiFailOnWriteWriter struct {
	buf    bytes.Buffer
	writes int
	failAt int
	err    error
}

func (w *apiFailOnWriteWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, w.err
	}
	return w.buf.Write(p)
}

func newAPIPaginateTestHarness(t *testing.T) (*client.APIClient, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	previousNotice := output.PendingNotice
	output.PendingNotice = nil
	t.Cleanup(func() { output.PendingNotice = previousNotice })

	config := &core.CliConfig{
		AppID:     "test-app",
		AppSecret: "test-secret",
		Brand:     core.BrandFeishu,
	}
	f, out, errOut, reg := cmdutil.TestFactory(t, config)
	ac, err := f.NewAPIClientWithConfig(config)
	if err != nil {
		t.Fatalf("NewAPIClientWithConfig() error = %v", err)
	}
	ac.ErrOut = io.Discard
	return ac, out, errOut, reg
}

func apiPaginateRequest() client.RawApiRequest {
	return client.RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test/v1/items",
		As:     core.AsBot,
	}
}

func assertAPIPaginateJSONBytes(t *testing.T, got []byte, want interface{}) {
	t.Helper()
	wantBytes, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatalf("marshal expected JSON: %v", err)
	}
	wantBytes = append(wantBytes, '\n')
	if !bytes.Equal(got, wantBytes) {
		t.Fatalf("stdout bytes mismatch\ngot:\n%s\nwant:\n%s", got, wantBytes)
	}
}

func TestAPIPaginate_DefaultAggregatesAllPages(t *testing.T) {
	ac, out, errOut, reg := newAPIPaginateTestHarness(t)
	calls := 0
	wantTokens := []string{"", "next-1", "next-2"}
	for i, wantToken := range wantTokens {
		page := i + 1
		hasMore := page < len(wantTokens)
		data := map[string]interface{}{
			"items":    []interface{}{map[string]interface{}{"id": string(rune('0' + page))}},
			"has_more": hasMore,
		}
		if hasMore {
			data["page_token"] = wantTokens[page]
		}
		reg.Register(&httpmock.Stub{
			URL: "/open-apis/test/v1/items",
			OnMatch: func(req *http.Request) {
				calls++
				if got := req.URL.Query().Get("page_token"); got != wantToken {
					t.Errorf("request %d page_token = %q, want %q", page, got, wantToken)
				}
			},
			Body: map[string]interface{}{
				"code": 0,
				"msg":  "ok",
				"data": data,
			},
		})
	}

	err := apiPaginate(context.Background(), ac, apiPaginateRequest(),
		output.FormatJSON, "", out, errOut, "lark-cli api GET", client.PaginationOptions{
			PageLimit: 10,
			PageDelay: -1,
		})

	if err != nil {
		t.Fatalf("apiPaginate() error = %v, want nil", err)
	}
	if calls != 3 {
		t.Fatalf("pagination requests = %d, want 3", calls)
	}
	assertAPIPaginateJSONBytes(t, out.Bytes(), output.Envelope{
		OK:       true,
		Identity: "bot",
		Data: map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"id": "1"},
				map[string]interface{}{"id": "2"},
				map[string]interface{}{"id": "3"},
			},
			"has_more": false,
		},
	})
	if got := errOut.String(); got != "" {
		t.Fatalf("stderr bytes = %q, want empty", got)
	}
}

func TestAPIPaginate_StreamingFormatsEmitExactMultiPageBytes(t *testing.T) {
	tests := []struct {
		name   string
		format output.Format
		want   string
	}{
		{
			name:   "ndjson",
			format: output.FormatNDJSON,
			want:   "{\"id\":\"1\",\"name\":\"Alice\"}\n{\"id\":\"2\",\"name\":\"Carol\",\"page_only\":\"ignored\"}\n",
		},
		{
			name:   "table",
			format: output.FormatTable,
			want:   "id  name \n──  ─────\n1   Alice\n2   Carol\n",
		},
		{
			name:   "csv",
			format: output.FormatCSV,
			want:   "id,name\n1,Alice\n2,Carol\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ac, out, errOut, reg := newAPIPaginateTestHarness(t)
			reg.Register(&httpmock.Stub{
				URL: "/open-apis/test/v1/items",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "ok",
					"data": map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{"id": "1", "name": "Alice"},
						},
						"has_more":   true,
						"page_token": "next-1",
					},
				},
			})
			reg.Register(&httpmock.Stub{
				URL: "/open-apis/test/v1/items",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "ok",
					"data": map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{"id": "2", "name": "Carol", "page_only": "ignored"},
						},
						"has_more": false,
					},
				},
			})

			err := apiPaginate(context.Background(), ac, apiPaginateRequest(),
				tt.format, "", out, errOut, "lark-cli api GET", client.PaginationOptions{
					PageLimit: 10,
					PageDelay: -1,
				})

			if err != nil {
				t.Fatalf("apiPaginate() error = %v, want nil", err)
			}
			if got := out.String(); got != tt.want {
				t.Fatalf("stdout byte mismatch\ngot (%d bytes):\n%q\nwant (%d bytes):\n%q", len(got), got, len(tt.want), tt.want)
			}
			if got := errOut.String(); got != "" {
				t.Fatalf("stderr bytes = %q, want empty", got)
			}
		})
	}
}

func TestAPIPaginate_StreamingWriteFailureStopsFurtherPages(t *testing.T) {
	ac, _, errOut, reg := newAPIPaginateTestHarness(t)
	sentinel := errors.New("page write failed")
	out := &apiFailOnWriteWriter{failAt: 2, err: sentinel}
	calls := 0
	for page := 1; page <= 2; page++ {
		hasMore := true
		data := map[string]interface{}{
			"items":    []interface{}{map[string]interface{}{"id": page}},
			"has_more": hasMore,
		}
		if hasMore {
			data["page_token"] = fmt.Sprintf("next-%d", page)
		}
		reg.Register(&httpmock.Stub{
			URL: "/open-apis/test/v1/items",
			OnMatch: func(*http.Request) {
				calls++
			},
			Body: map[string]interface{}{
				"code": 0,
				"msg":  "ok",
				"data": data,
			},
		})
	}

	err := apiPaginate(context.Background(), ac, apiPaginateRequest(),
		output.FormatNDJSON, "", out, errOut, "lark-cli api GET",
		client.PaginationOptions{PageLimit: 10, PageDelay: -1})

	if !errors.Is(err, sentinel) {
		t.Fatalf("apiPaginate() error = %v, want preserved writer cause", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal {
		t.Fatalf("apiPaginate() problem = %#v, %v; want internal typed error", problem, ok)
	}
	if calls != 2 {
		t.Fatalf("pagination requests = %d, want 2", calls)
	}
	if got, want := out.buf.String(), "{\"id\":1}\n"; got != want {
		t.Fatalf("stdout bytes = %q, want %q", got, want)
	}
}

func TestAPIPaginate_StreamingFormatFallsBackToJSONWithoutList(t *testing.T) {
	ac, out, errOut, reg := newAPIPaginateTestHarness(t)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/test/v1/items",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"name":    "Test User",
				"user_id": "u123",
			},
		},
	})

	err := apiPaginate(context.Background(), ac, apiPaginateRequest(),
		output.FormatNDJSON, "", out, errOut, "lark-cli api GET", client.PaginationOptions{PageDelay: -1})

	if err != nil {
		t.Fatalf("apiPaginate() error = %v, want nil", err)
	}
	assertAPIPaginateJSONBytes(t, out.Bytes(), output.Envelope{
		OK:       true,
		Identity: "bot",
		Data: map[string]interface{}{
			"name":    "Test User",
			"user_id": "u123",
		},
	})
	wantWarning := "warning: this API does not return a list, format \"ndjson\" is not supported, falling back to json\n"
	if got := errOut.String(); got != wantWarning {
		t.Fatalf("stderr bytes = %q, want %q", got, wantWarning)
	}
}

func TestAPIPaginate_BusinessErrorsWriteRawAndAreMarkedRaw(t *testing.T) {
	businessResponse := map[string]interface{}{
		"code": 123456,
		"msg":  "fixture business error",
		"data": map[string]interface{}{"detail": "business failed"},
	}
	tests := []struct {
		name   string
		format output.Format
		jqExpr string
	}{
		{name: "jq", format: output.FormatJSON, jqExpr: ".data.items"},
		{name: "default_json", format: output.FormatJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ac, out, errOut, reg := newAPIPaginateTestHarness(t)
			reg.Register(&httpmock.Stub{
				URL:  "/open-apis/test/v1/items",
				Body: businessResponse,
			})

			err := apiPaginate(context.Background(), ac, apiPaginateRequest(),
				tt.format, tt.jqExpr, out, errOut, "lark-cli api GET", client.PaginationOptions{PageDelay: -1})

			if err == nil {
				t.Fatal("apiPaginate() error = nil, want business error")
			}
			if !errs.IsRaw(err) {
				t.Fatalf("errs.IsRaw(error) = false, want true; error = %T: %v", err, err)
			}
			assertAPIPaginateJSONBytes(t, out.Bytes(), businessResponse)
			if bytes.Contains(out.Bytes(), []byte(`"ok": true`)) {
				t.Fatalf("business-error stdout contains a success envelope:\n%s", out.Bytes())
			}
			if got := errOut.String(); got != "" {
				t.Fatalf("stderr bytes = %q, want empty", got)
			}
		})
	}
}

func TestAPIPaginate_TransportErrorsAreMarkedRaw(t *testing.T) {
	tests := []struct {
		name   string
		format output.Format
		jqExpr string
	}{
		{name: "jq_paginate_all", format: output.FormatJSON, jqExpr: ".data.items"},
		{name: "stream_pages", format: output.FormatNDJSON},
		{name: "default_paginate_all", format: output.FormatJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ac, out, errOut, _ := newAPIPaginateTestHarness(t)

			err := apiPaginate(context.Background(), ac, apiPaginateRequest(),
				tt.format, tt.jqExpr, out, errOut, "lark-cli api GET", client.PaginationOptions{PageDelay: -1})

			if err == nil {
				t.Fatal("apiPaginate() error = nil, want transport error")
			}
			if !errs.IsRaw(err) {
				t.Fatalf("errs.IsRaw(error) = false, want true; error = %T: %v", err, err)
			}
			if got := out.String(); got != "" {
				t.Fatalf("stdout bytes = %q, want empty", got)
			}
			if got := errOut.String(); got != "" {
				t.Fatalf("stderr bytes = %q, want empty", got)
			}
		})
	}
}

func TestAPIPaginate_StreamBusinessErrorIsMarkedRaw(t *testing.T) {
	ac, out, errOut, reg := newAPIPaginateTestHarness(t)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/test/v1/items",
		Body: map[string]interface{}{
			"code": 123456,
			"msg":  "fixture business error",
			"data": map[string]interface{}{},
		},
	})

	err := apiPaginate(context.Background(), ac, apiPaginateRequest(),
		output.FormatNDJSON, "", out, errOut, "lark-cli api GET", client.PaginationOptions{PageDelay: -1})

	if err == nil {
		t.Fatal("apiPaginate() error = nil, want business error")
	}
	if !errs.IsRaw(err) {
		t.Fatalf("errs.IsRaw(error) = false, want true; error = %T: %v", err, err)
	}
	if got := out.String(); got != "" {
		t.Fatalf("stdout bytes = %q, want empty", got)
	}
	if got := errOut.String(); got != "" {
		t.Fatalf("stderr bytes = %q, want empty", got)
	}
}

// firstPageHasMoreStub registers a successful page 1 that advertises a next
// page, so the loop is guaranteed to attempt page 2.
func firstPageHasMoreStub(reg *httpmock.Registry) {
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/test/v1/items",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"id": "first"}},
				"has_more":   true,
				"page_token": "next-1",
			},
		},
	})
}

// The command-layer view of the first-page rule. Exactly one stub is registered,
// so a second request would fail the run; the method is POST because the hazard
// pinned is a replayed write.
//
//   - A first page that passed classification and needs no pagination is output
//     on each format's own contract: json wraps it in the envelope, ndjson
//     streams its records — a code-less list is still a list.
//   - A first page that did not declare success but carries a continuation token
//     is refused before output, with a non-zero exit, so the streaming formats —
//     which have no envelope to carry has_more — get a machine-readable signal.
//   - A business error dumps the raw response (json) or writes nothing
//     (ndjson) and fails, as before.
func TestAPIPaginate_FirstPageIsOutputOnlyIfItNeedsNoPagination(t *testing.T) {
	page := func(code string, cursor bool) []byte {
		data := `"data":{"items":[{"id":"first"}],"has_more":false}`
		if cursor {
			data = `"data":{"items":[{"id":"first"}],"has_more":true,"page_token":"next-1"}`
		}
		if code == "" {
			return []byte(`{"msg":"m",` + data + `}`)
		}
		return []byte(`{"code":` + code + `,"msg":"m",` + data + `}`)
	}
	for _, tc := range []struct {
		name          string
		code          string
		cursor        bool
		format        output.Format
		wantErrCat    errs.Category // "" = expect nil error
		wantSubtype   errs.Subtype
		wantStdout    []string
		wantNotStdout []string
		wantNotStderr string
	}{
		{"json, code-less list, no cursor", "", false, output.FormatJSON, "", "", []string{`"has_more": false`, `"first"`}, nil, ""},
		{"json, missing code, cursor", "", true, output.FormatJSON, errs.CategoryInternal, errs.SubtypeInvalidResponse, nil, []string{`first`}, ""},
		{"json, fractional code, cursor", "0.5", true, output.FormatJSON, errs.CategoryInternal, errs.SubtypeInvalidResponse, nil, []string{`first`}, ""},
		{"json, null code, cursor", "null", true, output.FormatJSON, errs.CategoryInternal, errs.SubtypeInvalidResponse, nil, []string{`first`}, ""},
		{"json, business error", "230027", true, output.FormatJSON, errs.CategoryAuthorization, "", []string{`"code": 230027`}, nil, ""},
		{"ndjson, code-less list, no cursor", "", false, output.FormatNDJSON, "", "", []string{`"first"`}, []string{`has_more`}, "does not return a list"},
		{"ndjson, missing code, cursor", "", true, output.FormatNDJSON, errs.CategoryInternal, errs.SubtypeInvalidResponse, nil, []string{`first`}, ""},
		{"ndjson, fractional code, cursor", "0.5", true, output.FormatNDJSON, errs.CategoryInternal, errs.SubtypeInvalidResponse, nil, []string{`first`}, ""},
		{"ndjson, business error", "230027", true, output.FormatNDJSON, errs.CategoryAuthorization, "", nil, []string{`first`}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ac, out, errOut, reg := newAPIPaginateTestHarness(t)
			reg.Register(&httpmock.Stub{URL: "/open-apis/test/v1/items", RawBody: page(tc.code, tc.cursor), ContentType: "text/json"})

			err := apiPaginate(context.Background(), ac,
				client.RawApiRequest{Method: "POST", URL: "/open-apis/test/v1/items", As: core.AsBot},
				tc.format, "", out, errOut, "lark-cli api POST",
				client.PaginationOptions{PageLimit: 0, PageDelay: -1})

			if tc.wantErrCat == "" {
				if err != nil {
					t.Fatalf("apiPaginate() error = %v, want nil (a second request would have failed the run)", err)
				}
			} else {
				if got := errs.CategoryOf(err); err == nil || got != tc.wantErrCat {
					t.Fatalf("apiPaginate() error = %v (category %q), want %q", err, got, tc.wantErrCat)
				}
				if tc.wantSubtype != "" {
					p, ok := errs.ProblemOf(err)
					if !ok || p.Subtype != tc.wantSubtype {
						t.Fatalf("subtype = %v, want %q", err, tc.wantSubtype)
					}
					if tc.cursor {
						const wantHint = "verify whether the first request changed remote state before retrying it"
						if p.Hint != wantHint {
							t.Errorf("Hint = %q, want %q", p.Hint, wantHint)
						}
					}
				}
			}
			for _, want := range tc.wantStdout {
				if !strings.Contains(out.String(), want) {
					t.Errorf("stdout = %q, want it to contain %q", out.String(), want)
				}
			}
			for _, notWant := range tc.wantNotStdout {
				if strings.Contains(out.String(), notWant) {
					t.Errorf("stdout = %q, want it NOT to contain %q", out.String(), notWant)
				}
			}
			if tc.wantNotStderr != "" && strings.Contains(errOut.String(), tc.wantNotStderr) {
				t.Errorf("stderr = %q, want it NOT to contain %q: the page is a list and must be streamed", errOut.String(), tc.wantNotStderr)
			}
		})
	}
}

// The counterpart to the JSON cases, and the reason "a failed run writes no
// stdout" is only true for the buffered formats. A streaming format has already
// emitted the pages that succeeded by the time a later one fails; those lines
// stay, by design (see apiPaginate's note that callers must use the exit code
// to tell complete from partial output). What this change fixes is that the
// exit code now actually says so.
func TestAPIPaginate_LaterPageFailureKeepsStreamedStdout(t *testing.T) {
	ac, out, errOut, reg := newAPIPaginateTestHarness(t)
	firstPageHasMoreStub(reg)
	reg.Register(&httpmock.Stub{
		URL:    "/open-apis/test/v1/items",
		Status: 502,
		Body:   map[string]interface{}{"msg": "Bad Gateway"},
	})

	err := apiPaginate(context.Background(), ac, apiPaginateRequest(),
		output.FormatNDJSON, "", out, errOut, "lark-cli api GET",
		client.PaginationOptions{PageLimit: 0, PageDelay: -1})

	if err == nil {
		t.Fatal("apiPaginate() error = nil, want HTTP 502 from page 2")
	}
	if got := errs.CategoryOf(err); got != errs.CategoryNetwork {
		t.Errorf("errs.CategoryOf(err) = %q, want %q", got, errs.CategoryNetwork)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"first"`)) {
		t.Fatalf("streamed stdout = %q, want it to keep the page-1 item it already wrote", out.Bytes())
	}
}

// apiPaginate's return value is what determines the process exit code, so a nil
// here means the CLI would exit 0 on a partial result — the shape #2477 reported.
// The buffered formats must also leave stdout empty: a success envelope written
// alongside a non-zero exit is the same lie in a different place.
//
// Each case asserts the classification, not merely that something failed. A 502
// body carrying no code is also an unreadable page, so the later-page guard would
// catch it too; only the category shows which branch actually ran.
func TestAPIPaginate_LaterPageFailureEmitsNoStdout(t *testing.T) {
	transportErr := errors.New("simulated transport failure")

	for _, tc := range []struct {
		name        string
		stub        *httpmock.Stub
		wantCat     errs.Category
		wantSubtype errs.Subtype
		wantCause   error
	}{
		{
			name:      "transport failure",
			stub:      &httpmock.Stub{URL: "/open-apis/test/v1/items", Error: transportErr},
			wantCat:   errs.CategoryNetwork,
			wantCause: transportErr,
		},
		{
			name:        "business error",
			stub:        &httpmock.Stub{URL: "/open-apis/test/v1/items", Body: map[string]interface{}{"code": 230027, "msg": "user not authorized"}},
			wantCat:     errs.CategoryAuthorization,
			wantSubtype: errs.SubtypeUserUnauthorized,
		},
		{
			name:    "gateway 502 carrying no code",
			stub:    &httpmock.Stub{URL: "/open-apis/test/v1/items", Status: 502, Body: map[string]interface{}{"msg": "Bad Gateway"}},
			wantCat: errs.CategoryNetwork,
		},
		{
			// The page before it promised more data; this one carries no data and
			// a code the float decoder reads as 0. Accepting it would merge into
			// ok:true / has_more:false over the first page alone — #2477 exactly.
			name:        "later page did not declare success and carries no data",
			stub:        &httpmock.Stub{URL: "/open-apis/test/v1/items", RawBody: []byte(`{"code":1e-324,"msg":"underflow"}`), ContentType: "text/json"},
			wantCat:     errs.CategoryInternal,
			wantSubtype: errs.SubtypeInvalidResponse,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ac, out, errOut, reg := newAPIPaginateTestHarness(t)
			firstPageHasMoreStub(reg)
			reg.Register(tc.stub)

			err := apiPaginate(context.Background(), ac, apiPaginateRequest(),
				output.FormatJSON, "", out, errOut, "lark-cli api GET",
				client.PaginationOptions{PageLimit: 0, PageDelay: -1})

			if err == nil {
				t.Fatal("apiPaginate() error = nil, want the page-2 failure to reach the exit code")
			}
			if got := errs.CategoryOf(err); got != tc.wantCat {
				t.Errorf("errs.CategoryOf(err) = %q, want %q", got, tc.wantCat)
			}
			if tc.wantSubtype != "" {
				p, ok := errs.ProblemOf(err)
				if !ok {
					t.Fatalf("errs.ProblemOf(err) = _, false; want a typed problem; err = %T: %v", err, err)
				}
				if p.Subtype != tc.wantSubtype {
					t.Errorf("subtype = %q, want %q", p.Subtype, tc.wantSubtype)
				}
			}
			if tc.wantCause != nil && !errors.Is(err, tc.wantCause) {
				t.Errorf("errors.Is(err, cause) = false; the cause did not survive the command layer; err = %v", err)
			}
			if got := out.String(); got != "" {
				t.Fatalf("stdout bytes = %q, want empty on a failed pagination run", got)
			}
		})
	}
}
