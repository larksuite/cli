// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	allowBlockTypeAllow = "allow"
	allowBlockTypeBlock = "block"
	allowBlockTypeAll   = "all"

	allowBlockDefaultPageSize = 50
	allowBlockMaxPageSize     = 100
	allowBlockMaxQueryLen     = 255
	allowBlockMaxAddresses    = 100
)

var allowBlockListFlags = []common.Flag{
	{Name: "mailbox", Default: "me", Desc: "Mailbox email address (default: me)."},
	{Name: "type", Default: allowBlockTypeAll, Desc: "Sender list type: allow, block, or all.", Enum: []string{allowBlockTypeAllow, allowBlockTypeBlock, allowBlockTypeAll}},
	{Name: "page-size", Type: "int", Default: fmt.Sprintf("%d", allowBlockDefaultPageSize), Desc: "Page size, 1-100."},
	{Name: "page-token", Desc: "Next page token from the previous response."},
}

var allowBlockWriteFlags = []common.Flag{
	{Name: "mailbox", Default: "me", Desc: "Mailbox email address (default: me)."},
	{Name: "type", Desc: "Sender list type: allow or block.", Required: true, Enum: []string{allowBlockTypeAllow, allowBlockTypeBlock}},
	{Name: "address", Desc: "Comma-separated email addresses or domains."},
	{Name: "address-file", Desc: "Relative path to a file containing one email address or domain per line."},
}

// MailAllowBlockList lists user-level allow and block sender records.
var MailAllowBlockList = common.Shortcut{
	Service:     "mail",
	Command:     "+allow-block-list",
	Description: "List user-level allowed or blocked senders for a mailbox. Defaults to both lists for the current user mailbox.",
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox.message:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags:       allowBlockListFlags,
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateAllowBlockList(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return dryRunAllowBlockListSearch(runtime, "")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeAllowBlockListSearch(runtime, "")
	},
}

