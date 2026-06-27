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
	Move         string        `json:"move,omitempty"`
	BeforeRuleID string        `json:"before_rule_id,omitempty"`
	AfterRuleID  string        `json:"after_rule_id,omitempty"`
	ToTop        bool          `json:"to_top,omitempty"`
	Before       []mailboxRule `json:"before"`
	After        []mailboxRule `json:"after"`
	CompletedIDs []string      `json:"completed_rule_ids"`
	DryRun       bool          `json:"dry_run"`
}

type ruleReorderInput struct {
	SpecifiedIDs []string
	MoveRuleID   string
	BeforeRuleID string
	AfterRuleID  string
	ToTop        bool
}

var MailRuleReorder = common.Shortcut{
	Service:     "mail",
	Command:     "+rule-reorder",
	Description: "Reorder inbox rules. Accepts either a partial --rule-ids list or --move with --before/--after/--to-top, then fetches the full current order and completes the final reorder request locally.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.rule:read", "mail:user_mailbox.rule:write"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "email address (default: me)"},
		{Name: "rule-ids", Desc: "Comma or whitespace separated rule IDs. Partial input is allowed; omitted rules keep their relative order and are appended automatically."},
		{Name: "move", Desc: "Rule ID to move. Must be used with exactly one of --before, --after, or --to-top."},
		{Name: "before", Desc: "Anchor rule ID. Insert --move before this rule."},
		{Name: "after", Desc: "Anchor rule ID. Insert --move after this rule."},
		{Name: "to-top", Type: "bool", Desc: "Move the rule to the top of the current order."},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateBotMailboxNotMe(runtime); err != nil {
			return err
		}
		_, err := parseRuleReorderInput(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveMailboxID(runtime)
		input, err := parseRuleReorderInput(runtime)
		api := common.NewDryRunAPI().
			Desc("Fetch current mailbox rules, compute the full reorder plan locally, then reorder with the completed list").
			GET(mailboxPath(mailboxID, "rules")).
			POST(mailboxPath(mailboxID, "rules", "reorder")).
			Body(map[string]interface{}{"rule_ids": []string{"<full_rule_id_list>"}})
		if err == nil {
			if len(input.SpecifiedIDs) > 0 {
				api = api.Set("specified_rule_ids", input.SpecifiedIDs)
			}
			if input.MoveRuleID != "" {
				api = api.Set("move", input.MoveRuleID)
			}
			if input.BeforeRuleID != "" {
				api = api.Set("before", input.BeforeRuleID)
			}
			if input.AfterRuleID != "" {
				api = api.Set("after", input.AfterRuleID)
			}
			if input.ToTop {
				api = api.Set("to_top", true)
			}
		} else {
			api = api.Set("reorder_input_error", err.Error())
		}
		return api
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveMailboxID(runtime)
		input, err := parseRuleReorderInput(runtime)
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

		completedIDs, reorderedRules, err := buildRuleReorderPlan(input, rules)
		if err != nil {
			return err
		}

		preview := buildRuleReorderPreview(mailboxID, input, rules, reorderedRules, runtime.Bool("dry-run"))
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

func parseRuleReorderInput(runtime *common.RuntimeContext) (ruleReorderInput, error) {
	ruleIDsRaw := strings.TrimSpace(runtime.Str("rule-ids"))
	moveRaw := strings.TrimSpace(runtime.Str("move"))
	beforeRaw := strings.TrimSpace(runtime.Str("before"))
	afterRaw := strings.TrimSpace(runtime.Str("after"))
	toTop := runtime.Bool("to-top")

	hasRuleIDs := ruleIDsRaw != ""
	hasMove := moveRaw != ""
	hasPlacement := beforeRaw != "" || afterRaw != "" || toTop

	switch {
	case hasRuleIDs && hasMove:
		return ruleReorderInput{}, mailValidationError("--rule-ids and --move are mutually exclusive; choose exactly one input mode")
	case !hasRuleIDs && !hasMove:
		return ruleReorderInput{}, mailValidationError("either --rule-ids or --move is required")
	case hasRuleIDs:
		if hasPlacement {
			return ruleReorderInput{}, mailValidationError("--before, --after, and --to-top require --move")
		}
		ids, err := parseRuleIDsInput(ruleIDsRaw)
		if err != nil {
			return ruleReorderInput{}, err
		}
		return ruleReorderInput{SpecifiedIDs: ids}, nil
	default:
		if !hasPlacement {
			return ruleReorderInput{}, mailValidationError("--move requires exactly one of --before, --after, or --to-top")
		}
		placementCount := 0
		if beforeRaw != "" {
			placementCount++
		}
		if afterRaw != "" {
			placementCount++
		}
		if toTop {
			placementCount++
		}
		if placementCount != 1 {
			return ruleReorderInput{}, mailValidationError("--move requires exactly one of --before, --after, or --to-top")
		}

		moveID, err := parseSingleRuleID("move", moveRaw)
		if err != nil {
			return ruleReorderInput{}, err
		}
		input := ruleReorderInput{
			MoveRuleID: moveID,
			ToTop:      toTop,
		}
		if beforeRaw != "" {
			input.BeforeRuleID, err = parseSingleRuleID("before", beforeRaw)
			if err != nil {
				return ruleReorderInput{}, err
			}
		}
		if afterRaw != "" {
			input.AfterRuleID, err = parseSingleRuleID("after", afterRaw)
			if err != nil {
				return ruleReorderInput{}, err
			}
		}
		if input.MoveRuleID == input.BeforeRuleID && input.BeforeRuleID != "" {
			return ruleReorderInput{}, mailValidationParamError("before", "--before cannot reference the same rule as --move")
		}
		if input.MoveRuleID == input.AfterRuleID && input.AfterRuleID != "" {
			return ruleReorderInput{}, mailValidationParamError("after", "--after cannot reference the same rule as --move")
		}
		return input, nil
	}
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

func parseSingleRuleID(flagName, raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", mailValidationParamError("--"+flagName, "--%s is required", flagName)
	}
	if strings.ContainsAny(id, ", \n\t") {
		return "", mailValidationParamError("--"+flagName, "--%s accepts exactly one numeric rule ID", flagName)
	}
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return "", mailValidationParamError("--"+flagName, "--%s must be a numeric rule ID", flagName).WithCause(err)
	}
	return id, nil
}

