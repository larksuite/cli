// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// triageFilter represents shortcut filters that are mapped to list/search APIs.
type triageTimeRange struct {
	StartTime string `json:"start_time,omitempty"`
	EndTime   string `json:"end_time,omitempty"`
}

type triageFilter struct {
	Folder        string           `json:"folder,omitempty"`         // folder name
	FolderID      string           `json:"folder_id,omitempty"`      // explicit folder ID, higher priority than folder
	Label         string           `json:"label,omitempty"`          // label name
	LabelID       string           `json:"label_id,omitempty"`       // explicit label ID, higher priority than label
	From          []string         `json:"from,omitempty"`           // query path only
	To            []string         `json:"to,omitempty"`             // query path only
	CC            []string         `json:"cc,omitempty"`             // query path only
	BCC           []string         `json:"bcc,omitempty"`            // query path only
	Subject       string           `json:"subject,omitempty"`        // query path only
	HasAttachment *bool            `json:"has_attachment,omitempty"` // query path only
	IsUnread      *bool            `json:"is_unread,omitempty"`      // query path only
	TimeRange     *triageTimeRange `json:"time_range,omitempty"`     // query path only
}

const (
	searchPageMax    = 15 // max items per search API page
	listPageMax      = 20 // max items per list API page
	triageMaxLimit   = 400
	triageAPIRetries = 2 // retry count in addition to the first attempt
)

