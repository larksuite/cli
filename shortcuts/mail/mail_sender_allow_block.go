// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"io"
	netmail "net/mail"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	senderListAllow = "allow"
	senderListBlock = "block"
	senderListAll   = "all"

	maxSenderAddressCount = 100
	maxSenderQueryLength  = 255
	defaultSenderPageSize = 50
)

type senderRecord struct {
	Address   string      `json:"address"`
	Timestamp interface{} `json:"timestamp,omitempty"`
	ListType  string      `json:"list_type"`
}

type senderListOutput struct {
	Items          []senderRecord    `json:"items"`
	HasMore        bool              `json:"has_more,omitempty"`
	NextPageToken  string            `json:"next_page_token,omitempty"`
	NextPageTokens map[string]string `json:"next_page_tokens,omitempty"`
	ListType       string            `json:"list_type"`
	Total          int               `json:"total"`
}

var MailSenderList = common.Shortcut{
	Service:     "mail",
	Command:     "+sender-list",
	Description: "List user-level mail sender allow/block entries. Use --type allow, block, or all.",
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox email address (default: me)"},
		{Name: "type", Default: senderListAll, Desc: "Sender list type: allow, block, or all", Enum: []string{senderListAllow, senderListBlock, senderListAll}},
		{Name: "page-size", Type: "int", Default: fmt.Sprintf("%d", defaultSenderPageSize), Desc: "Page size, 1-100"},
		{Name: "page-token", Desc: "Page token returned by a previous list/query call"},
	},
	Validate: validateSenderRead,
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return dryRunSenderRead(runtime, "")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		out, err := executeSenderRead(runtime, "")
		if err != nil {
			return err
		}
		outputSenderList(runtime, out)
		return nil
	},
}

var MailSenderQuery = common.Shortcut{
	Service:     "mail",
	Command:     "+sender-query",
	Description: "Query user-level mail sender allow/block entries. Use --exact for case-insensitive exact address filtering.",
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox email address (default: me)"},
		{Name: "type", Default: senderListAll, Desc: "Sender list type: allow, block, or all", Enum: []string{senderListAllow, senderListBlock, senderListAll}},
		{Name: "query", Desc: "Required. Keyword to search, up to 255 characters"},
		{Name: "page-size", Type: "int", Default: fmt.Sprintf("%d", defaultSenderPageSize), Desc: "Page size, 1-100"},
		{Name: "page-token", Desc: "Page token returned by a previous list/query call"},
		{Name: "exact", Type: "bool", Desc: "Filter results to addresses that exactly match --query, case-insensitive"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateSenderRead(ctx, runtime); err != nil {
			return err
		}
		query := strings.TrimSpace(runtime.Str("query"))
		if query == "" {
			return mailValidationParamError("--query", "--query is required")
		}
		if len([]rune(query)) > maxSenderQueryLength {
			return mailValidationParamError("--query", "--query must be at most %d characters", maxSenderQueryLength)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return dryRunSenderRead(runtime, strings.TrimSpace(runtime.Str("query")))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		query := strings.TrimSpace(runtime.Str("query"))
		out, err := executeSenderRead(runtime, query)
		if err != nil {
			return err
		}
		if runtime.Bool("exact") {
			filtered := out.Items[:0]
			for _, item := range out.Items {
				if strings.EqualFold(item.Address, query) {
					filtered = append(filtered, item)
				}
			}
			out.Items = filtered
			out.Total = len(filtered)
		}
		outputSenderList(runtime, out)
		return nil
	},
}

var MailSenderSet = common.Shortcut{
	Service:     "mail",
	Command:     "+sender-set",
	Description: "Add sender addresses to the user-level allow or block list.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox email address (default: me)"},
		{Name: "type", Desc: "Sender list type: allow or block", Enum: []string{senderListAllow, senderListBlock}},
		{Name: "address", Type: "string_slice", Desc: "Sender email address(es). Repeat flag or use comma-separated values; maximum 100."},
	},
	Validate: validateSenderWrite,
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		addresses, _ := parseSenderAddressList(runtime.StrSlice("address"), true)
		body := map[string]interface{}{"items": senderAddressItems(addresses)}
		return common.NewDryRunAPI().
			Desc("Add sender addresses to the selected user-level allow/block list").
			POST(senderAllowBlockPath(resolveMailboxID(runtime), runtime.Str("type"), "batch_create")).
			Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		addresses, err := parseSenderAddressList(runtime.StrSlice("address"), true)
		if err != nil {
			return err
		}
		data, err := runtime.DoAPIJSONTyped("POST",
			senderAllowBlockPath(resolveMailboxID(runtime), runtime.Str("type"), "batch_create"),
			nil,
			map[string]interface{}{"items": senderAddressItems(addresses)})
		if err != nil {
			return decorateSenderAPIError(err, "set sender list")
		}
		out := map[string]interface{}{
			"list_type":    runtime.Str("type"),
			"addresses":    addresses,
			"failed_items": normalizeSenderFailedItems(data["failed_items"]),
		}
		runtime.OutFormat(out, &output.Meta{Count: len(addresses)}, nil)
		return nil
	},
}

