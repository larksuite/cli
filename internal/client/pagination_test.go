// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestPaginateAll_EmptyPagesPreserveArray(t *testing.T) {
	tests := []struct {
		name      string
		pages     int
		pageLimit int
	}{
		{name: "single page", pages: 1, pageLimit: 10},
		{name: "multiple pages", pages: 2, pageLimit: 10},
		{name: "page limit", pages: 3, pageLimit: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiCalls := 0
			rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				apiCalls++
				return jsonResponse(map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"items":      []interface{}{},
						"has_more":   apiCalls < tt.pages,
						"page_token": "next",
					},
				}), nil
			})
			ac, _ := newTestAPIClient(t, rt)
			result, err := ac.PaginateAll(context.Background(), RawApiRequest{
				Method: "GET",
				URL:    "/open-apis/test",
				As:     "bot",
			}, PaginationOptions{PageLimit: tt.pageLimit, PageDelay: -1})
			if err != nil {
				t.Fatalf("PaginateAll() error = %v", err)
			}
			if want := min(tt.pages, tt.pageLimit); apiCalls != want {
				t.Fatalf("API calls = %d, want %d", apiCalls, want)
			}

			data := result.(map[string]interface{})["data"].(map[string]interface{})
			itemsJSON, err := json.Marshal(data["items"])
			if err != nil {
				t.Fatalf("marshal items: %v", err)
			}
			if string(itemsJSON) != "[]" {
				t.Errorf("items = %s, want []", itemsJSON)
			}
			if want := tt.pages > tt.pageLimit; data["has_more"] != want {
				t.Errorf("has_more = %v, want %v", data["has_more"], want)
			}
		})
	}
}
