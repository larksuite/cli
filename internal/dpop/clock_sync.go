// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dpop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

const HeartbeatPath = "/accounts/status/heartbeat"

type heartbeatResponse struct {
	Code int `json:"code"`
	Data struct {
		Now string `json:"now"`
	} `json:"data"`
}

// SynchronizeClock obtains the Accounts server time before a DPoP token
// request. The calibrated clock is kept on the key; callers decide when the
// corresponding key metadata becomes part of their token transaction.
func SynchronizeClock(ctx context.Context, httpClient *http.Client, brand core.LarkBrand, key *Key) error {
	if key == nil || key.Clock() == nil {
		return clockSyncError(errors.New("DPoP key clock is unavailable"))
	}
	if httpClient == nil {
		return clockSyncError(errors.New("HTTP client is unavailable"))
	}
	endpoint := strings.TrimRight(core.ResolveEndpoints(brand).Accounts, "/") + HeartbeatPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return clockSyncError(err)
	}
	resp, err := httpClient.Do(req)
	localReceiveTime := time.Now()
	if err != nil {
		return clockSyncError(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return clockSyncError(err)
	}
	var result heartbeatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return clockSyncError(fmt.Errorf("decode heartbeat response: %w", err))
	}
	if resp.StatusCode != http.StatusOK || result.Code != 0 {
		return clockSyncError(fmt.Errorf("heartbeat returned HTTP %d code %d", resp.StatusCode, result.Code))
	}
	serverUnix, err := strconv.ParseInt(result.Data.Now, 10, 64)
	if err != nil || serverUnix <= 0 {
		return clockSyncError(fmt.Errorf("heartbeat returned invalid server time %q", result.Data.Now))
	}
	key.Clock().SetServerTime(time.Unix(serverUnix, 0), localReceiveTime)
	return nil
}

func clockSyncError(cause error) error {
	return errs.NewAuthenticationError(errs.SubtypeDPoPClockSyncFailed,
		"failed to synchronize DPoP clock: %v", cause).
		WithCause(cause).
		WithHint("check connectivity to the official Accounts service and verify the local system clock")
}
