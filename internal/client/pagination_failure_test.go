// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/larksuite/cli/errs"
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

func transportFailure() (*http.Response, error) {
	return nil, errors.New("simulated transport failure")
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
}

// 230027 is in the codemeta table, so it classifies as authorization. This case
// exists to prove a later page classifies exactly like page 1 does — the same
// code through the same CheckResponse path.
func TestPaginateAll_LaterPageKnownBusinessCodeMatchesPageOneClassification(t *testing.T) {
	ac, _ := newTestAPIClient(t, pageSeqTransport(
		firstPageHasMore, businessFailure(230027, "user not authorized")))

	_, err := ac.PaginateAll(context.Background(), RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test",
		As:     "bot",
	}, PaginationOptions{PageLimit: 0, PageDelay: -1, Identity: "bot"})

	if err == nil {
		t.Fatal("PaginateAll() error = nil, want business error from page 2")
	}
	if got := errs.CategoryOf(err); got != errs.CategoryAuthorization {
		t.Errorf("errs.CategoryOf(err) = %q, want %q", got, errs.CategoryAuthorization)
	}
}

// The streaming path already surfaced non-zero codes before this change (the
// failing page was appended and returned as the last result, where the command
// layer's CheckResponse caught it). This case locks that behaviour against
// regression now that the error is built inside the loop instead.
//
// Deliberately does NOT assert the "[pagination] streamed N pages" summary:
// that line sits after the loopErr check, so it is no longer printed on failure.
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
