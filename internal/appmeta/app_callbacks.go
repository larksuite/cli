// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package appmeta

import (
	"context"
	"encoding/json"
	"fmt"
)

// FetchSubscribedCallbacks returns the app's currently subscribed callback names
// from application/get. On a successful fetch it always returns a non-nil slice
// (empty when callback_info is absent or lists no callbacks) so callers can
// distinguish "fetched, zero callbacks subscribed" — a definitive console state
// that must fail the precheck — from a fetch error (nil), which is a
// weak-dependency skip. Identity must be bot: the endpoint is app-level.
func FetchSubscribedCallbacks(ctx context.Context, client APIClient, appID string) ([]string, error) {
	path := fmt.Sprintf("/open-apis/application/v6/applications/%s?lang=zh_cn", appID)
	raw, err := client.CallAPI(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Data struct {
			App struct {
				CallbackInfo *struct {
					SubscribedCallbacks []string `json:"subscribed_callbacks"`
				} `json:"callback_info"`
			} `json:"app"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode application response: %w", err)
	}
	// callback_info also carries callback_type (e.g. "websocket"); it is
	// intentionally not parsed or validated. Feishu open-platform callbacks are
	// delivered over WebSocket only (confirmed), matching the CLI's WebSocket
	// event source, so subscribed_callbacks alone is sufficient for the precheck.
	// Revisit and validate callback_type if non-WebSocket delivery ever appears.
	callbacks := []string{}
	if ci := envelope.Data.App.CallbackInfo; ci != nil {
		callbacks = append(callbacks, ci.SubscribedCallbacks...)
	}
	return callbacks, nil
}
