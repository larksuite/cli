// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/larksuite/cli/internal/event"
)

type APIClient = event.APIClient

// CheckRemoteConnections returns the count of active WebSocket connections for this app.
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
	// Distinguish "verified zero" from "check failed" — non-zero code decodes Cnt=0.
	if result.Code != 0 {
		return 0, fmt.Errorf("connection check: api error code=%d msg=%q", result.Code, result.Msg)
	}
	return result.Data.OnlineInstanceCnt, nil
}

// truncateForError bounds length and collapses control chars to defang log injection.
func truncateForError(b []byte) string {
	const max = 256
	s := string(b)
	if len(s) > max {
		s = s[:max] + "…(truncated)"
	}
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
