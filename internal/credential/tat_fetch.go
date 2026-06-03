// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/larksuite/cli/internal/core"
)

// FetchTAT performs a single HTTP POST to mint a tenant access token with the
// given credentials. It does not read configuration or keychain, so callers
// that already hold plaintext credentials (e.g. the post-`config init` probe)
// can validate them without a second keychain round-trip.
//
// A non-zero TAT response code means the server inspected the payload and
// rejected the credentials; FetchTAT returns the canonical typed error from
// classifyTATResponseCode — the SAME classification doResolveTAT (and thus
// every token-resolving command) produces, so callers see one consistent
// envelope (CategoryConfig / SubtypeInvalidClient for 10003 / 10014, etc.).
// Transport, HTTP-status and JSON-parse failures are returned raw (untyped),
// leaving them ambiguous; a caller can use errs.IsTyped to tell a deterministic
// credential rejection apart from upstream/transport noise.
//
// The caller owns the context timeout.
func FetchTAT(ctx context.Context, httpClient *http.Client, brand core.LarkBrand, appID, appSecret string) (string, error) {
	ep := core.ResolveEndpoints(brand)
	url := ep.Open + "/open-apis/auth/v3/tenant_access_token/internal"

	body, err := json.Marshal(map[string]string{
		"app_id":     appID,
		"app_secret": appSecret,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal TAT request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("TAT API returned HTTP %d", resp.StatusCode)
	}

	var result struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse TAT response: %w", err)
	}
	if result.Code != 0 {
		return "", classifyTATResponseCode(result.Code, result.Msg, string(brand), appID)
	}
	return result.TenantAccessToken, nil
}
