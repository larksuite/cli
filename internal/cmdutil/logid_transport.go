// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/larksuite/cli/internal/transport"
)

// ShowLogIDEnv, when set to a truthy value (any non-empty non-"0" string),
// causes LogIDTransport to print the x-tt-logid header for every response.
// When unset, the logid is only printed for HTTP error responses (status >= 400).
const ShowLogIDEnv = "LARK_CLI_SHOW_LOGID"

// LogIDTransport is an http.RoundTripper that surfaces the x-tt-logid response
// header to stderr so users can correlate CLI failures with backend logs.
type LogIDTransport struct {
	Base http.RoundTripper
	Out  io.Writer // defaults to os.Stderr
}

func (t *LogIDTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return transport.Fallback()
}

func (t *LogIDTransport) out() io.Writer {
	if t.Out != nil {
		return t.Out
	}
	return os.Stderr
}

// showAllLogIDs reports whether every response's logid should be printed.
func showAllLogIDs() bool {
	v := strings.TrimSpace(os.Getenv(ShowLogIDEnv))
	return v != "" && v != "0"
}

// RoundTrip implements http.RoundTripper.
func (t *LogIDTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base().RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}
	logID := resp.Header.Get("x-tt-logid")
	if logID == "" {
		return resp, nil
	}
	if showAllLogIDs() || resp.StatusCode >= 400 {
		fmt.Fprintf(t.out(), "[lark-cli] x-tt-logid=%s status=%d path=%s\n", logID, resp.StatusCode, req.URL.Path)
	}
	return resp, nil
}