// MailAllowBlockSearch searches user-level allow and block sender records.
var MailAllowBlockSearch = common.Shortcut{
	Service:     "mail",
	Command:     "+allow-block-search",
	Description: "Search user-level allowed or blocked senders for a mailbox. Search may return a retry hint when the backend cache is still warming.",
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox.message:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: append(append([]common.Flag{}, allowBlockListFlags...),
		common.Flag{Name: "query", Desc: "Required. Email address, domain, or keyword to search.", Required: true},
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateAllowBlockList(runtime); err != nil {
			return err
		}
		query := strings.TrimSpace(runtime.Str("query"))
		if query == "" {
			return mailValidationParamError("--query", "--query is required for +allow-block-search")
		}
		if len(query) > allowBlockMaxQueryLen {
			return mailValidationParamError("--query", "--query must be at most %d bytes", allowBlockMaxQueryLen)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return dryRunAllowBlockListSearch(runtime, strings.TrimSpace(runtime.Str("query")))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeAllowBlockListSearch(runtime, strings.TrimSpace(runtime.Str("query")))
	},
}

// MailAllowBlockSet adds user-level allowed or blocked senders.
var MailAllowBlockSet = common.Shortcut{
	Service:     "mail",
	Command:     "+allow-block-set",
	Description: "Add email addresses or domains to the user-level allow or block sender list. Adding to one list removes the sender from the opposite list server-side.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags:       allowBlockWriteFlags,
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateAllowBlockWrite(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		addresses, _ := allowBlockAddresses(runtime)
		body := map[string]interface{}{"items": allowBlockSenderItems(addresses)}
		return common.NewDryRunAPI().
			Desc("Add senders to the user-level allow or block list").
			POST(allowBlockResourcePath(resolveMailboxID(runtime), runtime.Str("type")) + "/batch_create").
			Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		addresses, err := allowBlockAddresses(runtime)
		if err != nil {
			return err
		}
		listType := runtime.Str("type")
		data, err := runtime.CallAPITyped("POST",
			allowBlockResourcePath(resolveMailboxID(runtime), listType)+"/batch_create",
			nil,
			map[string]interface{}{"items": allowBlockSenderItems(addresses)},
		)
		if err != nil {
			return allowBlockDecorateAPIError(err)
		}
		out := allowBlockWriteOutput{
			Type:        listType,
			Mailbox:     resolveMailboxID(runtime),
			Addresses:   addresses,
			Requested:   len(addresses),
			FailedItems: interfaceSlice(data["failed_items"]),
			Raw:         data,
		}
		out.Succeeded = out.Requested - len(out.FailedItems)
		runtime.OutFormat(out, &output.Meta{Count: out.Requested}, func(w io.Writer) {
			fmt.Fprintf(w, "Added %d/%d sender(s) to %s list.\n", out.Succeeded, out.Requested, listType)
		})
		return nil
	},
}

// MailAllowBlockDelete removes user-level allowed or blocked senders.
var MailAllowBlockDelete = common.Shortcut{
	Service:     "mail",
	Command:     "+allow-block-delete",
	Description: "Delete email addresses or domains from the user-level allow or block sender list. Sender casing is sent exactly as provided.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags:       allowBlockWriteFlags,
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateAllowBlockWrite(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		addresses, _ := allowBlockAddresses(runtime)
		return common.NewDryRunAPI().
			Desc("Delete senders from the user-level allow or block list; casing is preserved").
			DELETE(allowBlockResourcePath(resolveMailboxID(runtime), runtime.Str("type")) + "/batch_delete").
			Body(map[string]interface{}{"senders": addresses})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		addresses, err := allowBlockAddresses(runtime)
		if err != nil {
			return err
		}
		listType := runtime.Str("type")
		data, err := runtime.CallAPITyped("DELETE",
			allowBlockResourcePath(resolveMailboxID(runtime), listType)+"/batch_delete",
			nil,
			map[string]interface{}{"senders": addresses},
		)
		if err != nil {
			return allowBlockDecorateAPIError(err)
		}
		out := allowBlockDeleteOutput{
			Type:         listType,
			Mailbox:      resolveMailboxID(runtime),
			Addresses:    addresses,
			Requested:    len(addresses),
			DeletedCount: intFromAny(data["deleted_count"]),
			Raw:          data,
		}
		runtime.OutFormat(out, &output.Meta{Count: out.Requested}, func(w io.Writer) {
			fmt.Fprintf(w, "Deleted %d sender(s) from %s list.\n", out.DeletedCount, listType)
		})
		return nil
	},
}

type allowBlockListOutput struct {
	Mailbox string                            `json:"mailbox"`
	Type    string                            `json:"type"`
	Query   string                            `json:"query,omitempty"`
	Lists   map[string]map[string]interface{} `json:"lists"`
}

type allowBlockWriteOutput struct {
	Type        string                 `json:"type"`
	Mailbox     string                 `json:"mailbox"`
	Addresses   []string               `json:"addresses"`
	Requested   int                    `json:"requested"`
	Succeeded   int                    `json:"succeeded"`
	FailedItems []interface{}          `json:"failed_items,omitempty"`
	Raw         map[string]interface{} `json:"raw,omitempty"`
}

type allowBlockDeleteOutput struct {
	Type         string                 `json:"type"`
	Mailbox      string                 `json:"mailbox"`
	Addresses    []string               `json:"addresses"`
	Requested    int                    `json:"requested"`
	DeletedCount int                    `json:"deleted_count"`
	Raw          map[string]interface{} `json:"raw,omitempty"`
}

func validateAllowBlockList(runtime *common.RuntimeContext) error {
	if err := validateAllowBlockType(runtime.Str("type"), true); err != nil {
		return err
	}
	size := runtime.Int("page-size")
	if size < 1 || size > allowBlockMaxPageSize {
		return mailValidationParamError("--page-size", "--page-size must be between 1 and %d", allowBlockMaxPageSize)
	}
	return nil
}

func validateAllowBlockWrite(runtime *common.RuntimeContext) error {
	if err := validateAllowBlockType(runtime.Str("type"), false); err != nil {
		return err
	}
	_, err := allowBlockAddresses(runtime)
	return err
}

func validateAllowBlockType(listType string, allowAll bool) error {
	switch listType {
	case allowBlockTypeAllow, allowBlockTypeBlock:
		return nil
	case allowBlockTypeAll:
		if allowAll {
			return nil
		}
		return mailValidationParamError("--type", "--type all is only supported for list/search; use allow or block")
	default:
		if allowAll {
			return mailValidationParamError("--type", "--type must be one of allow, block, all")
		}
		return mailValidationParamError("--type", "--type must be one of allow, block")
	}
}

func dryRunAllowBlockListSearch(runtime *common.RuntimeContext, query string) *common.DryRunAPI {
	dr := common.NewDryRunAPI().Desc("List or search user-level allow/block senders")
	params := allowBlockListParams(runtime, query)
	for _, listType := range allowBlockSelectedTypes(runtime.Str("type")) {
		dr.GET(allowBlockResourcePath(resolveMailboxID(runtime), listType)).Params(params)
	}
	return dr
}

func executeAllowBlockListSearch(runtime *common.RuntimeContext, query string) error {
	mailboxID := resolveMailboxID(runtime)
	out := allowBlockListOutput{
		Mailbox: mailboxID,
		Type:    runtime.Str("type"),
		Query:   query,
		Lists:   make(map[string]map[string]interface{}, 2),
	}
	for _, listType := range allowBlockSelectedTypes(runtime.Str("type")) {
		data, err := runtime.CallAPITyped("GET", allowBlockResourcePath(mailboxID, listType), allowBlockListParams(runtime, query), nil)
		if err != nil {
			return allowBlockDecorateAPIError(err)
		}
		out.Lists[listType] = data
	}
	runtime.OutFormat(out, &output.Meta{Count: allowBlockOutputCount(out.Lists)}, func(w io.Writer) {
		for _, listType := range allowBlockSelectedTypes(out.Type) {
			items := interfaceSlice(out.Lists[listType]["items"])
			fmt.Fprintf(w, "%s: %d sender(s)\n", listType, len(items))
			for _, item := range items {
				if rec, ok := item.(map[string]interface{}); ok {
					fmt.Fprintf(w, "- %s\n", strVal(rec["sender"]))
				}
			}
		}
	})
	return nil
}

func allowBlockListParams(runtime *common.RuntimeContext, query string) map[string]interface{} {
	params := map[string]interface{}{
		"page_size": runtime.Int("page-size"),
	}
	if token := runtime.Str("page-token"); token != "" {
		params["page_token"] = token
	}
	if query != "" {
		params["keyword"] = query
	}
	return params
}

func allowBlockSelectedTypes(listType string) []string {
	if listType == allowBlockTypeAll || listType == "" {
		return []string{allowBlockTypeAllow, allowBlockTypeBlock}
	}
	return []string{listType}
}

func allowBlockResourcePath(mailboxID, listType string) string {
	resource := "blocked_senders"
	if listType == allowBlockTypeAllow {
		resource = "allow_senders"
	}
	return mailboxPath(mailboxID, resource)
}

func allowBlockAddresses(runtime *common.RuntimeContext) ([]string, error) {
	values := splitAllowBlockAddresses(runtime.Str("address"))
	if file := strings.TrimSpace(runtime.Str("address-file")); file != "" {
		fromFile, err := readAllowBlockAddressFile(file)
		if err != nil {
			return nil, err
		}
		values = append(values, fromFile...)
	}
	values = uniqueNonEmpty(values)
	if len(values) == 0 {
		return nil, mailValidationParamError("--address", "provide at least one sender using --address or --address-file")
	}
	if len(values) > allowBlockMaxAddresses {
		return nil, mailValidationParamError("--address", "sender count must be at most %d per request", allowBlockMaxAddresses)
	}
	return values, nil
}

func splitAllowBlockAddresses(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if v := strings.TrimSpace(part); v != "" {
			values = append(values, v)
		}
	}
	return values
}

