// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	allowBlockTypeAllow = "allow"
	allowBlockTypeBlock = "block"
	allowBlockTypeAll   = "all"

	allowBlockResourceAllow = "allow_senders"
	allowBlockResourceBlock = "blocked_senders"

	defaultAllowBlockPageSize = "50"
	maxAllowBlockAddresses    = 100
	maxAllowBlockQueryLength  = 255
)

var allowBlockReadScopes = []string{"mail:user_mailbox.message:readonly", "mail:user_mailbox.message:modify"}
var allowBlockWriteScopes = []string{"mail:user_mailbox.message:modify"}

type allowBlockListOutput struct {
	MailboxID     string                     `json:"mailbox_id"`
	Type          string                     `json:"type"`
	Items         []allowBlockListOutputItem `json:"items"`
	HasMore       bool                       `json:"has_more,omitempty"`
	NextPageToken string                     `json:"next_page_token,omitempty"`
	Allow         map[string]interface{}     `json:"allow,omitempty"`
	Block         map[string]interface{}     `json:"block,omitempty"`
}

type allowBlockListOutputItem struct {
	Type string                 `json:"type"`
	Item map[string]interface{} `json:"item"`
}

type allowBlockBatchOutput struct {
	MailboxID    string                   `json:"mailbox_id"`
	Type         string                   `json:"type"`
	Requested    int                      `json:"requested"`
	SuccessCount int                      `json:"success_count"`
	FailedItems  []map[string]interface{} `json:"failed_items,omitempty"`
	Response     map[string]interface{}   `json:"response"`
}

// MailAllowBlockList lists or searches the current user's mail allow/block
// sender list. --type all fans out to both resources and merges the output.
var MailAllowBlockList = common.Shortcut{
	Service:     "mail",
	Command:     "+allow-block-list",
	Description: "List or search the current user's mail allow/block sender lists. Use --type allow, block, or all; --type all calls both resources and merges the result.",
	Risk:        "read",
	Scopes:      allowBlockReadScopes,
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "email address (default: me)"},
		{Name: "type", Default: allowBlockTypeAll, Enum: []string{allowBlockTypeAllow, allowBlockTypeBlock, allowBlockTypeAll}, Desc: "Which list to read: allow, block, or all."},
		{Name: "query", Desc: "Optional sender address/domain keyword. Empty means list mode."},
		{Name: "page-size", Type: "int", Default: defaultAllowBlockPageSize, Desc: "Page size, 1-100."},
		{Name: "page-token", Desc: "Cursor from a previous response."},
	},
	Validate: validateAllowBlockList,
	DryRun:   dryRunAllowBlockList,
	Execute:  executeAllowBlockList,
}

// MailAllowBlockSet adds sender addresses/domains to the allow or block list.
var MailAllowBlockSet = common.Shortcut{
	Service:     "mail",
	Command:     "+allow-block-set",
	Description: "Add addresses or domains to the current user's mail allow/block sender list.",
	Risk:        "write",
	Scopes:      allowBlockWriteScopes,
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "email address (default: me)"},
		{Name: "type", Required: true, Enum: []string{allowBlockTypeAllow, allowBlockTypeBlock}, Desc: "Target list: allow or block."},
		{Name: "address", Type: "string_slice", Required: true, Desc: "Sender addresses or domains to add; repeat the flag or pass comma-separated values (max 100)."},
		{Name: "scene", Default: "sender", Enum: []string{"sender", "web_image"}, Desc: "Write scene: sender or web_image."},
	},
	Validate: validateAllowBlockSet,
	DryRun:   dryRunAllowBlockSet,
	Execute:  executeAllowBlockSet,
}

// MailAllowBlockDelete removes sender addresses/domains from the allow or
// block list.
var MailAllowBlockDelete = common.Shortcut{
	Service:     "mail",
	Command:     "+allow-block-delete",
	Description: "Remove addresses or domains from the current user's mail allow/block sender list.",
	Risk:        "write",
	Scopes:      allowBlockWriteScopes,
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "email address (default: me)"},
		{Name: "type", Required: true, Enum: []string{allowBlockTypeAllow, allowBlockTypeBlock}, Desc: "Target list: allow or block."},
		{Name: "address", Type: "string_slice", Required: true, Desc: "Sender addresses or domains to remove; repeat the flag or pass comma-separated values (max 100)."},
	},
	Validate: validateAllowBlockDelete,
	DryRun:   dryRunAllowBlockDelete,
	Execute:  executeAllowBlockDelete,
}

