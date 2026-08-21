// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
)

func TestWalkPagesPreservesConsumedPagesOnLaterFailure(t *testing.T) {
	runtime, _, _ := newPaginateIntoTestRuntime(t, nil)
	wantErr := errs.NewNetworkError(errs.SubtypeNetworkTimeout, "request timed out")
	consumed := 0
	fetches := 0
	status, err := WalkPages(runtime, PageWalkOptions{
		AutoPaginate:   true,
		PageLimit:      2,
		StartPageToken: "",
	}, func(string) (map[string]any, error) {
		fetches++
		if fetches == 1 {
			return map[string]any{"items": []any{"first"}, "has_more": true, "page_token": "p2"}, nil
		}
		return nil, wantErr
	}, func(map[string]any, int) error {
		consumed++
		return nil
	})

	if !errors.Is(err, wantErr) {
		t.Fatalf("WalkPages() error = %v, want %v", err, wantErr)
	}
	if consumed != 1 || status.PagesFetched != 1 || status.StopReason != client.StopReasonTransportError || status.NextPageToken != "p2" {
		t.Fatalf("consumed/status = %d/%#v, want one resumable page and transport_error", consumed, status)
	}
}

func TestWalkPagesReportsCallerDefinedTruncation(t *testing.T) {
	runtime, _, _ := newPaginateIntoTestRuntime(t, nil)
	status, err := WalkPages(runtime, PageWalkOptions{
		AutoPaginate: true,
		PageLimit:    2,
		IsTruncated: func(page map[string]any) bool {
			truncated, _ := page["domain_truncated"].(bool)
			return truncated
		},
	}, func(string) (map[string]any, error) {
		return map[string]any{"has_more": false, "domain_truncated": true}, nil
	}, func(map[string]any, int) error { return nil })
	if err != nil {
		t.Fatalf("WalkPages() error = %v", err)
	}
	if status.PagesFetched != 1 || status.StopReason != client.StopReasonServerTruncation {
		t.Fatalf("status = %#v, want server_truncation after one page", status)
	}
}

func TestWalkPagesFirstFailurePreservesStartingCursor(t *testing.T) {
	runtime, _, _ := newPaginateIntoTestRuntime(t, nil)
	wantErr := errs.NewNetworkError(errs.SubtypeNetworkTimeout, "request timed out")
	status, err := WalkPages(runtime, PageWalkOptions{
		AutoPaginate:   true,
		PageLimit:      2,
		StartPageToken: "resume",
	}, func(token string) (map[string]any, error) {
		if token != "resume" {
			t.Fatalf("first token = %q, want resume", token)
		}
		return nil, wantErr
	}, func(map[string]any, int) error {
		t.Fatal("consumer must not run when the first fetch fails")
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WalkPages() error = %v, want %v", err, wantErr)
	}
	if status.PagesFetched != 0 || !status.HasMore || status.NextPageToken != "resume" || status.StopReason != client.StopReasonTransportError {
		t.Fatalf("status = %#v, want first-page transport error resumable at supplied cursor", status)
	}
}