var MailTriage = common.Shortcut{
	Service:     "mail",
	Command:     "+triage",
	Description: `List mail summaries (date/from/subject/message_id). Use --query for full-text search, --filter for exact-match conditions.`,
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox.message:readonly", "mail:user_mailbox.message.address:read", "mail:user_mailbox.message.subject:read", "mail:user_mailbox.message.body:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "format", Default: "table", Desc: "output format: table | json | data (json/data output object with pagination fields)"},
		{Name: "max", Type: "int", Default: "20", Desc: "maximum number of messages to fetch (1-400; auto-paginates internally)"},
		{Name: "page-size", Type: "int", Desc: "alias for --max"},
		{Name: "page-token", Desc: "pagination token from a previous response to fetch the next page"},
		{Name: "filter", Desc: `exact-match condition filter (JSON). Narrow results by folder, label, sender, recipient, etc. Run --print-filter-schema to see all fields. Example: {"folder":"INBOX","from":["alice@example.com"]}`},
		{Name: "mailbox", Default: "me", Desc: "email address (default: me)"},
		{Name: "query", Desc: `full-text keyword search across from/to/subject/body (max 50 chars). Example: "budget report"`},
		{Name: "labels", Type: "bool", Desc: "include label IDs in output"},
		{Name: "print-filter-schema", Type: "bool", Desc: "print --filter field reference and exit"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailbox := resolveMailboxID(runtime)
		query := runtime.Str("query")
		showLabels := runtime.Bool("labels")
		maxCount := resolveTriagePageSize(runtime)
		parsed, parseErr := parseTriagePageToken(runtime.Str("page-token"))
		filter, err := parseTriageFilter(runtime.Str("filter"))
		d := common.NewDryRunAPI().Set("input_filter", runtime.Str("filter"))
		if parseErr != nil {
			return d.Set("filter_error", parseErr.Error())
		}
		if err != nil {
			return d.Set("filter_error", err.Error())
		}
		useSearch, pathErr := resolveTriagePath(parsed, query, filter)
		if pathErr != nil {
			return d.Set("filter_error", pathErr.Error())
		}
		if useSearch {
			resolvedFilter, err := resolveSearchFilter(runtime, mailbox, filter, true)
			if err != nil {
				return d.Set("filter_error", err.Error())
			}
			pageSize := maxCount
			if pageSize > searchPageMax {
				pageSize = searchPageMax
			}
			searchDesc := "search messages (auto-paginates up to --max)"
			if parsed.RawToken != "" {
				searchDesc = "search messages (continues from --page-token, up to --max)"
			}
			searchParams, searchBody, _ := buildSearchParams(runtime, mailbox, query, resolvedFilter, pageSize, parsed.RawToken, true)
			d = d.POST(mailboxPath(mailbox, "search")).
				Params(searchParams).
				Body(searchBody).
				Desc(searchDesc)
			if showLabels {
				d = d.POST(mailboxPath(mailbox, "messages", "batch_get")).
					Body(map[string]interface{}{"format": "metadata", "message_ids": []string{"<message_id>"}}).
					Desc("batch_get messages with format=metadata to populate labels")
			}
			return d
		}
		resolvedFilter, err := resolveListFilter(runtime, mailbox, filter, true)
		if err != nil {
			return d.Set("filter_error", err.Error())
		}
		pageSize := maxCount
		if pageSize > listPageMax {
			pageSize = listPageMax
		}
		listDesc := "list message IDs (auto-paginates up to --max); batch_get with format=metadata"
		if parsed.RawToken != "" {
			listDesc = "list message IDs (continues from --page-token, up to --max); batch_get with format=metadata"
		}
		listParams, _ := buildListParams(runtime, mailbox, resolvedFilter, pageSize, parsed.RawToken, true)
		return d.GET(mailboxPath(mailbox, "messages")).
			Params(listParams).
			POST(mailboxPath(mailbox, "messages", "batch_get")).
			Body(map[string]interface{}{"format": "metadata", "message_ids": []string{"<message_id>"}}).
			Desc(listDesc).
			Set("resolve_note", "name→ID resolution for filter.folder/filter.label runs during execution; dry-run does not call folders/labels list APIs")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Bool("print-filter-schema") {
			printTriageFilterSchema(runtime)
			return nil
		}
		mailbox := resolveMailboxID(runtime)
		hintIdentityFirst(runtime, mailbox)
		outFormat := runtime.Str("format")
		query := runtime.Str("query")
		if query != "" {
			if err := common.RejectDangerousChars("--query", query); err != nil {
				return err
			}
		}
		showLabels := runtime.Bool("labels")
		filter, err := parseTriageFilter(runtime.Str("filter"))
		if err != nil {
			return err
		}
		maxCount := resolveTriagePageSize(runtime)
		parsed, err := parseTriagePageToken(runtime.Str("page-token"))
		if err != nil {
			return err
		}

		var messages []map[string]interface{}
		var hasMore bool
		var nextPageToken string

		useSearch, err := resolveTriagePath(parsed, query, filter)
		if err != nil {
			return err
		}

		if useSearch {
			resolvedFilter, err := resolveSearchFilter(runtime, mailbox, filter, false)
			if err != nil {
				return err
			}
			pageToken := parsed.RawToken
			for len(messages) < maxCount {
				pageSize := maxCount - len(messages)
				if pageSize > searchPageMax {
					pageSize = searchPageMax
				}
				searchParams, searchBody, err := buildSearchParams(runtime, mailbox, query, resolvedFilter, pageSize, pageToken, false)
				if err != nil {
					return err
				}
				searchData, err := doJSONAPI(runtime, &larkcore.ApiReq{
					HttpMethod:  http.MethodPost,
					ApiPath:     mailboxPath(mailbox, "search"),
					QueryParams: toQueryParams(searchParams),
					Body:        searchBody,
				}, "API call failed")
				if err != nil {
					return err
				}
				pageMessages := buildTriageMessagesFromSearchItems(searchData["items"])
				messages = append(messages, pageMessages...)
				pageHasMore, _ := searchData["has_more"].(bool)
				pageToken, _ = searchData["page_token"].(string)
				if !pageHasMore || pageToken == "" {
					hasMore = false
					nextPageToken = ""
					break
				}
				hasMore = pageHasMore
				nextPageToken = encodeTriagePageToken("search", pageToken)
			}
			if len(messages) > maxCount {
				messages = messages[:maxCount]
			}
			if showLabels && len(messages) > 0 {
				messageIDs := make([]string, len(messages))
				for i, m := range messages {
					messageIDs[i] = strVal(m["message_id"])
				}
				enriched, err := fetchMessageMetas(runtime, mailbox, messageIDs)
				if err != nil {
					return err
				}
				mergeTriageLabels(messages, enriched)
			}
		} else {
			resolvedFilter, err := resolveListFilter(runtime, mailbox, filter, false)
			if err != nil {
				return err
			}
			var (
				messageIDs []string
				pageToken  = parsed.RawToken
			)
			for len(messageIDs) < maxCount {
				pageSize := maxCount - len(messageIDs)
				if pageSize > listPageMax {
					pageSize = listPageMax
				}
				listParams, err := buildListParams(runtime, mailbox, resolvedFilter, pageSize, pageToken, false)
				if err != nil {
					return err
				}
				listData, err := doJSONAPI(runtime, &larkcore.ApiReq{
					HttpMethod:  http.MethodGet,
					ApiPath:     mailboxPath(mailbox, "messages"),
					QueryParams: toQueryParams(listParams),
				}, "API call failed")
				if err != nil {
					return err
				}
				ids := extractTriageMessageIDs(listData["items"])
				messageIDs = append(messageIDs, ids...)
				pageHasMore, _ := listData["has_more"].(bool)
				pageToken, _ = listData["page_token"].(string)
				if !pageHasMore || pageToken == "" {
					hasMore = false
					nextPageToken = ""
					break
				}
				hasMore = pageHasMore
				nextPageToken = encodeTriagePageToken("list", pageToken)
			}
			if len(messageIDs) > maxCount {
				messageIDs = messageIDs[:maxCount]
			}
			messages, err = fetchMessageMetas(runtime, mailbox, messageIDs)
			if err != nil {
				return err
			}
		}

		if messages == nil {
			messages = []map[string]interface{}{}
		}

		switch outFormat {
		case "json", "data":
			outData := map[string]interface{}{
				"messages":   messages,
				"count":      len(messages),
				"has_more":   hasMore,
				"page_token": nextPageToken,
			}
			output.PrintJson(runtime.IO().Out, outData)
		default: // "table"
			if len(messages) == 0 {
				fmt.Fprintln(runtime.IO().ErrOut, "No messages found.")
				return nil
			}
			var rows []map[string]interface{}
			for _, msg := range messages {
				row := map[string]interface{}{
					"date":       sanitizeForTerminal(strVal(msg["date"])),
					"from":       sanitizeForTerminal(strVal(msg["from"])),
					"subject":    sanitizeForTerminal(strVal(msg["subject"])),
					"message_id": msg["message_id"],
				}
				if showLabels {
					row["labels"] = msg["labels"]
				}
				rows = append(rows, row)
			}
			output.PrintTable(runtime.IO().Out, rows)
			fmt.Fprintf(runtime.IO().ErrOut, "\n%d message(s)\n", len(messages))
			if hasMore && nextPageToken != "" {
				var hint strings.Builder
				hint.WriteString("next page: mail +triage")
				if query != "" {
					hint.WriteString(" --query " + shellQuote(query))
				}
				if filterStr := runtime.Str("filter"); filterStr != "" {
					hint.WriteString(" --filter " + shellQuote(filterStr))
				}
				hint.WriteString(" --page-token " + shellQuote(nextPageToken))
				fmt.Fprintln(runtime.IO().ErrOut, hint.String())
			}
			fmt.Fprintln(runtime.IO().ErrOut, "tip: use mail +message --message-id <id> to read full content")
		}
		return nil
	},
}

