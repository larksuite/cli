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

// MailRuleReorder reorders inbox rules. Partial ID lists are auto-filled
// using slot-replacement from the current server-side order.
var MailRuleReorder = common.Shortcut{
	Service:     "mail",
	Command:     "+rule-reorder",
	Description: "Reorder inbox rules. Provide a partial or full list of rule IDs in the desired order; missing rules are auto-filled from the current order using slot-replacement.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.rule:write"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "mailbox address (default: me)"},
		{Name: "rule-ids", Desc: "comma-separated rule IDs in desired order (required); omitted rules are auto-filled"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		raw := runtime.Str("rule-ids")
		if strings.TrimSpace(raw) == "" {
			return output.ErrValidation("--rule-ids: required, must be a comma-separated list of rule IDs")
		}
		ids, err := parseRuleIDs(raw)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return output.ErrValidation("--rule-ids: must provide at least one rule ID")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveMailboxID(runtime)
		return common.NewDryRunAPI().
			Desc("Step 1: list rules; Step 2: apply slot-replacement fill; Step 3: reorder with merged IDs").
			GET(mailboxPath(mailboxID, "rules")).
			Set("user_rule_ids", runtime.Str("rule-ids"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveMailboxID(runtime)
		userIDs, err := parseRuleIDs(runtime.Str("rule-ids"))
		if err != nil {
			return err
		}

		// Step 1: list current rules
		listResp, err := runtime.CallAPI("GET", mailboxPath(mailboxID, "rules"), nil, nil)
		if err != nil {
			return err
		}
		currentIDs, err := extractRuleIDs(listResp)
		if err != nil {
			return err
		}

		// Step 2: validate all user IDs exist in the current list
		if err := validateRuleIDsExist(userIDs, currentIDs); err != nil {
			return err
		}

		// Step 3: slot-replacement merge
		mergedIDs := slotReplace(currentIDs, userIDs)

		// Step 4: reorder (write op — do NOT auto-retry)
		_, err = runtime.CallAPI("POST", mailboxPath(mailboxID, "rules", "reorder"), nil,
			map[string]interface{}{"rule_ids": mergedIDs})
		if err != nil {
			return fmt.Errorf("%w; rule list may have changed, please re-run the full command", err)
		}

		out := map[string]interface{}{"rule_ids": mergedIDs}
		runtime.OutFormat(out, &output.Meta{Count: len(mergedIDs)}, func(w io.Writer) {
			fmt.Fprintf(w, "reordered %d rules. new order: %v\n", len(mergedIDs), mergedIDs)
		})
		return nil
	},
}

// slotReplace fills userIDs into the positional slots they occupy in currentIDs.
// Non-specified IDs stay in their original positions.
func slotReplace(currentIDs, userIDs []string) []string {
	userSet := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		userSet[id] = true
	}
	slots := make([]int, 0, len(userIDs))
	for i, id := range currentIDs {
		if userSet[id] {
			slots = append(slots, i)
		}
	}
	result := make([]string, len(currentIDs))
	copy(result, currentIDs)
	for j, slot := range slots {
		result[slot] = userIDs[j]
	}
	return result
}

func parseRuleIDs(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	seen := make(map[string]bool, len(parts))
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		id := strings.TrimSpace(p)
		if id == "" {
			continue
		}
		if seen[id] {
			return nil, output.ErrValidation("--rule-ids: duplicate rule ID %q", id)
		}
		seen[id] = true
		result = append(result, id)
	}
	return result, nil
}

func validateRuleIDsExist(userIDs, currentIDs []string) error {
	currentSet := make(map[string]bool, len(currentIDs))
	for _, id := range currentIDs {
		currentSet[id] = true
	}
	for _, id := range userIDs {
		if !currentSet[id] {
			return output.ErrValidation("--rule-ids: rule ID %q not found in mailbox", id)
		}
	}
	return nil
}

func extractRuleIDs(resp map[string]interface{}) ([]string, error) {
	// CallAPI/HandleApiResult already extracts the "data" field, so resp is directly {"items": [...]}.
	items, ok := resp["items"].([]interface{})
	if !ok {
		return []string{}, nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
