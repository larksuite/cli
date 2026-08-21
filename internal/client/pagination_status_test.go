// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestInspectPaginationPageStatus(t *testing.T) {
	tests := []struct {
		name       string
		data       map[string]interface{}
		startToken string
		want       StopReason
		wantMore   bool
		wantToken  string
		wantErr    bool
	}{
		{
			name: "exhausted",
			data: map[string]interface{}{"has_more": false},
			want: StopReasonExhausted,
		},
		{
			name:      "single page",
			data:      map[string]interface{}{"has_more": true, "page_token": "next"},
			want:      StopReasonSinglePage,
			wantMore:  true,
			wantToken: "next",
		},
		{
			name:       "start page token",
			data:       map[string]interface{}{"has_more": false},
			startToken: "middle",
			want:       StopReasonStartPageToken,
		},
		{
			name:     "missing token",
			data:     map[string]interface{}{"has_more": true},
			want:     StopReasonMissingToken,
			wantMore: true,
			wantErr:  true,
		},
		{
			name: "server truncation",
			data: map[string]interface{}{"has_more": false, "truncated": true},
			want: StopReasonServerTruncation,
		},
		{
			name: "message text does not imply server truncation",
			data: map[string]interface{}{"has_more": false, "message": "result was truncated"},
			want: StopReasonExhausted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := map[string]interface{}{
				"code": float64(0),
				"data": tt.data,
			}
			status, err := InspectPaginationPage(result, tt.startToken)
			if (err != nil) != tt.wantErr {
				t.Fatalf("InspectPaginationPage() error = %v, wantErr %v", err, tt.wantErr)
			}
			if status.StopReason != tt.want {
				t.Errorf("StopReason = %q, want %q", status.StopReason, tt.want)
			}
			if status.PagesFetched != 1 {
				t.Errorf("PagesFetched = %d, want 1", status.PagesFetched)
			}
			if status.HasMore != tt.wantMore {
				t.Errorf("HasMore = %v, want %v", status.HasMore, tt.wantMore)
			}
			if status.NextPageToken != tt.wantToken {
				t.Errorf("NextPageToken = %q, want %q", status.NextPageToken, tt.wantToken)
			}
			if status.Cause != err {
				t.Errorf("Cause = %v, want returned error %v", status.Cause, err)
			}
		})
	}
}

func TestPaginationStatusCauseIsNotSerialized(t *testing.T) {
	status := PaginationStatus{
		PagesFetched:  1,
		HasMore:       true,
		NextPageToken: "next",
		StopReason:    StopReasonTransportError,
		Cause:         errors.New("contains sensitive transport details"),
	}

	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(raw), "sensitive") || strings.Contains(string(raw), "cause") {
		t.Fatalf("serialized status leaked Cause: %s", raw)
	}
}