func validateAllowBlockList(ctx context.Context, runtime *common.RuntimeContext) error {
	if err := validateBotMailboxNotMe(runtime); err != nil {
		return err
	}
	if err := validateAllowBlockType(runtime.Str("type"), true); err != nil {
		return err
	}
	if query := runtime.Str("query"); len(query) > maxAllowBlockQueryLength {
		return mailValidationParamError("--query", "--query must be at most %d characters", maxAllowBlockQueryLength)
	}
	pageSize := runtime.Int("page-size")
	if pageSize < 1 || pageSize > 100 {
		return mailValidationParamError("--page-size", "--page-size must be between 1 and 100")
	}
	return nil
}

func validateAllowBlockSet(ctx context.Context, runtime *common.RuntimeContext) error {
	if err := validateBotMailboxNotMe(runtime); err != nil {
		return err
	}
	if err := validateAllowBlockType(runtime.Str("type"), false); err != nil {
		return err
	}
	if err := validateAllowBlockAddresses(runtime.StrSlice("address")); err != nil {
		return err
	}
	scene := runtime.Str("scene")
	if scene != "sender" && scene != "web_image" {
		return mailValidationParamError("--scene", "--scene must be sender or web_image")
	}
	return nil
}

func validateAllowBlockDelete(ctx context.Context, runtime *common.RuntimeContext) error {
	if err := validateBotMailboxNotMe(runtime); err != nil {
		return err
	}
	if err := validateAllowBlockType(runtime.Str("type"), false); err != nil {
		return err
	}
	return validateAllowBlockAddresses(runtime.StrSlice("address"))
}

func validateAllowBlockType(typ string, allowAll bool) error {
	switch typ {
	case allowBlockTypeAllow, allowBlockTypeBlock:
		return nil
	case allowBlockTypeAll:
		if allowAll {
			return nil
		}
		return mailValidationParamError("--type", "--type all is only supported by +allow-block-list; use allow or block")
	default:
		if allowAll {
			return mailValidationParamError("--type", "--type must be allow, block, or all")
		}
		return mailValidationParamError("--type", "--type must be allow or block")
	}
}

func validateAllowBlockAddresses(raw []string) error {
	addresses := normalizeAllowBlockAddresses(raw)
	if len(addresses) == 0 {
		return mailValidationParamError("--address", "--address is required; provide at least one address or domain")
	}
	if len(addresses) > maxAllowBlockAddresses {
		return mailValidationParamError("--address", "--address accepts at most %d values", maxAllowBlockAddresses)
	}
	return nil
}

func normalizeAllowBlockAddresses(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, value := range raw {
		addr := strings.TrimSpace(value)
		if addr == "" {
			continue
		}
		key := strings.ToLower(addr)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, addr)
	}
	return out
}

func dryRunAllowBlockList(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	mailboxID := resolveMailboxID(runtime)
	resource := allowBlockResource(runtime.Str("type"))
	desc := "List or search mail allow/block senders"
	if runtime.Str("type") == allowBlockTypeAll {
		resource = allowBlockResourceAllow
		desc = "List or search mail allow/block senders; execution also calls blocked_senders when --type all"
	}
	query := map[string]interface{}{
		"page_size": runtime.Int("page-size"),
	}
	if runtime.Str("query") != "" {
		query["keyword"] = runtime.Str("query")
	}
	if runtime.Str("page-token") != "" {
		query["page_token"] = runtime.Str("page-token")
	}
	return common.NewDryRunAPI().
		Desc(desc).
		GET(mailboxPath(mailboxID, resource)).
		Params(query)
}

func dryRunAllowBlockSet(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	mailboxID := resolveMailboxID(runtime)
	return common.NewDryRunAPI().
		Desc("Add mail allow/block senders").
		POST(mailboxPath(mailboxID, allowBlockResource(runtime.Str("type")), "batch_create")).
		Body(map[string]interface{}{"items": buildAllowBlockItems(runtime)})
}

