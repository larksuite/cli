// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

type mailRuleSummary struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type mailRuleMove struct {
	ID   string `json:"id"`
	From int    `json:"from"`
	To   int    `json:"to"`
}

// MailRuleReorder is the `+reorder-rules` shortcut: it lets callers provide
// a prioritized subset of rule IDs and submits the full reordered list.
var MailRuleReorder = common.Shortcut{
	Service:     "mail",
	Command:     "+reorder-rules",
	Description: "Reorder mail rules by listing the current full order, filling missing rule IDs, and submitting the complete rule_ids list.",
	Risk:        "write",
	Scopes: []string{
		"mail:user_mailbox.rule:write",
		"mail:user_mailbox.rule:read",
	},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox email address or 'me' (default: me)"},
		{Name: "rule-ids", Desc: "Comma-separated rule IDs in the desired order; missing rule IDs are filled automatically"},
		{Name: "append", Type: "bool", Desc: "Place --rule-ids at the end instead of the front"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveMailboxID(runtime)
		given := parseRuleIDs(runtime.Str("rule-ids"))
		api := common.NewDryRunAPI().
			Desc("Reorder mail rules: list all rules, fill missing IDs, skip reorder POST").
			GET(mailboxPath(mailboxID, "rules"))

		currentIDs, nameMap, err := fetchMailRuleOrder(runtime, mailboxID)
		if err != nil {
			return api.Set("error", mailDecorateProblemMessage(err, "list rules").Error())
		}
		if err := validateRuleIDsExist(given, currentIDs); err != nil {
			return api.Set("error", err.Error())
		}
		if len(currentIDs) == 0 {
			return api.
				Set("dry_run", true).
				Set("reordered", false).
				Set("reason", "no rules").
				Set("before", []string{}).
				Set("after", []string{})
		}
		final := reorderRuleIDs(currentIDs, given, runtime.Bool("append"))
		return api.
			Set("dry_run", true).
			Set("before", currentIDs).
			Set("after", final).
			Set("moved", diffRuleMoves(currentIDs, final)).
			Set("rule_name_map", nameMap)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		given := parseRuleIDs(runtime.Str("rule-ids"))
		if len(given) == 0 {
			return mailValidationParamError("--rule-ids", "--rule-ids is required (the rules to prioritize, in order)")
		}
		if dup := firstDuplicateRuleID(given); dup != "" {
			return mailValidationParamError("--rule-ids", "--rule-ids contains duplicate rule id: %s", dup)
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveMailboxID(runtime)
		given := parseRuleIDs(runtime.Str("rule-ids"))
		currentIDs, nameMap, err := fetchMailRuleOrder(runtime, mailboxID)
		if err != nil {
			return mailDecorateProblemMessage(err, "list rules")
		}
		if err := validateRuleIDsExist(given, currentIDs); err != nil {
			return err
		}
		if len(currentIDs) == 0 {
			runtime.Out(map[string]interface{}{
				"reordered": false,
				"reason":    "no rules",
				"before":    []string{},
				"after":     []string{},
			}, nil)
			return nil
		}

		final := reorderRuleIDs(currentIDs, given, runtime.Bool("append"))
		if _, err := runtime.CallAPITyped("POST", mailboxPath(mailboxID, "rules", "reorder"), nil, map[string]interface{}{
			"rule_ids": final,
		}); err != nil {
			return mailAppendProblemHint(
				mailDecorateProblemMessage(err, "reorder rules"),
				`The mail rule set may have changed between list and reorder; list the latest rules and retry mail +reorder-rules.`,
			)
		}

		runtime.Out(map[string]interface{}{
			"reordered":     true,
			"before":        currentIDs,
			"after":         final,
			"moved":         diffRuleMoves(currentIDs, final),
			"rule_name_map": nameMap,
		}, nil)
		return nil
	},
}

func parseRuleIDs(raw string) []string {
	parts := strings.Split(raw, ",")
	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func firstDuplicateRuleID(ids []string) string {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return id
		}
		seen[id] = true
	}
	return ""
}

func fetchMailRuleOrder(runtime *common.RuntimeContext, mailboxID string) ([]string, map[string]string, error) {
	data, err := runtime.CallAPITyped("GET", mailboxPath(mailboxID, "rules"), nil, nil)
	if err != nil {
		return nil, nil, err
	}
	items, ok := data["items"].([]interface{})
	if !ok {
		return nil, nil, mailInvalidResponseError("list rules response missing items array")
	}
	currentIDs := make([]string, 0, len(items))
	nameMap := make(map[string]string, len(items))
	for i, item := range items {
		rule, ok := item.(map[string]interface{})
		if !ok {
			return nil, nil, mailInvalidResponseError("list rules response item %d is not an object", i)
		}
		id := strVal(rule["id"])
		if id == "" {
			return nil, nil, mailInvalidResponseError("list rules response item %d missing string id", i)
		}
		currentIDs = append(currentIDs, id)
		if name := strVal(rule["name"]); name != "" {
			nameMap[id] = name
		}
	}
	return currentIDs, nameMap, nil
}

func validateRuleIDsExist(given, current []string) error {
	if len(given) == 0 || len(current) > 0 {
		if bad := firstUnknownRuleID(given, current); bad != "" {
			return unknownRuleIDError(bad, current)
		}
		return nil
	}
	return mailValidationParamError("--rule-ids", "mailbox has no mail rules; rule id %q cannot be reordered", given[0])
}

func firstUnknownRuleID(given, current []string) string {
	currentSet := make(map[string]bool, len(current))
	for _, id := range current {
		currentSet[id] = true
	}
	for _, id := range given {
		if !currentSet[id] {
			return id
		}
	}
	return ""
}

func unknownRuleIDError(id string, current []string) error {
	return mailValidationParamError("--rule-ids", "rule id %q not found; valid rule ids: %s", id, strings.Join(current, ", "))
}

func reorderRuleIDs(current, given []string, appendMode bool) []string {
	givenSet := make(map[string]bool, len(given))
	for _, id := range given {
		givenSet[id] = true
	}
	rest := make([]string, 0, len(current))
	for _, id := range current {
		if !givenSet[id] {
			rest = append(rest, id)
		}
	}
	out := make([]string, 0, len(current))
	if appendMode {
		out = append(out, rest...)
		out = append(out, given...)
		return out
	}
	out = append(out, given...)
	out = append(out, rest...)
	return out
}

func diffRuleMoves(before, after []string) []mailRuleMove {
	beforePos := make(map[string]int, len(before))
	for i, id := range before {
		beforePos[id] = i
	}
	moves := make([]mailRuleMove, 0)
	for to, id := range after {
		from, ok := beforePos[id]
		if !ok || from == to {
			continue
		}
		moves = append(moves, mailRuleMove{ID: id, From: from, To: to})
	}
	return moves
}
