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

// processMailEvent enriches and optionally drops a mail event based on params:
//   - msg-format: determines fetch depth (event/metadata/plain_text_full/full)
//   - folders, labels: drop events whose mail metadata doesn't match
//
// Returns (nil, nil) to signal the framework "drop this event".
// Returns (nil, err) on transport / parsing errors that should bubble up.
func processMailEvent(ctx context.Context, rt event.APIClient, raw *event.RawEvent, params map[string]string) (json.RawMessage, error) {
	envFields, err := extractEventFields(raw.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode mail event envelope: %w", err)
	}

	payload := MailReceivedPayload{
		MessageID:   envFields.MessageID,
		MailAddress: envFields.MailAddress,
		MailboxType: envFields.MailboxType,
		Subscriber:  envFields.Subscriber,
	}

	msgFormat := params["msg-format"]
	if msgFormat == "" {
		msgFormat = "event"
	}

	folders := splitCSV(params["folders"])
	labels := splitCSV(params["labels"])
	needFetch := msgFormat != "event" || len(folders) > 0 || len(labels) > 0

	if !needFetch {
		return json.Marshal(payload)
	}

	fetchFormat := "metadata"
	if msgFormat == "plain_text_full" {
		fetchFormat = "plain_text_full"
	} else if msgFormat == "full" {
		fetchFormat = "full"
	}
	mailbox := params["mailbox"]
	if mailbox == "" {
		return nil, fmt.Errorf("mailbox param required for fetch (msg-format=%s)", msgFormat)
	}

	path := fmt.Sprintf("/open-apis/mail/v1/user_mailboxes/%s/messages/%s?format=%s",
		url.PathEscape(mailbox), url.PathEscape(envFields.MessageID), fetchFormat)
	data, err := rt.CallAPI(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch mail message %s: %w", envFields.MessageID, err)
	}

	var fetched struct {
		Data struct {
			From        string           `json:"from"`
			Subject     string           `json:"subject"`
			Snippet     string           `json:"snippet"`
			FolderID    string           `json:"folder_id"`
			LabelIDs    []string         `json:"label_ids"`
			BodyText    string           `json:"body_text"`
			BodyHTML    string           `json:"body_html"`
			Attachments []MailAttachment `json:"attachments"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &fetched); err != nil {
		return nil, fmt.Errorf("decode fetched mail %s: %w", envFields.MessageID, err)
	}

	// Filter: folders
	if len(folders) > 0 && !contains(folders, fetched.Data.FolderID) {
		return nil, nil
	}
	// Filter: labels (event must have ALL of the requested labels)
	if len(labels) > 0 && !allIn(fetched.Data.LabelIDs, labels) {
		return nil, nil
	}

	// Enrich payload based on msg-format
	if msgFormat == "metadata" || msgFormat == "plain_text_full" || msgFormat == "full" {
		payload.From = fetched.Data.From
		payload.Subject = fetched.Data.Subject
		payload.Snippet = fetched.Data.Snippet
		payload.FolderID = fetched.Data.FolderID
		payload.LabelIDs = fetched.Data.LabelIDs
	}
	if msgFormat == "plain_text_full" || msgFormat == "full" {
		payload.BodyText = fetched.Data.BodyText
	}
	if msgFormat == "full" {
		payload.BodyHTML = fetched.Data.BodyHTML
		payload.Attachments = fetched.Data.Attachments
	}

	return json.Marshal(payload)
}

type eventEnvelopeFields struct {
	MessageID   string
	MailAddress string
	MailboxType int
	Subscriber  Subscriber
}

func extractEventFields(rawPayload json.RawMessage) (eventEnvelopeFields, error) {
	var env struct {
		Event struct {
			MessageID   string     `json:"message_id"`
			MailAddress string     `json:"mail_address"`
			MailboxType int        `json:"mailbox_type"`
			Subscriber  Subscriber `json:"subscriber"`
		} `json:"event"`
	}
	if err := json.Unmarshal(rawPayload, &env); err != nil {
		return eventEnvelopeFields{}, err
	}
	return eventEnvelopeFields{
		MessageID:   env.Event.MessageID,
		MailAddress: env.Event.MailAddress,
		MailboxType: env.Event.MailboxType,
		Subscriber:  env.Event.Subscriber,
	}, nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func allIn(haystack, needles []string) bool {
	for _, n := range needles {
		if !contains(haystack, n) {
			return false
		}
	}
	return true
}