func dryRunAllowBlockDelete(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	mailboxID := resolveMailboxID(runtime)
	return common.NewDryRunAPI().
		Desc("Remove mail allow/block senders").
		POST(mailboxPath(mailboxID, allowBlockResource(runtime.Str("type")), "batch_remove")).
		Body(map[string]interface{}{"senders": normalizeAllowBlockAddresses(runtime.StrSlice("address"))})
}

func executeAllowBlockList(ctx context.Context, runtime *common.RuntimeContext) error {
	mailboxID := resolveMailboxID(runtime)
	typ := runtime.Str("type")
	query := allowBlockListQuery(runtime)
	if typ != allowBlockTypeAll {
		data, err := callAllowBlockList(runtime, mailboxID, typ, query)
		if err != nil {
			return decorateAllowBlockAPIError(err)
		}
		out := buildAllowBlockListOutput(mailboxID, typ, data)
		runtime.OutFormat(out, &output.Meta{Count: len(out.Items)}, nil)
		return nil
	}

	allowData, err := callAllowBlockList(runtime, mailboxID, allowBlockTypeAllow, query)
	if err != nil {
		return decorateAllowBlockAPIError(err)
	}
	blockData, err := callAllowBlockList(runtime, mailboxID, allowBlockTypeBlock, query)
	if err != nil {
		return decorateAllowBlockAPIError(err)
	}
	out := mergeAllowBlockListOutput(mailboxID, allowData, blockData)
	runtime.OutFormat(out, &output.Meta{Count: len(out.Items)}, nil)
	return nil
}

func executeAllowBlockSet(ctx context.Context, runtime *common.RuntimeContext) error {
	mailboxID := resolveMailboxID(runtime)
	typ := runtime.Str("type")
	addresses := normalizeAllowBlockAddresses(runtime.StrSlice("address"))
	data, err := runtime.CallAPITyped("POST",
		mailboxPath(mailboxID, allowBlockResource(typ), "batch_create"),
		nil, map[string]interface{}{"items": buildAllowBlockItems(runtime)})
	if err != nil {
		return decorateAllowBlockAPIError(err)
	}
	out := buildAllowBlockBatchOutput(mailboxID, typ, len(addresses), data)
	emitAllowBlockFailedItemsWarning(runtime, out.FailedItems)
	runtime.OutFormat(out, &output.Meta{Count: out.SuccessCount}, nil)
	return nil
}

func executeAllowBlockDelete(ctx context.Context, runtime *common.RuntimeContext) error {
	mailboxID := resolveMailboxID(runtime)
	typ := runtime.Str("type")
	addresses := normalizeAllowBlockAddresses(runtime.StrSlice("address"))
	data, err := runtime.CallAPITyped("POST",
		mailboxPath(mailboxID, allowBlockResource(typ), "batch_remove"),
		nil, map[string]interface{}{"senders": addresses})
	if err != nil {
		return decorateAllowBlockAPIError(err)
	}
	out := buildAllowBlockBatchOutput(mailboxID, typ, len(addresses), data)
	runtime.OutFormat(out, &output.Meta{Count: out.SuccessCount}, nil)
	return nil
}

func allowBlockResource(typ string) string {
	if typ == allowBlockTypeAllow {
		return allowBlockResourceAllow
	}
	return allowBlockResourceBlock
}

func allowBlockListQuery(runtime *common.RuntimeContext) map[string]interface{} {
	query := map[string]interface{}{
		"page_size": runtime.Int("page-size"),
	}
	if keyword := strings.TrimSpace(runtime.Str("query")); keyword != "" {
		query["keyword"] = keyword
	}
	if token := strings.TrimSpace(runtime.Str("page-token")); token != "" {
		query["page_token"] = token
	}
	return query
}

func callAllowBlockList(runtime *common.RuntimeContext, mailboxID, typ string, query map[string]interface{}) (map[string]interface{}, error) {
	return runtime.CallAPITyped("GET", mailboxPath(mailboxID, allowBlockResource(typ)), query, nil)
}

