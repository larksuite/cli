// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

// stubRoundTripper returns a canned response for every request.
type stubRoundTripper struct {
	status int
	body   string
}

func (s stubRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: s.status,
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Header:     make(http.Header),
	}, nil
}

// TestAppRegSentinelMessages locks the user-facing message text so the
// interactive create flow (which renders these via "%v") does not regress when
// the errors gained errors.Is support.
func TestAppRegSentinelMessages(t *testing.T) {
	cases := map[string]string{
		ErrAppRegDenied.Error():                                      "app registration denied by user",
		ErrAppRegCancelled.Error():                                   "polling was cancelled",
		fmt.Errorf("%w, please try again", ErrAppRegExpired).Error(): "device code expired, please try again",
		fmt.Errorf("%w, please try again", ErrAppRegTimeout).Error(): "app registration timed out, please try again",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("message = %q, want %q", got, want)
		}
	}
}

// TestPollAppRegistration_Classifies verifies that terminal poll outcomes are
// returned as the matching sentinel error (interval 0 keeps the test fast).
func TestPollAppRegistration_Classifies(t *testing.T) {
	cases := []struct {
		name string
		body string
		want error
	}{
		{"access_denied", `{"error":"access_denied"}`, ErrAppRegDenied},
		{"expired_token", `{"error":"expired_token"}`, ErrAppRegExpired},
		{"invalid_grant", `{"error":"invalid_grant"}`, ErrAppRegExpired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := &http.Client{Transport: stubRoundTripper{status: 200, body: c.body}}
			_, err := PollAppRegistration(context.Background(), client, core.BrandFeishu, "dc", 0, 60, io.Discard)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want errors.Is(%v)", err, c.want)
			}
		})
	}
}

func TestPollAppRegistration_Success(t *testing.T) {
	body := `{"client_id":"cli_x","client_secret":"sec","user_info":{"tenant_brand":"feishu","open_id":"ou_1"}}`
	client := &http.Client{Transport: stubRoundTripper{status: 200, body: body}}
	res, err := PollAppRegistration(context.Background(), client, core.BrandFeishu, "dc", 0, 60, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ClientID != "cli_x" || res.ClientSecret != "sec" {
		t.Errorf("got client_id=%q secret=%q, want cli_x/sec", res.ClientID, res.ClientSecret)
	}
	if res.UserInfo == nil || res.UserInfo.TenantBrand != "feishu" {
		t.Errorf("user info not parsed: %+v", res.UserInfo)
	}
}

func TestPollAppRegistration_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel up front
	client := &http.Client{Transport: stubRoundTripper{status: 200, body: `{"error":"authorization_pending"}`}}
	_, err := PollAppRegistration(ctx, client, core.BrandFeishu, "dc", 0, 60, io.Discard)
	if !errors.Is(err, ErrAppRegCancelled) {
		t.Fatalf("err = %v, want errors.Is(ErrAppRegCancelled)", err)
	}
}
