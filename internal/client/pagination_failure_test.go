// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

// pageSeqTransport replays responders in order, one per request. It lets a test
// make page 1 succeed and page 2 fail, which is the shape this file is about.
func pageSeqTransport(responders ...func() (*http.Response, error)) http.RoundTripper {
	i := 0
	return roundTripFunc(func(*http.Request) (*http.Response, error) {
		if i >= len(responders) {
			return nil, errors.New("unexpected extra request")
		}
		r := responders[i]
		i++
		return r()
	})
}

// firstPageHasMore is a successful page 1 that advertises a next page, so the
// loop is guaranteed to attempt page 2.
func firstPageHasMore() (*http.Response, error) {
	return jsonResponse(map[string]interface{}{
		"code": 0, "msg": "ok",
		"data": map[string]interface{}{
			"items":      []interface{}{map[string]interface{}{"id": "first"}},
			"has_more":   true,
			"page_token": "tok-2",
		},
	}), nil
}

// errSimulatedTransport is a stable sentinel so the tests can assert the
// transport error is carried through the pagination loop rather than merely
// classified as a network failure — the loop must not swallow the cause.
var errSimulatedTransport = errors.New("simulated transport failure")

func transportFailure() (*http.Response, error) {
	return nil, errSimulatedTransport
}

func TestPaginateAll_LaterPageTransportErrorPropagates(t *testing.T) {
	ac, _ := newTestAPIClient(t, pageSeqTransport(firstPageHasMore, transportFailure))

	_, err := ac.PaginateAll(context.Background(), RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test",
		As:     "bot",
	}, PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: "bot"})

	if err == nil {
		t.Fatal("PaginateAll() error = nil, want transport error from page 2")
	}
	if got := errs.CategoryOf(err); got != errs.CategoryNetwork {
		t.Errorf("errs.CategoryOf(err) = %q, want %q", got, errs.CategoryNetwork)
	}
	if !errors.Is(err, errSimulatedTransport) {
		t.Errorf("errors.Is(err, errSimulatedTransport) = false; cause was not preserved; err = %v", err)
	}
}

func TestStreamPages_LaterPageTransportErrorPropagates(t *testing.T) {
	ac, _ := newTestAPIClient(t, pageSeqTransport(firstPageHasMore, transportFailure))

	_, _, err := ac.StreamPages(context.Background(), RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test",
		As:     "bot",
	}, func([]interface{}) error { return nil },
		PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: "bot"})

	if err == nil {
		t.Fatal("StreamPages() error = nil, want transport error from page 2")
	}
	if got := errs.CategoryOf(err); got != errs.CategoryNetwork {
		t.Errorf("errs.CategoryOf(err) = %q, want %q", got, errs.CategoryNetwork)
	}
	if !errors.Is(err, errSimulatedTransport) {
		t.Errorf("errors.Is(err, errSimulatedTransport) = false; cause was not preserved; err = %v", err)
	}
}

// businessFailure builds a page that failed with a non-zero business code.
// Such a page carries no data object, which is why the merged view used to
// report has_more=false and hide the failure.
func businessFailure(code int, msg string) func() (*http.Response, error) {
	return func() (*http.Response, error) {
		return jsonResponse(map[string]interface{}{"code": code, "msg": msg}), nil
	}
}

// Unknown codes fall through errclass's codemeta table into CategoryAPI.
func TestPaginateAll_LaterPageUnknownBusinessCodePropagates(t *testing.T) {
	ac, _ := newTestAPIClient(t, pageSeqTransport(
		firstPageHasMore, businessFailure(999999, "fixture unknown error")))

	_, err := ac.PaginateAll(context.Background(), RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test",
		As:     "bot",
	}, PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: "bot"})

	if err == nil {
		t.Fatal("PaginateAll() error = nil, want business error from page 2")
	}
	if got := errs.CategoryOf(err); got != errs.CategoryAPI {
		t.Errorf("errs.CategoryOf(err) = %q, want %q", got, errs.CategoryAPI)
	}

	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("errs.ProblemOf(err) = _, false; want a typed problem; err = %T: %v", err, err)
	}
	if p.Subtype != errs.SubtypeUnknown {
		t.Errorf("subtype = %q, want %q", p.Subtype, errs.SubtypeUnknown)
	}
}

