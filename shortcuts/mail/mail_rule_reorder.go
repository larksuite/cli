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

// MailRuleReorder is the `+rule-reorder` shortcut: reorders mail inbox rules
// with automatic gap-filling. The backend ReorderUserMailboxRule API requires
// all rule IDs to be provided in the desired order; this shortcut lets users
// specify only the rules they want to reposition and auto-fills the remaining
// IDs by preserving their current relative order in the gaps between anchors.
var MailRuleReorder = common.Shortcut{
	Service:     "mail",
	Command:     "+rule-reorder",
	Description: "Reorder mail inbox rules; auto-fills missing rule IDs to match backend requirement",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.rule:read", "mail:user_mailbox.rule:write"},
	AuthTypes:   []string{"user", "tenant"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "rule-ids", Desc: "Comma-separated rule IDs in desired priority order (partial list allowed)"},
		{Name: "mailbox", Desc: "User mailbox address (default: me)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateBotMailboxNotMe(runtime); err != nil {
			return err
		}
		ids := runtime.Str("rule-ids")
		if ids == "" {
			return output.ErrValidation("--rule-ids is required")
		}
		parts := strings.Split(ids, ",")
		seen := make(map[string]bool)
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				return output.ErrValidation("--rule-ids contains empty ID")
			}
			if seen[p] {
				return output.ErrValidation("duplicate rule ID: %s", p)
			}
			seen[p] = true
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveMailboxID(runtime)
		return common.NewDryRunAPI().
			GET(mailboxPath(mailboxID, "rules")).
			Desc("List current rules to determine existing order").
			POST(mailboxPath(mailboxID, "rules", "reorder")).
			Body(map[string]interface{}{"rule_ids": []string{"<auto-filled>"}}).
			Desc("Reorder rules with auto-filled complete ID list")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveMailboxID(runtime)
		userIDs := parseRuleIDs(runtime.Str("rule-ids"))

		// Step 1: List current rules
		listData, err := runtime.DoAPIJSON("GET",
			mailboxPath(mailboxID, "rules"),
			nil, nil)
		if err != nil {
			return output.Errorf(output.ExitAPI, "api_error", "list rules failed: %s", err)
		}
		currentIDs := extractRuleIDs(listData)

		// Step 2: Auto-fill missing IDs
		reordered, err := reorderRuleIDs(userIDs, currentIDs)
		if err != nil {
			return err
		}

		// Step 3: Call reorder API
		_, err = runtime.DoAPIJSON("POST",
			mailboxPath(mailboxID, "rules", "reorder"),
			nil, map[string]interface{}{"rule_ids": reordered})
		if err != nil {
			return output.Errorf(output.ExitAPI, "api_error", "reorder rules failed: %s", err)
		}

		runtime.OutFormat(map[string]interface{}{
			"rule_ids": reordered,
		}, &output.Meta{Count: len(reordered)}, func(w io.Writer) {
			fmt.Fprintf(w, "Reordered %d rules successfully.\n", len(reordered))
		})
		return nil
	},
}

// parseRuleIDs splits a comma-separated string of rule IDs into a slice,
// trimming whitespace from each ID.
func parseRuleIDs(s string) []string {
	parts := strings.Split(s, ",")
	ids := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			ids = append(ids, p)
		}
	}
	return ids
}

// extractRuleIDs extracts rule IDs from the ListUserMailboxRule response.
// The response has the shape {"items": [{"id": "..."}, ...]} where items are
// ordered by execution priority (sequence).
func extractRuleIDs(data map[string]interface{}) []string {
	if data == nil {
		return nil
	}
	items, ok := data["items"].([]interface{})
	if !ok {
		return nil
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
	return ids
}

// reorderRuleIDs implements the gap-filling algorithm.
// userIDs: user-specified rule IDs (partial or full), in desired priority order
// currentIDs: all rule IDs in current priority order (from ListUserMailboxRule)
// Returns: complete rule ID list with user-specified IDs in desired positions
//
// Algorithm: assign each remaining ID to a gap between anchors based on which
// two anchors surround it in the current order. Within each gap, remaining IDs
// preserve their current relative order.
func reorderRuleIDs(userIDs, currentIDs []string) ([]string, error) {
	currentSet := make(map[string]bool, len(currentIDs))
	for _, id := range currentIDs {
		currentSet[id] = true
	}
	// Validate all user IDs exist
	for _, id := range userIDs {
		if !currentSet[id] {
			return nil, output.ErrValidation("rule ID %s not found in current rules", id)
		}
	}
	// Full list provided
	if len(userIDs) == len(currentIDs) {
		return userIDs, nil
	}

	// Build position map and user set
	currentPos := make(map[string]int, len(currentIDs))
	for i, id := range currentIDs {
		currentPos[id] = i
	}
	userSet := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		userSet[id] = true
	}

	// Collect remaining IDs in current order
	var remainingIDs []string
	for _, id := range currentIDs {
		if !userSet[id] {
			remainingIDs = append(remainingIDs, id)
		}
	}

	// Build anchor position list (sorted by current position)
	type anchorPos struct {
		id  string
		pos int
	}
	var anchors []anchorPos
	for _, id := range userIDs {
		anchors = append(anchors, anchorPos{id: id, pos: currentPos[id]})
	}

	// Assign each remaining ID to a gap index:
	// gap 0 = before first anchor in result
	// gap i = between anchor i-1 and anchor i in result (1 <= i <= len(anchors)-1)
	// gap len(anchors) = after last anchor in result
	//
	// Gap assignment: for each remaining ID r, scan consecutive anchor pairs
	// in result order. r belongs to the first pair (in result order) whose
	// current-position range encloses r. If no pair encloses r, r goes into
	// gap 0 (before all anchors) or gap N (after all anchors).
	gapCount := len(anchors) + 1
	gaps := make([][]string, gapCount)

	for _, r := range remainingIDs {
		rPos := currentPos[r]

		gapIdx := -1
		// Scan consecutive anchor pairs in result order
		for i := 0; i < len(anchors)-1; i++ {
			posA := anchors[i].pos
			posB := anchors[i+1].pos
			// r is between this pair if its position lies strictly between
			// the two anchors' positions (regardless of which is larger)
			minPos, maxPos := posA, posB
			if minPos > maxPos {
				minPos, maxPos = maxPos, minPos
			}
			if rPos > minPos && rPos < maxPos {
				gapIdx = i + 1 // gap between anchor i and anchor i+1
				break
			}
		}
		if gapIdx == -1 {
			// Not enclosed by any consecutive pair — must be before first
			// or after last anchor in current order
			if rPos < anchors[0].pos || (len(anchors) == 1 && rPos != anchors[0].pos) {
				gapIdx = 0 // before first anchor
			} else {
				gapIdx = len(anchors) // after last anchor
			}
		}

		gaps[gapIdx] = append(gaps[gapIdx], r)
	}

	// Assemble result: gap0 + anchor0 + gap1 + anchor1 + ... + gapN
	var result []string
	for i := 0; i < len(anchors); i++ {
		result = append(result, gaps[i]...)
		result = append(result, anchors[i].id)
	}
	result = append(result, gaps[len(anchors)]...)

	return result, nil
}