func printTriageFilterSchema(runtime *common.RuntimeContext) {
	schema := map[string]interface{}{
		"_description": "--filter field reference for mail +triage. All fields are optional. --filter narrows results by exact conditions; --query does full-text search.",
		"fields": map[string]interface{}{
			"folder": map[string]string{
				"type":    "string",
				"desc":    "文件夹名称筛选，仅返回在所选目录下的邮件。仅支持传入系统文件夹名称（固定值 inbox/sent/draft/trash/spam/archive/priority/flagged/other/scheduled）、自定义文件夹名称。子文件夹需使用 parent_name/child_name 格式，可通过 folder list 接口查看文件夹名称。",
				"example": "inbox",
			},
			"folder_id": map[string]string{
				"type":    "string",
				"desc":    "Folder ID (takes priority over folder). System IDs: INBOX, SENT, DRAFT, TRASH, SPAM, ARCHIVED. Custom folders use numeric IDs from the folder list API.",
				"example": "INBOX",
			},
			"label": map[string]string{
				"type":    "string",
				"desc":    "自定义标签名称筛选，仅返回包含指定自定义标签的邮件。子标签需使用 parent_name/child_name 格式，可通过 label list 接口查看标签名称。",
				"example": "my-label",
			},
			"label_id": map[string]string{
				"type":    "string",
				"desc":    "Label ID (takes priority over label). Custom labels use numeric IDs from the label list API.",
				"example": "7543462602685287954",
			},
			"from": map[string]string{
				"type":    "[]string",
				"desc":    "Sender email addresses (OR logic within list). Triggers search path.",
				"example": `["alice@example.com"]`,
			},
			"to": map[string]string{
				"type":    "[]string",
				"desc":    "To-recipient email addresses (OR logic within list). Triggers search path.",
				"example": `["bob@example.com"]`,
			},
			"cc": map[string]string{
				"type":    "[]string",
				"desc":    "CC-recipient email addresses. Triggers search path.",
				"example": `["cc@example.com"]`,
			},
			"bcc": map[string]string{
				"type":    "[]string",
				"desc":    "BCC-recipient email addresses. Triggers search path.",
				"example": `["bcc@example.com"]`,
			},
			"subject": map[string]string{
				"type":    "string",
				"desc":    "Subject keyword match. Triggers search path.",
				"example": "report",
			},
			"has_attachment": map[string]string{
				"type":    "bool",
				"desc":    "Filter by attachment presence (true/false). Triggers search path.",
				"example": "true",
			},
			"is_unread": map[string]string{
				"type":    "bool",
				"desc":    "Filter by read status. On list path only is_unread=true is supported; on search path both true/false work.",
				"example": "true",
			},
			"time_range": map[string]string{
				"type":    "object",
				"desc":    "Time range filter with start_time and/or end_time (ISO 8601 with timezone). Triggers search path.",
				"example": `{"start_time":"2026-03-10T00:00:00+08:00","end_time":"2026-03-17T23:59:59+08:00"}`,
			},
		},
		"notes": []string{
			"folder/folder_id and label/label_id work on both list and search paths.",
			"from, to, cc, bcc, subject, has_attachment, time_range trigger the search path.",
			"--query and search-path filter fields can be combined.",
			"folder and label cannot be set at the same time on the list path.",
			"System labels (IMPORTANT/FLAGGED/OTHER) are automatically passed as folder (priority/flagged/other) in search.",
		},
		"examples": []string{
			`{"folder":"INBOX"}`,
			`{"folder":"INBOX","from":["alice@example.com"]}`,
			`{"label":"FLAGGED","is_unread":true}`,
			`{"folder":"SENT","time_range":{"start_time":"2026-03-01T00:00:00+08:00"}}`,
		},
	}
	runtime.Out(schema, nil)
}

