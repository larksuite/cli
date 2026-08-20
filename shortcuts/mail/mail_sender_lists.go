// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const mailSenderListMaxPageSize = 100

var MailSenderAllowlist = newMailSenderListShortcut(senderListShortcutConfig{
	command:     "+sender-allowlist",
	resource:    "allow_senders",
	description: "List or search the user mailbox sender allowlist.",
})

var MailSenderBlocklist = newMailSenderListShortcut(senderListShortcutConfig{
	command:     "+sender-blocklist",
	resource:    "blocked_senders",
	description: "List or search the user mailbox sender blocklist.",
})

var MailSenderAllowlistModify = newMailSenderListModifyShortcut(senderListShortcutConfig{
	command:     "+sender-allowlist-modify",
	resource:    "allow_senders",
	description: "Add or remove senders in the user mailbox sender allowlist.",
})

var MailSenderBlocklistModify = newMailSenderListModifyShortcut(senderListShortcutConfig{
	command:     "+sender-blocklist-modify",
	resource:    "blocked_senders",
	description: "Add or remove senders in the user mailbox sender blocklist.",
})

type senderListShortcutConfig struct {
	command     string
	resource    string
	description string
}

type senderListModifyInput struct {
	Mode      string
	Addresses []string
}

func newMailSenderListShortcut(cfg senderListShortcutConfig) common.Shortcut {
	return common.Shortcut{
		Service:     "mail",
		Command:     cfg.command,
		Description: cfg.description,
		Risk:        "read",
		Scopes:      []string{"mail:user_mailbox.message:readonly"},
		AuthTypes:   []string{"user"},
		HasFormat:   true,
		Flags: []common.Flag{
			{Name: "mailbox", Default: "me", Desc: "Mailbox email address or user_mailbox_id (default: me)."},
			{Name: "query", Desc: "Prefix keyword to search sender email addresses or domains."},
			{Name: "page-size", Type: "int", Default: "20", Desc: fmt.Sprintf("Page size for list/search mode, range 1-%d.", mailSenderListMaxPageSize)},
			{Name: "page-token", Desc: "Page token returned by a previous list/search response."},
		},
		Validate: func(ctx context.Context, rt *common.RuntimeContext) error {
			pageSize := rt.Int("page-size")
			if pageSize < 1 || pageSize > mailSenderListMaxPageSize {
				return mailValidationParamError("--page-size", "must be between 1 and %d", mailSenderListMaxPageSize)
			}
			return nil
		},
		DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
			mailboxID := resolveMailboxID(rt)
			return common.NewDryRunAPI().
				Desc("List or search user mailbox sender list").
				GET(mailSenderListPath(mailboxID, cfg.resource)).
				Params(mailSenderListParams(rt))
		},
		Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
			mailboxID := resolveMailboxID(rt)
			data, err := callMailSenderListSkippingEmptyPages(rt, mailSenderListPath(mailboxID, cfg.resource), mailSenderListParams(rt))
			if err != nil {
				return err
			}
			rt.Out(data, nil)
			return nil
		},
	}
}

func newMailSenderListModifyShortcut(cfg senderListShortcutConfig) common.Shortcut {
	return common.Shortcut{
		Service:     "mail",
		Command:     cfg.command,
		Description: cfg.description,
		Risk:        "write",
		Scopes:      []string{"mail:user_mailbox.message:modify"},
		AuthTypes:   []string{"user"},
		HasFormat:   true,
		Flags: []common.Flag{
			{Name: "mailbox", Default: "me", Desc: "Mailbox email address or user_mailbox_id (default: me)."},
			{Name: "add", Type: "string_array", Desc: "Sender email addresses or domains to add; comma-separated or repeat the flag."},
			{Name: "create", Type: "string_array", Desc: "Alias of --add; sender email addresses or domains to add."},
			{Name: "remove", Type: "string_array", Desc: "Sender email addresses or domains to remove; comma-separated or repeat the flag."},
			{Name: "delete", Type: "string_array", Desc: "Alias of --remove; sender email addresses or domains to remove."},
			{Name: "trash", Type: "string_array", Desc: "Alias of --remove; sender email addresses or domains to remove."},
		},
		Validate: func(ctx context.Context, rt *common.RuntimeContext) error {
			_, err := buildSenderListModifyInput(rt)
			return err
		},
		DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
			input, err := buildSenderListModifyInput(rt)
			if err != nil {
				return common.NewDryRunAPI().Set("error", err.Error())
			}
			mailboxID := resolveMailboxID(rt)
			if input.Mode == "add" {
				return common.NewDryRunAPI().
					Desc("Add senders to user mailbox sender list").
					POST(mailSenderListPath(mailboxID, cfg.resource, "batch_create")).
					Body(senderListAddBody(input.Addresses))
			}
			return common.NewDryRunAPI().
				Desc("Remove senders from user mailbox sender list").
				POST(mailSenderListPath(mailboxID, cfg.resource, "batch_remove")).
				Body(map[string]interface{}{"senders": input.Addresses})
		},
		Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
			input, err := buildSenderListModifyInput(rt)
			if err != nil {
				return err
			}
			mailboxID := resolveMailboxID(rt)
			if input.Mode == "add" {
				data, err := rt.CallAPITyped("POST", mailSenderListPath(mailboxID, cfg.resource, "batch_create"), nil, senderListAddBody(input.Addresses))
				if err != nil {
					return err
				}
				rt.Out(data, nil)
				return nil
			}
			data, err := rt.CallAPITyped("POST", mailSenderListPath(mailboxID, cfg.resource, "batch_remove"), nil, map[string]interface{}{"senders": input.Addresses})
			if err != nil {
				return err
			}
			rt.Out(data, nil)
			return nil
		},
	}
}

