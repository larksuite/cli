// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/larksuite/cli/internal/event"
)

// APIClient aliases event.APIClient so the same concrete adapter the
// cmd layer constructs satisfies every consume-time HTTP call site.
type APIClient = event.APIClient

// CheckRemoteConnections calls GET /open-apis/event/v1/connection to learn
// how many WebSocket connections are currently active for this app globally.
// Returns the online instance count, or an error if the API call fails.
//
// Field verified via `lark-cli api GET /open-apis/event/v1/connection`:
//
//	{"code":0,"msg":"","data":{"online_instance_cnt":N}}
//
// Mismatching the tag name would silently decode to 0 and defeat the check.
func CheckRemoteConnections(ctx context.Context, client APIClient) (int, error) {
	raw, err := client.CallAPI(ctx, "GET", "/open-apis/event/v1/connection", nil)
	if err != nil {
		return 0, fmt.Errorf("connection check: %w", err)
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OnlineInstanceCnt int `json:"online_instance_cnt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return 0, fmt.Errorf("connection check: decode: %w (body=%s)", err, truncateForError(raw))
	}
	// A non-zero business code (auth failure, rate-limit, etc.) produces
	// no `data` payload and would decode to OnlineInstanceCnt=0 — which
	// callers interpret as "no remote buses". Surface the OAPI error
	// instead so the caller can distinguish "verified zero" from "check
	// failed and we don't actually know".
	if result.Code != 0 {
		return 0, fmt.Errorf("connection check: api error code=%d msg=%q", result.Code, result.Msg)
	}
	return result.Data.OnlineInstanceCnt, nil
}

// truncateForError bounds body length and strips control chars so a
// malformed OAPI response (or one reflecting auth headers) doesn't flood
// stderr and doesn't forge log lines via embedded newlines.
func truncateForError(b []byte) string {
	const max = 256
	s := string(b)
	if len(s) > max {
		s = s[:max] + "…(truncated)"
	}
	// Collapse control chars to spaces for log hygiene.
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			out = append(out, ' ')
			continue
		}
		out = append(out, string(r)...)
	}
	return string(out)
}
