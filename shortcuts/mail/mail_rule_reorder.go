// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

type mailRuleOrderItem struct {
	ID       string
	Sequence *int64
	Index    int
}

// MailRuleReorder lets callers provide the rule IDs they want at the front.
// Execute fetches the current full rule list, appends untouched rules in their
// current relative order, then submits the full list required by the API.
var MailRuleReorder = common.Shortcut{
	Service:     "mail",
	Command:     "+rule-reorder",
	Description: "Reorder mail rules by providing the rule IDs to move to the front. The CLI fetches current rules and submits the complete rule ID list.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.rule:read", "mail:user_mailbox.rule:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Desc: "Mailbox email address that owns the rules (default: me)."},
		{Name: "rule-ids", Type: "string_slice", Desc: "Rule IDs to place first; comma-separated or repeat the flag. The CLI appends all other current rules automatically."},
	},
	Validate: validateMailRuleReorder,
	DryRun:   dryRunMailRuleReorder,
	Execute:  executeMailRuleReorder,
}

func validateMailRuleReorder(ctx context.Context, rt *common.RuntimeContext) error {
	if err := validateBotMailboxNotMe(rt); err != nil {
		return err
	}
	_, err := normalizeMailRuleReorderInput(rt.StrSlice("rule-ids"))
	return err
}

func dryRunMailRuleReorder(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
	mailboxID := resolveMailboxID(rt)
	inputIDs, _ := normalizeMailRuleReorderInput(rt.StrSlice("rule-ids"))
	return common.NewDryRunAPI().
		GET(mailboxPath(mailboxID, "rules")).
		Desc("Fetch current mail rules to complete the full rule order").
		POST(mailboxPath(mailboxID, "rules", "reorder")).
		Body(map[string]interface{}{"rule_ids": inputIDs}).
		Desc("Submit the complete rule ID order; dry-run shows only the user-provided front segment")
}

func executeMailRuleReorder(ctx context.Context, rt *common.RuntimeContext) error {
	mailboxID := resolveMailboxID(rt)
	inputIDs, err := normalizeMailRuleReorderInput(rt.StrSlice("rule-ids"))
	if err != nil {
		return err
	}
	currentRules, err := fetchMailRulesForReorder(rt, mailboxID)
	if err != nil {
		return err
	}
	finalIDs, err := buildMailRuleReorderIDs(inputIDs, currentRules)
	if err != nil {
		return err
	}

	_, err = rt.CallAPITyped("POST", mailboxPath(mailboxID, "rules", "reorder"), nil, map[string]interface{}{
		"rule_ids": finalIDs,
	})
	if err != nil {
		return mailAppendProblemHint(err, "submitted_rule_ids="+mustJSONList(finalIDs))
	}

	result := map[string]interface{}{
		"mailbox":            mailboxID,
		"input_rule_ids":     inputIDs,
		"submitted_rule_ids": finalIDs,
		"total":              len(finalIDs),
	}
	rt.OutFormat(result, nil, func(w io.Writer) {
		fmt.Fprintf(w, "Successfully reordered %d mail rule(s)\n", len(finalIDs))
		fmt.Fprintln(w, "Submitted order:")
		for i, id := range finalIDs {
			fmt.Fprintf(w, "  Position %d: %s\n", i+1, id)
		}
	})
	return nil
}

func normalizeMailRuleReorderInput(raw []string) ([]string, error) {
	ids := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	duplicateIDs := make([]string, 0)
	emptyPositions := make([]errs.InvalidParam, 0)
	for i, v := range raw {
		id := strings.TrimSpace(v)
		if id == "" {
			emptyPositions = append(emptyPositions, mailInvalidParam(fmt.Sprintf("--rule-ids[%d]", i), "empty rule ID"))
			continue
		}
		if seen[id] {
			duplicateIDs = append(duplicateIDs, id)
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(emptyPositions) > 0 {
		return nil, mailValidationError("--rule-ids contains empty rule ID(s)").WithParam("--rule-ids").WithParams(emptyPositions...)
	}
	if len(ids) == 0 {
		return nil, mailValidationParamError("--rule-ids", "--rule-ids must contain at least one rule ID")
	}
	if len(duplicateIDs) > 0 {
		return nil, mailValidationParamError("--rule-ids", "--rule-ids contains duplicate rule ID(s): %s", strings.Join(duplicateIDs, ", "))
	}
	return ids, nil
}

func fetchMailRulesForReorder(rt *common.RuntimeContext, mailboxID string) ([]mailRuleOrderItem, error) {
	data, err := rt.CallAPITyped("GET", mailboxPath(mailboxID, "rules"), nil, nil)
	if err != nil {
		return nil, err
	}
	items, _ := data["items"].([]interface{})
	rules := make([]mailRuleOrderItem, 0, len(items))
	for i, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			return nil, mailInvalidResponseError("rules[%d] is not an object", i)
		}
		id := strings.TrimSpace(mailRuleStringValue(item["id"]))
		if id == "" {
			return nil, mailInvalidResponseError("rules[%d].id is empty", i)
		}
		rules = append(rules, mailRuleOrderItem{
			ID:       id,
			Sequence: int64PointerValue(item["sequence"]),
			Index:    i,
		})
	}
	sortMailRulesBySequenceWhenComplete(rules)
	return rules, nil
}

func buildMailRuleReorderIDs(inputIDs []string, currentRules []mailRuleOrderItem) ([]string, error) {
	currentSet := make(map[string]bool, len(currentRules))
	for _, rule := range currentRules {
		if currentSet[rule.ID] {
			return nil, mailInvalidResponseError("rules list contains duplicate rule ID %q", rule.ID)
		}
		currentSet[rule.ID] = true
	}
	unknownIDs := make([]string, 0)
	inputSet := make(map[string]bool, len(inputIDs))
	for _, id := range inputIDs {
		if !currentSet[id] {
			unknownIDs = append(unknownIDs, id)
			continue
		}
		inputSet[id] = true
	}
	if len(unknownIDs) > 0 {
		return nil, mailValidationParamError("--rule-ids", "unknown rule ID(s): %s; run `lark-cli mail user_mailbox.rules list --params '{\"user_mailbox_id\":\"me\"}'` to fetch current IDs", strings.Join(unknownIDs, ", "))
	}

	finalIDs := append([]string(nil), inputIDs...)
	for _, rule := range currentRules {
		if !inputSet[rule.ID] {
			finalIDs = append(finalIDs, rule.ID)
		}
	}
	return finalIDs, nil
}

func sortMailRulesBySequenceWhenComplete(rules []mailRuleOrderItem) {
	if len(rules) < 2 {
		return
	}
	for _, rule := range rules {
		if rule.Sequence == nil {
			return
		}
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if *rules[i].Sequence == *rules[j].Sequence {
			return rules[i].Index < rules[j].Index
		}
		return *rules[i].Sequence < *rules[j].Sequence
	})
}

func int64PointerValue(v interface{}) *int64 {
	switch x := v.(type) {
	case int:
		n := int64(x)
		return &n
	case int64:
		n := x
		return &n
	case float64:
		n := int64(x)
		return &n
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return &n
		}
	}
	return nil
}

func mailRuleStringValue(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	case json.Number:
		return x.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func mustJSONList(ids []string) string {
	b, err := json.Marshal(ids)
	if err != nil {
		return fmt.Sprintf("%v", ids)
	}
	return string(b)
}
