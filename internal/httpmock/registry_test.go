// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package httpmock

import (
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestRegistry_RoundTrip(t *testing.T) {
	reg := &Registry{}
	reg.Register(&Stub{
		Method: "GET",
		URL:    "/open-apis/test",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})

	client := NewClient(reg)
	req, _ := http.NewRequest("GET", "https://open.feishu.cn/open-apis/test", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("want status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if got := string(body); got == "" {
		t.Error("expected non-empty body")
	}
}

func TestRegistry_NoStub(t *testing.T) {
	reg := &Registry{}
	client := NewClient(reg)
	req, _ := http.NewRequest("GET", "https://example.com/missing", nil)
	_, err := client.Do(req)
	if err == nil {
		t.Fatal("expected error for unmatched request")
	}
}

func TestRegistry_MethodMismatch(t *testing.T) {
	reg := &Registry{}
	reg.Register(&Stub{
		Method: "POST",
		URL:    "/open-apis/test",
		Body:   "ok",
	})

	client := NewClient(reg)
	req, _ := http.NewRequest("GET", "https://open.feishu.cn/open-apis/test", nil)
	_, err := client.Do(req)
	if err == nil {
		t.Fatal("expected error for method mismatch")
	}
}

func TestRegistry_Verify_AllMatched(t *testing.T) {
	reg := &Registry{}
	reg.Register(&Stub{
		Method: "GET",
		URL:    "/used",
		Body:   "ok",
	})

	client := NewClient(reg)
	req, _ := http.NewRequest("GET", "https://example.com/used", nil)
	resp, _ := client.Do(req)
	resp.Body.Close()

	reg.Verify(t)
}

func TestRegistry_Verify_Unmatched(t *testing.T) {
	reg := &Registry{}
	reg.Register(&Stub{
		Method: "DELETE",
		URL:    "/unused",
		Body:   "ok",
	})

	fakeT := &testing.T{}
	reg.Verify(fakeT)
	if !fakeT.Failed() {
		t.Error("Verify should report failure for unmatched stub")
	}
}

func TestRegistry_CustomStatus(t *testing.T) {
	reg := &Registry{}
	reg.Register(&Stub{
		URL:    "/error",
		Status: 500,
		Body:   `{"error":"internal"}`,
	})

	client := NewClient(reg)
	req, _ := http.NewRequest("GET", "https://example.com/error", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 500 {
		t.Errorf("want status 500, got %d", resp.StatusCode)
	}
}

func TestRegistry_TransportError(t *testing.T) {
	wantErr := errors.New("connection reset")
	reg := &Registry{}
	reg.Register(&Stub{
		Method: "POST",
		URL:    "/transport-error",
		Error:  wantErr,
	})

	client := NewClient(reg)
	req, _ := http.NewRequest("POST", "https://example.com/transport-error", nil)
	_, err := client.Do(req)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want transport error %v", err, wantErr)
	}
	reg.Verify(t)
}
