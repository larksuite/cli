// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// logidRT returns a response carrying the given status and x-tt-logid header.
type logidRT struct {
	status int
	logID  string
}

func (r *logidRT) RoundTrip(req *http.Request) (*http.Response, error) {
	h := http.Header{}
	if r.logID != "" {
		h.Set("x-tt-logid", r.logID)
	}
	return &http.Response{StatusCode: r.status, Header: h, Body: http.NoBody, Request: req}, nil
}

func runLogID(t *testing.T, status, showEnv string, logID string) string {
	t.Helper()
	t.Setenv(ShowLogIDEnv, showEnv)

	var buf bytes.Buffer
	code := 200
	if status == "500" {
		code = 500
	}
	rt := &LogIDTransport{Base: &logidRT{status: code, logID: logID}, Out: &buf}
	req := httptest.NewRequest("GET", "https://open.feishu-boe.cn/open-apis/x", nil)
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	return buf.String()
}

func TestLogIDTransport_PrintsOnErrorByDefault(t *testing.T) {
	out := runLogID(t, "500", "", "abc123")
	if !strings.Contains(out, "x-tt-logid=abc123") || !strings.Contains(out, "status=500") {
		t.Errorf("output = %q, want it to carry logid and status", out)
	}
}

func TestLogIDTransport_SilentOnSuccessByDefault(t *testing.T) {
	if out := runLogID(t, "200", "", "abc123"); out != "" {
		t.Errorf("output = %q, want empty", out)
	}
}

func TestLogIDTransport_PrintsAllWhenEnabled(t *testing.T) {
	if out := runLogID(t, "200", "1", "abc123"); !strings.Contains(out, "x-tt-logid=abc123") {
		t.Errorf("output = %q, want logid printed", out)
	}
}

func TestLogIDTransport_ZeroIsFalsy(t *testing.T) {
	if out := runLogID(t, "200", "0", "abc123"); out != "" {
		t.Errorf("output = %q, want empty for LARK_CLI_SHOW_LOGID=0", out)
	}
}

func TestLogIDTransport_NoHeaderNoOutput(t *testing.T) {
	if out := runLogID(t, "500", "1", ""); out != "" {
		t.Errorf("output = %q, want empty when the response has no logid", out)
	}
}
