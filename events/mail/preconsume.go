// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
	shortmail "github.com/larksuite/cli/shortcuts/mail"
)

const cleanupTimeout = 5 * time.Second

func mailboxEventPreConsume(_ string) func(context.Context, event.APIClient, map[string]string) (func() error, error) {
	return func(ctx context.Context, rt event.APIClient, params map[string]string) (func() error, error) {
		if rt == nil {
			return nil, errs.NewInternalError(errs.SubtypeUnknown,
				"runtime API client is required for mail event subscription")
		}
		mailbox := params["mailbox_api"]
		if mailbox == "" {
			mailbox = params["mailbox"]
		}
		if mailbox == "" {
			mailbox = "me"
		}

		body := map[string]interface{}{"event_type": 1}
		if _, err := rt.CallAPI(ctx, "POST", shortmail.MailboxPath(mailbox, "event", "subscribe"), body); err != nil {
			return nil, shortmail.WrapWatchSubscribeError(err)
		}

		return func() error {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
			defer cancel()
			_, err := rt.CallAPI(cleanupCtx, "POST", shortmail.MailboxPath(mailbox, "event", "unsubscribe"), body)
			return err
		}, nil
	}
}