// 230027 is in the codemeta table, so it classifies as authorization.
//
// The point of this case is the comparison in its name, so it runs both pages
// rather than asserting a hardcoded pair and calling that "matches page 1".
// Page 1 reaches its classification through the command layer: the loop hands
// back (failing page, nil) and CheckResponse turns it into the typed error.
// A later page is classified inside the loop. Both must land on the same
// category and subtype, or --page-all would report the same API failure
// differently depending on which page it arrived on.
func TestPaginateAll_LaterPageKnownBusinessCodeMatchesPageOneClassification(t *testing.T) {
	const code, msg = 230027, "user not authorized"
	opts := PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: "bot"}
	req := RawApiRequest{Method: "GET", URL: "/open-apis/test", As: "bot"}

	laterAC, _ := newTestAPIClient(t, pageSeqTransport(firstPageHasMore, businessFailure(code, msg)))
	_, laterErr := laterAC.PaginateAll(context.Background(), req, opts)
	if laterErr == nil {
		t.Fatal("PaginateAll() error = nil, want business error from page 2")
	}

	firstAC, _ := newTestAPIClient(t, pageSeqTransport(businessFailure(code, msg)))
	firstResult, err := firstAC.PaginateAll(context.Background(), req, opts)
	if err != nil {
		t.Fatalf("PaginateAll() on a failing page 1 = %v, want nil so the command layer can dump the raw response", err)
	}
	firstErr := firstAC.CheckResponse(firstResult, opts.Identity)
	if firstErr == nil {
		t.Fatal("CheckResponse() on a failing page 1 = nil, want the typed error the command layer reports")
	}

	if got, want := errs.CategoryOf(laterErr), errs.CategoryOf(firstErr); got != want {
		t.Errorf("category: later page = %q, page 1 = %q; want identical", got, want)
	}
	if got := errs.CategoryOf(laterErr); got != errs.CategoryAuthorization {
		t.Errorf("errs.CategoryOf(laterErr) = %q, want %q", got, errs.CategoryAuthorization)
	}

	laterP, ok := errs.ProblemOf(laterErr)
	if !ok {
		t.Fatalf("errs.ProblemOf(laterErr) = _, false; want a typed problem; err = %T: %v", laterErr, laterErr)
	}
	firstP, ok := errs.ProblemOf(firstErr)
	if !ok {
		t.Fatalf("errs.ProblemOf(firstErr) = _, false; want a typed problem; err = %T: %v", firstErr, firstErr)
	}
	if laterP.Subtype != firstP.Subtype {
		t.Errorf("subtype: later page = %q, page 1 = %q; want identical", laterP.Subtype, firstP.Subtype)
	}
	if laterP.Subtype != errs.SubtypeUserUnauthorized {
		t.Errorf("subtype = %q, want %q", laterP.Subtype, errs.SubtypeUserUnauthorized)
	}
}

// The streaming path already surfaced non-zero codes before this change (the
// failing page was appended and returned as the last result, where the command
// layer's CheckResponse caught it). This case locks that behaviour against
// regression now that the error is built inside the loop instead.
//
// The "[pagination] streamed N pages" summary has its own case below; this one
// stays scoped to the error the caller receives.
func TestStreamPages_LaterPageKnownBusinessCodePropagates(t *testing.T) {
	ac, _ := newTestAPIClient(t, pageSeqTransport(
		firstPageHasMore, businessFailure(230027, "user not authorized")))

	_, _, err := ac.StreamPages(context.Background(), RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test",
		As:     "bot",
	}, func([]interface{}) error { return nil },
		PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: "bot"})

	if err == nil {
		t.Fatal("StreamPages() error = nil, want business error from page 2")
	}
	if got := errs.CategoryOf(err); got != errs.CategoryAuthorization {
		t.Errorf("errs.CategoryOf(err) = %q, want %q", got, errs.CategoryAuthorization)
	}
}

// statusResponse builds a page with an explicit HTTP status and a raw body,
// which jsonResponse cannot express (it hardcodes 200 and marshals a value).
func statusResponse(status int, body string) func() (*http.Response, error) {
	return func() (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}
}

