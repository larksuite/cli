// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

// captureRT records the last request + body and returns a canned device-auth response.
func captureDeviceAuthClient(gotReq **http.Request, gotBody *string, respJSON string) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		*gotReq = req
		if req.Body != nil {
			b, _ := io.ReadAll(req.Body)
			*gotBody = string(b)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(respJSON)),
		}, nil
	})}
}

const deviceAuthRespJSON = `{"device_code":"dc","user_code":"uc","verification_uri":"https://example/verify","expires_in":300,"interval":5}`

func TestRequestDeviceAuthorization_PrivateKeyJWT_UsesAssertionNotBasic(t *testing.T) {
	var req *http.Request
	var body string
	client := captureDeviceAuthClient(&req, &body, deviceAuthRespJSON)

	ca := ClientAuth{AppID: "cli_a", AuthMethod: core.AuthMethodPrivateKeyJWT, Signer: newFakeAuthSigner(t), KeyLabel: "k"}
	if _, err := RequestDeviceAuthorization(context.Background(), client, ca, core.BrandFeishu, "im:message:send", nil); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Errorf("private_key_jwt must NOT send Basic auth, got %q", req.Header.Get("Authorization"))
	}
	form, _ := url.ParseQuery(body)
	if form.Get("client_assertion") == "" {
		t.Error("missing client_assertion")
	}
	if form.Get("client_assertion_type") != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" {
		t.Errorf("client_assertion_type = %q", form.Get("client_assertion_type"))
	}
	if form.Has("client_secret") {
		t.Error("client_secret must not be present for private_key_jwt")
	}
}

func TestRequestDeviceAuthorization_ClientSecret_UsesBasic(t *testing.T) {
	var req *http.Request
	var body string
	client := captureDeviceAuthClient(&req, &body, deviceAuthRespJSON)

	ca := ClientAuth{AppID: "cli_a", AppSecret: "test-secret"} // client_secret
	if _, err := RequestDeviceAuthorization(context.Background(), client, ca, core.BrandFeishu, "", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "Basic ") {
		t.Errorf("client_secret should use Basic auth, got %q", req.Header.Get("Authorization"))
	}
	form, _ := url.ParseQuery(body)
	if form.Has("client_assertion") {
		t.Error("client_secret must not send a client_assertion")
	}
}

func TestPollDeviceToken_ReturnsAssertionErrorWithoutRequest(t *testing.T) {
	var requests atomic.Int32
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests.Add(1)
			return nil, errors.New("unexpected request")
		}),
	}

	result, err := PollDeviceToken(
		context.Background(),
		client,
		ClientAuth{AppID: "cli_a", AuthMethod: core.AuthMethodPrivateKeyJWT},
		core.BrandFeishu,
		"device-code",
		1,
		3,
		nil,
	)
	if result != nil {
		t.Fatalf("PollDeviceToken() result = %#v, want nil", result)
	}
	if err == nil || !strings.Contains(err.Error(), "requires a key signer") {
		t.Fatalf("PollDeviceToken() error = %v, want key signer error", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("PollDeviceToken() sent %d requests, want 0", got)
	}
}
