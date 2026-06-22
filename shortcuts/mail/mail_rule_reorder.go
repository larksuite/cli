// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var MailRuleReorder = common.Shortcut{
	Service:     "mail",
	Command:     "+rule-reorder",
	Description: "Reorder mail rules from a partial rule ID prefix. The CLI lists current rules, appends the remaining IDs in current order, then submits a complete reorder request.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.rule:read", "mail:user_mailbox.rule:write"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox email address or user_mailbox_id (default: me)"},
		{Name: "rule-id", Type: "string_array", Desc: "Rule ID to move to the front. Repeat to specify the desired prefix order."},
		{Name: "rule-ids", Desc: `Rule IDs as a JSON array (["r3","r1"]) or comma-separated string (r3,r1). Appended after repeated --rule-id values.`},
		{Name: "print-output-schema", Type: "bool", Desc: "Print output field reference (run this first to learn field names before parsing output)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Bool("print-output-schema") {
			return nil
		}
		_, err := parseRuleReorderInput(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveMailboxID(runtime)
		requested, _ := parseRuleReorderInput(runtime)
		return common.NewDryRunAPI().
			Desc("Reorder mail rules: list current rules → auto-complete full rule_ids → submit reorder").
			GET(mailboxPath(mailboxID, "rules")).
			Desc("List current rule order").
			POST(mailboxPath(mailboxID, "rules", "reorder")).
			Desc("Submit complete rule order after auto-completion").
			Body(map[string]interface{}{
				"rule_ids":           "<auto-completed-after-list>",
				"requested_rule_ids": requested,
			})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Bool("print-output-schema") {
			printRuleReorderOutputSchema(runtime)
			return nil
		}

		mailboxID := resolveMailboxID(runtime)
		requested, err := parseRuleReorderInput(runtime)
		if err != nil {
			return err
		}

		existing, err := fetchRuleIDs(runtime, mailboxID)
		if err != nil {
			return mailDecorateProblemMessage(err, "failed to list current mail rules")
		}
		finalIDs, appendedIDs, err := mergeRuleIDs(requested, existing)
		if err != nil {
			return err
		}

		if _, err := runtime.DoAPIJSONTyped("POST",
			mailboxPath(mailboxID, "rules", "reorder"),
			nil,
			map[string]interface{}{"rule_ids": finalIDs},
		); err != nil {
			return mailDecorateProblemMessage(err, "failed to reorder mail rules")
		}

		result := map[string]interface{}{
			"mailbox":            mailboxID,
			"requested_rule_ids": requested,
			"appended_rule_ids":  appendedIDs,
			"final_rule_ids":     finalIDs,
			"total":              len(finalIDs),
		}
		runtime.OutFormat(result, &output.Meta{Count: len(finalIDs)}, func(w io.Writer) {
			fmt.Fprintf(w, "mailbox: %s\n", mailboxID)
			fmt.Fprintf(w, "requested_rule_ids: %s\n", strings.Join(requested, ", "))
			if len(appendedIDs) > 0 {
				fmt.Fprintf(w, "appended_rule_ids: %s\n", strings.Join(appendedIDs, ", "))
			} else {
				fmt.Fprintln(w, "appended_rule_ids: (none)")
			}
			fmt.Fprintf(w, "final_rule_ids: %s\n", strings.Join(finalIDs, ", "))
			fmt.Fprintf(w, "total: %d\n", len(finalIDs))
		})
		return nil
	},
}

func parseRuleReorderInput(runtime *common.RuntimeContext) ([]string, error) {
	ids := make([]string, 0, len(runtime.StrArray("rule-id")))
	for _, id := range runtime.StrArray("rule-id") {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			ids = append(ids, trimmed)
		}
	}

	extra, err := parseRuleIDsFlag(runtime.Str("rule-ids"))
	if err != nil {
		return nil, err
	}
	ids = append(ids, extra...)

	if len(ids) == 0 {
		return nil, mailValidationError("at least one rule ID is required; pass --rule-id repeatedly or --rule-ids as JSON/CSV")
	}
	if duplicate := firstDuplicate(ids); duplicate != "" {
		return nil, mailValidationError("duplicate rule ID %q is not allowed", duplicate).
			WithParams(mailInvalidParam("--rule-id/--rule-ids", "duplicate rule ID: "+duplicate))
	}
	return ids, nil
}