// The [page N] progress lines are the only place the loop treats page 1 and a
// later page differently on the failure path, and paginateLoop's comment names
// that guard specifically. It has to be asserted here, on the client's own
// ErrOut: the cmd-layer harnesses point ac.ErrOut at io.Discard, so the errOut
// assertions there check the command's output and can never fail on this.
func TestPaginateAll_StoppingLineIsLaterPagesOnly(t *testing.T) {
	req := RawApiRequest{Method: "GET", URL: "/open-apis/test", As: "bot"}
	opts := PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: "bot"}

	t.Run("later page reports why it stopped", func(t *testing.T) {
		ac, errBuf := newTestAPIClient(t, pageSeqTransport(firstPageHasMore, transportFailure))
		if _, err := ac.PaginateAll(context.Background(), req, opts); err == nil {
			t.Fatal("PaginateAll() error = nil, want transport error from page 2")
		}
		if want := "[page 2] error, stopping pagination"; !strings.Contains(errBuf.String(), want) {
			t.Errorf("ErrOut = %q, want it to contain %q", errBuf.String(), want)
		}
	})

	t.Run("page 1 does not, having stopped nothing", func(t *testing.T) {
		ac, errBuf := newTestAPIClient(t, pageSeqTransport(transportFailure))
		if _, err := ac.PaginateAll(context.Background(), req, opts); err == nil {
			t.Fatal("PaginateAll() error = nil, want transport error from page 1")
		}
		if strings.Contains(errBuf.String(), "stopping pagination") {
			t.Errorf("ErrOut = %q, want no \"stopping pagination\" line for a page-1 failure", errBuf.String())
		}
	})
}

