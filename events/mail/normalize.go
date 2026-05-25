// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/event"
)

// normalizeMailParams resolves the mailbox alias "me" (or empty) into the
// user's real primary email so fingerprint / Match / Process all see the
// canonical value.
//
// API: GET /open-apis/mail/v1/user_mailboxes/me/profile — returns
// {data:{primary_email_address:"..."}}. Mirrors the same OAPI call that
// shortcuts/mail/helpers.go::fetchMailboxPrimaryEmail uses, so both code
// paths (event consume and mail +watch) resolve "me" identically.
func normalizeMailParams(ctx context.Context, rt event.APIClient, params map[string]string) error {
	mbox := strings.TrimSpace(params["mailbox"])
	if mbox == "" || mbox == "me" {
		data, err := rt.CallAPI(ctx, "GET", "/open-apis/mail/v1/user_mailboxes/me/profile", nil)
		if err != nil {
			return fmt.Errorf("resolve mailbox 'me': %w", err)
		}
		email, err := extractPrimaryEmail(data)
		if err != nil {
			return fmt.Errorf("decode user_mailboxes/me/profile response: %w", err)
		}
		if email == "" {
			return fmt.Errorf("user_mailboxes/me/profile returned empty primary_email_address")
		}
		params["mailbox"] = email
		return nil
	}
	params["mailbox"] = mbox
	return nil
}

// extractPrimaryEmail pulls primary_email_address out of the profile response.
// Tolerates both top-level shape (test fixtures) and the canonical nested
// `data` wrapper used by production responses.
func extractPrimaryEmail(raw json.RawMessage) (string, error) {
	var asTop struct {
		PrimaryEmailAddress string `json:"primary_email_address"`
		Data                struct {
			PrimaryEmailAddress string `json:"primary_email_address"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &asTop); err != nil {
		return "", err
	}
	if asTop.PrimaryEmailAddress != "" {
		return asTop.PrimaryEmailAddress, nil
	}
	return asTop.Data.PrimaryEmailAddress, nil
}
