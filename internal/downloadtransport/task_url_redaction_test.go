// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package downloadtransport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	extdownload "github.com/larksuite/cli/extension/download"
)

func TestTaskURLDoesNotLeakSignedURLOrResponseBody(t *testing.T) {
	const signedURL = "https://example.com/object?authcode=secret-value"
	httpClient := &http.Client{Transport: taskURLRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("denied for authcode=secret-value")),
		}, nil
	})}
	_, err := URL(httpClient, signedURL)(context.Background(), extdownload.Request{})
	if err == nil {
		t.Fatal("URL() error = nil, want HTTP error")
	}
	if message := err.Error(); strings.Contains(message, "secret-value") || strings.Contains(message, "authcode") {
		t.Fatalf("URL() error leaked signed URL material: %q", message)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkTransport || problem.Code != http.StatusForbidden {
		t.Fatalf("problem = %#v, %v; want network/transport HTTP 403", problem, ok)
	}
}

func TestTaskURLDoesNotLeakSignedURLOnTransportFailure(t *testing.T) {
	const signedURL = "https://example.com/object?authcode=secret-value"
	cause := errors.New("connection refused")
	httpClient := &http.Client{Transport: taskURLRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, cause
	})}
	_, err := URL(httpClient, signedURL)(context.Background(), extdownload.Request{})
	if err == nil {
		t.Fatal("URL() error = nil, want transport error")
	}
	if message := err.Error(); strings.Contains(message, "secret-value") || strings.Contains(message, "authcode") {
		t.Fatalf("URL() error leaked signed URL material: %q", message)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkTransport || !problem.Retryable || !errors.Is(err, cause) {
		t.Fatalf("problem = %#v, %v; want retryable network/transport", problem, ok)
	}
}

func TestTaskURLDoesNotLeakMalformedSignedURL(t *testing.T) {
	const signedURL = "https://%secret-value?authcode=secret-value"
	httpClient := &http.Client{}
	_, err := URL(httpClient, signedURL)(context.Background(), extdownload.Request{})
	if err == nil {
		t.Fatal("URL() error = nil, want invalid URL error")
	}
	if message := err.Error(); strings.Contains(message, "secret-value") || strings.Contains(message, "authcode") {
		t.Fatalf("URL() error leaked signed URL material: %q", message)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkTransport || problem.Retryable {
		t.Fatalf("problem = %#v, %v; want non-retryable network/transport", problem, ok)
	}
}

func TestTaskURLDoesNotLeakSignedURLOnRedirectFailure(t *testing.T) {
	const signedURL = "https://example.com/object?authcode=secret-value"
	cause := errors.New("blocked redirect target")
	httpClient := &http.Client{
		Transport: taskURLRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": {"https://redirect.example/object?authcode=redirect-secret-value"}},
				Body:       io.NopCloser(strings.NewReader("redirect")),
				Request:    req,
			}, nil
		}),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return cause
		},
	}
	_, err := URL(httpClient, signedURL)(context.Background(), extdownload.Request{})
	if err == nil {
		t.Fatal("URL() error = nil, want redirect error")
	}
	if message := err.Error(); strings.Contains(message, "secret-value") || strings.Contains(message, "authcode") {
		t.Fatalf("URL() error leaked signed URL material: %q", message)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkTransport || problem.Retryable || !errors.Is(err, cause) {
		t.Fatalf("problem = %#v, %v; want non-retryable network/transport with redirect cause", problem, ok)
	}
}

type taskURLRoundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip taskURLRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}
