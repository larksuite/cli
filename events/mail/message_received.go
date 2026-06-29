// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
)

const cleanupTimeout = 5 * time.Second
const watchOutputDirFullParam = "watch_output_dir_full"

type MessageReceivedOutput struct {
	Message map[string]interface{} `json:"message,omitempty" desc:"Fetched message payload. Present for metadata, minimal, plain_text_full, and full formats."`
	Event   map[string]interface{} `json:"event,omitempty"   desc:"Raw mail event body. Present for fetch failures and raw event output."`
	OK      *bool                  `json:"ok,omitempty"      desc:"false when message fetch failed but the event was still emitted."`
	Error   map[string]interface{} `json:"error,omitempty"   desc:"Fetch failure details."`
}

func normalizeParams(ctx context.Context, rt event.APIClient, params map[string]string) error {
	mailbox := strings.TrimSpace(params["mailbox"])
	if mailbox == "" {
		mailbox = "me"
	}
	if strings.EqualFold(mailbox, "me") {
		resolved, err := fetchMailboxPrimaryEmail(ctx, rt, "me")
		if err != nil {
			return enhanceProfileError(err)
		}
		mailbox = resolved
	}
	params["mailbox"] = mailbox

	labelIDs, err := resolveFilterIDs(ctx, rt, mailbox, params["label_ids"], params["labels"], resolveLabelSystemID, "label_ids", "labels", "label")
	if err != nil {
		return err
	}
	folderIDs, err := resolveFilterIDs(ctx, rt, mailbox, params["folder_ids"], params["folders"], resolveFolderSystemAliasOrID, "folder_ids", "folders", "folder")
	if err != nil {
		return err
	}
	if labelIDs != nil {
		params["label_ids"] = mustJSONArray(labelIDs)
	}
	if folderIDs != nil {
		params["folder_ids"] = mustJSONArray(folderIDs)
	}
	return nil
}

func preConsume(ctx context.Context, rt event.APIClient, params map[string]string) (func() error, error) {
	if rt == nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown,
			"runtime API client is required for pre-consume subscription")
	}
	mailbox := strings.TrimSpace(params["mailbox"])
	if mailbox == "" {
		mailbox = "me"
	}
	body := map[string]interface{}{"event_type": 1}
	if _, err := rt.CallAPI(ctx, "POST", mailboxPath(mailbox, "event", "subscribe"), body); err != nil {
		return nil, wrapSubscribeError(err)
	}
	return func() error {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		_, err := rt.CallAPI(cleanupCtx, "POST", mailboxPath(mailbox, "event", "unsubscribe"), body)
		return err
	}, nil
}

func matchMailbox(raw *event.RawEvent, params map[string]string) bool {
	mailbox := strings.TrimSpace(params["mailbox"])
	if mailbox == "" {
		return true
	}
	body := extractMailEventBody(raw.Payload)
	mailAddress, _ := body["mail_address"].(string)
	return strings.EqualFold(mailAddress, mailbox)
}

func processMessageReceived(ctx context.Context, rt event.APIClient, raw *event.RawEvent, params map[string]string) (json.RawMessage, error) {
	msgFormat := strings.TrimSpace(params["msg_format"])
	if msgFormat == "" {
		msgFormat = "metadata"
	}
	eventBody := extractMailEventBody(raw.Payload)
	messageID, _ := eventBody["message_id"].(string)
	if messageID == "" {
		return nil, nil
	}

	labelIDSet := idSetFromParam(params["label_ids"])
	folderIDSet := idSetFromParam(params["folder_ids"])
	forceFull := params[watchOutputDirFullParam] == "true"
	needMessage := forceFull || msgFormat != "event" || len(labelIDSet) > 0 || len(folderIDSet) > 0
	if !needMessage {
		return raw.Payload, nil
	}

	fetchMailbox := params["mailbox"]
	if eventAddr, _ := eventBody["mail_address"].(string); eventAddr != "" {
		fetchMailbox = eventAddr
	}
	fetchFormat := watchFetchFormat(msgFormat, len(labelIDSet) > 0 || len(folderIDSet) > 0)
	if forceFull {
		fetchFormat = "full"
	}
	message, err := fetchMessage(ctx, rt, fetchMailbox, messageID, fetchFormat)
	if err != nil {
		return json.Marshal(watchFetchFailureValue(messageID, fetchFormat, err, eventBody))
	}

	if len(folderIDSet) > 0 {
		folderID, _ := message["folder_id"].(string)
		if !folderIDSet[folderID] {
			return nil, nil
		}
	}
	if len(labelIDSet) > 0 && !messageHasLabel(message, labelIDSet) {
		return nil, nil
	}

	if forceFull {
		return json.Marshal(message)
	}
	if msgFormat == "event" {
		return raw.Payload, nil
	}
	if msgFormat == "minimal" {
		message = minimalWatchMessage(message)
	}
	return json.Marshal(map[string]interface{}{"message": message})
}

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