func parseTriageFilter(filterStr string) (triageFilter, error) {
	var filter triageFilter
	if strings.TrimSpace(filterStr) == "" {
		return filter, nil
	}
	dec := json.NewDecoder(strings.NewReader(filterStr))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&filter); err != nil {
		if hint := triageFilterUnknownFieldHint(err.Error()); hint != "" {
			return triageFilter{}, output.ErrValidation("invalid --filter: %s", hint)
		}
		return triageFilter{}, output.ErrValidation("invalid --filter: %s", err)
	}
	return filter, nil
}

func triageFilterUnknownFieldHint(msg string) string {
	const unknownFieldPrefix = `json: unknown field "`
	if !strings.HasPrefix(msg, unknownFieldPrefix) || !strings.HasSuffix(msg, `"`) {
		return ""
	}
	field := strings.TrimSuffix(strings.TrimPrefix(msg, unknownFieldPrefix), `"`)
	if field == "" {
		return ""
	}
	suggestions := map[string]string{
		"unread":      "is_unread",
		"create_time": "time_range",
		"after":       "time_range.start_time",
		"before":      "time_range.end_time",
	}
	const validFields = "folder, folder_id, label, label_id, is_unread, from, to, cc, bcc, subject, has_attachment, time_range"
	const timeRangeExample = ` Example: {"time_range":{"start_time":"2026-03-10T00:00:00+08:00","end_time":"2026-03-17T23:59:59+08:00"}}`
	if suggestion, ok := suggestions[field]; ok {
		msg := fmt.Sprintf("unknown field %q; did you mean %q? Valid fields: %s", field, suggestion, validFields)
		if strings.HasPrefix(suggestion, "time_range.") || suggestion == "time_range" {
			msg += timeRangeExample
		}
		return msg
	}
	return fmt.Sprintf("unknown field %q. Valid fields: %s", field, validFields)
}

func usesTriageSearchPath(query string, filter triageFilter) bool {
	if strings.TrimSpace(query) != "" || len(triageQueryFilterFields(filter)) > 0 {
		return true
	}
	// System labels (important/flagged/other and their aliases) can appear in either
	// folder or label field. They always require the search path because the search API
	// treats them as folder values, and they are not real folder IDs for the list API.
	if v := strings.TrimSpace(filter.Folder); v != "" {
		if _, ok := resolveSystemLabel(v); ok {
			return true
		}
		if searchOnlyFolderNames[strings.ToLower(v)] {
			return true
		}
	}
	if v := strings.TrimSpace(filter.Label); v != "" {
		if _, ok := resolveSystemLabel(v); ok {
			return true
		}
	}
	if v := strings.TrimSpace(filter.LabelID); v != "" {
		if _, ok := resolveSystemLabel(v); ok {
			return true
		}
	}
	return false
}

func fetchMessageMetas(runtime *common.RuntimeContext, mailbox string, messageIDs []string) ([]map[string]interface{}, error) {
	if len(messageIDs) == 0 {
		return nil, nil
	}
	const maxBatchGetIDs = 20
	byID := make(map[string]map[string]interface{}, len(messageIDs))
	for start := 0; start < len(messageIDs); start += maxBatchGetIDs {
		end := start + maxBatchGetIDs
		if end > len(messageIDs) {
			end = len(messageIDs)
		}
		data, err := doJSONAPI(runtime, &larkcore.ApiReq{
			HttpMethod: http.MethodPost,
			ApiPath:    mailboxPath(mailbox, "messages", "batch_get"),
			Body: map[string]interface{}{
				"format":      "metadata",
				"message_ids": messageIDs[start:end],
			},
		}, "API call failed")
		if err != nil {
			return nil, err
		}
		rawMessages, _ := data["messages"].([]interface{})
		for _, item := range rawMessages {
			msg, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			messageID := strVal(msg["message_id"])
			if messageID == "" {
				continue
			}
			byID[messageID] = buildTriageMessageMeta(msg, messageID)
		}
	}

	messages := make([]map[string]interface{}, 0, len(messageIDs))
	for _, messageID := range messageIDs {
		if msg, ok := byID[messageID]; ok {
			messages = append(messages, msg)
			continue
		}
		messages = append(messages, map[string]interface{}{"message_id": messageID, "error": "metadata not returned by batch_get"})
	}
	return messages, nil
}

func buildTriageMessageMeta(msg map[string]interface{}, fallbackMessageID string) map[string]interface{} {
	item := map[string]interface{}{
		"message_id": fallbackMessageID,
	}
	if v := strVal(msg["message_id"]); v != "" {
		item["message_id"] = v
	}
	item["thread_id"] = strVal(msg["thread_id"])
	item["subject"] = strVal(msg["subject"])
	item["folder"] = strVal(msg["folder_id"])
	if d := strVal(msg["date"]); d != "" {
		item["date"] = d
	} else if ts, ok := msg["internal_date"]; ok {
		item["date"] = common.FormatTime(ts)
	}
	if from, ok := msg["head_from"].(map[string]interface{}); ok {
		item["from"] = formatAddress(from)
	}
	var labelIDs []string
	if labels, ok := msg["label_ids"].([]interface{}); ok {
		for _, l := range labels {
			if s, ok := l.(string); ok {
				labelIDs = append(labelIDs, s)
			}
		}
	}
	item["labels"] = strings.Join(labelIDs, ",")
	return item
}

