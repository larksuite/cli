// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"net/http"
	"os"
	"strings"

	"github.com/larksuite/cli/internal/transport"
)

// ExtraHeadersEnv is the environment variable carrying extra request headers,
// formatted as "Key: Value" pairs separated by ";"
// (e.g. "X-TT-ENV: boe_lane; X-Other: 1"). Primarily used to select a
// boe/pre swimlane via X-TT-ENV.
const ExtraHeadersEnv = "LARKSUITE_CLI_EXTRA_HEADERS"

// parseExtraHeaders parses an ExtraHeadersEnv value into a header map.
// Entries without a colon are skipped.
func parseExtraHeaders(raw string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(raw, ";") {
		i := strings.Index(part, ":")
		if i < 0 {
			continue
		}
		k := strings.TrimSpace(part[:i])
		v := strings.TrimSpace(part[i+1:])
		if k != "" {
			out[k] = v
		}
	}
	return out
}

// EnvHeaderTransport is an http.RoundTripper that injects headers declared in
// ExtraHeadersEnv into every request. It is a no-op when the variable is unset.
type EnvHeaderTransport struct {
	Base http.RoundTripper
}

func (t *EnvHeaderTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return transport.Fallback()
}

// RoundTrip implements http.RoundTripper.
func (t *EnvHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	headers := parseExtraHeaders(os.Getenv(ExtraHeadersEnv))
	if len(headers) == 0 {
		return t.base().RoundTrip(req)
	}
	req = req.Clone(req.Context())
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return t.base().RoundTrip(req)
}