func callMailSenderListSkippingEmptyPages(rt *common.RuntimeContext, path string, params map[string]interface{}) (map[string]interface{}, error) {
	const maxEmptyPageRetries = 3

	currentParams := cloneSenderListParams(params)
	var data map[string]interface{}
	var err error
	for attempt := 0; attempt <= maxEmptyPageRetries; attempt++ {
		data, err = rt.CallAPITyped("GET", path, currentParams, nil)
		if err != nil {
			return nil, err
		}
		if senderListHasItems(data) {
			return data, nil
		}
		hasMore, pageToken := common.PaginationMeta(data)
		if !hasMore || pageToken == "" || attempt == maxEmptyPageRetries {
			return data, nil
		}
		currentParams = cloneSenderListParams(currentParams)
		currentParams["page_token"] = pageToken
	}
	return data, nil
}

func cloneSenderListParams(params map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(params))
	for key, value := range params {
		cloned[key] = value
	}
	return cloned
}

func senderListHasItems(data map[string]interface{}) bool {
	if items, ok := data["items"].([]interface{}); ok {
		return len(items) > 0
	}
	return false
}

func buildSenderListModifyInput(rt *common.RuntimeContext) (senderListModifyInput, error) {
	addValues := append([]string{}, rt.StrArray("add")...)
	addValues = append(addValues, rt.StrArray("create")...)
	removeValues := append([]string{}, rt.StrArray("remove")...)
	removeValues = append(removeValues, rt.StrArray("delete")...)
	removeValues = append(removeValues, rt.StrArray("trash")...)

	hasAdd := len(addValues) > 0
	hasRemove := len(removeValues) > 0
	switch {
	case hasAdd && hasRemove:
		return senderListModifyInput{}, mailValidationError("add and remove flags are mutually exclusive")
	case hasAdd:
		addresses, err := normalizeMailSenderAddresses(addValues, "--add")
		if err != nil {
			return senderListModifyInput{}, err
		}
		return senderListModifyInput{Mode: "add", Addresses: addresses}, nil
	case hasRemove:
		addresses, err := normalizeMailSenderAddresses(removeValues, "--remove")
		if err != nil {
			return senderListModifyInput{}, err
		}
		return senderListModifyInput{Mode: "remove", Addresses: addresses}, nil
	default:
		return senderListModifyInput{}, mailValidationError("one of --add, --create, --remove, --delete, or --trash is required")
	}
}

func mailSenderListParams(rt *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{"page_size": rt.Int("page-size")}
	if query := strings.TrimSpace(rt.Str("query")); query != "" {
		params["keyword"] = query
	}
	if pageToken := strings.TrimSpace(rt.Str("page-token")); pageToken != "" {
		params["page_token"] = pageToken
	}
	return params
}

func senderListAddBody(addresses []string) map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(addresses))
	for _, address := range addresses {
		items = append(items, map[string]interface{}{
			"sender":      address,
			"sender_type": inferSenderType(address),
		})
	}
	return map[string]interface{}{"items": items}
}

func inferSenderType(address string) int {
	if strings.Contains(address, "@") {
		return 1
	}
	return 2
}

func normalizeMailSenderAddresses(raw []string, param string) ([]string, error) {
	addresses := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, value := range raw {
		for _, part := range strings.Split(value, ",") {
			address := strings.TrimSpace(part)
			if address == "" {
				return nil, mailValidationParamError(param, "must not contain empty values")
			}
			if _, ok := seen[address]; ok {
				continue
			}
			seen[address] = struct{}{}
			addresses = append(addresses, address)
		}
	}
	if len(addresses) == 0 {
		return nil, mailValidationParamError(param, "must include at least one sender")
	}
	return addresses, nil
}

func mailSenderListPath(mailboxID, resource string, segments ...string) string {
	args := make([]string, 0, len(segments)+1)
	args = append(args, resource)
	args = append(args, segments...)
	return mailboxPath(mailboxID, args...)
}