func TestPaginateAllWithStatusStopReasons(t *testing.T) {
	tests := []struct {
		name       string
		firstToken string
		pageLimit  int
		pages      []map[string]interface{}
		wantCalls  int
		wantReason StopReason
		wantPages  int
		wantMore   bool
		wantToken  string
		wantErr    bool
	}{
		{
			name: "exhausted with unlimited page limit",
			pages: []map[string]interface{}{
				pageResult(true, "next", false, "1"),
				pageResult(false, "", false, "2"),
			},
			wantCalls:  2,
			wantReason: StopReasonExhausted,
			wantPages:  2,
		},
		{
			name: "page limit",
			pages: []map[string]interface{}{
				pageResult(true, "next", false, "1"),
				pageResult(true, "last", false, "2"),
			},
			pageLimit:  2,
			wantCalls:  2,
			wantReason: StopReasonPageLimit,
			wantPages:  2,
			wantMore:   true,
			wantToken:  "last",
		},
		{
			name:       "start page token stays incomplete after exhaustion",
			firstToken: "middle",
			pages: []map[string]interface{}{
				pageResult(false, "", false, "1"),
			},
			wantCalls:  1,
			wantReason: StopReasonStartPageToken,
			wantPages:  1,
		},
		{
			name: "missing token fails closed",
			pages: []map[string]interface{}{
				pageResult(true, "", false, "1"),
			},
			wantCalls:  1,
			wantReason: StopReasonMissingToken,
			wantPages:  1,
			wantMore:   true,
			wantErr:    true,
		},
		{
			name: "repeated token fails closed",
			pages: []map[string]interface{}{
				pageResult(true, "secret-token-x", false, "1"),
				pageResult(true, "secret-token-x", false, "2"),
			},
			wantCalls:  2,
			wantReason: StopReasonRepeatedToken,
			wantPages:  2,
			wantMore:   true,
			wantToken:  "secret-token-x",
			wantErr:    true,
		},
		{
			name: "server truncation is explicit structured fact",
			pages: []map[string]interface{}{
				pageResult(false, "", true, "1"),
			},
			wantCalls:  1,
			wantReason: StopReasonServerTruncation,
			wantPages:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			ac, _ := newTestAPIClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
				if calls >= len(tt.pages) {
					t.Fatalf("unexpected API call %d", calls+1)
				}
				body := tt.pages[calls]
				calls++
				return jsonResponse(body), nil
			}))
			params := map[string]interface{}{}
			if tt.firstToken != "" {
				params["page_token"] = tt.firstToken
			}

			result, status, err := ac.PaginateAllWithStatus(context.Background(), &RawApiRequest{
				Method: "GET",
				URL:    "/open-apis/test",
				Params: params,
				As:     "bot",
			}, PaginationOptions{PageLimit: tt.pageLimit, PageDelay: -1})

			if (err != nil) != tt.wantErr {
				t.Fatalf("PaginateAllWithStatus() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				switch tt.wantReason {
				case StopReasonMissingToken:
					if err.Error() != "paginated response has_more=true but next page token is missing" {
						t.Fatalf("missing-token error = %q", err)
					}
				case StopReasonRepeatedToken:
					if err.Error() != "paginated response repeated the same next page token" {
						t.Fatalf("repeated-token error = %q", err)
					}
				}
			}
			if calls != tt.wantCalls {
				t.Errorf("API calls = %d, want %d", calls, tt.wantCalls)
			}
			if status.StopReason != tt.wantReason {
				t.Errorf("StopReason = %q, want %q", status.StopReason, tt.wantReason)
			}
			if status.PagesFetched != tt.wantPages {
				t.Errorf("PagesFetched = %d, want %d", status.PagesFetched, tt.wantPages)
			}
			if status.HasMore != tt.wantMore {
				t.Errorf("HasMore = %v, want %v", status.HasMore, tt.wantMore)
			}
			if status.NextPageToken != tt.wantToken {
				t.Errorf("NextPageToken = %q, want %q", status.NextPageToken, tt.wantToken)
			}
			if result == nil {
				t.Fatal("result must preserve successfully fetched pages")
			}
			if tt.wantErr {
				var internalErr *errs.InternalError
				if !errors.As(err, &internalErr) || internalErr.Subtype != errs.SubtypeInvalidResponse {
					t.Fatalf("error = %T %v, want invalid_response InternalError", err, err)
				}
				if tt.wantToken != "" && strings.Contains(err.Error(), tt.wantToken) {
					t.Fatalf("error leaked page token: %v", err)
				}
			}
		})
	}
}

func TestPaginateAllWithStatusPreservesPartialResultAndTypedLateError(t *testing.T) {
	t.Run("transport error", func(t *testing.T) {
		calls := 0
		ac, _ := newTestAPIClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return jsonResponse(pageResult(true, "next", false, "1")), nil
			}
			return nil, &net.DNSError{Err: "no such host", Name: "example.invalid"}
		}))

		result, status, err := ac.PaginateAllWithStatus(context.Background(), &RawApiRequest{
			Method: "GET",
			URL:    "/open-apis/test",
			As:     "bot",
		}, PaginationOptions{PageDelay: -1})

		var networkErr *errs.NetworkError
		if !errors.As(err, &networkErr) {
			t.Fatalf("error = %T %v, want typed NetworkError", err, err)
		}
		assertPartialPage(t, result, "1")
		if status.StopReason != StopReasonTransportError || status.PagesFetched != 1 || status.NextPageToken != "next" {
			t.Fatalf("status = %#v, want late transport error with resumable token", status)
		}
		if status.Cause != err {
			t.Fatalf("Cause = %v, want returned error %v", status.Cause, err)
		}
	})

	t.Run("API error", func(t *testing.T) {
		calls := 0
		ac, _ := newTestAPIClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return jsonResponse(pageResult(true, "next", false, "1")), nil
			}
			return jsonResponse(map[string]interface{}{"code": 999, "msg": "failed"}), nil
		}))

		result, status, err := ac.PaginateAllWithStatus(context.Background(), &RawApiRequest{
			Method: "GET",
			URL:    "/open-apis/test",
			As:     "bot",
		}, PaginationOptions{PageDelay: -1})

		var apiErr *errs.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("error = %T %v, want typed APIError", err, err)
		}
		assertPartialPage(t, result, "1")
		if status.StopReason != StopReasonAPIError || status.PagesFetched != 1 || status.NextPageToken != "next" {
			t.Fatalf("status = %#v, want late API error with resumable token", status)
		}
	})
}

