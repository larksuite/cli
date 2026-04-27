// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// mailboxPath joins mailboxID and the given segments under the
// /open-apis/mail/v1/user_mailboxes/ root, URL-escaping each component.
// Empty segments are skipped.
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

// GetRaw fetches the raw EML of a draft via drafts.get(format=raw) and
// returns the draft ID alongside the EML. If the backend response omits
// draft_id, the input draftID is echoed back so callers always have a
// non-empty identifier to round-trip.
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

// CreateWithRaw creates a draft in mailboxID from a pre-built base64url-encoded
// EML payload and returns the server-assigned draft ID along with the
// optional preview reference URL. Use this when the caller has already
// assembled the EML with emlbuilder; for high-level compose paths use the
// MailDraftCreate shortcut instead.
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

// UpdateWithRaw overwrites an existing draft's content with a pre-built
// base64url-encoded EML. Existing headers / body / attachments in the draft
// are replaced wholesale; callers that want to patch individual parts should
// use draftpkg.Apply on a parsed snapshot instead. The returned DraftResult
// carries the (possibly re-issued) draft ID and the preview reference URL
// when the backend provides one.
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

// Send dispatches a previously created draft. When sendTime is a non-empty
// Unix-seconds string the backend schedules delivery; otherwise delivery is
// immediate. The returned map is the raw API response body, typically
// including message_id / thread_id / recall_status.
func Send(runtime *common.RuntimeContext, mailboxID, draftID, sendTime string) (map[string]interface{}, error) {
	var bodyParams map[string]interface{}
	if sendTime != "" {
		bodyParams = map[string]interface{}{"send_time": sendTime}
	}
	return runtime.CallAPI("POST", mailboxPath(mailboxID, "drafts", draftID, "send"), nil, bodyParams)
}

// extractDraftID returns the first non-empty draft identifier found in the
// API response. Looks at draft_id / id at the top level, then recurses into a
// nested "draft" object. Returns "" when no identifier is present.
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

// extractRawEML returns the base64url-encoded raw EML from the response,
// looking at top-level "raw", a nested "message.raw", or a nested "draft"
// object. Returns "" when no EML is present.
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

// extractReference returns the optional preview "reference" URL from the
// response, recursing into a nested "draft" object when present. Returns ""
// when no reference is present.
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