// A body that claims to be JSON is decoded by the SDK inside DoAPI, which fails
// there and returns no response — so these never reach callPage's parse branch
// and stay internal errors. Only a body whose Content-Type is not JSON gets that
// far with its status intact; that case is covered by
// TestPaginateAll_LaterPageHTTPErrorIsClassifiedByStatus, and the two
// together are what separate the paths.
//
// The Content-Type is the whole distinction, so both fixtures here declare
// application/json deliberately.
func TestPaginateAll_UnparseableJSONLaterPageFailsInsideDoAPI(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"truncated JSON on 200", 200, `{"code":0,`},
		{"HTML on 502", 502, `<html>Bad Gateway</html>`},
		{"code is a string", 200, `{"code":"230027","msg":"user not authorized"}`},
		{"code is an object", 200, `{"code":{},"msg":"nonsense"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ac, _ := newTestAPIClient(t, pageSeqTransport(
				firstPageHasMore, statusResponse(tc.status, tc.body)))

			_, err := ac.PaginateAll(context.Background(), RawApiRequest{
				Method: "GET", URL: "/open-apis/test", As: "bot",
			}, PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: "bot"})

			if err == nil {
				t.Fatal("PaginateAll() error = nil, want an unparseable page 2 to fail")
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("errs.ProblemOf(err) = _, false; want a typed problem; err = %T: %v", err, err)
			}
			if p.Subtype != errs.SubtypeInvalidResponse {
				t.Errorf("subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidResponse)
			}
		})
	}
}

// ctResponse builds a page with an explicit Content-Type. statusResponse always
// says application/json, and that difference is load-bearing: the SDK decodes a
// body only when it claims to be JSON, so the Content-Type decides whether an
// unparseable page fails inside DoAPI or reaches callPage with its status still
// readable. A fixture that only ever says application/json cannot see the
// difference — which is how the misclassification below went unnoticed.
func ctResponse(status int, contentType, body string) func() (*http.Response, error) {
	return func() (*http.Response, error) {
		h := http.Header{}
		if contentType != "" {
			h.Set("Content-Type", contentType)
		}
		return &http.Response{
			StatusCode: status,
			Header:     h,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}
}

// The classification a later page's failure gets must match the identity the
// request was actually sent with. An empty opts.Identity is not neutral:
// errclass rewrites it to "user" and the bot then receives the user-facing
// recovery text instead of the app-console scope guidance it needs.
//
// The command entry points happen to fill Identity today, so this asserts the
// contract at the layer that owns CheckResponse rather than at the layer that
// currently happens to protect it.
func TestPaginateAll_LaterPageErrorKeepsRequestIdentityWhenOptsOmitsIt(t *testing.T) {
	for _, tc := range []struct {
		name      string
		optsID    core.Identity
		requestAs core.Identity
		want      core.Identity
	}{
		{"opts empty falls back to the request", "", core.AsBot, core.AsBot},
		{"opts auto is treated as unresolved", core.AsAuto, core.AsBot, core.AsBot},
		{"opts wins when it is a real identity", core.AsUser, core.AsBot, core.AsUser},
		{"both unset lands on user", "", "", core.AsUser},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ac, _ := newTestAPIClient(t, pageSeqTransport(
				firstPageHasMore, businessFailure(230027, "user not authorized")))

			_, err := ac.PaginateAll(context.Background(), RawApiRequest{
				Method: "GET", URL: "/open-apis/test", As: tc.requestAs,
			}, PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: tc.optsID})

			if err == nil {
				t.Fatal("PaginateAll() error = nil, want business error from page 2")
			}
			var perm *errs.PermissionError
			if !errors.As(err, &perm) {
				t.Fatalf("error is %T, want *errs.PermissionError; err = %v", err, err)
			}
			if core.Identity(perm.Identity) != tc.want {
				t.Errorf("PermissionError.Identity = %q, want %q", perm.Identity, tc.want)
			}
		})
	}
}

func TestStreamPages_LaterPageErrorKeepsRequestIdentityWhenOptsOmitsIt(t *testing.T) {
	ac, _ := newTestAPIClient(t, pageSeqTransport(
		firstPageHasMore, businessFailure(230027, "user not authorized")))

	_, _, err := ac.StreamPages(context.Background(), RawApiRequest{
		Method: "GET", URL: "/open-apis/test", As: core.AsBot,
	}, func([]interface{}) error { return nil },
		PaginationOptions{PageLimit: 0, PageDelay: -1})

	if err == nil {
		t.Fatal("StreamPages() error = nil, want business error from page 2")
	}
	var perm *errs.PermissionError
	if !errors.As(err, &perm) {
		t.Fatalf("error is %T, want *errs.PermissionError; err = %v", err, err)
	}
	if core.Identity(perm.Identity) != core.AsBot {
		t.Errorf("PermissionError.Identity = %q, want %q", perm.Identity, core.AsBot)
	}
}

// The business code is consulted first and the HTTP status classifies whatever
// it leaves behind: a 5xx carrying no code, or one whose body still says code 0.
//
// Content-Type decides which layer sees the failure. The SDK decodes only a body
// it believes is JSON, so a text/html gateway page reaches the loop with its
// status intact; reporting that as a decode failure would exit 5 where plain
// `api` exits 4. The fixtures declare Content-Type explicitly rather than taking
// a helper default, because that default is what once hid this.
//
// Which guard catches a row is itself Content-Type-dependent: the rows that
// declare a non-JSON type are taken by callPage before it parses anything, and
// the row with no Content-Type at all is the one that still reaches the
// parse-failure branch. Both are asserted here because both must classify by
// status.
func TestPaginateAll_LaterPageHTTPErrorIsClassifiedByStatus(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		contentType string
		body        string
		wantCat     errs.Category
	}{
		{"502 JSON without a code", 502, "application/json", `{"msg":"Bad Gateway"}`, errs.CategoryNetwork},
		{"500 JSON still saying code 0", 500, "application/json", `{"code":0,"msg":"ok"}`, errs.CategoryNetwork},
		{"404 JSON still saying code 0", 404, "application/json", `{"code":0,"msg":"ok"}`, errs.CategoryAPI},
		{"502 text/html", 502, "text/html", `<html>Bad Gateway</html>`, errs.CategoryNetwork},
		{"502 text/plain", 502, "text/plain", `Bad Gateway`, errs.CategoryNetwork},
		{"502 with no Content-Type", 502, "", `<html>Bad Gateway</html>`, errs.CategoryNetwork},
		{"404 text/html", 404, "text/html", `<html>Not Found</html>`, errs.CategoryAPI},
		// 504 is deliberately absent: the SDK maps it to a server timeout inside
		// DoAPI, so it never reaches this branch and a case for it would pass
		// whether or not the branch exists.
	} {
		t.Run(tc.name, func(t *testing.T) {
			ac, _ := newTestAPIClient(t, pageSeqTransport(
				firstPageHasMore, ctResponse(tc.status, tc.contentType, tc.body)))

			_, err := ac.PaginateAll(context.Background(), RawApiRequest{
				Method: "GET", URL: "/open-apis/test", As: "bot",
			}, PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: "bot"})

			if err == nil {
				t.Fatalf("PaginateAll() error = nil, want HTTP %d from page 2", tc.status)
			}
			if got := errs.CategoryOf(err); got != tc.wantCat {
				t.Errorf("errs.CategoryOf(err) = %q, want %q", got, tc.wantCat)
			}
		})
	}
}

// The parity this change claims is that `api GET <url>` and the same command
// with --page-all classify one response identically. A failed response that
// declares a non-JSON body is where the two paths could drift: HandleResponse
// refuses to read a business code out of it, callPage used to read one anyway,
// and a 403 text/plain carrying 230027 then exited 3 paginated and 1 plain.
// Asserting against HandleResponse rather than a hardcoded category pins the
// two paths to each other, so a later change to either one fails here.
func TestPaginateAll_NonJSONErrorPageMatchesPlainAPIClassification(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		contentType string
		body        string
	}{
		{"403 text/plain carrying a business code", 403, "text/plain", `{"code":230027,"msg":"user not authorized"}`},
		{"502 text/html gateway page", 502, "text/html", `<html>Bad Gateway</html>`},
		{"404 text/plain still saying code 0", 404, "text/plain", `{"code":0,"msg":"ok"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			h.Set("Content-Type", tc.contentType)
			var out, errOut bytes.Buffer
			plainErr := HandleResponse(&larkcore.ApiResp{
				StatusCode: tc.status, Header: h, RawBody: []byte(tc.body),
			}, ResponseOptions{Identity: core.AsBot, Out: &out, ErrOut: &errOut})
			if plainErr == nil {
				t.Fatal("HandleResponse() error = nil; the fixture must fail on both paths for the comparison to mean anything")
			}

			ac, _ := newTestAPIClient(t, pageSeqTransport(
				firstPageHasMore, ctResponse(tc.status, tc.contentType, tc.body)))
			_, pageErr := ac.PaginateAll(context.Background(), RawApiRequest{
				Method: "GET", URL: "/open-apis/test", As: "bot",
			}, PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: "bot"})
			if pageErr == nil {
				t.Fatalf("PaginateAll() error = nil, want HTTP %d from page 2", tc.status)
			}

			if got, want := errs.CategoryOf(pageErr), errs.CategoryOf(plainErr); got != want {
				t.Errorf("--page-all category = %q, plain api = %q; one response must not exit two ways", got, want)
			}
			if got, want := pageErr.Error(), plainErr.Error(); got != want {
				t.Errorf("--page-all error = %q, plain api = %q", got, want)
			}
		})
	}
}

