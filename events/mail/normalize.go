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
// API: GET /open-apis/mail/v1/user_mailboxes/me — returns {data:{email:"..."}}
// Verified via: lark-cli api GET /open-apis/mail/v1/user_mailboxes/me --as user
func normalizeMailParams(ctx context.Context, rt event.APIClient, params map[string]string) error {
	mbox := strings.TrimSpace(params["mailbox"])
	if mbox == "" || mbox == "me" {
		data, err := rt.CallAPI(ctx, "GET", "/open-apis/mail/v1/user_mailboxes/me", nil)
		if err != nil {
			return fmt.Errorf("resolve mailbox 'me': %w", err)
		}
		var parsed struct {
			Data struct {
				Email string `json:"email"`
			} `json:"data"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return fmt.Errorf("decode user_mailboxes/me response: %w", err)
		}
		if parsed.Data.Email == "" {
			return fmt.Errorf("user_mailboxes/me returned empty email")
		}
		params["mailbox"] = parsed.Data.Email
		return nil
	}
	params["mailbox"] = mbox
	return nil
}