func buildRuleReorderPlan(input ruleReorderInput, currentRules []mailboxRule) ([]string, []mailboxRule, error) {
	if len(input.SpecifiedIDs) > 0 {
		return buildCompletedRuleOrder(input.SpecifiedIDs, currentRules)
	}
	return buildMoveRuleOrder(input, currentRules)
}

func buildMoveRuleOrder(input ruleReorderInput, currentRules []mailboxRule) ([]string, []mailboxRule, error) {
	indexByID := make(map[string]int, len(currentRules))
	for i, rule := range currentRules {
		indexByID[rule.ID] = i
	}

	moveIdx, ok := indexByID[input.MoveRuleID]
	if !ok {
		return nil, nil, mailValidationParamError("--move", "rule %q not found in current mailbox rules", input.MoveRuleID)
	}

	moveRule := currentRules[moveIdx]
	withoutMove := make([]mailboxRule, 0, len(currentRules)-1)
	for _, rule := range currentRules {
		if rule.ID == input.MoveRuleID {
			continue
		}
		withoutMove = append(withoutMove, rule)
	}

	insertAt := 0
	switch {
	case input.ToTop:
		insertAt = 0
	case input.BeforeRuleID != "":
		insertAt = indexOfRuleID(withoutMove, input.BeforeRuleID)
		if insertAt < 0 {
			return nil, nil, mailValidationParamError("--before", "rule %q not found in current mailbox rules", input.BeforeRuleID)
		}
	case input.AfterRuleID != "":
		insertAt = indexOfRuleID(withoutMove, input.AfterRuleID)
		if insertAt < 0 {
			return nil, nil, mailValidationParamError("--after", "rule %q not found in current mailbox rules", input.AfterRuleID)
		}
		insertAt++
	default:
		return nil, nil, mailValidationError("--move requires exactly one of --before, --after, or --to-top")
	}

	reordered := make([]mailboxRule, 0, len(currentRules))
	reordered = append(reordered, withoutMove[:insertAt]...)
	reordered = append(reordered, moveRule)
	reordered = append(reordered, withoutMove[insertAt:]...)

	completed := make([]string, 0, len(reordered))
	for _, rule := range reordered {
		completed = append(completed, rule.ID)
	}
	return completed, reordered, nil
}

func indexOfRuleID(rules []mailboxRule, id string) int {
	for i, rule := range rules {
		if rule.ID == id {
			return i
		}
	}
	return -1
}

func buildRuleReorderPreview(mailboxID string, input ruleReorderInput, before, after []mailboxRule, dryRun bool) ruleReorderPreview {
	completedIDs := make([]string, 0, len(after))
	for _, rule := range after {
		completedIDs = append(completedIDs, rule.ID)
	}
	return ruleReorderPreview{
		Mailbox:      mailboxID,
		SpecifiedIDs: slices.Clone(input.SpecifiedIDs),
		Move:         input.MoveRuleID,
		BeforeRuleID: input.BeforeRuleID,
		AfterRuleID:  input.AfterRuleID,
		ToTop:        input.ToTop,
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