func buildTriageMessagesFromSearchItems(raw interface{}) []map[string]interface{} {
	rawItems, _ := raw.([]interface{})
	messages := make([]map[string]interface{}, 0, len(rawItems))
	for _, item := range rawItems {
		searchItem, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		meta, _ := searchItem["meta_data"].(map[string]interface{})
		messageID := ""
		if meta != nil {
			messageID = strVal(meta["message_biz_id"])
		}
		if messageID == "" {
			continue
		}
		message := map[string]interface{}{
			"message_id": messageID,
			"labels":     "",
		}
		if meta != nil {
			message["thread_id"] = strVal(meta["thread_id"])
			message["subject"] = strVal(meta["title"])
			message["date"] = strVal(meta["create_time"])
			if from, ok := meta["from"].(map[string]interface{}); ok {
				message["from"] = formatAddress(from)
			}
			// Decode body fields when present in search meta_data (base64url-encoded by the server).
			decodeBodyFields(meta, message)
		}
		messages = append(messages, message)
	}
	return messages
}

func mergeTriageLabels(messages, enriched []map[string]interface{}) {
	labelsByID := make(map[string]string, len(enriched))
	for _, item := range enriched {
		messageID := strVal(item["message_id"])
		if messageID == "" {
			continue
		}
		labelsByID[messageID] = strVal(item["labels"])
	}
	for _, item := range messages {
		messageID := strVal(item["message_id"])
		if messageID == "" {
			continue
		}
		if labels, ok := labelsByID[messageID]; ok {
			item["labels"] = labels
		}
	}
}

func resolveListFilter(runtime *common.RuntimeContext, mailboxID string, f triageFilter, dryRun bool) (triageFilter, error) {
	resolved := f
	if dryRun {
		if value := strings.TrimSpace(f.FolderID); value != "" {
			if sysID, ok := resolveFolderSystemAliasOrID(value); ok {
				resolved.FolderID = sysID
			} else {
				resolved.FolderID = value
			}
			resolved.Folder = ""
		} else if value := strings.TrimSpace(f.Folder); value != "" {
			if sysID, ok := resolveFolderSystemAliasOrID(value); ok {
				resolved.FolderID = sysID
				resolved.Folder = ""
			}
		}
		if value := strings.TrimSpace(f.LabelID); value != "" {
			if sysID, ok := resolveLabelSystemID(value); ok {
				resolved.LabelID = sysID
			} else {
				resolved.LabelID = value
			}
			resolved.Label = ""
		} else if value := strings.TrimSpace(f.Label); value != "" {
			if sysID, ok := resolveLabelSystemID(value); ok {
				resolved.LabelID = sysID
				resolved.Label = ""
			}
		}
		return resolved, nil
	}
	if value := strings.TrimSpace(f.FolderID); value != "" {
		id, err := resolveFolderID(runtime, mailboxID, value)
		if err != nil {
			return triageFilter{}, err
		}
		resolved.FolderID = id
		resolved.Folder = ""
	} else if value := strings.TrimSpace(f.Folder); value != "" {
		id, err := resolveFolderName(runtime, mailboxID, value)
		if err != nil {
			return triageFilter{}, err
		}
		resolved.FolderID = id
		resolved.Folder = ""
	}
	if value := strings.TrimSpace(f.LabelID); value != "" {
		id, err := resolveLabelID(runtime, mailboxID, value)
		if err != nil {
			return triageFilter{}, err
		}
		resolved.LabelID = id
		resolved.Label = ""
	} else if value := strings.TrimSpace(f.Label); value != "" {
		id, err := resolveLabelName(runtime, mailboxID, value)
		if err != nil {
			return triageFilter{}, err
		}
		resolved.LabelID = id
		resolved.Label = ""
	}
	return resolved, nil
}

