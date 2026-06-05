// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signature

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// processCache holds per-mailbox cached responses.
// CLI runs one command per process, so a package-level map is sufficient —
// it is naturally scoped to a single Execute lifecycle.
var processCache = map[string]*GetSignaturesResponse{}

func signaturesPath(mailboxID string) string {
	return "/open-apis/mail/v1/user_mailboxes/" + url.PathEscape(mailboxID) + "/settings/signatures"
}

// ListAll fetches all signatures and usage info for a mailbox.
// Results are cached per mailboxID within the current Execute lifecycle.
func ListAll(runtime *common.RuntimeContext, mailboxID string) (*GetSignaturesResponse, error) {
	if cached, ok := processCache[mailboxID]; ok {
		return cached, nil
	}

	data, err := runtime.CallAPITyped("GET", signaturesPath(mailboxID), nil, nil)
	if err != nil {
		return nil, err
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeSDKError, "get signatures: marshal response: %v", err).WithCause(err)
	}

	var resp GetSignaturesResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "get signatures: unmarshal response: %v", err).WithCause(err)
	}

	processCache[mailboxID] = &resp
	return &resp, nil
}

// List returns all signatures for a mailbox.
func List(runtime *common.RuntimeContext, mailboxID string) ([]Signature, error) {
	resp, err := ListAll(runtime, mailboxID)
	if err != nil {
		return nil, err
	}
	return resp.Signatures, nil
}

// Get returns a single signature by ID. Returns an error if not found.
func Get(runtime *common.RuntimeContext, mailboxID, signatureID string) (*Signature, error) {
	resp, err := ListAll(runtime, mailboxID)
	if err != nil {
		return nil, err
	}
	for i := range resp.Signatures {
		if resp.Signatures[i].ID == signatureID {
			return &resp.Signatures[i], nil
		}
	}
	return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "signature not found: %s", signatureID)
}

// DefaultSendID returns the send_mail_signature_id for the given addr.
// Falls back to usages[0] if no entry matches, but returns "" when
// no default is configured (id empty or "0").
// Returns "" if usages is empty.
func DefaultSendID(usages []SignatureUsage, addr string) string {
	return pickSignatureID(usages, addr, func(u SignatureUsage) string {
		return u.SendMailSignatureID
	})
}

// DefaultReplyID returns the reply_signature_id for the given addr.
// Used by reply/reply-all/forward shortcuts.
// Returns "" if usages is empty or no default is configured.
func DefaultReplyID(usages []SignatureUsage, addr string) string {
	return pickSignatureID(usages, addr, func(u SignatureUsage) string {
		return u.ReplySignatureID
	})
}

func pickSignatureID(usages []SignatureUsage, addr string, pick func(SignatureUsage) string) string {
	if len(usages) == 0 {
		return ""
	}
	laddr := strings.ToLower(strings.TrimSpace(addr))
	for _, u := range usages {
		if strings.ToLower(strings.TrimSpace(u.EmailAddress)) == laddr {
			id := pick(u)
			if id == "" || id == "0" {
				return ""
			}
			return id
		}
	}
	return ""
}
