// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/transport"
)

// DebugHeadersEnv, when set to a truthy value, makes DebugHeaderTransport dump
// the outbound request line and headers to stderr. It sits at the innermost
// position of the transport chain, so what it prints is what actually goes on
// the wire — after every other layer has contributed its headers.
const DebugHeadersEnv = "LARK_CLI_DEBUG_HEADERS"

// sensitiveHeaders are redacted in the dump; their presence is still reported.
var sensitiveHeaders = map[string]bool{
	"authorization":   true,
	"x-lark-mcp-uat":  true,
	"x-lark-mcp-tat":  true,
	"cookie":          true,
	"x-larksuite-key": true,
}

// DebugHeaderTransport dumps outbound request headers when DebugHeadersEnv is set.
type DebugHeaderTransport struct {
	Base http.RoundTripper
	Out  io.Writer // defaults to os.Stderr
}

func (t *DebugHeaderTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return transport.Fallback()
}

func (t *DebugHeaderTransport) out() io.Writer {
	if t.Out != nil {
		return t.Out
	}
	return os.Stderr
}

func debugHeadersEnabled() bool {
	v := strings.TrimSpace(os.Getenv(DebugHeadersEnv))
	return v != "" && v != "0"
}

// RoundTrip implements http.RoundTripper.
func (t *DebugHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !debugHeadersEnabled() {
		return t.base().RoundTrip(req)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[lark-cli] --> %s %s\n", req.Method, req.URL.String())

	keys := make([]string, 0, len(req.Header))
	for k := range req.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		val := strings.Join(req.Header[k], ", ")
		if sensitiveHeaders[strings.ToLower(k)] {
			val = fmt.Sprintf("<redacted, %d chars>", len(val))
		}
		fmt.Fprintf(&b, "[lark-cli]     %s: %s\n", k, val)
	}
	fmt.Fprint(t.out(), b.String())

	return t.base().RoundTrip(req)
}