func resolveSearchFilter(runtime *common.RuntimeContext, mailboxID string, f triageFilter, dryRun bool) (triageFilter, error) {
	resolved := f

	// Step 1: Check if folder or label/label_id contains a system label.
	// System labels (important/flagged/other) are passed as folder in the search API.
	var systemLabelFolder string
	if v := strings.TrimSpace(f.Folder); v != "" {
		if id, ok := resolveSystemLabel(v); ok {
			systemLabelFolder = systemLabelSearchName[id]
		}
	}
	if systemLabelFolder == "" {
		if v := strings.TrimSpace(f.LabelID); v != "" {
			if id, ok := resolveSystemLabel(v); ok {
				systemLabelFolder = systemLabelSearchName[id]
			}
		}
	}
	if systemLabelFolder == "" {
		if v := strings.TrimSpace(f.Label); v != "" {
			if id, ok := resolveSystemLabel(v); ok {
				systemLabelFolder = systemLabelSearchName[id]
			}
		}
	}

	if systemLabelFolder != "" {
		// System label found: clear label fields and set as folder.
		resolved.Label = ""
		resolved.LabelID = ""
		// If the user also specified a real folder, keep the system label as folder
		// (it takes precedence since system labels are the primary intent).
		resolved.Folder = systemLabelFolder
		resolved.FolderID = ""
		return resolved, nil
	}

	// Step 2: Normal folder resolution.
	folderName, err := resolveSearchFolderFilter(runtime, mailboxID, f, dryRun)
	if err != nil {
		return triageFilter{}, err
	}
	resolved.Folder = folderName
	resolved.FolderID = ""

	// Step 3: Normal label resolution (custom labels only, since system labels handled above).
	labelName, err := resolveSearchLabelFilter(runtime, mailboxID, f, dryRun)
	if err != nil {
		return triageFilter{}, err
	}
	resolved.Label = labelName
	resolved.LabelID = ""
	return resolved, nil
}

func buildListParams(runtime *common.RuntimeContext, mailboxID string, f triageFilter, pageSize int, pageToken string, dryRun bool) (map[string]interface{}, error) {
	// folder_id is required by the API unless label_id is specified
	// (folder_id and label_id cannot be set at the same time)
	hasLabel := strings.TrimSpace(f.LabelID) != "" || strings.TrimSpace(f.Label) != ""
	params := map[string]interface{}{
		"page_size": pageSize,
	}
	if !hasLabel {
		params["folder_id"] = "INBOX"
	}
	if pageToken != "" {
		params["page_token"] = pageToken
	}

	folderIDFromFilter := strings.TrimSpace(f.FolderID)
	folderFromFilter := strings.TrimSpace(f.Folder)
	if folderIDFromFilter != "" {
		if dryRun {
			if sysID, ok := resolveFolderSystemAliasOrID(folderIDFromFilter); ok {
				params["folder_id"] = sysID
			} else {
				params["folder_id"] = folderIDFromFilter
			}
		} else {
			resolved, err := resolveFolderID(runtime, mailboxID, folderIDFromFilter)
			if err != nil {
				return nil, err
			}
			if resolved != "" {
				params["folder_id"] = resolved
			}
		}
	} else if folderFromFilter != "" {
		if dryRun {
			if sysID, ok := resolveFolderSystemAliasOrID(folderFromFilter); ok {
				params["folder_id"] = sysID
			} else {
				params["folder_id"] = folderFromFilter
			}
		} else {
			resolved, err := resolveFolderName(runtime, mailboxID, folderFromFilter)
			if err != nil {
				return nil, err
			}
			if resolved != "" {
				params["folder_id"] = resolved
			}
		}
	}

	// list API uses "only_unread" (true-only flag); false has no list-path equivalent
	if f.IsUnread != nil && *f.IsUnread {
		params["only_unread"] = true
	}

	labelIDFromFilter := strings.TrimSpace(f.LabelID)
	labelFromFilter := strings.TrimSpace(f.Label)
	if labelIDFromFilter != "" {
		if dryRun {
			if sysID, ok := resolveLabelSystemID(labelIDFromFilter); ok {
				params["label_id"] = sysID
			} else {
				params["label_id"] = labelIDFromFilter
			}
		} else {
			resolved, err := resolveLabelID(runtime, mailboxID, labelIDFromFilter)
			if err != nil {
				return nil, err
			}
			if resolved != "" {
				params["label_id"] = resolved
			}
		}
	} else if labelFromFilter != "" {
		if dryRun {
			if sysID, ok := resolveLabelSystemID(labelFromFilter); ok {
				params["label_id"] = sysID
			} else {
				params["label_id"] = labelFromFilter
			}
		} else {
			resolved, err := resolveLabelName(runtime, mailboxID, labelFromFilter)
			if err != nil {
				return nil, err
			}
			if resolved != "" {
				params["label_id"] = resolved
			}
		}
	}

	return params, nil
}