func fetchMailboxPrimaryEmail(ctx context.Context, rt event.APIClient, mailboxID string) (string, error) {
	if rt == nil {
		return "", errs.NewInternalError(errs.SubtypeUnknown, "runtime API client is required to resolve mailbox profile")
	}
	if mailboxID == "" {
		mailboxID = "me"
	}
	raw, err := rt.CallAPI(ctx, "GET", mailboxPath(mailboxID, "profile"), nil)
	if err != nil {
		return "", err
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	if email := extractPrimaryEmail(resp.Data); email != "" {
		return email, nil
	}
	return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "profile API returned no primary_email_address")
}

func extractPrimaryEmail(data map[string]interface{}) string {
	if email, ok := data["primary_email_address"].(string); ok && strings.TrimSpace(email) != "" {
		return strings.TrimSpace(email)
	}
	if mailbox, ok := data["user_mailbox"].(map[string]interface{}); ok {
		if email, ok := mailbox["primary_email_address"].(string); ok && strings.TrimSpace(email) != "" {
			return strings.TrimSpace(email)
		}
	}
	return ""
}

func fetchMessage(ctx context.Context, rt event.APIClient, mailbox, messageID, format string) (map[string]interface{}, error) {
	if rt == nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "runtime API client is required to fetch mail message")
	}
	path := fmt.Sprintf("%s?format=%s", mailboxPath(mailbox, "messages", messageID), url.QueryEscape(format))
	raw, err := rt.CallAPI(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	if msg, _ := resp.Data["message"].(map[string]interface{}); msg != nil {
		return msg, nil
	}
	return resp.Data, nil
}

func resolveFilterIDs(
	ctx context.Context,
	rt event.APIClient,
	mailboxID, explicitIDsInput, namesInput string,
	systemResolver func(string) (string, bool),
	explicitFlagName, namesFlagName, kind string,
) ([]string, error) {
	explicitIDs, err := parseJSONArrayFlag(explicitIDsInput, explicitFlagName)
	if err != nil {
		return nil, err
	}
	names, err := parseJSONArrayFlag(namesInput, namesFlagName)
	if err != nil {
		return nil, err
	}
	if len(explicitIDs) == 0 && len(names) == 0 {
		return nil, nil
	}
	set := make(map[string]bool)
	for _, raw := range explicitIDs {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if id, ok := systemResolver(value); ok {
			set[id] = true
			continue
		}
		set[value] = true
	}

	var unresolved []string
	for _, raw := range names {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if id, ok := systemResolver(value); ok {
			set[id] = true
			continue
		}
		unresolved = append(unresolved, value)
	}
	if len(unresolved) > 0 {
		var resolved []string
		if kind == "folder" {
			resolved, err = resolveFolderNames(ctx, rt, mailboxID, unresolved)
		} else {
			resolved, err = resolveLabelNames(ctx, rt, mailboxID, unresolved)
		}
		if err != nil {
			return nil, err
		}
		for _, id := range resolved {
			if id != "" {
				set[id] = true
			}
		}
	}
	return setKeys(set), nil
}

func parseJSONArrayFlag(input, flagName string) ([]string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(trimmed), &values); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"invalid %s: expected JSON array of strings", flagName).
			WithParam(flagName).
			WithCause(err)
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v != "" {
			out = append(out, v)
		}
	}
	return out, nil
}

