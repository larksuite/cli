// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// captureRT records the request it saw and returns a canned 200.
type captureRT struct{ got *http.Request }

func (c *captureRT) RoundTrip(req *http.Request) (*http.Response, error) {
	c.got = req
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: http.NoBody, Request: req}, nil
}

func TestParseExtraHeaders(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"single", "X-TT-ENV: boe_lane", map[string]string{"X-TT-ENV": "boe_lane"}},
		{"multi", "X-TT-ENV: lane; X-Other:1", map[string]string{"X-TT-ENV": "lane", "X-Other": "1"}},
		{"skips entries without colon", "bogus; X-A: 1", map[string]string{"X-A": "1"}},
		{"skips blank key", ": v; X-A: 1", map[string]string{"X-A": "1"}},
		{"allows empty value", "X-A:", map[string]string{"X-A": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseExtraHeaders(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("parseExtraHeaders(%q) = %v, want %v", tt.raw, got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("header %q = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestEnvHeaderTransport_InjectsHeaders(t *testing.T) {
	t.Setenv(ExtraHeadersEnv, "X-TT-ENV: my_lane")

	cap := &captureRT{}
	rt := &EnvHeaderTransport{Base: cap}
	req := httptest.NewRequest("GET", "https://open.feishu-boe.cn/open-apis/x", nil)

	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if got := cap.got.Header.Get("X-TT-ENV"); got != "my_lane" {
		t.Errorf("X-TT-ENV = %q, want my_lane", got)
	}
	// The original request must not be mutated.
	if req.Header.Get("X-TT-ENV") != "" {
		t.Error("original request was mutated")
	}
}

func TestEnvHeaderTransport_NoopWhenUnset(t *testing.T) {
	t.Setenv(ExtraHeadersEnv, "")

	cap := &captureRT{}
	rt := &EnvHeaderTransport{Base: cap}
	req := httptest.NewRequest("GET", "https://open.feishu.cn/open-apis/x", nil)

	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	if cap.got != req {
		t.Error("request was cloned even though no headers were configured")
	}
}