func buildSearchParams(runtime *common.RuntimeContext, mailboxID, query string, f triageFilter, pageSize int, pageToken string, dryRun bool) (map[string]interface{}, map[string]interface{}, error) {
	params := map[string]interface{}{
		"page_size": pageSize,
	}
	if pageToken != "" {
		params["page_token"] = pageToken
	}

	filterBody := map[string]interface{}{}
	if values := trimStringList(f.From); len(values) > 0 {
		filterBody["from"] = values
	}
	if values := trimStringList(f.To); len(values) > 0 {
		filterBody["to"] = values
	}
	if values := trimStringList(f.CC); len(values) > 0 {
		filterBody["cc"] = values
	}
	if values := trimStringList(f.BCC); len(values) > 0 {
		filterBody["bcc"] = values
	}
	if subject := strings.TrimSpace(f.Subject); subject != "" {
		filterBody["subject"] = subject
	}
	if f.HasAttachment != nil {
		filterBody["has_attachment"] = *f.HasAttachment
	}
	if f.IsUnread != nil {
		filterBody["is_unread"] = *f.IsUnread
	}
	if createTime := buildSearchCreateTime(f.TimeRange); len(createTime) > 0 {
		filterBody["create_time"] = createTime
	}

	// f.Folder and f.Label are already resolved by resolveSearchFilter before this call.
	if folderName := strings.TrimSpace(f.Folder); folderName != "" {
		filterBody["folder"] = []string{folderName}
	}
	if labelName := strings.TrimSpace(f.Label); labelName != "" {
		filterBody["label"] = []string{labelName}
	}

	body := map[string]interface{}{}
	if strings.TrimSpace(query) != "" {
		body["query"] = strings.TrimSpace(query)
	}
	if len(filterBody) > 0 {
		body["filter"] = filterBody
	}
	return params, body, nil
}

func triageQueryFilterFields(f triageFilter) []string {
	fields := make([]string, 0, 8)
	if len(f.From) > 0 {
		fields = append(fields, "from")
	}
	if len(f.To) > 0 {
		fields = append(fields, "to")
	}
	if len(f.CC) > 0 {
		fields = append(fields, "cc")
	}
	if len(f.BCC) > 0 {
		fields = append(fields, "bcc")
	}
	if strings.TrimSpace(f.Subject) != "" {
		fields = append(fields, "subject")
	}
	if f.HasAttachment != nil {
		fields = append(fields, "has_attachment")
	}
	if f.TimeRange != nil && (strings.TrimSpace(f.TimeRange.StartTime) != "" || strings.TrimSpace(f.TimeRange.EndTime) != "") {
		fields = append(fields, "time_range")
	}
	sort.Strings(fields)
	return fields
}

func buildSearchCreateTime(rng *triageTimeRange) map[string]interface{} {
	if rng == nil {
		return nil
	}
	createTime := map[string]interface{}{}
	if start := strings.TrimSpace(rng.StartTime); start != "" {
		createTime["start_time"] = start
	}
	if end := strings.TrimSpace(rng.EndTime); end != "" {
		createTime["end_time"] = end
	}
	return createTime
}

// shellQuote wraps a string in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// resolveTriagePath determines whether to use the search API path,
// validating that --page-token prefix is consistent with query/filter params.
//
// Rules:
//   - No token: path decided by usesTriageSearchPath(query, filter).
//   - "search:" prefix: must not have list-only params (no query/search filter fields is OK for continuation).
//   - "list:" prefix: must not have query or search-only filter fields that would be silently ignored.
//   - Bare token (no prefix): rejected — all tokens emitted by triage carry a prefix.
func resolveTriagePath(parsed triagePageToken, query string, filter triageFilter) (useSearch bool, err error) {
	if parsed.RawToken == "" {
		return usesTriageSearchPath(query, filter), nil
	}
	paramWantsSearch := usesTriageSearchPath(query, filter)
	switch parsed.Path {
	case "search":
		if !paramWantsSearch && (strings.TrimSpace(query) != "" || len(triageQueryFilterFields(filter)) > 0) {
			return false, fmt.Errorf("--page-token has search: prefix but current --query/--filter parameters indicate list path; remove conflicting parameters or use the correct token")
		}
		return true, nil
	case "list":
		if paramWantsSearch {
			return false, fmt.Errorf("--page-token has list: prefix but --query or --filter contains search-only fields (e.g. from/to/subject); these parameters would be silently ignored — remove them or use a search: token")
		}
		return false, nil
	default:
		return false, fmt.Errorf("invalid --page-token: must start with 'search:' or 'list:' prefix (token was obtained from a previous mail +triage response)")
	}
}

// triagePageToken represents a parsed pagination token.
type triagePageToken struct {
	Path     string // "search" or "list"
	RawToken string // the actual API token
}

// encodeTriagePageToken encodes a pagination token with path prefix.
// Format: "search:abc123" or "list:abc123".
func encodeTriagePageToken(path string, rawToken string) string {
	if rawToken == "" {
		return ""
	}
	return path + ":" + rawToken
}

// parseTriagePageToken parses a token encoded by encodeTriagePageToken.
// Returns an error for bare tokens or malformed tokens.
func parseTriagePageToken(token string) (triagePageToken, error) {
	if token == "" {
		return triagePageToken{}, nil
	}
	idx := strings.IndexByte(token, ':')
	if idx < 0 {
		return triagePageToken{}, fmt.Errorf("invalid --page-token: must start with 'search:' or 'list:' prefix (token was obtained from a previous mail +triage response)")
	}
	path := token[:idx]
	raw := token[idx+1:]
	if path != "search" && path != "list" {
		return triagePageToken{}, fmt.Errorf("invalid --page-token: must start with 'search:' or 'list:' prefix, got %q", path)
	}
	if raw == "" {
		return triagePageToken{}, fmt.Errorf("invalid --page-token: token value is empty after '%s:' prefix", path)
	}
	return triagePageToken{Path: path, RawToken: raw}, nil
}

