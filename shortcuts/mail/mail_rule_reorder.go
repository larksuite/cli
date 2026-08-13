// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// MailRuleReorder is the `+rule-reorder` shortcut: accept a partial rule ID
// list, complete it from the server's current rule order, then submit the
// existing full-list reorder API.
var MailRuleReorder = common.Shortcut{
	Service:     "mail",
	Command:     "+rule-reorder",
	Description: "Reorder inbox rules by rule ID. You may pass only the rules to move first; the CLI reads the current rule order, appends untouched rules, and submits the full order.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.rule:read", "mail:user_mailbox.rule:write"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Desc: "Mailbox email address that owns the rules (default: me)."},
		{Name: "rule-ids", Type: "string_array", Required: true, Desc: "Rule IDs to place first, in order; comma-separated or repeat the flag."},
	},
	Validate: validateRuleReorder,
	DryRun:   dryRunRuleReorder,
	Execute:  executeRuleReorder,
}

type mailRuleOrderEntry struct {
	ID string
}

type mailRuleReorderResult struct {
	MailboxID       string   `json:"mailbox_id"`
	SubmittedRuleID []string `json:"submitted_rule_ids"`
	SubmittedCount  int      `json:"submitted_count"`
	InputCount      int      `json:"input_count"`
	DedupedCount    int      `json:"deduplicated_count"`
}

func validateRuleReorder(ctx context.Context, rt *common.RuntimeContext) error {
	_, err := normalizeRuleReorderInput(rt.StrArray("rule-ids"))
	return err
}

func dryRunRuleReorder(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
	mailboxID := resolveMailboxID(rt)
	input, _ := normalizeRuleReorderInput(rt.StrArray("rule-ids"))
	return common.NewDryRunAPI().
		Desc("Read current inbox rules, complete the submitted rule ID list, then call the existing reorder API with the full order").
		GET(mailboxPath(mailboxID, "rules")).
		POST(mailboxPath(mailboxID, "rules", "reorder")).
		Body(map[string]interface{}{"rule_ids": input})
}

func executeRuleReorder(ctx context.Context, rt *common.RuntimeContext) error {
	mailboxID := resolveMailboxID(rt)
	rawInput := rt.StrArray("rule-ids")
	input, err := normalizeRuleReorderInput(rawInput)
	if err != nil {
		return err
	}

	rules, err := fetchMailRules(rt, mailboxID)
	if err != nil {
		return mailDecorateProblemMessage(err, "list current inbox rules failed")
	}
	completed, err := completeRuleOrder(input, rules)
	if err != nil {
		return err
	}
	_, err = rt.CallAPITyped("POST", mailboxPath(mailboxID, "rules", "reorder"), nil, map[string]interface{}{
		"rule_ids": completed,
	})
	if err != nil {
		return mailDecorateProblemMessage(err, "submit inbox rule reorder failed")
	}

	result := mailRuleReorderResult{
		MailboxID:       mailboxID,
		SubmittedRuleID: completed,
		SubmittedCount:  len(completed),
		InputCount:      len(input),
		DedupedCount:    len(rawInput) - len(input),
	}
	rt.OutFormat(result, &output.Meta{Count: len(completed)}, func(w io.Writer) {
		fmt.Fprintf(w, "reordered %d inbox rules for %s\n", len(completed), mailboxID)
		for i, id := range completed {
			fmt.Fprintf(w, "%d. %s\n", i+1, id)
		}
	})
	return nil
}

func normalizeRuleReorderInput(raw []string) ([]string, error) {
	ids := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		ids = append(ids, id)
		seen[id] = true
	}
	if len(ids) == 0 {
		return nil, mailValidationParamError("--rule-ids", "provide at least one rule ID")
	}
	return ids, nil
}

func fetchMailRules(rt *common.RuntimeContext, mailboxID string) ([]mailRuleOrderEntry, error) {
	data, err := rt.CallAPITyped("GET", mailboxPath(mailboxID, "rules"), nil, nil)
	if err != nil {
		return nil, err
	}
	rawRules, ok := firstRuleList(data)
	if !ok {
		return nil, mailInvalidResponseError("rules list response missing rules")
	}
	rules := make([]mailRuleOrderEntry, 0, len(rawRules))
	for _, raw := range rawRules {
		rule, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		id := strings.TrimSpace(strVal(rule["rule_id"]))
		if id == "" {
			id = strings.TrimSpace(strVal(rule["id"]))
		}
		if id != "" {
			rules = append(rules, mailRuleOrderEntry{ID: id})
		}
	}
	return rules, nil
}

func firstRuleList(data map[string]interface{}) ([]interface{}, bool) {
	for _, key := range []string{"rules", "items", "user_mailbox_rules"} {
		if rules, ok := data[key].([]interface{}); ok {
			return rules, true
		}
	}
	if nested, ok := data["data"].(map[string]interface{}); ok {
		return firstRuleList(nested)
	}
	return nil, false
}

func completeRuleOrder(inputIDs []string, currentRules []mailRuleOrderEntry) ([]string, error) {
	selected, err := normalizeRuleReorderInput(inputIDs)
	if err != nil {
		return nil, err
	}
	if len(currentRules) == 0 {
		return nil, mailFailedPreconditionError("current mailbox has no inbox rules to reorder")
	}

	existing := map[string]bool{}
	for _, rule := range currentRules {
		id := strings.TrimSpace(rule.ID)
		if id != "" {
			existing[id] = true
		}
	}
	if len(existing) == 0 {
		return nil, mailFailedPreconditionError("current mailbox has no inbox rules to reorder")
	}

	missing := make([]string, 0)
	selectedSet := map[string]bool{}
	for _, id := range selected {
		if !existing[id] {
			missing = append(missing, id)
			continue
		}
		selectedSet[id] = true
	}
	if len(missing) > 0 {
		return nil, mailValidationParamError("--rule-ids", "rule ID not found: %s", strings.Join(missing, ", "))
	}

	completed := append([]string(nil), selected...)
	for _, rule := range currentRules {
		id := strings.TrimSpace(rule.ID)
		if id == "" || selectedSet[id] {
			continue
		}
		completed = append(completed, id)
	}
	return completed, nil
}
