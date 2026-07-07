// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
)

const cleanupTimeout = 5 * time.Second

func mailSubscriptionPreConsume(eventType string) func(context.Context, event.APIClient, map[string]string) (func() error, error) {
	return func(ctx context.Context, rt event.APIClient, params map[string]string) (func() error, error) {
		if rt == nil {
			return nil, errs.NewInternalError(errs.SubtypeUnknown,
				"runtime API client is required for pre-consume subscription")
		}
		mailbox := normalizedMailbox(params)
		if mailbox == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"param mailbox is required for %s", eventType).
				WithParam("--param").
				WithHint("pass it as --param mailbox=me or --param mailbox=<email>; run `lark-cli event schema %s` for details", eventType)
		}

		body := map[string]int{"event_type": 1}
		subscribePath := mailboxPath(mailbox, "event", "subscribe")
		unsubscribePath := mailboxPath(mailbox, "event", "unsubscribe")
		if _, err := rt.CallAPI(ctx, "POST", subscribePath, body); err != nil {
			return nil, err
		}

		return func() error {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
			defer cancel()
			if _, err := rt.CallAPI(cleanupCtx, "POST", unsubscribePath, body); err != nil {
				return err
			}
			return nil
		}, nil
	}
}