// resolveTriagePageSize returns the effective max count from --page-size or --max.
// --page-size is an alias for --max; if both are set, --page-size takes priority.
func resolveTriagePageSize(runtime *common.RuntimeContext) int {
	if ps := runtime.Int("page-size"); ps > 0 {
		return normalizeTriageMax(ps)
	}
	return normalizeTriageMax(runtime.Int("max"))
}

func normalizeTriageMax(maxCount int) int {
	if maxCount <= 0 {
		return 20
	}
	if maxCount > triageMaxLimit {
		return triageMaxLimit
	}
	return maxCount
}

func resolveSearchFolderFilter(runtime *common.RuntimeContext, mailboxID string, f triageFilter, dryRun bool) (string, error) {
	if value := strings.TrimSpace(f.FolderID); value != "" {
		if dryRun {
			if id, ok := resolveFolderSystemAliasOrID(value); ok {
				return folderSystemIDToAlias[id], nil
			}
			return value, nil
		}
		return resolveFolderQueryNameFromID(runtime, mailboxID, value)
	}
	if value := strings.TrimSpace(f.Folder); value != "" {
		if dryRun {
			if searchOnlyFolderNames[strings.ToLower(value)] {
				return strings.ToLower(value), nil
			}
			if id, ok := resolveFolderSystemAliasOrID(value); ok {
				return folderSystemIDToAlias[id], nil
			}
			return value, nil
		}
		return resolveFolderQueryName(runtime, mailboxID, value)
	}
	return "", nil
}

func resolveSearchLabelFilter(runtime *common.RuntimeContext, mailboxID string, f triageFilter, dryRun bool) (string, error) {
	if value := strings.TrimSpace(f.LabelID); value != "" {
		if dryRun {
			if id, ok := resolveLabelSystemID(value); ok {
				return systemLabelSearchName[id], nil
			}
			return value, nil
		}
		return resolveLabelQueryNameFromID(runtime, mailboxID, value)
	}
	if value := strings.TrimSpace(f.Label); value != "" {
		if dryRun {
			if id, ok := resolveLabelSystemID(value); ok {
				return systemLabelSearchName[id], nil
			}
			return value, nil
		}
		return resolveLabelQueryName(runtime, mailboxID, value)
	}
	return "", nil
}

func trimStringList(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		trimmed = append(trimmed, value)
	}
	return trimmed
}

func extractTriageMessageIDs(raw interface{}) []string {
	rawItems, _ := raw.([]interface{})
	messageIDs := make([]string, 0, len(rawItems))
	for _, item := range rawItems {
		msgID, _ := item.(string)
		if msgID == "" {
			if m, ok := item.(map[string]interface{}); ok {
				msgID, _ = m["message_id"].(string)
				if msgID == "" {
					msgID, _ = m["id"].(string)
				}
			}
		}
		if msgID != "" {
			messageIDs = append(messageIDs, msgID)
		}
	}
	return messageIDs
}

func formatAddress(addr map[string]interface{}) string {
	name, _ := addr["name"].(string)
	email, _ := addr["mail_address"].(string)
	if email == "" {
		email, _ = addr["address"].(string)
	}
	if name != "" && email != "" {
		return fmt.Sprintf("%s <%s>", name, email)
	}
	if email != "" {
		return email
	}
	return name
}

func doJSONAPI(runtime *common.RuntimeContext, req *larkcore.ApiReq, action string) (map[string]interface{}, error) {
	var lastErr error
	for attempt := 0; attempt <= triageAPIRetries; attempt++ {
		apiResp, err := runtime.DoAPI(req)
		if err == nil {
			var result interface{}
			dec := json.NewDecoder(bytes.NewReader(apiResp.RawBody))
			dec.UseNumber()
			if err := dec.Decode(&result); err != nil {
				return nil, output.Errorf(output.ExitAPI, "api_error", "%s: response parse error: %s", action, err)
			}
			data, handleErr := common.HandleApiResult(result, nil, action)
			if handleErr == nil {
				return data, nil
			}
			lastErr = handleErr
			if !shouldRetryTriageAPIError(handleErr) || attempt == triageAPIRetries {
				return nil, handleErr
			}
		} else {
			lastErr = output.Errorf(output.ExitAPI, "api_error", "%s: %s", action, err)
			if attempt == triageAPIRetries {
				return nil, lastErr
			}
		}
		time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
	}
	return nil, lastErr
}

func shouldRetryTriageAPIError(err error) bool {
	exitErr, ok := err.(*output.ExitError)
	if !ok || exitErr.Detail == nil {
		return false
	}
	return exitErr.Detail.Type == "rate_limit" || exitErr.Code == output.ExitNetwork
}

func toQueryParams(params map[string]interface{}) larkcore.QueryParams {
	queryParams := make(larkcore.QueryParams)
	for key, value := range params {
		queryParams.Set(key, fmt.Sprintf("%v", value))
	}
	return queryParams
}
