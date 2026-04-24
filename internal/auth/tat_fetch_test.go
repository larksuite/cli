// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchTenantAccessToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %q", r.Method)
		}
		if r.URL.Path != PathTenantAccessTokenInternal {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"app_id":"app"`) || !strings.Contains(string(body), `"app_secret":"sec"`) {
			t.Fatalf("unexpected request body: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","tenant_access_token":"t-abc","expire":7200}`)
	}))
	defer srv.Close()

	res, err := FetchTenantAccessToken(context.Background(), srv.Client(), srv.URL, "app", "sec")
	if err != nil {
		t.Fatal(err)
	}
	if res.Token != "t-abc" || res.ExpiresIn != 7200 {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestFetchTenantAccessToken_RejectsEmptyURL(t *testing.T) {
	_, err := FetchTenantAccessToken(context.Background(), http.DefaultClient, "", "app", "sec")
	if err == nil {
		t.Fatal("expected error for empty openBaseURL")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention empty URL, got: %v", err)
	}
}

func TestFetchTenantAccessToken_RejectsMissingScheme(t *testing.T) {
	_, err := FetchTenantAccessToken(context.Background(), http.DefaultClient, "open.feishu.cn", "app", "sec")
	if err == nil {
		t.Fatal("expected error for missing scheme")
	}
	if !strings.Contains(err.Error(), "http://") {
		t.Errorf("error should mention scheme requirement, got: %v", err)
	}
}

func TestFetchTenantAccessToken_RejectsEmptyAppID(t *testing.T) {
	_, err := FetchTenantAccessToken(context.Background(), http.DefaultClient, "https://open.feishu.cn", "", "sec")
	if err == nil {
		t.Fatal("expected error for empty app id")
	}
	if !strings.Contains(err.Error(), "app id") {
		t.Errorf("error should mention app id, got: %v", err)
	}
}

func TestFetchTenantAccessToken_Non200IncludesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":10012,"msg":"invalid app_id"}`)
	}))
	defer srv.Close()

	_, err := FetchTenantAccessToken(context.Background(), srv.Client(), srv.URL, "app", "sec")
	if err == nil {
		t.Fatal("expected error on HTTP 400")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should include status code: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid app_id") {
		t.Errorf("error should include server body snippet: %v", err)
	}
}

func TestFetchTenantAccessToken_APIErrorCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":10003,"msg":"app_secret invalid"}`)
	}))
	defer srv.Close()

	_, err := FetchTenantAccessToken(context.Background(), srv.Client(), srv.URL, "app", "sec")
	if err == nil {
		t.Fatal("expected error on non-zero API code")
	}
	if !strings.Contains(err.Error(), "app_secret invalid") {
		t.Errorf("error should surface API msg, got: %v", err)
	}
}

func TestFetchTenantAccessToken_TrimsTrailingSlash(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","tenant_access_token":"t","expire":7200}`)
	}))
	defer srv.Close()

	if _, err := FetchTenantAccessToken(context.Background(), srv.Client(), srv.URL+"/", "app", "sec"); err != nil {
		t.Fatal(err)
	}
	if gotPath != PathTenantAccessTokenInternal {
		t.Errorf("expected single-slash path %q, got %q", PathTenantAccessTokenInternal, gotPath)
	}
}
