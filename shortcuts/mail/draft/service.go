// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

func mailboxPath(mailboxID string, segments ...string) string {
	parts := make([]string, 0, len(segments)+1)
	parts = append(parts, url.PathEscape(mailboxID))
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		parts = append(parts, url.PathEscape(seg))
	}
	return "/open-apis/mail/v1/user_mailboxes/" + strings.Join(parts, "/")
}

func GetRaw(runtime *common.RuntimeContext, mailboxID, draftID string) (DraftRaw, error) {
	data, err := runtime.CallAPI("GET", mailboxPath(mailboxID, "drafts", draftID), map[string]interface{}{"format": "raw"}, nil)
	if err != nil {
		return DraftRaw{}, err
	}
	raw := extractRawEML(data)
	if raw == "" {
		return DraftRaw{}, fmt.Errorf("API response missing draft raw EML; the backend returned an empty raw body for this draft")
	}
	gotDraftID := extractDraftID(data)
	if gotDraftID == "" {
		gotDraftID = draftID
	}
	return DraftRaw{
		DraftID: gotDraftID,
		RawEML:  raw,
	}, nil
}

func CreateWithRaw(runtime *common.RuntimeContext, mailboxID, rawEML string) (DraftResult, error) {
	data, err := runtime.CallAPI("POST", mailboxPath(mailboxID, "drafts"), nil, map[string]interface{}{"raw": rawEML})
	if err != nil {
		return DraftResult{}, err
	}
	draftID := extractDraftID(data)
	if draftID == "" {
		return DraftResult{}, fmt.Errorf("API response missing draft_id")
	}
	return DraftResult{
		DraftID:   draftID,
		Reference: extractReference(data),
	}, nil
}

func UpdateWithRaw(runtime *common.RuntimeContext, mailboxID, draftID, rawEML string) (DraftResult, error) {
	data, err := runtime.CallAPI("PUT", mailboxPath(mailboxID, "drafts", draftID), nil, map[string]interface{}{"raw": rawEML})
	if err != nil {
		return DraftResult{}, err
	}
	gotDraftID := extractDraftID(data)
	if gotDraftID == "" {
		gotDraftID = draftID
	}
	return DraftResult{
		DraftID:   gotDraftID,
		Reference: extractReference(data),
	}, nil
}

func Send(runtime *common.RuntimeContext, mailboxID, draftID string) (map[string]interface{}, error) {
	return runtime.CallAPI("POST", mailboxPath(mailboxID, "drafts", draftID, "send"), nil, nil)
}

func extractDraftID(data map[string]interface{}) string {
	if id, ok := data["draft_id"].(string); ok && strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	if id, ok := data["id"].(string); ok && strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	if draft, ok := data["draft"].(map[string]interface{}); ok {
		return extractDraftID(draft)
	}
	return ""
}

func extractRawEML(data map[string]interface{}) string {
	if raw, ok := data["raw"].(string); ok && strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw)
	}
	if msg, ok := data["message"].(map[string]interface{}); ok {
		if raw, ok := msg["raw"].(string); ok && strings.TrimSpace(raw) != "" {
			return strings.TrimSpace(raw)
		}
	}
	if draft, ok := data["draft"].(map[string]interface{}); ok {
		return extractRawEML(draft)
	}
	return ""
}

func extractReference(data map[string]interface{}) string {
	if data == nil {
		return ""
	}
	if ref, ok := data["reference"].(string); ok && strings.TrimSpace(ref) != "" {
		return strings.TrimSpace(ref)
	}
	if draft, ok := data["draft"].(map[string]interface{}); ok {
		return extractReference(draft)
	}
	return ""
}