func buildAllowBlockItems(runtime *common.RuntimeContext) []map[string]interface{} {
	addresses := normalizeAllowBlockAddresses(runtime.StrSlice("address"))
	items := make([]map[string]interface{}, 0, len(addresses))
	scene := runtime.Str("scene")
	for _, address := range addresses {
		items = append(items, map[string]interface{}{
			"sender":      address,
			"sender_type": allowBlockSenderType(address),
			"scene":       scene,
		})
	}
	return items
}

func allowBlockSenderType(sender string) int {
	if strings.Contains(sender, "@") {
		return 1
	}
	return 2
}

func buildAllowBlockListOutput(mailboxID, typ string, data map[string]interface{}) allowBlockListOutput {
	items := extractAllowBlockItems(typ, data)
	return allowBlockListOutput{
		MailboxID:     mailboxID,
		Type:          typ,
		Items:         items,
		HasMore:       boolVal(data["has_more"]),
		NextPageToken: strVal(data["next_page_token"]),
	}
}

func mergeAllowBlockListOutput(mailboxID string, allowData, blockData map[string]interface{}) allowBlockListOutput {
	items := append(extractAllowBlockItems(allowBlockTypeAllow, allowData), extractAllowBlockItems(allowBlockTypeBlock, blockData)...)
	return allowBlockListOutput{
		MailboxID: mailboxID,
		Type:      allowBlockTypeAll,
		Items:     items,
		HasMore:   boolVal(allowData["has_more"]) || boolVal(blockData["has_more"]),
		Allow: map[string]interface{}{
			"has_more":        boolVal(allowData["has_more"]),
			"next_page_token": strVal(allowData["next_page_token"]),
		},
		Block: map[string]interface{}{
			"has_more":        boolVal(blockData["has_more"]),
			"next_page_token": strVal(blockData["next_page_token"]),
		},
	}
}

func extractAllowBlockItems(typ string, data map[string]interface{}) []allowBlockListOutputItem {
	raw, _ := data["items"].([]interface{})
	items := make([]allowBlockListOutputItem, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]interface{}); ok {
			items = append(items, allowBlockListOutputItem{Type: typ, Item: m})
		}
	}
	return items
}

func buildAllowBlockBatchOutput(mailboxID, typ string, requested int, data map[string]interface{}) allowBlockBatchOutput {
	failedItems := extractAllowBlockFailedItems(data)
	successCount := intVal(data["success_count"])
	if successCount == 0 {
		successCount = intVal(data["added_count"])
	}
	if successCount == 0 {
		successCount = intVal(data["deleted_count"])
	}
	if successCount == 0 && len(failedItems) == 0 {
		successCount = requested
	}
	return allowBlockBatchOutput{
		MailboxID:    mailboxID,
		Type:         typ,
		Requested:    requested,
		SuccessCount: successCount,
		FailedItems:  failedItems,
		Response:     data,
	}
}

func extractAllowBlockFailedItems(data map[string]interface{}) []map[string]interface{} {
	raw, _ := data["failed_items"].([]interface{})
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

func emitAllowBlockFailedItemsWarning(runtime *common.RuntimeContext, failedItems []map[string]interface{}) {
	if len(failedItems) == 0 {
		return
	}
	fmt.Fprintf(runtime.IO().ErrOut, "warning: %d allow/block item(s) were not applied; inspect failed_items in stdout\n", len(failedItems))
}

func decorateAllowBlockAPIError(err error) error {
	err = mailDecorateProblemMessage(err, "mail allow/block API failed")
	if strings.Contains(strings.ToLower(err.Error()), "cache") || strings.Contains(err.Error(), "456") {
		return mailAppendProblemHint(err, "search cache may still be building; retry later or list without --query")
	}
	if strings.Contains(strings.ToLower(err.Error()), "self address") {
		return mailAppendProblemHint(err, "do not add your own email address to the allow/block list")
	}
	if strings.Contains(strings.ToLower(err.Error()), "self domain") {
		return mailAppendProblemHint(err, "do not add your tenant internal domain to the allow/block list")
	}
	return err
}
