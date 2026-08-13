// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/requestcontext"
)

type testRemoteFilePolicy struct {
	allowed string
	managed bool
}

func (p testRemoteFilePolicy) ValidateRemoteFile(rawURL string) error {
	if p.allowed != "" && rawURL != p.allowed {
		return errors.New("remote file rejected")
	}
	return nil
}

func (p testRemoteFilePolicy) UsesManagedFilePlane() bool { return p.managed }

func TestRemoteFilesDirectPreservesCallerClientAndPolicy(t *testing.T) {
	var managedCalls, directCalls, validationCalls int
	direct := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		directCalls++
		if got := requestcontext.Identity(req.Context()); got != "" {
			t.Fatalf("direct request context was changed: identity = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    req,
		}, nil
	})}
	files := NewRemoteFiles(nil, func() (*http.Client, error) {
		managedCalls++
		return nil, errors.New("must not be called")
	}, core.AsUser)

	file, err := files.Validate(context.Background(), "https://files.example/object?signature=x",
		func(_ context.Context, rawURL string) error {
			validationCalls++
			if rawURL != "https://files.example/object?signature=x" {
				t.Fatalf("validator URL = %q", rawURL)
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	req, err := file.NewRequest(context.Background(), http.MethodGet, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := files.Do(req, file, direct)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if directCalls != 1 || managedCalls != 0 || validationCalls != 1 {
		t.Fatalf("calls direct=%d managed=%d validation=%d", directCalls, managedCalls, validationCalls)
	}
}

func TestRemoteFilesManagedValidatesAndRoutesOpaqueHandle(t *testing.T) {
	const opaqueURL = "https://proxy.example/lark-cli/v1/files/opaque_1"
	policy := testRemoteFilePolicy{allowed: opaqueURL, managed: true}
	var managedCalls, directCalls, directValidationCalls int
	managed := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		managedCalls++
		if got := requestcontext.Identity(req.Context()); got != core.AsBot {
			t.Fatalf("request identity = %q, want bot", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    req,
		}, nil
	})}
	direct := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		directCalls++
		return nil, errors.New("must not be called")
	})}
	files := NewRemoteFiles(policy, func() (*http.Client, error) { return managed, nil }, core.AsBot)

	if _, err := files.Validate(context.Background(), "https://objects.example/raw?signature=secret"); err == nil {
		t.Fatal("raw object URL accepted in managed mode")
	}
	file, err := files.Validate(context.Background(), opaqueURL,
		func(context.Context, string) error {
			directValidationCalls++
			return errors.New("must not be called")
		})
	if err != nil {
		t.Fatal(err)
	}
	req, err := file.NewRequest(context.Background(), http.MethodGet, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := files.Do(req, file, direct)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if managedCalls != 1 || directCalls != 0 || directValidationCalls != 0 {
		t.Fatalf("calls managed=%d direct=%d validation=%d", managedCalls, directCalls, directValidationCalls)
	}
}

func TestRemoteFilesDoRejectsReferenceMismatch(t *testing.T) {
	files := NewRemoteFiles(nil, nil, core.AsUser)
	file, err := files.Validate(context.Background(), "https://files.example/one")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://files.example/two", nil)
	if _, err := files.Do(req, file, http.DefaultClient); err == nil {
		t.Fatal("mismatched request and validated file accepted")
	}
}

func TestRemoteFilesRequirePortableURL(t *testing.T) {
	files := NewRemoteFiles(testRemoteFilePolicy{managed: true}, nil, core.AsBot)
	err := files.RequirePortableURL("--url-only")
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want ValidationError", err, err)
	}
	if validationErr.Subtype != errs.SubtypeFailedPrecondition || validationErr.Param != "--url-only" {
		t.Fatalf("error = subtype %q param %q", validationErr.Subtype, validationErr.Param)
	}
}

func TestCallAPIPreservesURLsInArbitraryJSON(t *testing.T) {
	const rawURL = "https://objects.example/raw?signature=opaque"
	apiClient, _ := newTestAPIClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := jsonResponse(map[string]any{
			"code": 0,
			"data": map[string]any{
				"download_url": rawURL,
				"nested":       []any{map[string]any{"url": rawURL}},
			},
		})
		resp.Request = req
		return resp, nil
	}))
	result, err := apiClient.CallAPI(context.Background(), RawApiRequest{
		Method: http.MethodGet,
		URL:    "/open-apis/example/v1/raw",
		As:     core.AsBot,
	})
	if err != nil {
		t.Fatal(err)
	}
	data := result.(map[string]any)["data"].(map[string]any)
	if got := data["download_url"]; got != rawURL {
		t.Fatalf("download_url = %v, want exact arbitrary JSON value %q", got, rawURL)
	}
	nested := data["nested"].([]any)[0].(map[string]any)
	if got := nested["url"]; got != rawURL {
		t.Fatalf("nested url = %v, want exact arbitrary JSON value %q", got, rawURL)
	}
}
