// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signature

import (
	"encoding/json"
	"fmt"
	"net/url"

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

	data, err := runtime.CallAPI("GET", signaturesPath(mailboxID), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get signatures: %w", err)
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("get signatures: marshal response: %w", err)
	}

	var resp GetSignaturesResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("get signatures: unmarshal response: %w", err)
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
	return nil, fmt.Errorf("signature not found: %s", signatureID)
}

// Create creates a USER signature and clears the per-process list cache.
func Create(runtime *common.RuntimeContext, mailboxID string, sig Signature) (*Signature, error) {
	data, err := runtime.CallAPI("POST", signaturesPath(mailboxID), nil, WriteSignatureRequest{Signature: sig})
	if err != nil {
		return nil, fmt.Errorf("create signature: %w", err)
	}
	delete(processCache, mailboxID)
	return decodeWriteSignature("create signature", data)
}

// Update full-replaces a USER signature and clears the per-process list cache.
func Update(runtime *common.RuntimeContext, mailboxID, signatureID string, sig Signature) (*Signature, error) {
	data, err := runtime.CallAPI("PUT", signaturesPath(mailboxID)+"/"+url.PathEscape(signatureID), nil, WriteSignatureRequest{Signature: sig})
	if err != nil {
		return nil, fmt.Errorf("update signature: %w", err)
	}
	delete(processCache, mailboxID)
	return decodeWriteSignature("update signature", data)
}

// Delete deletes a USER signature and clears the per-process list cache.
func Delete(runtime *common.RuntimeContext, mailboxID, signatureID string) error {
	if _, err := runtime.CallAPI("DELETE", signaturesPath(mailboxID)+"/"+url.PathEscape(signatureID), nil, nil); err != nil {
		return fmt.Errorf("delete signature: %w", err)
	}
	delete(processCache, mailboxID)
	return nil
}

// ClearCache clears the per-process signature cache for a mailbox.
func ClearCache(mailboxID string) {
	delete(processCache, mailboxID)
}

func decodeWriteSignature(op string, data map[string]interface{}) (*Signature, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal response: %w", op, err)
	}
	var resp WriteSignatureResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("%s: unmarshal response: %w", op, err)
	}
	if resp.Signature.ID == "" {
		return nil, fmt.Errorf("%s: response signature.id is empty", op)
	}
	return &resp.Signature, nil
}
