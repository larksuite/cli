// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

type mailboxRule struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	IsEnable bool   `json:"is_enable,omitempty"`
}

type ruleReorderPreview struct {
	Mailbox      string        `json:"mailbox"`
	SpecifiedIDs []string      `json:"specified_rule_ids"`
	Before       []mailboxRule `json:"before"`
	After        []mailboxRule `json:"after"`
	CompletedIDs []string      `json:"completed_rule_ids"`
	DryRun       bool          `json:"dry_run"`
}

var MailRuleReorder = common.Shortcut{
	Service:     "mail",
	Command:     "+rule-reorder",
	Description: "Reorder inbox rules. Accepts a partial --rule-ids list, fetches the full current order, and appends omitted rules automatically before calling reorder.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.rule:read", "mail:user_mailbox.rule:write"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "email address (default: me)"},
		{Name: "rule-ids", Desc: "Required. Comma or whitespace separated rule IDs. Partial input is allowed; omitted rules keep their relative order and are appended automatically.", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateBotMailboxNotMe(runtime); err != nil {
			return err
		}
		_, err := parseRuleIDsInput(runtime.Str("rule-ids"))
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveMailboxID(runtime)
		ruleIDsInput := runtime.Str("rule-ids")
		api := common.NewDryRunAPI().
			Desc("Fetch current mailbox rules, complete omitted rule IDs locally, then reorder with the full list").
			GET(mailboxPath(mailboxID, "rules")).
			POST(mailboxPath(mailboxID, "rules", "reorder")).
			Body(map[string]interface{}{"rule_ids": []string{"<full_rule_id_list>"}})
		if ids, err := parseRuleIDsInput(ruleIDsInput); err == nil {
			api = api.Set("specified_rule_ids", ids)
		} else {
			api = api.Set("rule_ids_error", err.Error())
		}
		return api
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveMailboxID(runtime)
		ruleIDs, err := parseRuleIDsInput(runtime.Str("rule-ids"))
		if err != nil {
			return err
		}

		rules, err := listMailboxRules(runtime, mailboxID)
		if err != nil {
			return mailDecorateProblemMessage(err, "failed to list mailbox rules")
		}
		if len(rules) == 0 {
			return mailValidationError("no mailbox rules found to reorder")
		}
		if len(rules) == 1 {
			preview := buildRuleReorderPreview(mailboxID, ruleIDs, rules, rules, runtime.Bool("dry-run"))
			runtime.Out(preview, nil)
			return nil
		}

		completedIDs, reorderedRules, err := buildCompletedRuleOrder(ruleIDs, rules)
		if err != nil {
			return err
		}

		preview := buildRuleReorderPreview(mailboxID, ruleIDs, rules, reorderedRules, runtime.Bool("dry-run"))
		if runtime.Bool("dry-run") {
			runtime.Out(preview, nil)
			return nil
		}

		_, err = doJSONAPI(runtime, &larkcore.ApiReq{
			HttpMethod: http.MethodPost,
			ApiPath:    mailboxPath(mailboxID, "rules", "reorder"),
			Body: map[string]interface{}{
				"rule_ids": completedIDs,
			},
		}, "failed to reorder mailbox rules")
		if err != nil {
			return err
		}

		runtime.Out(preview, nil)
		return nil
	},
}

func parseRuleIDsInput(raw string) ([]string, error) {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\t' || r == ' '
	})
	if len(fields) == 0 {
		return nil, mailValidationParamError("rule-ids", "--rule-ids is required")
	}

	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		id := strings.TrimSpace(field)
		if id == "" {
			continue
		}
		if _, err := strconv.ParseInt(id, 10, 64); err != nil {
			return nil, mailValidationParamError("rule-ids", "--rule-ids must contain numeric rule IDs only: %q", id).WithCause(err)
		}
		if _, ok := seen[id]; ok {
			return nil, mailValidationParamError("rule-ids", "--rule-ids contains duplicate rule ID %q", id)
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, mailValidationParamError("rule-ids", "--rule-ids is required")
	}
	return out, nil
}

func listMailboxRules(runtime *common.RuntimeContext, mailboxID string) ([]mailboxRule, error) {
	data, err := doJSONAPI(runtime, &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    mailboxPath(mailboxID, "rules"),
	}, "failed to list mailbox rules")
	if err != nil {
		return nil, err
	}

	items, _ := data["items"].([]interface{})
	if len(items) == 0 {
		if nested, ok := data["data"].(map[string]interface{}); ok {
			items, _ = nested["items"].([]interface{})
		}
	}

	rules := make([]mailboxRule, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id := anyToString(m["id"])
		if id == "" {
			continue
		}
		rules = append(rules, mailboxRule{
			ID:       id,
			Name:     anyToString(m["name"]),
			IsEnable: boolVal(m["is_enable"]),
		})
	}
	return rules, nil
}

func buildCompletedRuleOrder(specifiedIDs []string, currentRules []mailboxRule) ([]string, []mailboxRule, error) {
	indexByID := make(map[string]int, len(currentRules))
	for i, rule := range currentRules {
		indexByID[rule.ID] = i
	}

	completed := make([]string, 0, len(currentRules))
	reordered := make([]mailboxRule, 0, len(currentRules))
	seen := make(map[string]struct{}, len(currentRules))

	for _, id := range specifiedIDs {
		idx, ok := indexByID[id]
		if !ok {
			return nil, nil, mailValidationParamError("rule-ids", "rule %q not found in current mailbox rules", id)
		}
		completed = append(completed, id)
		reordered = append(reordered, currentRules[idx])
		seen[id] = struct{}{}
	}
	for _, rule := range currentRules {
		if _, ok := seen[rule.ID]; ok {
			continue
		}
		completed = append(completed, rule.ID)
		reordered = append(reordered, rule)
	}
	return completed, reordered, nil
}

func buildRuleReorderPreview(mailboxID string, specifiedIDs []string, before, after []mailboxRule, dryRun bool) ruleReorderPreview {
	completedIDs := make([]string, 0, len(after))
	for _, rule := range after {
		completedIDs = append(completedIDs, rule.ID)
	}
	return ruleReorderPreview{
		Mailbox:      mailboxID,
		SpecifiedIDs: slices.Clone(specifiedIDs),
		Before:       slices.Clone(before),
		After:        slices.Clone(after),
		CompletedIDs: completedIDs,
		DryRun:       dryRun,
	}
}

func anyToString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case json.Number:
		return x.String()
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strings.TrimSpace(fmt.Sprintf("%v", x))
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	default:
		return ""
	}
}