func resolveFolderNames(ctx context.Context, rt event.APIClient, mailboxID string, values []string) ([]string, error) {
	folders, err := listMailboxFolders(ctx, rt, mailboxID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		id, err := resolveByName("folder", value, mailboxID, folders, func(item folderInfo) string { return item.ID }, func(item folderInfo) string { return item.Name })
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func resolveLabelNames(ctx context.Context, rt event.APIClient, mailboxID string, values []string) ([]string, error) {
	labels, err := listMailboxLabels(ctx, rt, mailboxID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		id, err := resolveByName("label", value, mailboxID, labels, func(item labelInfo) string { return item.ID }, func(item labelInfo) string { return item.Name })
		if err != nil {
			if matchID := matchLabelSuffixID(value, labels); matchID != "" {
				out = append(out, matchID)
				continue
			}
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

type folderInfo struct {
	ID   string
	Name string
}

type labelInfo struct {
	ID   string
	Name string
}

func listMailboxFolders(ctx context.Context, rt event.APIClient, mailboxID string) ([]folderInfo, error) {
	raw, err := rt.CallAPI(ctx, "GET", mailboxPath(mailboxID, "folders"), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			Items []map[string]interface{} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	out := make([]folderInfo, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		id, _ := item["id"].(string)
		if id == "" {
			continue
		}
		name, _ := item["name"].(string)
		out = append(out, folderInfo{ID: id, Name: name})
	}
	return out, nil
}

func listMailboxLabels(ctx context.Context, rt event.APIClient, mailboxID string) ([]labelInfo, error) {
	raw, err := rt.CallAPI(ctx, "GET", mailboxPath(mailboxID, "labels"), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			Items []map[string]interface{} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	out := make([]labelInfo, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		id, _ := item["id"].(string)
		if id == "" {
			continue
		}
		name, _ := item["name"].(string)
		out = append(out, labelInfo{ID: id, Name: name})
	}
	return out, nil
}

func resolveByName[T any](kind, value, mailboxID string, items []T, idFn, nameFn func(T) string) (string, error) {
	lower := strings.ToLower(strings.TrimSpace(value))
	var matches []T
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(nameFn(item))) == lower || idFn(item) == value {
			matches = append(matches, item)
		}
	}
	switch len(matches) {
	case 0:
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"unknown %s %q in mailbox %s", kind, value, mailboxID)
	case 1:
		return idFn(matches[0]), nil
	default:
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"ambiguous %s %q in mailbox %s", kind, value, mailboxID)
	}
}

var folderAliasToSystemID = map[string]string{
	"inbox":    "INBOX",
	"sent":     "SENT",
	"draft":    "DRAFT",
	"trash":    "TRASH",
	"spam":     "SPAM",
	"archive":  "ARCHIVED",
	"archived": "ARCHIVED",
}

var folderSystemIDs = map[string]bool{
	"INBOX": true, "SENT": true, "DRAFT": true, "TRASH": true, "SPAM": true, "ARCHIVED": true,
}

var labelSystemIDs = map[string]bool{
	"FLAGGED": true, "IMPORTANT": true, "OTHER": true,
}

var systemLabelAliases = map[string]string{
	"important": "IMPORTANT", "priority": "IMPORTANT", "重要邮件": "IMPORTANT",
	"flagged": "FLAGGED", "已加旗标": "FLAGGED",
	"other": "OTHER", "其他邮件": "OTHER",
}

func resolveFolderSystemAliasOrID(input string) (string, bool) {
	if id, ok := folderAliasToSystemID[strings.ToLower(strings.TrimSpace(input))]; ok {
		return id, true
	}
	return normalizeSystemID(input, folderSystemIDs)
}

func resolveLabelSystemID(input string) (string, bool) {
	if id, ok := systemLabelAliases[strings.ToLower(strings.TrimSpace(input))]; ok {
		return id, true
	}
	return normalizeSystemID(input, labelSystemIDs)
}

func normalizeSystemID(input string, systemIDs map[string]bool) (string, bool) {
	canonical := strings.ToUpper(strings.TrimSpace(input))
	if canonical == "" {
		return "", false
	}
	if systemIDs[canonical] {
		return canonical, true
	}
	return "", false
}

func mustJSONArray(values []string) string {
	b, _ := json.Marshal(values)
	return string(b)
}

func idSetFromParam(input string) map[string]bool {
	values, err := parseJSONArrayFlag(input, "")
	if err != nil {
		return nil
	}
	set := make(map[string]bool, len(values))
	for _, v := range values {
		if v != "" {
			set[v] = true
		}
	}
	return set
}

func setKeys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		if k != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func extractMailEventBody(raw json.RawMessage) map[string]interface{} {
	var data map[string]interface{}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&data); err != nil {
		return map[string]interface{}{}
	}
	if eventBody, ok := data["event"].(map[string]interface{}); ok {
		return eventBody
	}
	return data
}

func watchFetchFormat(msgFormat string, forceMetadata bool) string {
	if forceMetadata && msgFormat == "event" {
		return "metadata"
	}
	switch msgFormat {
	case "metadata", "plain_text_full", "full":
		return msgFormat
	case "minimal":
		return "metadata"
	default:
		return "metadata"
	}
}

func minimalWatchMessage(message map[string]interface{}) map[string]interface{} {
	if message == nil {
		return nil
	}
	out := make(map[string]interface{}, 6)
	for _, key := range []string{"message_id", "thread_id", "folder_id", "label_ids", "internal_date", "message_state"} {
		if value, ok := message[key]; ok {
			out[key] = value
		}
	}
	return out
}

func messageHasLabel(meta map[string]interface{}, labelIDSet map[string]bool) bool {
	labels, _ := meta["label_ids"].([]interface{})
	for _, l := range labels {
		if id, ok := l.(string); ok && labelIDSet[id] {
			return true
		}
	}
	return false
}

func watchFetchFailureValue(messageID, fetchFormat string, err error, eventBody map[string]interface{}) map[string]interface{} {
	payload := map[string]interface{}{
		"ok": false,
		"error": map[string]interface{}{
			"type":       "fetch_message_failed",
			"message_id": messageID,
			"format":     fetchFormat,
			"message":    err.Error(),
		},
	}
	if len(eventBody) > 0 {
		payload["event"] = eventBody
	}
	return payload
}

func wrapSubscribeError(err error) error {
	if err == nil {
		return nil
	}
	hint := "ensure the app has scope mail:event and the event mail.user_mailbox.event.message_received_v1 is enabled"
	if p, ok := errs.ProblemOf(err); ok {
		p.Message = "subscribe mailbox events failed: " + p.Message
		if strings.TrimSpace(p.Hint) != "" {
			p.Hint = p.Hint + "; " + hint
		} else {
			p.Hint = hint
		}
		return err
	}
	return errs.NewAPIError(errs.SubtypeUnknown, "subscribe mailbox events failed: %v", err).WithHint("%s", hint).WithCause(err)
}

func enhanceProfileError(err error) error {
	if p, ok := errs.ProblemOf(err); ok {
		lower := strings.ToLower(p.Message)
		if p.Category == errs.CategoryAuthorization {
			p.Message = "unable to resolve mailbox address: " + p.Message
			p.Hint = "run `lark-cli auth login --scope \"mail:user_mailbox:readonly\"` to grant mailbox profile access"
			return err
		}
		if strings.Contains(lower, "permission") || strings.Contains(lower, "scope") {
			permErr := errs.NewPermissionError(errs.SubtypeMissingScope, "unable to resolve mailbox address: %s", p.Message).
				WithHint("run `lark-cli auth login --scope \"mail:user_mailbox:readonly\"` to grant mailbox profile access").
				WithCause(err)
			if p.Code != 0 {
				permErr = permErr.WithCode(p.Code)
			}
			if p.LogID != "" {
				permErr = permErr.WithLogID(p.LogID)
			}
			return permErr
		}
	}
	return err
}

func matchLabelSuffixID(input string, labels []labelInfo) string {
	lower := strings.ToLower(input)
	suffix := "/" + lower
	for _, l := range labels {
		name := strings.TrimSpace(l.Name)
		if strings.HasSuffix(strings.ToLower(name), suffix) {
			return l.ID
		}
	}
	return ""
}
