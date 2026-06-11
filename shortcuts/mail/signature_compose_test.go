// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

func TestDownloadSignatureImageRejectsInvalidURLs(t *testing.T) {
	rt := newDownloadRuntime(t, &http.Client{})

	cases := []struct {
		name string
		url  string
	}{
		{name: "invalid", url: "https://[::1"},
		{name: "http", url: "http://example.com/sig.png"},
		{name: "no host", url: "https:///sig.png"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := downloadSignatureImage(rt, tc.url, "sig.png")
			var internalErr *errs.InternalError
			if !errors.As(err, &internalErr) {
				t.Fatalf("expected internal error, got %T (%v)", err, err)
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed problem, got %T", err)
			}
			if p.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidResponse)
			}
		})
	}
}

func TestDownloadSignatureImageHTTPErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name       string
		statusCode int
		wantType   any
		wantSub    errs.Subtype
		retryable  bool
	}{
		{
			name:       "server",
			statusCode: http.StatusInternalServerError,
			wantType:   (*errs.NetworkError)(nil),
			wantSub:    errs.SubtypeNetworkServer,
			retryable:  true,
		},
		{
			name:       "not found",
			statusCode: http.StatusNotFound,
			wantType:   (*errs.APIError)(nil),
			wantSub:    errs.SubtypeNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "download failed", tc.statusCode)
			}))
			t.Cleanup(srv.Close)
			rt := newDownloadRuntime(t, srv.Client())

			_, _, err := downloadSignatureImage(rt, srv.URL+"/sig.png", "sig.png")
			switch tc.wantType.(type) {
			case *errs.NetworkError:
				var networkErr *errs.NetworkError
				if !errors.As(err, &networkErr) {
					t.Fatalf("expected network error, got %T (%v)", err, err)
				}
			case *errs.APIError:
				var apiErr *errs.APIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("expected API error, got %T (%v)", err, err)
				}
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed problem, got %T", err)
			}
			if p.Code != tc.statusCode {
				t.Fatalf("code = %d, want %d", p.Code, tc.statusCode)
			}
			if p.Subtype != tc.wantSub {
				t.Fatalf("subtype = %q, want %q", p.Subtype, tc.wantSub)
			}
			if p.Retryable != tc.retryable {
				t.Fatalf("retryable = %v, want %v", p.Retryable, tc.retryable)
			}
		})
	}
}

func TestDownloadSignatureImageReadAndSizeErrors(t *testing.T) {
	readErr := errors.New("socket closed")
	rt := newDownloadRuntime(t, &http.Client{
		Transport: signatureRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       signatureErrorBody{err: readErr},
				Request:    req,
			}, nil
		}),
	})

	_, _, err := downloadSignatureImage(rt, "https://example.com/sig.png", "sig.png")
	var networkErr *errs.NetworkError
	if !errors.As(err, &networkErr) {
		t.Fatalf("expected network error, got %T (%v)", err, err)
	}
	if !errors.Is(err, readErr) {
		t.Fatalf("read cause not preserved: %v", err)
	}

	rt = newDownloadRuntime(t, &http.Client{
		Transport: signatureRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       &bodyFileTestFile{remaining: 10*1024*1024 + 1},
				Request:    req,
			}, nil
		}),
	})

	_, _, err = downloadSignatureImage(rt, "https://example.com/huge.png", "huge.png")
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T (%v)", err, err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T", err)
	}
	if p.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("subtype = %q, want %q", p.Subtype, errs.SubtypeFailedPrecondition)
	}
}

func TestDownloadSignatureImageSuccessUsesFilenameContentType(t *testing.T) {
	rt := newDownloadRuntime(t, &http.Client{
		Transport: signatureRoundTripper(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("gif-data")),
				Request:    req,
			}, nil
		}),
	})

	data, contentType, err := downloadSignatureImage(rt, "https://example.com/sig.gif", "sig.gif")
	if err != nil {
		t.Fatalf("downloadSignatureImage failed: %v", err)
	}
	if string(data) != "gif-data" {
		t.Fatalf("data = %q", string(data))
	}
	if contentType != "image/gif" {
		t.Fatalf("content type = %q, want image/gif", contentType)
	}
}

func TestValidateNoSignatureConflictTypedError(t *testing.T) {
	err := validateNoSignatureConflict(true, "sig_123")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// output.ErrValidation returns *output.ExitError with exit code ExitValidation (2).
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T (%v)", err, err)
	}
	if exitErr.Code != output.ExitValidation {
		t.Fatalf("exit code = %d, want %d (ExitValidation)", exitErr.Code, output.ExitValidation)
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error message = %q, want it to contain \"mutually exclusive\"", err.Error())
	}
}

func TestValidateNoSignatureConflictNoError(t *testing.T) {
	if err := validateNoSignatureConflict(false, "sig_123"); err != nil {
		t.Fatalf("expected no error when noSignature=false, got %v", err)
	}
	if err := validateNoSignatureConflict(true, ""); err != nil {
		t.Fatalf("expected no error when signatureID empty, got %v", err)
	}
}

func TestInjectPlainTextSignatureNilSig(t *testing.T) {
	body := "Hello world"
	got := injectPlainTextSignature(body, nil)
	if got != body {
		t.Fatalf("expected unchanged body %q, got %q", body, got)
	}
}

func TestInjectPlainTextSignatureEmptyHTML(t *testing.T) {
	sig := &signatureResult{RenderedContent: "   <br>   "}
	body := "Hello world"
	got := injectPlainTextSignature(body, sig)
	// PlainTextFromHTML on whitespace-only HTML collapses to empty → no change
	if got != body {
		t.Fatalf("expected unchanged body for empty HTML sig, got %q", got)
	}
}

func TestInjectPlainTextSignatureAppendsWithBlankLine(t *testing.T) {
	sig := &signatureResult{RenderedContent: "<div>Best,<br>Bob</div>"}
	body := "Hello world"
	got := injectPlainTextSignature(body, sig)
	if !strings.HasPrefix(got, body+"\n\n") {
		t.Fatalf("expected body followed by two newlines, got %q", got)
	}
	if !strings.Contains(got, "Best,") || !strings.Contains(got, "Bob") {
		t.Fatalf("expected sig text in result, got %q", got)
	}
}

func TestInjectPlainTextSignatureTrimsTrailingNewlines(t *testing.T) {
	// RenderedContent whose plain-text rendering ends in newlines — they must be trimmed.
	sig := &signatureResult{RenderedContent: "<p>Alice</p>"}
	body := "My message"
	got := injectPlainTextSignature(body, sig)
	// Result must not end with a bare newline after the signature text.
	if strings.HasSuffix(got, "\n") {
		t.Fatalf("result should not end with newline, got %q", got)
	}
}

type signatureRoundTripper func(*http.Request) (*http.Response, error)

func (rt signatureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}

type signatureErrorBody struct {
	err error
}

func (b signatureErrorBody) Read([]byte) (int, error) {
	return 0, b.err
}

func (b signatureErrorBody) Close() error {
	return nil
}
