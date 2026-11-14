// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const mailRulesPageSize = 100

// MailRulesReorder moves the supplied rules to the front in the requested
// order and preserves every other rule in its current relative order.
var MailRulesReorder = common.Shortcut{
	Service:     "mail",
	Command:     "+rules-reorder",
	Description: "Reorder incoming-mail rules. Specified rule IDs are placed first; all remaining rules keep their current order.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.rule:read", "mail:user_mailbox.rule:write"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Desc: "Mailbox address that owns the rules (default: me)."},
		{Name: "rule-ids", Type: "string_array", Required: true, Desc: "Rule IDs to prioritize; comma-separated or repeat the flag."},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := normalizeRuleReorderIDs(runtime.StrArray("rule-ids"))
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailbox := resolveMailboxID(runtime)
		return common.NewDryRunAPI().
			Desc("Read all mail rules, then submit the complete normalized order.").
			GET(mailboxPath(mailbox, "rules")).
			POST(mailboxPath(mailbox, "rules", "reorder")).
			Body(map[string]interface{}{"rule_ids": runtime.StrArray("rule-ids")})
	},
	Execute: executeMailRulesReorder,
}

func executeMailRulesReorder(ctx context.Context, runtime *common.RuntimeContext) error {
	requested, err := normalizeRuleReorderIDs(runtime.StrArray("rule-ids"))
	if err != nil {
		return err
	}

	mailbox := resolveMailboxID(runtime)
	current, err := listAllMailRuleIDs(runtime, mailbox)
	if err != nil {
		return err
	}
	final, err := completeMailRuleOrder(requested, current)
	if err != nil {
		return err
	}
	if _, err := runtime.CallAPITyped("POST", mailboxPath(mailbox, "rules", "reorder"), nil, map[string]interface{}{"rule_ids": final}); err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"rule_ids": final}, nil)
	return nil
}

func normalizeRuleReorderIDs(values []string) ([]string, error) {
	ids := make([]string, 0, len(values))
	for _, value := range values {
		for _, id := range strings.Split(value, ",") {
			id = strings.TrimSpace(id)
			if id == "" {
				return nil, mailValidationParamError("--rule-ids", "rule IDs must not be empty")
			}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, mailValidationParamError("--rule-ids", "at least one rule ID is required")
	}
	return ids, nil
}

func listAllMailRuleIDs(runtime *common.RuntimeContext, mailbox string) ([]string, error) {
	var ids []string
	pageToken := ""
	for {
		params := map[string]interface{}{"page_size": mailRulesPageSize}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		data, err := runtime.CallAPITyped("GET", mailboxPath(mailbox, "rules"), params, nil)
		if err != nil {
			return nil, err
		}
		ids = append(ids, extractMailRuleIDs(data["items"])...)
		hasMore, _ := data["has_more"].(bool)
		pageToken, _ = data["page_token"].(string)
		if !hasMore || pageToken == "" {
			return ids, nil
		}
	}
}

func extractMailRuleIDs(value interface{}) []string {
	items, _ := value.([]interface{})
	ids := make([]string, 0, len(items))
	for _, item := range items {
		rule, _ := item.(map[string]interface{})
		id, _ := rule["rule_id"].(string)
		if id == "" {
			id, _ = rule["id"].(string)
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func completeMailRuleOrder(requested, current []string) ([]string, error) {
	available := make(map[string]struct{}, len(current))
	for _, id := range current {
		available[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(requested))
	final := make([]string, 0, len(current))
	for _, id := range requested {
		if _, ok := available[id]; !ok {
			return nil, mailValidationParamError("--rule-ids", "rule not found: %s", id)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		final = append(final, id)
	}
	for _, id := range current {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			final = append(final, id)
		}
	}
	if len(final) != len(available) {
		return nil, mailInvalidResponseError("rules list contains duplicate or missing rule IDs")
	}
	return final, nil
}