func TestPaginateAllWithStatusHTTPNormalizerIsOptIn(t *testing.T) {
	newClient := func(t *testing.T) *APIClient {
		t.Helper()
		response := jsonResponse(pageResult(false, "", false, "1"))
		response.StatusCode = http.StatusServiceUnavailable
		ac, _ := newTestAPIClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return response, nil
		}))
		return ac
	}

	t.Run("normalizer classifies HTTP status", func(t *testing.T) {
		marker := errors.New("normalized HTTP failure")
		ac := newClient(t)
		_, status, err := ac.PaginateAllWithStatus(context.Background(), &RawApiRequest{
			Method: "GET",
			URL:    "/open-apis/test",
			As:     "bot",
		}, PaginationOptions{
			PageDelay: -1,
			NormalizeHTTPError: func(status int, _ string, err error) error {
				if status != http.StatusServiceUnavailable || err != nil {
					t.Fatalf("normalizer input = status %d, err %v", status, err)
				}
				return marker
			},
		})
		if !errors.Is(err, marker) || status.StopReason != StopReasonAPIError {
			t.Fatalf("err = %v, status = %#v", err, status)
		}
	})

	t.Run("nil normalizer preserves legacy behavior", func(t *testing.T) {
		ac := newClient(t)
		_, status, err := ac.PaginateAllWithStatus(context.Background(), &RawApiRequest{
			Method: "GET",
			URL:    "/open-apis/test",
			As:     "bot",
		}, PaginationOptions{PageDelay: -1})
		if err != nil || status.StopReason != StopReasonExhausted {
			t.Fatalf("err = %v, status = %#v", err, status)
		}
	})
}

func TestStreamPagesWithStatusPreservesEmittedPagesOnLateError(t *testing.T) {
	calls := 0
	ac, _ := newTestAPIClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return jsonResponse(pageResult(true, "next", false, "1")), nil
		}
		return nil, &net.DNSError{Err: "no such host", Name: "example.invalid"}
	}))

	var emitted []map[string]interface{}
	status, err := ac.StreamPagesWithStatus(context.Background(), &RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test",
		As:     "bot",
	}, PaginationOptions{PageDelay: -1}, func(page map[string]interface{}) error {
		emitted = append(emitted, page)
		return nil
	})

	var networkErr *errs.NetworkError
	if !errors.As(err, &networkErr) {
		t.Fatalf("error = %T %v, want typed NetworkError", err, err)
	}
	if len(emitted) != 1 {
		t.Fatalf("emitted pages = %d, want 1", len(emitted))
	}
	if status.StopReason != StopReasonTransportError || status.PagesFetched != 1 {
		t.Fatalf("status = %#v, want late transport error", status)
	}
}

func TestLegacyPaginateAllStillSwallowsLateTransportError(t *testing.T) {
	calls := 0
	ac, errOut := newTestAPIClient(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return jsonResponse(pageResult(true, "next", false, "1")), nil
		}
		return nil, &net.DNSError{Err: "no such host", Name: "example.invalid"}
	}))

	result, err := ac.PaginateAll(context.Background(), RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test",
		As:     "bot",
	}, PaginationOptions{PageDelay: -1})

	if err != nil {
		t.Fatalf("legacy PaginateAll() error = %v, want nil", err)
	}
	assertPartialPage(t, result, "1")
	if !strings.Contains(errOut.String(), "[page 2] error, stopping pagination") {
		t.Fatalf("legacy warning changed: %q", errOut.String())
	}
}

func pageResult(hasMore bool, token string, truncated bool, id string) map[string]interface{} {
	data := map[string]interface{}{
		"items":     []interface{}{map[string]interface{}{"id": id}},
		"has_more":  hasMore,
		"truncated": truncated,
	}
	if token != "" {
		data["page_token"] = token
	}
	return map[string]interface{}{"code": float64(0), "msg": "ok", "data": data}
}

func assertPartialPage(t *testing.T, result interface{}, wantID string) {
	t.Helper()
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result = %T, want map", result)
	}
	data, ok := resultMap["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data = %T, want map", resultMap["data"])
	}
	items, ok := data["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v, want one item", data["items"])
	}
	item, ok := items[0].(map[string]interface{})
	if !ok || item["id"] != wantID {
		t.Fatalf("item = %#v, want id %q", items[0], wantID)
	}
}
