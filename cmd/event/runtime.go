// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
)

// consumeRuntime routes event.APIClient calls through the shared client.APIClient with a pinned identity.
type consumeRuntime struct {
	client         *client.APIClient
	accessIdentity core.Identity
}

func (r *consumeRuntime) CallAPI(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	resp, err := r.client.DoAPI(ctx, client.RawApiRequest{
		Method: method,
		URL:    path,
		Data:   body,
		As:     r.accessIdentity,
	})
	if err != nil {
		return nil, err
	}
	// Non-JSON HTTP errors (gateway text/plain 404 etc.) skip OAPI envelope parsing.
	ct := resp.Header.Get("Content-Type")
	if resp.StatusCode >= 400 && !client.IsJSONContentType(ct) && ct != "" {
		const maxBodyEcho = 256
		body := string(resp.RawBody)
		if len(body) > maxBodyEcho {
			body = body[:maxBodyEcho] + "…(truncated)"
		}
		return nil, fmt.Errorf("api %s %s returned %d: %s", method, path, resp.StatusCode, body)
	}
	result, err := client.ParseJSONResponse(resp)
	if err != nil {
		return nil, err
	}
	if apiErr := client.CheckLarkResponse(result); apiErr != nil {
		return json.RawMessage(resp.RawBody), apiErr
	}
	return json.RawMessage(resp.RawBody), nil
}