var MailSenderDelete = common.Shortcut{
	Service:     "mail",
	Command:     "+sender-delete",
	Description: "Delete sender addresses from the user-level allow or block list.",
	Risk:        "delete",
	Scopes:      []string{"mail:user_mailbox"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox email address (default: me)"},
		{Name: "type", Desc: "Sender list type: allow or block", Enum: []string{senderListAllow, senderListBlock}},
		{Name: "address", Type: "string_slice", Desc: "Sender email address(es). Repeat flag or use comma-separated values; maximum 100."},
	},
	Validate: validateSenderWrite,
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		addresses, _ := parseSenderAddressList(runtime.StrSlice("address"), false)
		return common.NewDryRunAPI().
			Desc("Delete sender addresses from the selected user-level allow/block list").
			POST(senderAllowBlockPath(resolveMailboxID(runtime), runtime.Str("type"), "batch_remove")).
			Body(map[string]interface{}{"senders": addresses})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		addresses, err := parseSenderAddressList(runtime.StrSlice("address"), false)
		if err != nil {
			return err
		}
		data, err := runtime.DoAPIJSONTyped("POST",
			senderAllowBlockPath(resolveMailboxID(runtime), runtime.Str("type"), "batch_remove"),
			nil,
			map[string]interface{}{"senders": addresses})
		if err != nil {
			return decorateSenderAPIError(err, "delete sender list")
		}
		out := map[string]interface{}{
			"list_type":     runtime.Str("type"),
			"addresses":     addresses,
			"deleted_count": intVal(data["deleted_count"]),
		}
		runtime.OutFormat(out, &output.Meta{Count: len(addresses)}, nil)
		return nil
	},
}

func validateSenderRead(ctx context.Context, runtime *common.RuntimeContext) error {
	if err := validateBotMailboxNotMe(runtime); err != nil {
		return err
	}
	if err := validateSenderListType(runtime.Str("type"), true); err != nil {
		return err
	}
	size := runtime.Int("page-size")
	if size < 1 || size > maxSenderAddressCount {
		return mailValidationParamError("--page-size", "--page-size must be between 1 and %d", maxSenderAddressCount)
	}
	return nil
}

func validateSenderWrite(ctx context.Context, runtime *common.RuntimeContext) error {
	if err := validateBotMailboxNotMe(runtime); err != nil {
		return err
	}
	if err := validateSenderListType(runtime.Str("type"), false); err != nil {
		return err
	}
	_, err := parseSenderAddressList(runtime.StrSlice("address"), runtime.Command() == "+sender-set")
	return err
}

func validateSenderListType(listType string, allowAll bool) error {
	switch listType {
	case senderListAllow, senderListBlock:
		return nil
	case senderListAll:
		if allowAll {
			return nil
		}
		return mailValidationParamError("--type", "--type all is only supported for list/query; use allow or block")
	case "":
		return mailValidationParamError("--type", "--type is required; use allow or block")
	default:
		if allowAll {
			return mailValidationParamError("--type", "--type must be one of: allow, block, all")
		}
		return mailValidationParamError("--type", "--type must be one of: allow, block")
	}
}

func parseSenderAddressList(values []string, normalizeLower bool) ([]string, error) {
	var addresses []string
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			address := strings.TrimSpace(part)
			if address == "" {
				continue
			}
			parsed, err := netmail.ParseAddress(address)
			if err != nil {
				return nil, mailValidationParamError("--address", "invalid email address %q", address)
			}
			address = parsed.Address
			if normalizeLower {
				address = strings.ToLower(address)
			}
			key := strings.ToLower(address)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			addresses = append(addresses, address)
		}
	}
	if len(addresses) == 0 {
		return nil, mailValidationParamError("--address", "--address is required")
	}
	if len(addresses) > maxSenderAddressCount {
		return nil, mailValidationParamError("--address", "--address accepts at most %d entries", maxSenderAddressCount)
	}
	return addresses, nil
}

func dryRunSenderRead(runtime *common.RuntimeContext, keyword string) *common.DryRunAPI {
	api := common.NewDryRunAPI().Desc("List or query user-level sender allow/block entries")
	for _, listType := range senderReadTypes(runtime.Str("type")) {
		api.GET(senderAllowBlockPath(resolveMailboxID(runtime), listType, ""))
		api.Params(senderReadDryRunQuery(runtime, keyword))
	}
	return api
}

