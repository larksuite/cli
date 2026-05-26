// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/larksuite/cli/internal/event"
)

const mailEventType = "mail.user_mailbox.event.message_received_v1"

// MailMessageReceivedOutput is the flat shape; `desc` tags drive the reflected schema.
type MailMessageReceivedOutput struct {
	Type        string `json:"type"                   desc:"Event type; always mail.user_mailbox.event.message_received_v1"`
	EventID     string `json:"event_id,omitempty"     desc:"Globally unique event ID; safe for deduplication"`
	Timestamp   string `json:"timestamp,omitempty"    desc:"Event delivery time (ms timestamp string)"  kind:"timestamp_ms"`
	Mailbox     string `json:"mailbox,omitempty"      desc:"Mailbox address that received this message"  kind:"email"`
	MessageID   string `json:"message_id,omitempty"   desc:"Message ID (mail.open.access scoped)"`
	Sender      string `json:"sender,omitempty"       desc:"Sender email address"  kind:"email"`
	Subject     string `json:"subject,omitempty"      desc:"Message subject"`
	BodyExcerpt string `json:"body_excerpt,omitempty" desc:"Body excerpt (first ~140 chars, plain text)"`
}

func processMailMessageReceived(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	var envelope struct {
		Header struct {
			EventID    string `json:"event_id"`
			EventType  string `json:"event_type"`
			CreateTime string `json:"create_time"`
		} `json:"header"`
		Event struct {
			MailAddress string `json:"mail_address"`
			MessageID   string `json:"message_id"`
			Sender      string `json:"sender"`
			Subject     string `json:"subject"`
			Body        string `json:"body"`
		} `json:"event"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return raw.Payload, nil //nolint:nilerr // passthrough on malformed payload
	}
	body := envelope.Event.Body
	if len(body) > 140 {
		body = body[:140]
	}
	return json.Marshal(&MailMessageReceivedOutput{
		Type:        envelope.Header.EventType,
		EventID:     envelope.Header.EventID,
		Timestamp:   envelope.Header.CreateTime,
		Mailbox:     envelope.Event.MailAddress,
		MessageID:   envelope.Event.MessageID,
		Sender:      envelope.Event.Sender,
		Subject:     envelope.Event.Subject,
		BodyExcerpt: body,
	})
}

// parseMailboxes reads comma-separated `mailbox` param, trims whitespace, drops empties,
// dedupes preserving first-seen order, and defaults to []string{"me"} when empty.
// Order matters: PreConsume subscribes sequentially and rolls back in reverse.
func parseMailboxes(raw string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, mb := range strings.Split(raw, ",") {
		mb = strings.TrimSpace(mb)
		if mb == "" {
			continue
		}
		if _, dup := seen[mb]; dup {
			continue
		}
		seen[mb] = struct{}{}
		out = append(out, mb)
	}
	if len(out) == 0 {
		return []string{"me"}
	}
	return out
}

// mailMessageReceivedPreConsume runs once per (appID, EventKey) on the FirstForKey
// consumer (consume.Run:86-95). It sequentially calls the mailbox business
// subscribe API for each parsed mailbox; on any failure it rolls back the
// already-subscribed mailboxes in reverse order (best-effort) and returns the
// wrapped error. On full success it returns a cleanup closure that consume.Run
// will invoke on lastForKey exit (or unconditionally on panic).
func mailMessageReceivedPreConsume(ctx context.Context, rt event.APIClient, params map[string]string) (func(), error) {
	mailboxes := parseMailboxes(params["mailbox"])
	var subscribed []string
	for _, mb := range mailboxes {
		if _, err := rt.CallAPI(ctx, "POST",
			"/open-apis/mail/v1/user_mailboxes/"+url.PathEscape(mb)+"/event/subscribe",
			map[string]interface{}{"event_type": 1}); err != nil {
			for i := len(subscribed) - 1; i >= 0; i-- {
				_, _ = rt.CallAPI(ctx, "POST",
					"/open-apis/mail/v1/user_mailboxes/"+url.PathEscape(subscribed[i])+"/event/unsubscribe",
					map[string]interface{}{"event_type": 1})
			}
			return nil, fmt.Errorf("mail event subscribe failed for %s: %w "+
				"(hint: ensure (1) you are logged in as user with required mail scopes, "+
				"(2) the app has subscribed to %s in the developer console, "+
				"(3) the user has access to the target mailbox)",
				mb, err, mailEventType)
		}
		subscribed = append(subscribed, mb)
	}
	cleanup := func() {
		for i := len(subscribed) - 1; i >= 0; i-- {
			_, _ = rt.CallAPI(ctx, "POST",
				"/open-apis/mail/v1/user_mailboxes/"+url.PathEscape(subscribed[i])+"/event/unsubscribe",
				map[string]interface{}{"event_type": 1})
		}
	}
	return cleanup, nil
}
