// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package subscribeprep provides the shared PreConsume hook for EventKeys
// whose server-side subscription is a plain event_type register/unregister
// pair against fixed OAPI paths.
package subscribeprep

import (
	"context"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event/processing"
)

// CleanupTimeout bounds how long the unsubscribe call has to finish during
// PreConsume cleanup so a stuck OAPI cannot block process shutdown.
const CleanupTimeout = 5 * time.Second

// Hook returns a PreConsume that subscribes eventType via subscribePath and
// hands back a cleanup that unsubscribes it via unsubscribePath.
func Hook(eventType, subscribePath, unsubscribePath string) func(context.Context, processing.APIClient, map[string]string) (func() error, error) {
	return func(ctx context.Context, rt processing.APIClient, _ map[string]string) (func() error, error) {
		if rt == nil {
			return nil, errs.NewInternalError(errs.SubtypeUnknown,
				"runtime API client is required for pre-consume subscription")
		}
		return SubscribeWithCleanup(ctx, rt, eventType, subscribePath, unsubscribePath)
	}
}

// SubscribeWithCleanup calls the subscribe OAPI for eventType and returns a
// cleanup that invokes the matching unsubscribe, bounded by CleanupTimeout.
// rt must be non-nil; callers that validate their own params (e.g. to build
// per-resource paths) run those checks first and then delegate here.
func SubscribeWithCleanup(ctx context.Context, rt processing.APIClient, eventType, subscribePath, unsubscribePath string) (func() error, error) {
	body := map[string]string{"event_type": eventType}
	if _, err := rt.CallAPI(ctx, "POST", subscribePath, body); err != nil {
		return nil, err
	}

	return func() error {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), CleanupTimeout)
		defer cancel()
		if _, err := rt.CallAPI(cleanupCtx, "POST", unsubscribePath, body); err != nil {
			return err
		}
		return nil
	}, nil
}