func executeSenderRead(runtime *common.RuntimeContext, keyword string) (senderListOutput, error) {
	listType := runtime.Str("type")
	out := senderListOutput{
		ListType:       listType,
		NextPageTokens: map[string]string{},
	}
	for _, currentType := range senderReadTypes(listType) {
		data, err := runtime.DoAPIJSONTyped("GET",
			senderAllowBlockPath(resolveMailboxID(runtime), currentType, ""),
			senderReadQuery(runtime, keyword),
			nil)
		if err != nil {
			return out, decorateSenderAPIError(err, "read sender list")
		}
		out.Items = append(out.Items, senderRecordsFromData(data["items"], currentType)...)
		if hasMore, _ := data["has_more"].(bool); hasMore {
			out.HasMore = true
		}
		token := strVal(data["page_token"])
		if token != "" {
			out.NextPageTokens[currentType] = token
			if listType != senderListAll {
				out.NextPageToken = token
			}
		}
	}
	sort.SliceStable(out.Items, func(i, j int) bool {
		if out.Items[i].ListType == out.Items[j].ListType {
			return out.Items[i].Address < out.Items[j].Address
		}
		return out.Items[i].ListType < out.Items[j].ListType
	})
	out.Total = len(out.Items)
	if len(out.NextPageTokens) == 0 {
		out.NextPageTokens = nil
	}
	return out, nil
}

func senderReadTypes(listType string) []string {
	if listType == senderListAll || listType == "" {
		return []string{senderListAllow, senderListBlock}
	}
	return []string{listType}
}

func senderReadQuery(runtime *common.RuntimeContext, keyword string) larkcore.QueryParams {
	query := larkcore.QueryParams{
		"page_size": []string{fmt.Sprintf("%d", runtime.Int("page-size"))},
	}
	if keyword != "" {
		query["keyword"] = []string{keyword}
	}
	if token := runtime.Str("page-token"); token != "" {
		query["page_token"] = []string{token}
	}
	return query
}

func senderReadDryRunQuery(runtime *common.RuntimeContext, keyword string) map[string]interface{} {
	query := map[string]interface{}{
		"page_size": runtime.Int("page-size"),
	}
	if keyword != "" {
		query["keyword"] = keyword
	}
	if token := runtime.Str("page-token"); token != "" {
		query["page_token"] = token
	}
	return query
}

func senderAllowBlockPath(mailboxID, listType, action string) string {
	resource := "allow_senders"
	if listType == senderListBlock {
		resource = "blocked_senders"
	}
	if action == "" {
		return mailboxPath(mailboxID, resource)
	}
	return mailboxPath(mailboxID, resource, action)
}

func senderAddressItems(addresses []string) []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(addresses))
	for _, address := range addresses {
		items = append(items, map[string]interface{}{"address": address})
	}
	return items
}

func senderRecordsFromData(raw interface{}, listType string) []senderRecord {
	items, _ := raw.([]interface{})
	records := make([]senderRecord, 0, len(items))
	for _, item := range items {
		m, _ := item.(map[string]interface{})
		if len(m) == 0 {
			continue
		}
		records = append(records, senderRecord{
			Address:   strVal(m["address"]),
			Timestamp: m["timestamp"],
			ListType:  listType,
		})
	}
	return records
}

func normalizeSenderFailedItems(raw interface{}) []map[string]interface{} {
	items, _ := raw.([]interface{})
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		m, _ := item.(map[string]interface{})
		if len(m) == 0 {
			continue
		}
		out = append(out, m)
	}
	return out
}

func decorateSenderAPIError(err error, action string) error {
	if p, ok := errs.ProblemOf(err); ok && p.Code == 456 {
		return mailAppendProblemHint(err, "search cache warming; retry later")
	}
	return mailDecorateProblemMessage(err, "%s failed", action)
}

func outputSenderList(runtime *common.RuntimeContext, out senderListOutput) {
	runtime.OutFormat(out, &output.Meta{Count: len(out.Items)}, func(w io.Writer) {
		if len(out.Items) == 0 {
			fmt.Fprintln(w, "No sender entries found.")
			return
		}
		rows := make([]map[string]interface{}, 0, len(out.Items))
		for _, item := range out.Items {
			rows = append(rows, map[string]interface{}{
				"list_type": item.ListType,
				"address":   item.Address,
				"timestamp": item.Timestamp,
			})
		}
		output.PrintTable(w, rows)
		if out.NextPageToken != "" {
			fmt.Fprintf(w, "\nnext_page_token: %s\n", out.NextPageToken)
		}
	})
}
