// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
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
	data, meta, err := callDraftAPI(runtime, "GET", mailboxPath(mailboxID, "drafts", draftID), map[string]interface{}{"format": "raw"}, nil)
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
		DraftID:    gotDraftID,
		RawEML:     raw,
		PreviewURL: extractPreviewURL(meta),
	}, nil
}

func CreateWithRaw(runtime *common.RuntimeContext, mailboxID, rawEML string) (DraftResult, error) {
	data, meta, err := callDraftAPI(runtime, "POST", mailboxPath(mailboxID, "drafts"), nil, map[string]interface{}{"raw": rawEML})
	if err != nil {
		return DraftResult{}, err
	}
	draftID := extractDraftID(data)
	if draftID == "" {
		return DraftResult{}, fmt.Errorf("API response missing draft_id")
	}
	return DraftResult{
		DraftID:    draftID,
		PreviewURL: extractPreviewURL(meta),
	}, nil
}

func UpdateWithRaw(runtime *common.RuntimeContext, mailboxID, draftID, rawEML string) (DraftResult, error) {
	data, meta, err := callDraftAPI(runtime, "PUT", mailboxPath(mailboxID, "drafts", draftID), nil, map[string]interface{}{"raw": rawEML})
	if err != nil {
		return DraftResult{}, err
	}
	gotDraftID := extractDraftID(data)
	if gotDraftID == "" {
		gotDraftID = draftID
	}
	return DraftResult{
		DraftID:    gotDraftID,
		PreviewURL: extractPreviewURL(meta),
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

func callDraftAPI(runtime *common.RuntimeContext, method, path string, params map[string]interface{}, payload interface{}) (map[string]interface{}, map[string]interface{}, error) {
	result, err := runtime.RawAPI(method, path, params, payload)
	return extractAPIDataAndMeta(result, err, "API call failed")
}

func extractAPIDataAndMeta(result interface{}, err error, action string) (map[string]interface{}, map[string]interface{}, error) {
	if err != nil {
		return nil, nil, output.Errorf(output.ExitAPI, "api_error", "%s: %s", action, err)
	}
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("%s: unexpected response type %T", action, result)
	}
	code, _ := util.ToFloat64(resultMap["code"])
	if code != 0 {
		msg, _ := resultMap["msg"].(string)
		larkCode := int(code)
		fullMsg := fmt.Sprintf("%s: [%d] %s", action, larkCode, msg)
		return nil, nil, output.ErrAPI(larkCode, fullMsg, resultMap["error"])
	}
	data, _ := resultMap["data"].(map[string]interface{})
	meta, _ := resultMap["meta"].(map[string]interface{})
	if meta == nil && data != nil {
		meta, _ = data["meta"].(map[string]interface{})
	}
	return data, meta, nil
}

func extractPreviewURL(meta map[string]interface{}) string {
	if meta == nil {
		return ""
	}
	return extractPreviewURLValue(meta)
}

func extractPreviewURLValue(data map[string]interface{}) string {
	for _, key := range []string{"preview_url", "previewUrl"} {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	for _, value := range data {
		switch typed := value.(type) {
		case map[string]interface{}:
			if previewURL := extractPreviewURLValue(typed); previewURL != "" {
				return previewURL
			}
		}
	}
	return ""
}
