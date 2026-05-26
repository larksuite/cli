// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"fmt"
	"time"

	"github.com/larksuite/cli/internal/event"
)

const cleanupTimeout = 5 * time.Second

func subscriptionPreConsume(eventType, subscribePath, unsubscribePath string) func(context.Context, event.APIClient, map[string]string) (func(), error) {
	return func(ctx context.Context, rt event.APIClient, _ map[string]string) (func(), error) {
		if rt == nil {
			return nil, fmt.Errorf("runtime API client is required for pre-consume subscription")
		}

		body := map[string]string{"event_type": eventType}
		if _, err := rt.CallAPI(ctx, "POST", subscribePath, body); err != nil {
			return nil, err
		}

		return func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
			defer cancel()
			_, _ = rt.CallAPI(cleanupCtx, "POST", unsubscribePath, body)
		}, nil
	}
}
