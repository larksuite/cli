// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package mail registers Mail-domain EventKeys.
package mail

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/event"
	larkmail "github.com/larksuite/oapi-sdk-go/v3/service/mail/v1"
)

const (
	mailMessageReceivedKey = "mail.user_mailbox.event.message_received_v1"
	// Mail subscribe API takes event_type as an integer enum; 1 == new message received.
	mailSubscribeEventTypeNewMessage = 1
)

// Keys returns all Mail-domain EventKey definitions.
func Keys() []event.KeyDefinition {
	return []event.KeyDefinition{
		{
			Key:         mailMessageReceivedKey,
			DisplayName: "New mail received",
			Description: "Triggered when a new email arrives in the user's mailbox",
			EventType:   mailMessageReceivedKey,
			Schema: event.SchemaDef{
				Native: &event.SchemaSpec{Type: reflect.TypeOf(larkmail.P2UserMailboxEventMessageReceivedV1Data{})},
			},
			Params: []event.ParamDef{
				{
					Name:        "mailbox",
					Type:        event.ParamString,
					Required:    false,
					Default:     "me",
					Description: "Mailbox identifier (email address or 'me' for the current user)",
				},
			},
			Scopes:                []string{"mail:event", "mail:user_mailbox.event.mail_address:read"},
			AuthTypes:             []string{"user"},
			RequiredConsoleEvents: []string{mailMessageReceivedKey},
			PreConsume:            preConsumeMailSubscribe,
		},
	}
}

// preConsumeMailSubscribe opens the per-user mailbox event subscription before
// the consumer starts receiving events, and returns a cleanup that unsubscribes
// on graceful shutdown. The subscribe/unsubscribe APIs are idempotent on the
// server side keyed by (app, user, event_type).
func preConsumeMailSubscribe(ctx context.Context, rt event.APIClient, params map[string]string) (func(), error) {
	mailbox := strings.TrimSpace(params["mailbox"])
	if mailbox == "" {
		mailbox = "me"
	}

	body := map[string]interface{}{"event_type": mailSubscribeEventTypeNewMessage}
	if _, err := rt.CallAPI(ctx, "POST", mailboxEventPath(mailbox, "subscribe"), body); err != nil {
		return nil, fmt.Errorf("subscribe mailbox events failed for %q: %w", mailbox, err)
	}

	cleanup := func() {
		// Use a fresh context with a small budget: the parent ctx is already
		// cancelled when cleanup runs, but unsubscribe must still reach the
		// server to avoid leaking server-side subscriptions.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = rt.CallAPI(cleanupCtx, "POST", mailboxEventPath(mailbox, "unsubscribe"), body)
	}
	return cleanup, nil
}

// mailboxEventPath builds /open-apis/mail/v1/user_mailboxes/<mailbox>/event/<action>
// with each path segment URL-escaped to handle email addresses containing reserved chars.
func mailboxEventPath(mailbox, action string) string {
	return "/open-apis/mail/v1/user_mailboxes/" + url.PathEscape(mailbox) + "/event/" + url.PathEscape(action)
}