func readAllowBlockAddressFile(path string) ([]string, error) {
	safePath, err := validate.SafeInputPath(path)
	if err != nil {
		return nil, mailValidationParamError("--address-file", "unsafe address file path: %s", err).WithCause(err)
	}
	raw, err := vfs.ReadFile(safePath)
	if err != nil {
		return nil, mailFileIOError("cannot read address file %q: %s", err, path, err)
	}
	lines := strings.Split(string(raw), "\n")
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		if v := strings.TrimSpace(line); v != "" {
			values = append(values, v)
		}
	}
	return values, nil
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func allowBlockSenderItems(addresses []string) []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(addresses))
	for _, address := range addresses {
		items = append(items, map[string]interface{}{"sender": address})
	}
	return items
}

func allowBlockDecorateAPIError(err error) error {
	if err == nil {
		return nil
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		return mailDecorateProblemMessage(err, "allow/block sender API failed")
	}
	text := strings.ToLower(p.Message)
	switch {
	case p.Code == 456 || strings.Contains(text, "cache") || strings.Contains(text, "errcacheempty"):
		p.Retryable = true
		appendAllowBlockHint(p, "Search cache is not ready yet. Wait a moment and retry, or run +allow-block-list first to warm the sender cache.")
	case strings.Contains(text, "self_address") || strings.Contains(text, "self address"):
		appendAllowBlockHint(p, "Remove your own primary address or alias from the request; a user cannot add themselves to allow/block lists.")
	case strings.Contains(text, "self_domain") || strings.Contains(text, "self domain"):
		appendAllowBlockHint(p, "Remove internal tenant domains from the request; internal domains cannot be added to user allow/block lists.")
	case strings.Contains(text, "data_invalid") || strings.Contains(text, "invalid"):
		appendAllowBlockHint(p, "Check sender email/domain format and keep each request within 100 senders.")
	}
	return err
}

func appendAllowBlockHint(p *errs.Problem, hint string) {
	if p.Hint == "" {
		p.Hint = hint
		return
	}
	if !strings.Contains(p.Hint, hint) {
		p.Hint += "; " + hint
	}
}

func allowBlockOutputCount(lists map[string]map[string]interface{}) int {
	count := 0
	for _, data := range lists {
		count += len(interfaceSlice(data["items"]))
	}
	return count
}

func interfaceSlice(v interface{}) []interface{} {
	switch items := v.(type) {
	case []interface{}:
		return items
	case []map[string]interface{}:
		out := make([]interface{}, 0, len(items))
		for _, item := range items {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func intFromAny(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case jsonNumber:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

type jsonNumber interface {
	Int64() (int64, error)
}