// Streaming formats have already written the pages that succeeded, so this line
// is the only thing that says how much of stdout is real. It counts what onItems
// accepted, not what the loop fetched: a page the callback refused and a page
// whose data carries no array field both put nothing on stdout.
func TestStreamPages_FailureSummaryCountsOnlyWhatWasEmitted(t *testing.T) {
	fiveItems := func() (*http.Response, error) {
		items := make([]interface{}, 5)
		for i := range items {
			items[i] = map[string]interface{}{"id": "x"}
		}
		return jsonResponse(map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"items": items, "has_more": false},
		}), nil
	}
	noArray := func() (*http.Response, error) {
		return jsonResponse(map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"has_more": true, "page_token": "tok-3"},
		}), nil
	}
	failing := businessFailure(230027, "user not authorized")

	for _, tc := range []struct {
		name         string
		pages        []func() (*http.Response, error)
		refuseOnPage int
		wantStreamed int
		wantLine     string
	}{
		{
			name:         "page 2 fails outright",
			pages:        []func() (*http.Response, error){firstPageHasMore, failing},
			wantStreamed: 1,
			wantLine:     "[pagination] streamed 1 pages, 1 total items before the run failed",
		},
		{
			name:         "the writer refuses page 2's five items",
			pages:        []func() (*http.Response, error){firstPageHasMore, fiveItems},
			refuseOnPage: 2,
			wantStreamed: 1,
			wantLine:     "[pagination] streamed 1 pages, 1 total items before the run failed",
		},
		{
			name:         "page 2 emits nothing, page 3 fails",
			pages:        []func() (*http.Response, error){firstPageHasMore, noArray, failing},
			wantStreamed: 1,
			wantLine:     "[pagination] streamed 1 pages, 1 total items before the run failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ac, errBuf := newTestAPIClient(t, pageSeqTransport(tc.pages...))
			var streamed []interface{}
			seen := 0
			_, _, err := ac.StreamPages(context.Background(), RawApiRequest{
				Method: "GET", URL: "/open-apis/test", As: "bot",
			}, func(items []interface{}) error {
				seen++
				if seen == tc.refuseOnPage {
					return errors.New("writer refused the page")
				}
				streamed = append(streamed, items...)
				return nil
			}, PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: "bot"})

			if err == nil {
				t.Fatal("StreamPages() error = nil, want the run to fail")
			}
			if len(streamed) != tc.wantStreamed {
				t.Fatalf("onItems accepted %d items, want %d; got %#v", len(streamed), tc.wantStreamed, streamed)
			}
			if !strings.Contains(errBuf.String(), tc.wantLine) {
				t.Errorf("ErrOut = %q, want it to contain %q", errBuf.String(), tc.wantLine)
			}
		})
	}
}
