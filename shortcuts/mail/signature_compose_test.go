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
	"github.com/larksuite/cli/shortcuts/mail/signature"
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

func TestValidateSignatureWithPlainTextTypedError(t *testing.T) {
	err := validateSignatureWithPlainText(true, "sig_123")
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T (%v)", err, err)
	}
	if len(validationErr.Params) != 2 {
		t.Fatalf("params = %#v, want two conflicting params", validationErr.Params)
	}
	if validationErr.Params[0].Name != "--plain-text" || validationErr.Params[1].Name != "--signature-id" {
		t.Fatalf("unexpected params: %#v", validationErr.Params)
	}
}

func TestValidateSignatureOptionsRejectsNoSignatureWithExplicitID(t *testing.T) {
	err := validateSignatureOptions("sig_123", true)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T (%v)", err, err)
	}
	if !strings.Contains(validationErr.Error(), "--signature-id and --no-signature are mutually exclusive") {
		t.Fatalf("unexpected validation message: %v", validationErr)
	}
}

func TestSelectDefaultSendSignatureID(t *testing.T) {
	resp := &signature.GetSignaturesResponse{
		Usages: []signature.SignatureUsage{
			{EmailAddress: "primary@example.com", SendMailSignatureID: "sig_primary"},
			{EmailAddress: "alias@example.com", SendMailSignatureID: "sig_alias"},
		},
	}

	if got := selectDefaultSendSignatureID(resp, "ALIAS@example.com", "me"); got != "sig_alias" {
		t.Fatalf("alias match = %q, want sig_alias", got)
	}
	if got := selectDefaultSendSignatureID(resp, "", "primary@example.com"); got != "sig_primary" {
		t.Fatalf("mailbox match = %q, want sig_primary", got)
	}

	resp = &signature.GetSignaturesResponse{
		Usages: []signature.SignatureUsage{
			{EmailAddress: "only@example.com", SendMailSignatureID: " sig_only "},
			{EmailAddress: "none@example.com", SendMailSignatureID: "0"},
		},
	}
	if got := selectDefaultSendSignatureID(resp, "", "me"); got != "sig_only" {
		t.Fatalf("single usage fallback = %q, want sig_only", got)
	}

	resp.Usages = append(resp.Usages, signature.SignatureUsage{EmailAddress: "other@example.com", SendMailSignatureID: "sig_other"})
	if got := selectDefaultSendSignatureID(resp, "", "me"); got != "" {
		t.Fatalf("ambiguous fallback = %q, want empty", got)
	}
}

func TestFindUserSignatureByIDRequiresUserType(t *testing.T) {
	resp := &signature.GetSignaturesResponse{
		Signatures: []signature.Signature{
			{ID: "sig_user", SignatureType: signature.SignatureTypeUser},
			{ID: "sig_tenant", SignatureType: signature.SignatureTypeTenant},
			{ID: "sig_missing_type"},
		},
	}

	if got := findUserSignatureByID(resp, "sig_user"); got == nil || got.ID != "sig_user" {
		t.Fatalf("user signature not found: %#v", got)
	}
	if got := findUserSignatureByID(resp, "sig_tenant"); got != nil {
		t.Fatalf("tenant signature should not be auto-selected: %#v", got)
	}
	if got := findUserSignatureByID(resp, "sig_missing_type"); got != nil {
		t.Fatalf("missing signature_type should not be auto-selected: %#v", got)
	}
}

func TestAppendSignatureToPlainText(t *testing.T) {
	sig := &signatureResult{RenderedContent: "<div>Best<br>Alice</div>"}
	got := appendSignatureToPlainText("hello\r\n", sig)
	want := "hello\n\nBest\nAlice"
	if got != want {
		t.Fatalf("plain text signature = %q, want %q", got, want)
	}

	imageOnly := &signatureResult{RenderedContent: `<img src="cid:logo">`}
	if got := appendSignatureToPlainText("hello", imageOnly); got != "hello" {
		t.Fatalf("image-only signature should not change body, got %q", got)
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