func parseRuleIDsFlag(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	if strings.HasPrefix(raw, "[") || strings.Contains(raw, "]") {
		var ids []string
		if err := json.Unmarshal([]byte(raw), &ids); err != nil {
			return nil, mailValidationParamError("--rule-ids", `--rule-ids must be a JSON string array such as ["r3","r1"], or a comma-separated string`).WithCause(err)
		}
		return trimNonEmptyRuleIDs(ids), nil
	}

	parts := strings.Split(raw, ",")
	return trimNonEmptyRuleIDs(parts), nil
}

func trimNonEmptyRuleIDs(values []string) []string {
	ids := make([]string, 0, len(values))
	for _, value := range values {
		if id := strings.TrimSpace(value); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func firstDuplicate(ids []string) string {
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return id
		}
		seen[id] = true
	}
	return ""
}

func fetchRuleIDs(runtime *common.RuntimeContext, mailboxID string) ([]string, error) {
	data, err := runtime.DoAPIJSONTyped("GET", mailboxPath(mailboxID, "rules"), nil, nil)
	if err != nil {
		return nil, err
	}
	items, ok := data["items"].([]interface{})
	if !ok {
		if typedItems, ok := data["items"].([]map[string]interface{}); ok {
			ids := make([]string, 0, len(typedItems))
			for _, item := range typedItems {
				if id, _ := item["id"].(string); strings.TrimSpace(id) != "" {
					ids = append(ids, strings.TrimSpace(id))
				}
			}
			return ids, nil
		}
		return nil, mailInvalidResponseError("list mail rules response missing items array")
	}

	ids := make([]string, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if id, _ := item["id"].(string); strings.TrimSpace(id) != "" {
			ids = append(ids, strings.TrimSpace(id))
		}
	}
	return ids, nil
}

func mergeRuleIDs(requested, existing []string) ([]string, []string, error) {
	if len(existing) == 0 {
		return nil, nil, mailValidationError("no mail rules found in the mailbox; create rules before reordering")
	}

	exists := make(map[string]bool, len(existing))
	for _, id := range existing {
		exists[id] = true
	}
	var missing []string
	for _, id := range requested {
		if !exists[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return nil, nil, mailValidationError("rule ID(s) not found in current mailbox rules: %s; run `lark-cli mail user_mailbox.rules list --params '{\"user_mailbox_id\":\"me\"}'` to inspect current rules", strings.Join(missing, ", "))
	}

	selected := make(map[string]bool, len(requested))
	finalIDs := append([]string{}, requested...)
	for _, id := range requested {
		selected[id] = true
	}

	appended := make([]string, 0, len(existing)-len(requested))
	for _, id := range existing {
		if selected[id] {
			continue
		}
		finalIDs = append(finalIDs, id)
		appended = append(appended, id)
	}
	return finalIDs, appended, nil
}

func printRuleReorderOutputSchema(runtime *common.RuntimeContext) {
	runtime.Out(map[string]interface{}{
		"_description": "Output field reference for mail +rule-reorder",
		"fields": map[string]string{
			"mailbox":            "Mailbox ID used in the operation.",
			"requested_rule_ids": "Rule IDs explicitly supplied by the user, in requested prefix order.",
			"appended_rule_ids":  "Remaining current rule IDs appended by the CLI in list response order.",
			"final_rule_ids":     "Complete rule ID order submitted to user_mailbox.rules.reorder.",
			"total":              "Number of IDs in final_rule_ids.",
		},
	}, nil)
}
