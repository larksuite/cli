// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signature

import (
	"encoding/json"
	"net/url"

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
