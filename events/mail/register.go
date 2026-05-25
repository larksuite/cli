// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/event"
)

const (
	mailMessageReceivedKey           = "mail.user_mailbox.event.message_received_v1"
	mailSubscribeEventTypeNewMessage = 1
)

// Keys returns all Mail-domain EventKey definitions.
func Keys() []event.KeyDefinition {
	return []event.KeyDefinition{
		{
			Key:         mailMessageReceivedKey,
			DisplayName: "New mail received",
			Description: "Triggered when a new email arrives in the user's mailbox.",
			EventType:   mailMessageReceivedKey,
			// Schema.Custom is required here (not Native) because processMailEvent
			// produces the complete MailReceivedPayload shape. Schema.Native is
			// incompatible with Process per registry validation.
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(MailReceivedPayload{})},
			},
			Params: []event.ParamDef{
				{
					Name:            "mailbox",
					Type:            event.ParamString,
					Default:         "me",
					SubscriptionKey: true,
					Description:     "Mailbox to subscribe to (email address or 'me'). Determines subscription identity — different mailboxes get independent server-side subscriptions and event streams.",
				},
				{
					Name:        "folders",
					Type:        event.ParamString,
					Description: "Filter: comma-separated folder IDs. Drop events whose mail is not in any of these folders. Triggers a metadata fetch per event.",
				},
				{
					Name:        "labels",
					Type:        event.ParamString,
					Description: "Filter: comma-separated label IDs. Drop events whose mail does not carry ALL of these labels.",
				},
				{
					Name:    "msg-format",
					Type:    event.ParamEnum,
					Default: "event",
					Values: []event.ParamValue{
						{Value: "event", Desc: "Raw event payload only (no API call). Fields: message_id, mail_address, mailbox_type, subscriber."},
						{Value: "metadata", Desc: "event fields + from, subject, snippet, folder_id, label_ids (1 GET per event)."},
						{Value: "plain_text_full", Desc: "metadata + body_text (1 GET per event with format=plain_text_full)."},
						{Value: "full", Desc: "metadata + body_text + body_html + attachments (1 GET per event with format=full)."},
					},
					Description: "Output enrichment level. See Output Schema field descriptions for which fields are populated at each level.",
				},
			},
			Scopes: []string{
				"mail:event",
				"mail:user_mailbox.event.mail_address:read",
				"mail:user_mailbox:readonly",
				"mail:user_mailbox.message:readonly",
			},
			AuthTypes:             []string{"user"},
			RequiredConsoleEvents: []string{mailMessageReceivedKey},
			NormalizeParams:       normalizeMailParams,
			PreConsume:            preConsumeMailSubscribe,
			Match:                 matchMailbox,
			Process:               processMailEvent,
		},
	}
}

// preConsumeMailSubscribe opens the per-user mailbox event subscription before
// the consumer starts receiving events, and returns a cleanup that unsubscribes
// on graceful shutdown. The subscribe/unsubscribe APIs are idempotent on the
// server side keyed by (app, user, event_type).
func preConsumeMailSubscribe(ctx context.Context, rt event.APIClient, params map[string]string) (func() error, error) {
	mailbox := strings.TrimSpace(params["mailbox"])
	if mailbox == "" {
		mailbox = "me"
	}
	body := map[string]interface{}{"event_type": mailSubscribeEventTypeNewMessage}
	if _, err := rt.CallAPI(ctx, "POST", mailboxEventPath(mailbox, "subscribe"), body); err != nil {
		return nil, fmt.Errorf("subscribe mailbox events failed for %q: %w", mailbox, err)
	}
	cleanup := func() error {
		// Fresh context: the parent ctx is already cancelled when cleanup runs,
		// but unsubscribe must still reach the server. Budget is small (10s)
		// to keep graceful shutdown snappy on flaky networks.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := rt.CallAPI(cleanupCtx, "POST", mailboxEventPath(mailbox, "unsubscribe"), body); err != nil {
			return fmt.Errorf("unsubscribe mailbox=%s: %w", mailbox, err)
		}
		return nil
	}
	return cleanup, nil
}

// mailboxEventPath builds /open-apis/mail/v1/user_mailboxes/<mailbox>/event/<action>
// with each path segment URL-escaped to handle email addresses containing reserved chars.
func mailboxEventPath(mailbox, action string) string {
	return "/open-apis/mail/v1/user_mailboxes/" + url.PathEscape(mailbox) + "/event/" + url.PathEscape(action)
}
