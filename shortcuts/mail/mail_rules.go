// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	internalsuggest "github.com/larksuite/cli/internal/suggest"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	mailRuleSpecVersion       = "mail_rule/v1"
	mailRuleInputFileMaxBytes = 1 << 20
	mailRuleCollectionMax     = 100
)

type mailRuleSpec struct {
	Version string `json:"version"`
	Mailbox struct {
		UserMailboxID string `json:"user_mailbox_id"`
	} `json:"mailbox"`
	Rule mailRuleSemantic `json:"rule"`
}

type mailRuleSemantic struct {
	ID             string              `json:"id,omitempty"`
	Name           string              `json:"name,omitempty"`
	Enabled        bool                `json:"enabled"`
	StopAfterMatch bool                `json:"stop_after_match"`
	Match          string              `json:"match"`
	Conditions     []mailRuleCondition `json:"conditions,omitempty"`
	Actions        []mailRuleAction    `json:"actions,omitempty"`
}

type mailRuleCondition struct {
	Field    string `json:"field"`
	Operator string `json:"operator,omitempty"`
	Value    string `json:"value,omitempty"`
}

type mailRuleAction struct {
	Kind   string            `json:"kind"`
	Params map[string]string `json:"params,omitempty"`
}

type mailRuleUnknown struct {
	Path   string      `json:"path"`
	Reason string      `json:"reason"`
	Raw    interface{} `json:"raw,omitempty"`
}

type mailRuleEnvelope struct {
	RuleID       string            `json:"rule_id,omitempty"`
	Name         string            `json:"name,omitempty"`
	Enabled      bool              `json:"enabled"`
	Order        interface{}       `json:"order,omitempty"`
	Description  string            `json:"description"`
	SemanticSpec *mailRuleSpec     `json:"semantic_spec,omitempty"`
	Unknowns     []mailRuleUnknown `json:"unknowns,omitempty"`
	Raw          map[string]any    `json:"raw,omitempty"`
}

type mailRuleAlias struct {
	Canonical string
	Code      int
	Label     string
	NeedsArg  bool
	Aliases   []string
}

var mailRuleFields = []mailRuleAlias{
	{Canonical: "from", Code: 1, Label: "发件人", NeedsArg: true, Aliases: []string{"sender"}},
	{Canonical: "to", Code: 2, Label: "收件人", NeedsArg: true, Aliases: []string{"recipient"}},
	{Canonical: "cc", Code: 3, Label: "抄送人", NeedsArg: true},
	{Canonical: "to_or_cc", Code: 4, Label: "收件人或抄送人", NeedsArg: true, Aliases: []string{"recipient_or_cc"}},
	{Canonical: "subject", Code: 6, Label: "主题", NeedsArg: true, Aliases: []string{"title"}},
	{Canonical: "body", Code: 7, Label: "正文", NeedsArg: true, Aliases: []string{"content"}},
	{Canonical: "attachment_name", Code: 8, Label: "附件名称", NeedsArg: true, Aliases: []string{"attach_name"}},
	{Canonical: "attachment_type", Code: 9, Label: "附件类型", NeedsArg: true, Aliases: []string{"attach_type"}},
	{Canonical: "any_address", Code: 10, Label: "任意地址", NeedsArg: true, Aliases: []string{"any_recipient"}},
	{Canonical: "all_mail", Code: 12, Label: "所有邮件", Aliases: []string{"all"}},
	{Canonical: "external", Code: 13, Label: "外部邮件", Aliases: []string{"external_mail"}},
	{Canonical: "spam", Code: 14, Label: "垃圾邮件", Aliases: []string{"is_spam"}},
	{Canonical: "not_spam", Code: 15, Label: "非垃圾邮件", Aliases: []string{"is_not_spam"}},
	{Canonical: "has_attachment", Code: 16, Label: "带附件", Aliases: []string{"has_attach"}},
}

var mailRuleOperators = []mailRuleAlias{
	{Canonical: "contains", Code: 1, Label: "包含", NeedsArg: true, Aliases: []string{"include"}},
	{Canonical: "not_contains", Code: 2, Label: "不包含", NeedsArg: true, Aliases: []string{"exclude"}},
	{Canonical: "starts_with", Code: 3, Label: "开头是", NeedsArg: true, Aliases: []string{"prefix"}},
	{Canonical: "ends_with", Code: 4, Label: "结尾是", NeedsArg: true, Aliases: []string{"suffix"}},
	{Canonical: "equals", Code: 5, Label: "等于", NeedsArg: true, Aliases: []string{"eq", "is"}},
	{Canonical: "not_equals", Code: 6, Label: "不等于", NeedsArg: true, Aliases: []string{"ne"}},
	{Canonical: "contains_self", Code: 7, Label: "包含自己", Aliases: []string{"self"}},
	{Canonical: "empty", Code: 10, Label: "为空", Aliases: []string{"is_empty"}},
}

var mailRuleActions = []mailRuleAlias{
	{Canonical: "archive", Code: 1, Label: "归档"},
	{Canonical: "delete_mail", Code: 2, Label: "删除邮件", Aliases: []string{"trash"}},
	{Canonical: "mark_read", Code: 3, Label: "标为已读", Aliases: []string{"read"}},
	{Canonical: "move_spam", Code: 4, Label: "移入垃圾邮件", Aliases: []string{"spam"}},
	{Canonical: "not_spam", Code: 5, Label: "永不视为垃圾邮件", Aliases: []string{"never_spam"}},
	{Canonical: "star", Code: 9, Label: "加星标", Aliases: []string{"flag", "add_flag", "add_star"}},
	{Canonical: "mute_notification", Code: 10, Label: "免打扰", Aliases: []string{"mute"}},
	{Canonical: "move_folder", Code: 11, Label: "移动到文件夹", NeedsArg: true, Aliases: []string{"folder", "move_to"}},
}

var (
	mailRuleFieldByName  = aliasByName(mailRuleFields)
	mailRuleFieldByCode  = aliasByCode(mailRuleFields)
	mailRuleOpByName     = aliasByName(mailRuleOperators)
	mailRuleOpByCode     = aliasByCode(mailRuleOperators)
	mailRuleActionByName = aliasByName(mailRuleActions)
	mailRuleActionByCode = aliasByCode(mailRuleActions)
)

var mailRuleCommonFlags = []common.Flag{
	{Name: "user-mailbox-id", Default: "me", Desc: "User mailbox ID or address that owns the rules (default: me)."},
}

var mailRuleAuthTypes = []string{"user", "bot"}

var MailRuleList = common.Shortcut{
	Service:     "mail",
	Command:     "+rule-list",
	Description: "List mailbox rules with semantic aliases, Chinese descriptions, and unknown raw warnings.",
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox.rule:read"},
	AuthTypes:   mailRuleAuthTypes,
	HasFormat:   true,
	Flags: append([]common.Flag{}, append(mailRuleCommonFlags,
		common.Flag{Name: "name-contains", Desc: "Optional local filter on rule name."},
	)...),
	Validate: validateMailRuleMailbox,
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().Desc("List mailbox rules and decode known condition/action enums").
			GET(mailRuleCollectionPath(ruleMailboxID(rt)))
	},
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		envelopes, err := listMailRuleEnvelopes(rt, ruleMailboxID(rt))
		if err != nil {
			return mailDecorateProblemMessage(err, "list mail rules failed")
		}
		filter := strings.ToLower(strings.TrimSpace(rt.Str("name-contains")))
		if filter != "" {
			envelopes = filterMailRuleEnvelopes(envelopes, filter)
		}
		out := map[string]any{"rules": envelopes, "total": len(envelopes)}
		rt.OutFormat(out, &output.Meta{Count: len(envelopes)}, func(w io.Writer) {
			printMailRuleTable(w, envelopes)
		})
		return nil
	},
}

var MailRuleGet = common.Shortcut{
	Service:     "mail",
	Command:     "+rule-get",
	Description: "Get one mailbox rule by rule_id by scanning the rules collection when no atomic get is exposed.",
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox.rule:read"},
	AuthTypes:   mailRuleAuthTypes,
	HasFormat:   true,
	Flags: append([]common.Flag{}, append(mailRuleCommonFlags,
		common.Flag{Name: "rule-id", Required: true, Desc: "Rule ID to fetch."},
	)...),
	Validate: validateMailRuleMailbox,
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().Desc("List rules and return the matching rule_id locally").
			GET(mailRuleCollectionPath(ruleMailboxID(rt)))
	},
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		env, err := getMailRuleEnvelope(rt, ruleMailboxID(rt), rt.Str("rule-id"))
		if err != nil {
			return err
		}
		rt.OutFormat(env, &output.Meta{Count: 1}, func(w io.Writer) {
			printMailRuleTable(w, []mailRuleEnvelope{env})
		})
		return nil
	},
}

var MailRuleCreate = common.Shortcut{
	Service:     "mail",
	Command:     "+rule-create",
	Description: "Create a mailbox rule from semantic condition/action aliases; use --dry-run to inspect the raw request.",
	Risk:        "high-risk-write",
	Scopes:      []string{"mail:user_mailbox.rule:write"},
	AuthTypes:   mailRuleAuthTypes,
	HasFormat:   true,
	Flags: append([]common.Flag{}, append(mailRuleWriteFlags(),
		common.Flag{Name: "name", Desc: "Required. Rule name."},
	)...),
	Validate: func(ctx context.Context, rt *common.RuntimeContext) error {
		if err := validateMailRuleMailbox(ctx, rt); err != nil {
			return err
		}
		_, _, err := buildRuleSpecFromFlags(rt, true)
		return err
	},
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		spec, raw, _ := buildRuleSpecFromFlags(rt, true)
		return common.NewDryRunAPI().Desc("Create mailbox rule; dry-run only renders semantic_spec and raw_request").
			Set("semantic_spec", spec).
			Set("raw_request", raw).
			POST(mailRuleCollectionPath(ruleMailboxID(rt))).Body(raw)
	},
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		spec, raw, err := buildRuleSpecFromFlags(rt, true)
		if err != nil {
			return err
		}
		data, err := rt.CallAPITyped("POST", mailRuleCollectionPath(ruleMailboxID(rt)), nil, raw)
		if err != nil {
			return mailDecorateProblemMessage(err, "create mail rule failed")
		}
		created := firstRuleObject(data)
		var env mailRuleEnvelope
		if isRecognizableRuleObject(created) {
			env = decodeMailRuleEnvelope(created, ruleMailboxID(rt))
		} else {
			env = envelopeFromRuleSpec(spec, nil, raw)
		}
		out := map[string]any{"rule": env, "semantic_spec": spec}
		rt.OutFormat(out, &output.Meta{Count: 1}, func(w io.Writer) {
			fmt.Fprintln(w, env.Description)
		})
		return nil
	},
}

var MailRuleUpdate = common.Shortcut{
	Service:     "mail",
	Command:     "+rule-update",
	Description: "Update one mailbox rule by reading the current rule and preserving unspecified fields.",
	Risk:        "high-risk-write",
	Scopes:      []string{"mail:user_mailbox.rule:write"},
	AuthTypes:   mailRuleAuthTypes,
	HasFormat:   true,
	Flags: append(mailRuleWriteFlags(),
		common.Flag{Name: "name", Desc: "Optional new rule name."},
		common.Flag{Name: "rule-id", Required: true, Desc: "Rule ID to update."},
	),
	Validate: func(ctx context.Context, rt *common.RuntimeContext) error {
		if err := validateMailRuleMailbox(ctx, rt); err != nil {
			return err
		}
		if strings.TrimSpace(rt.Str("rule-id")) == "" {
			return mailValidationParamError("--rule-id", "--rule-id is required")
		}
		if !hasRuleUpdateChange(rt) {
			return mailValidationParamError("--rule-id", "at least one update field must be provided")
		}
		_, _, err := buildRuleSpecFromFlags(rt, false)
		return err
	},
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		partial, _, _ := buildRuleSpecFromFlags(rt, false)
		semanticPatch, rawPatch := buildRuleUpdateDryRunPatch(rt, partial)
		return common.NewDryRunAPI().Desc("Read current rule, merge explicit changes, then PUT full rule body").
			Set("semantic_patch", semanticPatch).
			Set("raw_request_patch", rawPatch).
			GET(mailRuleCollectionPath(ruleMailboxID(rt))).
			PUT(mailRuleItemPath(ruleMailboxID(rt), rt.Str("rule-id"))).Body("<merged rule body after reading current rule>").
			GET(mailRuleCollectionPath(ruleMailboxID(rt)))
	},
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		env, err := getMailRuleEnvelope(rt, ruleMailboxID(rt), rt.Str("rule-id"))
		if err != nil {
			return err
		}
		target, raw, diff, err := mergeRuleUpdate(rt, env)
		if err != nil {
			return err
		}
		if len(diff) == 0 {
			out := map[string]any{"before": env, "after": env, "diff": diff, "no_op": true}
			rt.OutFormat(out, &output.Meta{Count: 0}, func(w io.Writer) {
				fmt.Fprintf(w, "Rule %s already matches the requested state; no update sent.\n", rt.Str("rule-id"))
			})
			return nil
		}
		if _, err := rt.CallAPITyped("PUT", mailRuleItemPath(ruleMailboxID(rt), rt.Str("rule-id")), nil, raw); err != nil {
			return mailDecorateProblemMessage(err, "update mail rule failed")
		}
		after, getErr := getMailRuleEnvelope(rt, ruleMailboxID(rt), rt.Str("rule-id"))
		if getErr != nil {
			after = envelopeFromRuleSpec(target, env.Unknowns, raw)
			out := map[string]any{"before": env, "after": after, "diff": diff, "raw_request": raw, "after_is_fallback": true, "after_read_error": getErr.Error()}
			rt.OutFormat(out, &output.Meta{Count: 1}, func(w io.Writer) {
				fmt.Fprintf(w, "Updated rule %s; failed to read the updated rule: %v\n", rt.Str("rule-id"), getErr)
			})
			return nil
		}
		out := map[string]any{"before": env, "after": after, "diff": diff, "raw_request": raw}
		rt.OutFormat(out, &output.Meta{Count: 1}, func(w io.Writer) {
			fmt.Fprintln(w, after.Description)
		})
		return nil
	},
}

var MailRuleDelete = common.Shortcut{
	Service:     "mail",
	Command:     "+rule-delete",
	Description: "Delete one mailbox rule. Without --yes, the shortcut fetches the target and returns a confirmation summary without deleting.",
	Risk:        "high-risk-write",
	Scopes:      []string{"mail:user_mailbox.rule:write"},
	AuthTypes:   mailRuleAuthTypes,
	HasFormat:   true,
	Flags: append([]common.Flag{}, append(mailRuleCommonFlags,
		common.Flag{Name: "rule-id", Required: true, Desc: "Rule ID to delete."},
	)...),
	Validate: func(ctx context.Context, rt *common.RuntimeContext) error {
		if err := validateMailRuleMailbox(ctx, rt); err != nil {
			return err
		}
		if rt.Bool("dry-run") || rt.Bool("yes") {
			return nil
		}
		env, err := getMailRuleEnvelope(rt, ruleMailboxID(rt), rt.Str("rule-id"))
		if err != nil {
			return err
		}
		return cmdutil.RequireConfirmation(fmt.Sprintf("mail +rule-delete rule_id=%s: %s", env.RuleID, env.Description))
	},
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().Desc("Delete one mailbox rule; dry-run does not call the API. Run without --yes to fetch a confirmation summary.").
			DELETE(mailRuleItemPath(ruleMailboxID(rt), rt.Str("rule-id")))
	},
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		env, err := getMailRuleEnvelope(rt, ruleMailboxID(rt), rt.Str("rule-id"))
		if err != nil {
			return err
		}
		if _, err := rt.CallAPITyped("DELETE", mailRuleItemPath(ruleMailboxID(rt), rt.Str("rule-id")), nil, nil); err != nil {
			return mailDecorateProblemMessage(err, "delete mail rule failed")
		}
		out := map[string]any{"deleted": true, "rule": env}
		rt.OutFormat(out, &output.Meta{Count: 1}, func(w io.Writer) {
			fmt.Fprintf(w, "Deleted rule %s.\n", rt.Str("rule-id"))
		})
		return nil
	},
}

var MailRuleEnable = makeRuleToggleShortcut("+rule-enable", true)
var MailRuleDisable = makeRuleToggleShortcut("+rule-disable", false)

var MailRuleReorder = common.Shortcut{
	Service:     "mail",
	Command:     "+rule-reorder",
	Description: "Reorder mailbox rules by full rule_id list or by moving one rule before/after/top/bottom.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.rule:write"},
	AuthTypes:   mailRuleAuthTypes,
	HasFormat:   true,
	Flags: append([]common.Flag{}, append(mailRuleCommonFlags,
		common.Flag{Name: "rule-ids", Type: "string_slice", Desc: "Full target rule ID order. Must contain every current rule exactly once."},
		common.Flag{Name: "move-rule-id", Desc: "Rule ID to move in the current order."},
		common.Flag{Name: "before-rule-id", Desc: "Place --move-rule-id before this rule."},
		common.Flag{Name: "after-rule-id", Desc: "Place --move-rule-id after this rule."},
		common.Flag{Name: "to-top", Type: "bool", Desc: "Move --move-rule-id to the top."},
		common.Flag{Name: "to-bottom", Type: "bool", Desc: "Move --move-rule-id to the bottom."},
	)...),
	Validate: func(ctx context.Context, rt *common.RuntimeContext) error {
		if err := validateMailRuleMailbox(ctx, rt); err != nil {
			return err
		}
		return validateRuleReorderFlags(rt)
	},
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().Desc("Read current order, compute target order, then submit reorder request").
			GET(mailRuleCollectionPath(ruleMailboxID(rt))).
			POST(mailRuleReorderPath(ruleMailboxID(rt))).Body(map[string]any{"rule_ids": "<computed>"})
	},
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		before, err := listMailRuleEnvelopes(rt, ruleMailboxID(rt))
		if err != nil {
			return mailDecorateProblemMessage(err, "list mail rules before reorder failed")
		}
		target, err := buildRuleTargetOrder(rt, before)
		if err != nil {
			return err
		}
		raw := map[string]any{"rule_ids": target}
		if _, err := rt.CallAPITyped("POST", mailRuleReorderPath(ruleMailboxID(rt)), nil, raw); err != nil {
			return mailDecorateProblemMessage(err, "reorder mail rules failed")
		}
		out := map[string]any{"before_rule_ids": envelopeRuleIDs(before), "after_rule_ids": target, "raw_request": raw}
		rt.OutFormat(out, &output.Meta{Count: len(target)}, func(w io.Writer) {
			fmt.Fprintf(w, "Reordered %d rule(s).\n", len(target))
		})
		return nil
	},
}

func mailRuleWriteFlags() []common.Flag {
	flags := append([]common.Flag{}, mailRuleCommonFlags...)
	flags = append(flags,
		common.Flag{Name: "enable", Type: "bool", Desc: "Enable the rule."},
		common.Flag{Name: "disable", Type: "bool", Desc: "Disable the rule."},
		common.Flag{Name: "match", Default: "all", Enum: []string{"all", "any"}, Desc: "Condition match mode: all or any (default: all)."},
		common.Flag{Name: "stop-after-match", Type: "bool", Desc: "Stop evaluating later rules after this rule matches."},
		common.Flag{Name: "continue-after-match", Type: "bool", Desc: "Continue evaluating later rules after this rule matches."},
		common.Flag{Name: "condition", Type: "string_array", Desc: "Rule condition grammar. Repeat flag. Examples: subject:contains:Alpha, has_attachment, to_or_cc:contains_self."},
		common.Flag{Name: "conditions", Desc: "JSON array/object of conditions, or @file containing it."},
		common.Flag{Name: "action", Type: "string_array", Desc: "Rule action grammar. Repeat flag. Examples: mark_read, move_folder:folder_id=xxx."},
		common.Flag{Name: "actions", Desc: "JSON array/object of actions, or @file containing it."},
	)
	return flags
}

func makeRuleToggleShortcut(command string, enabled bool) common.Shortcut {
	desc := "Enable one mailbox rule while preserving unknown raw conditions/actions."
	if !enabled {
		desc = "Disable one mailbox rule while preserving unknown raw conditions/actions."
	}
	return common.Shortcut{
		Service:     "mail",
		Command:     command,
		Description: desc,
		Risk:        "write",
		Scopes:      []string{"mail:user_mailbox.rule:write"},
		AuthTypes:   mailRuleAuthTypes,
		HasFormat:   true,
		Flags: append([]common.Flag{}, append(mailRuleCommonFlags,
			common.Flag{Name: "rule-id", Required: true, Desc: "Rule ID to update."},
		)...),
		Validate: validateMailRuleMailbox,
		DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
			return common.NewDryRunAPI().Desc("Read current rule, merge is_enable into the full request body, then PUT").
				Set("raw_request_patch", map[string]any{"is_enable": enabled}).
				GET(mailRuleCollectionPath(ruleMailboxID(rt))).
				PUT(mailRuleItemPath(ruleMailboxID(rt), rt.Str("rule-id"))).Body("<merged rule body after reading current rule>")
		},
		Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
			env, err := getMailRuleEnvelope(rt, ruleMailboxID(rt), rt.Str("rule-id"))
			if err != nil {
				return err
			}
			if env.Enabled == enabled {
				out := map[string]any{
					"before": map[string]any{"enabled": env.Enabled},
					"after":  env,
					"diff":   []map[string]any{},
					"no_op":  true,
				}
				rt.OutFormat(out, &output.Meta{Count: 0}, func(w io.Writer) {
					fmt.Fprintf(w, "Rule %s already %s; no update sent.\n", rt.Str("rule-id"), mailRuleEnabledWord(enabled))
				})
				return nil
			}
			raw := ruleRequestBodyFromEnvelope(env)
			raw["is_enable"] = enabled
			if _, err := rt.CallAPITyped("PUT", mailRuleItemPath(ruleMailboxID(rt), rt.Str("rule-id")), nil, raw); err != nil {
				return mailDecorateProblemMessage(err, "toggle mail rule failed")
			}
			after := env
			after.Enabled = enabled
			after.Raw = copyMap(raw)
			if after.SemanticSpec != nil {
				after.SemanticSpec.Rule.Enabled = enabled
				after.Description = describeMailRule(after.SemanticSpec.Rule, after.Unknowns)
			}
			out := map[string]any{
				"before":      map[string]any{"enabled": env.Enabled},
				"after":       after,
				"diff":        []map[string]any{{"field": "enabled", "before": env.Enabled, "after": enabled}},
				"raw_request": raw,
			}
			rt.OutFormat(out, &output.Meta{Count: 1}, func(w io.Writer) {
				fmt.Fprintln(w, after.Description)
			})
			return nil
		},
	}
}

func mailRuleEnabledWord(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func aliasByName(items []mailRuleAlias) map[string]mailRuleAlias {
	out := make(map[string]mailRuleAlias)
	for _, item := range items {
		out[item.Canonical] = item
		for _, alias := range item.Aliases {
			out[alias] = item
		}
	}
	return out
}

func aliasByCode(items []mailRuleAlias) map[int]mailRuleAlias {
	out := make(map[int]mailRuleAlias)
	for _, item := range items {
		out[item.Code] = item
	}
	return out
}

func ruleMailboxID(rt *common.RuntimeContext) string {
	if v := strings.TrimSpace(rt.Str("user-mailbox-id")); v != "" {
		return v
	}
	return "me"
}

func validateMailRuleMailbox(ctx context.Context, rt *common.RuntimeContext) error {
	if rt == nil || !rt.IsBot() {
		return nil
	}
	mailboxID := strings.TrimSpace(rt.Str("user-mailbox-id"))
	if mailboxID == "" || mailboxID == "me" {
		return mailValidationParamError("--user-mailbox-id", "--as bot requires an explicit mailbox ID or address; the default me mailbox is only valid for user identity")
	}
	return nil
}

func mailRuleCollectionPath(mailboxID string) string {
	return mailboxPath(mailboxID, "rules")
}

func mailRuleItemPath(mailboxID, ruleID string) string {
	return mailboxPath(mailboxID, "rules", ruleID)
}

func mailRuleReorderPath(mailboxID string) string {
	return mailboxPath(mailboxID, "rules", "reorder")
}

func buildRuleSpecFromFlags(rt *common.RuntimeContext, requireCreateFields bool) (*mailRuleSpec, map[string]any, error) {
	spec := &mailRuleSpec{Version: mailRuleSpecVersion}
	spec.Mailbox.UserMailboxID = ruleMailboxID(rt)
	spec.Rule.Enabled = true
	if rt.Bool("disable") {
		spec.Rule.Enabled = false
	}
	if rt.Bool("enable") && rt.Bool("disable") {
		return nil, nil, mailValidationError("--enable and --disable are mutually exclusive").
			WithParams(mailInvalidParam("--enable", "mutually exclusive with --disable"), mailInvalidParam("--disable", "mutually exclusive with --enable"))
	}
	spec.Rule.Name = strings.TrimSpace(rt.Str("name"))
	spec.Rule.Match = strings.TrimSpace(rt.Str("match"))
	if spec.Rule.Match == "" {
		spec.Rule.Match = "all"
	}
	if spec.Rule.Match != "all" && spec.Rule.Match != "any" {
		return nil, nil, mailValidationParamError("--match", "--match must be one of: all, any")
	}
	if rt.Bool("stop-after-match") && rt.Bool("continue-after-match") {
		return nil, nil, mailValidationError("--stop-after-match and --continue-after-match are mutually exclusive")
	}
	spec.Rule.StopAfterMatch = rt.Bool("stop-after-match")
	if rt.Bool("continue-after-match") {
		spec.Rule.StopAfterMatch = false
	}

	conditions, err := parseRuleConditions(rt, rt.StrArray("condition"), rt.Str("conditions"))
	if err != nil {
		return nil, nil, err
	}
	if err := validateRuleCollectionSize("--condition", "condition", len(conditions)); err != nil {
		return nil, nil, err
	}
	actions, err := parseRuleActions(rt, rt.StrArray("action"), rt.Str("actions"))
	if err != nil {
		return nil, nil, err
	}
	if err := validateRuleCollectionSize("--action", "action", len(actions)); err != nil {
		return nil, nil, err
	}
	spec.Rule.Conditions = conditions
	spec.Rule.Actions = actions

	if requireCreateFields || rt.Changed("condition") || rt.Changed("conditions") {
		if spec.Rule.Name == "" {
			if requireCreateFields {
				return nil, nil, mailValidationParamError("--name", "--name is required")
			}
		}
		if len(spec.Rule.Conditions) == 0 {
			return nil, nil, mailValidationParamError("--condition", "at least one --condition or --conditions entry is required")
		}
	}
	if requireCreateFields || rt.Changed("action") || rt.Changed("actions") {
		if len(spec.Rule.Actions) == 0 {
			return nil, nil, mailValidationParamError("--action", "at least one --action or --actions entry is required")
		}
	}
	raw, err := encodeRuleSpec(spec)
	if err != nil {
		return nil, nil, err
	}
	return spec, raw, nil
}

func parseRuleConditions(rt *common.RuntimeContext, grammar []string, bulk string) ([]mailRuleCondition, error) {
	var out []mailRuleCondition
	totalBytes := 0
	for _, raw := range grammar {
		items, err := parseRuleConditionValueWithBudget(rt, raw, "--condition", "condition", &totalBytes)
		if err != nil {
			return nil, err
		}
		if err := validateRuleCollectionAppend("--condition", "condition", len(out), len(items)); err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	if strings.TrimSpace(bulk) != "" {
		items, err := parseRuleConditionValueWithBudget(rt, bulk, "--conditions", "condition", &totalBytes)
		if err != nil {
			return nil, err
		}
		if err := validateRuleCollectionAppend("--conditions", "condition", len(out), len(items)); err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

func parseRuleActions(rt *common.RuntimeContext, grammar []string, bulk string) ([]mailRuleAction, error) {
	var out []mailRuleAction
	totalBytes := 0
	for _, raw := range grammar {
		items, err := parseRuleActionValueWithBudget(rt, raw, "--action", "action", &totalBytes)
		if err != nil {
			return nil, err
		}
		if err := validateRuleCollectionAppend("--action", "action", len(out), len(items)); err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	if strings.TrimSpace(bulk) != "" {
		items, err := parseRuleActionValueWithBudget(rt, bulk, "--actions", "action", &totalBytes)
		if err != nil {
			return nil, err
		}
		if err := validateRuleCollectionAppend("--actions", "action", len(out), len(items)); err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

func expandRuleInput(rt *common.RuntimeContext, raw, flag string) (string, error) {
	if len(raw) > mailRuleInputFileMaxBytes {
		return "", mailValidationParamError(flag, "%s input exceeds %d bytes limit", flag, mailRuleInputFileMaxBytes)
	}
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "@") {
		if rt == nil {
			return "", mailValidationParamError(flag, "file input is not available in this context")
		}
		path := strings.TrimPrefix(raw, "@")
		f, err := rt.FileIO().Open(path)
		if err != nil {
			return "", mailValidationParamError(flag, "open %s %s: %v", flag, path, err).WithCause(mailInputStatError(err))
		}
		defer f.Close()
		buf, err := io.ReadAll(io.LimitReader(f, mailRuleInputFileMaxBytes+1))
		if err != nil {
			return "", mailValidationParamError(flag, "read %s %s: %v", flag, path, err).WithCause(err)
		}
		if len(buf) > mailRuleInputFileMaxBytes {
			return "", mailValidationParamError(flag, "%s file exceeds %d bytes limit", flag, mailRuleInputFileMaxBytes)
		}
		return strings.TrimSpace(string(buf)), nil
	}
	return raw, nil
}

func parseRuleConditionValue(rt *common.RuntimeContext, raw, flag string) ([]mailRuleCondition, error) {
	expanded, err := expandRuleInput(rt, raw, flag)
	if err != nil {
		return nil, err
	}
	return parseRuleConditionExpanded(expanded, flag)
}

func parseRuleConditionValueWithBudget(rt *common.RuntimeContext, raw, flag, noun string, totalBytes *int) ([]mailRuleCondition, error) {
	expanded, err := expandRuleInput(rt, raw, flag)
	if err != nil {
		return nil, err
	}
	if err := trackRuleAggregateInputBytes(flag, noun, totalBytes, expanded); err != nil {
		return nil, err
	}
	return parseRuleConditionExpanded(expanded, flag)
}

func parseRuleConditionExpanded(expanded, flag string) ([]mailRuleCondition, error) {
	if expanded == "" {
		return nil, nil
	}
	if strings.HasPrefix(expanded, "{") || strings.HasPrefix(expanded, "[") {
		v, err := decodeSingleRuleJSON(expanded, flag)
		if err != nil {
			return nil, err
		}
		return parseRuleConditionJSON(v, flag)
	}
	cond, err := parseRuleConditionGrammar(expanded, flag)
	if err != nil {
		return nil, err
	}
	return []mailRuleCondition{cond}, nil
}

func parseRuleActionValue(rt *common.RuntimeContext, raw, flag string) ([]mailRuleAction, error) {
	expanded, err := expandRuleInput(rt, raw, flag)
	if err != nil {
		return nil, err
	}
	return parseRuleActionExpanded(expanded, flag)
}

func parseRuleActionValueWithBudget(rt *common.RuntimeContext, raw, flag, noun string, totalBytes *int) ([]mailRuleAction, error) {
	expanded, err := expandRuleInput(rt, raw, flag)
	if err != nil {
		return nil, err
	}
	if err := trackRuleAggregateInputBytes(flag, noun, totalBytes, expanded); err != nil {
		return nil, err
	}
	return parseRuleActionExpanded(expanded, flag)
}

func parseRuleActionExpanded(expanded, flag string) ([]mailRuleAction, error) {
	if expanded == "" {
		return nil, nil
	}
	if strings.HasPrefix(expanded, "{") || strings.HasPrefix(expanded, "[") {
		v, err := decodeSingleRuleJSON(expanded, flag)
		if err != nil {
			return nil, err
		}
		return parseRuleActionJSON(v, flag)
	}
	action, err := parseRuleActionGrammar(expanded, flag)
	if err != nil {
		return nil, err
	}
	return []mailRuleAction{action}, nil
}

func parseRuleConditionJSON(v any, flag string) ([]mailRuleCondition, error) {
	switch typed := v.(type) {
	case []any:
		if err := validateRuleCollectionSize(flag, "condition", len(typed)); err != nil {
			return nil, err
		}
		out := make([]mailRuleCondition, 0, len(typed))
		for i, item := range typed {
			cond, err := parseRuleConditionJSONObject(item, fmt.Sprintf("%s[%d]", flag, i))
			if err != nil {
				return nil, err
			}
			out = append(out, cond)
		}
		return out, nil
	default:
		cond, err := parseRuleConditionJSONObject(typed, flag)
		if err != nil {
			return nil, err
		}
		return []mailRuleCondition{cond}, nil
	}
}

func parseRuleConditionJSONObject(v any, flag string) (mailRuleCondition, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return mailRuleCondition{}, mailValidationParamError(flag, "invalid condition: expected object")
	}
	field, _ := m["field"].(string)
	op, _ := m["operator"].(string)
	value := ""
	if raw, ok := m["value"]; ok && raw != nil {
		value = fmt.Sprint(raw)
	}
	if op == "" {
		op, _ = m["op"].(string)
	}
	return validateRuleCondition(mailRuleCondition{Field: field, Operator: op, Value: value}, flag)
}

func parseRuleActionJSON(v any, flag string) ([]mailRuleAction, error) {
	switch typed := v.(type) {
	case []any:
		if err := validateRuleCollectionSize(flag, "action", len(typed)); err != nil {
			return nil, err
		}
		out := make([]mailRuleAction, 0, len(typed))
		for i, item := range typed {
			action, err := parseRuleActionJSONObject(item, fmt.Sprintf("%s[%d]", flag, i))
			if err != nil {
				return nil, err
			}
			out = append(out, action)
		}
		return out, nil
	default:
		action, err := parseRuleActionJSONObject(typed, flag)
		if err != nil {
			return nil, err
		}
		return []mailRuleAction{action}, nil
	}
}

func parseRuleActionJSONObject(v any, flag string) (mailRuleAction, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return mailRuleAction{}, mailValidationParamError(flag, "invalid action: expected object")
	}
	kind, _ := m["kind"].(string)
	alias, err := canonicalRuleActionAlias(kind, flag)
	if err != nil {
		return mailRuleAction{}, err
	}
	params := map[string]string{}
	if rawParams, exists := m["params"]; exists {
		pm, ok := rawParams.(map[string]any)
		if !ok {
			return mailRuleAction{}, mailValidationParamError(flag, "invalid action params: expected object")
		}
		for k, v := range pm {
			params[k] = fmt.Sprint(v)
		}
	}
	allowed := map[string]struct{}{"kind": {}, "params": {}}
	if param := requiredActionParam(alias.Canonical); param != "" {
		allowed[param] = struct{}{}
		if v, ok := m[param]; ok {
			params[param] = fmt.Sprint(v)
		}
	}
	for key := range m {
		if _, ok := allowed[key]; !ok {
			return mailRuleAction{}, mailValidationParamError(flag, "action %s does not accept field %q", alias.Canonical, key)
		}
	}
	return validateRuleAction(mailRuleAction{Kind: alias.Canonical, Params: params}, flag)
}

func parseRuleActionGrammar(raw, flag string) (mailRuleAction, error) {
	parts := strings.SplitN(raw, ":", 2)
	action := mailRuleAction{Kind: parts[0], Params: map[string]string{}}
	if len(parts) == 2 {
		tail := strings.TrimSpace(parts[1])
		if strings.HasPrefix(tail, "json=") {
			rawParams, err := decodeSingleRuleJSON(strings.TrimPrefix(tail, "json="), flag)
			if err != nil {
				return mailRuleAction{}, err
			}
			params, ok := rawParams.(map[string]any)
			if !ok {
				return mailRuleAction{}, mailValidationParamError(flag, "invalid action json params: expected object")
			}
			for k, v := range params {
				action.Params[k] = fmt.Sprint(v)
			}
		} else {
			key, val, ok := strings.Cut(tail, "=")
			if !ok {
				return mailRuleAction{}, mailValidationParamError(flag, "invalid action %q: expected kind or kind:key=value. Accepted actions: %s", raw, acceptedAliasList(mailRuleActions))
			}
			action.Params[strings.TrimSpace(key)] = strings.TrimSpace(val)
		}
	}
	return validateRuleAction(action, flag)
}

func canonicalRuleActionAlias(kind, flag string) (mailRuleAlias, error) {
	alias, ok := mailRuleActionByName[strings.TrimSpace(kind)]
	if !ok {
		return mailRuleAlias{}, mailValidationParamError(flag, "%s", unknownRuleAliasMessage("action", kind, "actions", mailRuleActions))
	}
	return alias, nil
}

func validateRuleAction(action mailRuleAction, flag string) (mailRuleAction, error) {
	alias, err := canonicalRuleActionAlias(action.Kind, flag)
	if err != nil {
		return mailRuleAction{}, err
	}
	action.Kind = alias.Canonical
	needed := requiredActionParam(action.Kind)
	normalized := map[string]string{}
	for key, value := range action.Params {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if needed == "" {
			return mailRuleAction{}, mailValidationParamError(flag, "action %s accepts no parameters", action.Kind)
		}
		if key != needed {
			return mailRuleAction{}, mailValidationParamError(flag, "action %s only accepts %s", action.Kind, needed)
		}
		normalized[needed] = value
	}
	if needed != "" && normalized[needed] == "" {
		return mailRuleAction{}, mailValidationParamError(flag, "action %s requires %s (example: %s:%s=xxx)", action.Kind, needed, action.Kind, needed)
	}
	if len(normalized) == 0 {
		normalized = nil
	}
	action.Params = normalized
	return action, nil
}

func requiredActionParam(kind string) string {
	switch kind {
	case "move_folder":
		return "folder_id"
	default:
		return ""
	}
}

func parseRuleConditionGrammar(raw, flag string) (mailRuleCondition, error) {
	parts := strings.SplitN(raw, ":", 3)
	cond := mailRuleCondition{Field: parts[0]}
	if len(parts) >= 2 {
		cond.Operator = parts[1]
	}
	if len(parts) == 3 {
		cond.Value = parts[2]
	}
	return validateRuleCondition(cond, flag)
}

func decodeSingleRuleJSON(raw, flag string) (any, error) {
	var v any
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, mailValidationParamError(flag, "invalid %s JSON: %v", flag, err).WithCause(err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, mailValidationParamError(flag, "invalid %s JSON: multiple JSON values", flag)
		}
		return nil, mailValidationParamError(flag, "invalid %s JSON: trailing data after first value: %v", flag, err).WithCause(err)
	}
	return v, nil
}

func validateRuleCollectionSize(param, noun string, count int) error {
	if count > mailRuleCollectionMax {
		return mailValidationParamError(param, "rule %s count %d exceeds the limit of %d", noun, count, mailRuleCollectionMax)
	}
	return nil
}

func validateRuleCollectionAppend(param, noun string, current, added int) error {
	return validateRuleCollectionSize(param, noun, current+added)
}

func trackRuleAggregateInputBytes(param, noun string, total *int, expanded string) error {
	if total == nil || expanded == "" {
		return nil
	}
	*total += len(expanded)
	if *total > mailRuleInputFileMaxBytes {
		return mailValidationParamError(param, "aggregate rule %s input exceeds %d bytes limit", noun, mailRuleInputFileMaxBytes)
	}
	return nil
}

func appendUnknownObjectFields(unknowns []mailRuleUnknown, path string, raw map[string]interface{}, allowed map[string]struct{}) []mailRuleUnknown {
	for key, value := range raw {
		if _, ok := allowed[key]; ok {
			continue
		}
		unknowns = append(unknowns, mailRuleUnknown{Path: path + "." + key, Reason: "unknown field on known raw object", Raw: value})
	}
	return unknowns
}

func validateRuleCondition(cond mailRuleCondition, flag string) (mailRuleCondition, error) {
	field, ok := mailRuleFieldByName[strings.TrimSpace(cond.Field)]
	if !ok {
		return mailRuleCondition{}, mailValidationParamError(flag, "%s", unknownRuleAliasMessage("condition field", cond.Field, "fields", mailRuleFields))
	}
	cond.Field = field.Canonical
	if !field.NeedsArg {
		if cond.Operator != "" || cond.Value != "" {
			return mailRuleCondition{}, mailValidationParamError(flag, "condition field %s is boolean and accepts no operator/value", cond.Field)
		}
		return cond, nil
	}
	op, ok := mailRuleOpByName[strings.TrimSpace(cond.Operator)]
	if !ok {
		return mailRuleCondition{}, mailValidationParamError(flag, "%s", unknownRuleAliasMessage("condition operator", cond.Operator, "operators", mailRuleOperators))
	}
	cond.Operator = op.Canonical
	if op.NeedsArg && strings.TrimSpace(cond.Value) == "" {
		return mailRuleCondition{}, mailValidationParamError(flag, "condition %s:%s requires a non-empty value", cond.Field, cond.Operator)
	}
	if !op.NeedsArg && strings.TrimSpace(cond.Value) != "" {
		return mailRuleCondition{}, mailValidationParamError(flag, "condition operator %s accepts no value", cond.Operator)
	}
	return cond, nil
}

func acceptedAliasList(items []mailRuleAlias) string {
	var values []string
	for _, item := range items {
		values = append(values, item.Canonical)
		values = append(values, item.Aliases...)
	}
	sort.Strings(values)
	return strings.Join(values, ", ")
}

func unknownRuleAliasMessage(noun, raw, acceptedKind string, items []mailRuleAlias) string {
	accepted := acceptedAliasList(items)
	if suggestion := closestRuleAlias(raw, items); suggestion != "" {
		return fmt.Sprintf("unknown rule %s %q; did you mean %q? Accepted %s and aliases: %s", noun, raw, suggestion, acceptedKind, accepted)
	}
	return fmt.Sprintf("unknown rule %s %q. Accepted %s and aliases: %s", noun, raw, acceptedKind, accepted)
}

func closestRuleAlias(raw string, items []mailRuleAlias) string {
	typed := strings.ToLower(strings.TrimSpace(raw))
	if typed == "" {
		return ""
	}
	candidates := make([]string, 0, len(items)*2)
	for _, item := range items {
		candidates = append(candidates, item.Canonical)
		candidates = append(candidates, item.Aliases...)
	}
	if matches := internalsuggest.Closest(typed, candidates, 1); len(matches) > 0 {
		return matches[0]
	}
	return ""
}

func encodeRuleSpec(spec *mailRuleSpec) (map[string]any, error) {
	raw := map[string]any{
		"name":                     spec.Rule.Name,
		"is_enable":                spec.Rule.Enabled,
		"ignore_the_rest_of_rules": spec.Rule.StopAfterMatch,
		"condition": map[string]any{
			"match_type": encodeRuleMatch(spec.Rule.Match),
			"items":      encodeRuleConditions(spec.Rule.Conditions),
		},
		"action": map[string]any{
			"items": encodeRuleActions(spec.Rule.Actions),
		},
	}
	if spec.Rule.ID != "" {
		raw["rule_id"] = spec.Rule.ID
	}
	return raw, nil
}

func encodeRuleMatch(match string) int {
	if match == "any" {
		return 2
	}
	return 1
}

func encodeRuleConditions(conditions []mailRuleCondition) []map[string]any {
	out := make([]map[string]any, 0, len(conditions))
	for _, cond := range conditions {
		item := map[string]any{"type": mailRuleFieldByName[cond.Field].Code}
		if cond.Operator != "" {
			item["operator"] = mailRuleOpByName[cond.Operator].Code
		}
		if cond.Value != "" {
			item["input"] = cond.Value
		}
		out = append(out, item)
	}
	return out
}

func encodeRuleActions(actions []mailRuleAction) []map[string]any {
	out := make([]map[string]any, 0, len(actions))
	for _, action := range actions {
		item := map[string]any{"type": mailRuleActionByName[action.Kind].Code}
		for k, v := range action.Params {
			if strings.TrimSpace(v) != "" {
				if action.Kind == "move_folder" && k == "folder_id" {
					item["input"] = v
					continue
				}
				item[k] = v
			}
		}
		out = append(out, item)
	}
	return out
}

func listMailRuleEnvelopes(rt *common.RuntimeContext, mailboxID string) ([]mailRuleEnvelope, error) {
	var out []mailRuleEnvelope
	data, err := rt.CallAPITyped("GET", mailRuleCollectionPath(mailboxID), nil, nil)
	if err != nil {
		return nil, err
	}
	for _, raw := range extractRuleItems(data) {
		out = append(out, decodeMailRuleEnvelope(raw, mailboxID))
	}
	return out, nil
}

func extractRuleItems(data map[string]interface{}) []map[string]any {
	for _, key := range []string{"rules", "items", "rule_list", "user_rules"} {
		if items, ok := data[key].([]interface{}); ok {
			return mapsFromSlice(items)
		}
	}
	if rule, ok := data["rule"].(map[string]interface{}); ok {
		return []map[string]any{copyInterfaceMap(rule)}
	}
	if len(data) > 0 {
		if _, hasName := data["name"]; hasName {
			return []map[string]any{copyInterfaceMap(data)}
		}
	}
	return nil
}

func mapsFromSlice(items []interface{}) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, copyInterfaceMap(m))
		}
	}
	return out
}

func firstRuleObject(data map[string]interface{}) map[string]any {
	items := extractRuleItems(data)
	if len(items) > 0 {
		return items[0]
	}
	return copyInterfaceMap(data)
}

func isRecognizableRuleObject(raw map[string]any) bool {
	if len(raw) == 0 {
		return false
	}
	for _, key := range []string{"rule_id", "id", "name", "rule_name", "is_enable", "enabled", "condition", "action"} {
		if _, ok := raw[key]; ok {
			return true
		}
	}
	return false
}

func envelopeFromRuleSpec(spec *mailRuleSpec, unknowns []mailRuleUnknown, raw map[string]any) mailRuleEnvelope {
	if spec == nil {
		return mailRuleEnvelope{Unknowns: unknowns, Raw: copyMap(raw)}
	}
	return mailRuleEnvelope{
		RuleID:       spec.Rule.ID,
		Name:         spec.Rule.Name,
		Enabled:      spec.Rule.Enabled,
		Order:        firstPresent(raw, "order", "sequence", "priority", "index"),
		Description:  describeMailRule(spec.Rule, unknowns),
		SemanticSpec: spec,
		Unknowns:     unknowns,
		Raw:          copyMap(raw),
	}
}

func decodeMailRuleEnvelope(raw map[string]any, mailboxID string) mailRuleEnvelope {
	spec := &mailRuleSpec{Version: mailRuleSpecVersion}
	spec.Mailbox.UserMailboxID = mailboxID
	spec.Rule.ID = mailRuleFirstString(raw, "rule_id", "id")
	spec.Rule.Name = mailRuleFirstString(raw, "name", "rule_name")
	spec.Rule.Enabled = boolValueDefault(raw["is_enable"], boolValueDefault(raw["enabled"], true))
	spec.Rule.StopAfterMatch = boolValueDefault(raw["ignore_the_rest_of_rules"], false)
	spec.Rule.Match = "all"
	var unknowns []mailRuleUnknown
	if condition, ok := mapValue(raw["condition"]); ok {
		unknowns = appendUnknownObjectFields(unknowns, "condition", condition, map[string]struct{}{"match_type": {}, "items": {}})
		if mt, ok := intValue(condition["match_type"]); ok {
			switch mt {
			case 1:
				spec.Rule.Match = "all"
			case 2:
				spec.Rule.Match = "any"
			default:
				spec.Rule.Match = ""
				unknowns = append(unknowns, mailRuleUnknown{Path: "condition.match_type", Reason: "unknown match_type enum", Raw: condition["match_type"]})
			}
		}
		spec.Rule.Conditions, unknowns = decodeRuleConditions(condition["items"], unknowns)
	}
	if action, ok := mapValue(raw["action"]); ok {
		unknowns = appendUnknownObjectFields(unknowns, "action", action, map[string]struct{}{"items": {}})
		spec.Rule.Actions, unknowns = decodeRuleActions(action["items"], unknowns)
	}
	env := mailRuleEnvelope{
		RuleID:       spec.Rule.ID,
		Name:         spec.Rule.Name,
		Enabled:      spec.Rule.Enabled,
		Order:        firstPresent(raw, "order", "sequence", "priority", "index"),
		Description:  describeMailRule(spec.Rule, unknowns),
		SemanticSpec: spec,
		Unknowns:     unknowns,
		Raw:          copyMap(raw),
	}
	return env
}

func decodeRuleConditions(raw any, unknowns []mailRuleUnknown) ([]mailRuleCondition, []mailRuleUnknown) {
	items, _ := raw.([]interface{})
	out := make([]mailRuleCondition, 0, len(items))
	for i, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			unknowns = append(unknowns, mailRuleUnknown{Path: fmt.Sprintf("condition.items[%d]", i), Reason: "condition item is not an object", Raw: item})
			continue
		}
		code, ok := intValue(m["type"])
		if !ok {
			unknowns = append(unknowns, mailRuleUnknown{Path: fmt.Sprintf("condition.items[%d].type", i), Reason: "missing condition type", Raw: m})
			continue
		}
		field, ok := mailRuleFieldByCode[code]
		if !ok {
			unknowns = append(unknowns, mailRuleUnknown{Path: fmt.Sprintf("condition.items[%d]", i), Reason: "unknown condition type enum", Raw: m})
			continue
		}
		cond := mailRuleCondition{Field: field.Canonical}
		if field.NeedsArg {
			opCode, ok := intValue(m["operator"])
			if !ok {
				unknowns = append(unknowns, mailRuleUnknown{Path: fmt.Sprintf("condition.items[%d].operator", i), Reason: "missing operator for value condition", Raw: m})
				continue
			}
			op, ok := mailRuleOpByCode[opCode]
			if !ok {
				unknowns = append(unknowns, mailRuleUnknown{Path: fmt.Sprintf("condition.items[%d].operator", i), Reason: "unknown condition operator enum", Raw: m})
				continue
			}
			allowed := map[string]struct{}{"type": {}}
			if field.NeedsArg {
				allowed["operator"] = struct{}{}
				allowed["input"] = struct{}{}
				allowed["value"] = struct{}{}
			}
			unknowns = appendUnknownObjectFields(unknowns, fmt.Sprintf("condition.items[%d]", i), m, allowed)
			cond.Operator = op.Canonical
			cond.Value = mailRuleFirstString(m, "input", "value")
		} else {
			unknowns = appendUnknownObjectFields(unknowns, fmt.Sprintf("condition.items[%d]", i), m, map[string]struct{}{"type": {}})
		}
		out = append(out, cond)
	}
	return out, unknowns
}

func decodeRuleActions(raw any, unknowns []mailRuleUnknown) ([]mailRuleAction, []mailRuleUnknown) {
	items, _ := raw.([]interface{})
	out := make([]mailRuleAction, 0, len(items))
	for i, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			unknowns = append(unknowns, mailRuleUnknown{Path: fmt.Sprintf("action.items[%d]", i), Reason: "action item is not an object", Raw: item})
			continue
		}
		code, ok := intValue(m["type"])
		if !ok {
			unknowns = append(unknowns, mailRuleUnknown{Path: fmt.Sprintf("action.items[%d].type", i), Reason: "missing action type", Raw: m})
			continue
		}
		action, ok := mailRuleActionByCode[code]
		if !ok {
			unknowns = append(unknowns, mailRuleUnknown{Path: fmt.Sprintf("action.items[%d]", i), Reason: "unknown action type enum", Raw: m})
			continue
		}
		allowed := map[string]struct{}{"type": {}}
		if param := requiredActionParam(action.Canonical); param != "" {
			allowed[param] = struct{}{}
			allowed["input"] = struct{}{}
			allowed["params"] = struct{}{}
			if nested, ok := mapValue(m["params"]); ok {
				unknowns = appendUnknownObjectFields(unknowns, fmt.Sprintf("action.items[%d].params", i), nested, map[string]struct{}{param: {}, "input": {}})
			}
		}
		unknowns = appendUnknownObjectFields(unknowns, fmt.Sprintf("action.items[%d]", i), m, allowed)
		out = append(out, mailRuleAction{Kind: action.Canonical, Params: decodeActionParams(action.Canonical, m)})
	}
	return out, unknowns
}

func decodeActionParams(kind string, m map[string]interface{}) map[string]string {
	key := requiredActionParam(kind)
	if key == "" {
		return nil
	}
	params := map[string]string{}
	if v := mailRuleFirstString(m, key); v != "" {
		params[key] = v
	}
	if kind == "move_folder" {
		if v := mailRuleFirstString(m, "input"); v != "" {
			params[key] = v
		}
	}
	if nested, ok := mapValue(m["params"]); ok {
		if v := mailRuleFirstString(nested, key); v != "" {
			params[key] = v
		}
		if kind == "move_folder" {
			if v := mailRuleFirstString(nested, "input"); v != "" {
				params[key] = v
			}
		}
	}
	if len(params) == 0 {
		return nil
	}
	return params
}

func describeMailRule(rule mailRuleSemantic, unknowns []mailRuleUnknown) string {
	name := rule.Name
	if name == "" {
		name = rule.ID
	}
	if name == "" {
		name = "未命名规则"
	}
	state := "已启用"
	if !rule.Enabled {
		state = "已停用"
	}
	conditions := describeConditions(rule.Match, rule.Conditions)
	actions := describeActions(rule.Actions)
	flow := "命中后继续执行后续规则"
	if rule.StopAfterMatch {
		flow = "命中后停止执行后续规则"
	}
	desc := fmt.Sprintf("规则「%s」%s：当%s时，%s；%s。", name, state, conditions, actions, flow)
	if len(unknowns) > 0 {
		desc += fmt.Sprintf(" 包含 %d 个当前 shortcut 无法识别的 raw 片段，已保留 raw。", len(unknowns))
	}
	return desc
}

func describeConditions(match string, conditions []mailRuleCondition) string {
	if len(conditions) == 0 {
		return "满足未知条件"
	}
	parts := make([]string, 0, len(conditions))
	for _, cond := range conditions {
		field := mailRuleFieldByName[cond.Field]
		if !field.NeedsArg {
			parts = append(parts, field.Label)
			continue
		}
		op := mailRuleOpByName[cond.Operator]
		if cond.Value == "" {
			parts = append(parts, field.Label+op.Label)
		} else {
			parts = append(parts, fmt.Sprintf("%s%s「%s」", field.Label, op.Label, cond.Value))
		}
	}
	joiner := "且"
	if match == "any" {
		joiner = "或"
	}
	return strings.Join(parts, joiner)
}

func describeActions(actions []mailRuleAction) string {
	if len(actions) == 0 {
		return "执行未知动作"
	}
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		label := mailRuleActionByName[action.Kind].Label
		if p := requiredActionParam(action.Kind); p != "" && action.Params[p] != "" {
			label = fmt.Sprintf("%s（%s=%s）", label, p, action.Params[p])
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "，")
}

func getMailRuleEnvelope(rt *common.RuntimeContext, mailboxID, ruleID string) (mailRuleEnvelope, error) {
	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" {
		return mailRuleEnvelope{}, mailValidationParamError("--rule-id", "--rule-id is required")
	}
	envelopes, err := listMailRuleEnvelopes(rt, mailboxID)
	if err != nil {
		return mailRuleEnvelope{}, mailDecorateProblemMessage(err, "list mail rules failed")
	}
	for _, env := range envelopes {
		if env.RuleID == ruleID {
			return env, nil
		}
	}
	return mailRuleEnvelope{}, mailFailedPreconditionError("rule_id %s not found after scanning %d rule(s)", ruleID, len(envelopes))
}

func hasRuleUpdateChange(rt *common.RuntimeContext) bool {
	for _, flag := range []string{"name", "enable", "disable", "match", "stop-after-match", "continue-after-match", "condition", "conditions", "action", "actions"} {
		if rt.Changed(flag) {
			return true
		}
	}
	return false
}

func buildRuleUpdateDryRunPatch(rt *common.RuntimeContext, partial *mailRuleSpec) (map[string]any, map[string]any) {
	semanticPatch := map[string]any{}
	rawPatch := map[string]any{}
	if partial == nil {
		return semanticPatch, rawPatch
	}
	if rt.Changed("name") {
		semanticPatch["name"] = partial.Rule.Name
		rawPatch["name"] = partial.Rule.Name
	}
	if rt.Changed("enable") || rt.Changed("disable") {
		semanticPatch["enabled"] = partial.Rule.Enabled
		rawPatch["is_enable"] = partial.Rule.Enabled
	}
	if rt.Changed("match") {
		semanticPatch["match"] = partial.Rule.Match
		rawPatch["condition"] = map[string]any{"match_type": encodeRuleMatch(partial.Rule.Match)}
	}
	if rt.Changed("stop-after-match") || rt.Changed("continue-after-match") {
		semanticPatch["stop_after_match"] = partial.Rule.StopAfterMatch
		rawPatch["ignore_the_rest_of_rules"] = partial.Rule.StopAfterMatch
	}
	if rt.Changed("condition") || rt.Changed("conditions") {
		semanticPatch["conditions"] = partial.Rule.Conditions
		conditionPatch, _ := rawPatch["condition"].(map[string]any)
		if conditionPatch == nil {
			conditionPatch = map[string]any{}
		}
		conditionPatch["items"] = encodeRuleConditions(partial.Rule.Conditions)
		rawPatch["condition"] = conditionPatch
	}
	if rt.Changed("action") || rt.Changed("actions") {
		semanticPatch["actions"] = partial.Rule.Actions
		rawPatch["action"] = map[string]any{"items": encodeRuleActions(partial.Rule.Actions)}
	}
	return semanticPatch, rawPatch
}

func mergeRuleUpdate(rt *common.RuntimeContext, current mailRuleEnvelope) (*mailRuleSpec, map[string]any, []map[string]any, error) {
	if !hasRuleUpdateChange(rt) {
		return nil, nil, nil, mailValidationParamError("--rule-id", "at least one update field must be provided")
	}
	partial, _, err := buildRuleSpecFromFlags(rt, false)
	if err != nil {
		return nil, nil, nil, err
	}
	if current.SemanticSpec == nil {
		return nil, nil, nil, mailFailedPreconditionError("current rule contains no decodable semantic spec; use mail user_mailbox.rules update with raw body")
	}
	target := *current.SemanticSpec
	target.Rule.ID = current.RuleID
	diff := []map[string]any{}
	if rt.Changed("name") {
		diff = appendDiffIfChanged(diff, "name", target.Rule.Name, partial.Rule.Name)
		target.Rule.Name = partial.Rule.Name
	}
	if rt.Changed("enable") || rt.Changed("disable") {
		diff = appendDiffIfChanged(diff, "enabled", target.Rule.Enabled, partial.Rule.Enabled)
		target.Rule.Enabled = partial.Rule.Enabled
	}
	if rt.Changed("match") {
		diff = appendDiffIfChanged(diff, "match", target.Rule.Match, partial.Rule.Match)
		target.Rule.Match = partial.Rule.Match
	}
	if rt.Changed("stop-after-match") || rt.Changed("continue-after-match") {
		diff = appendDiffIfChanged(diff, "stop_after_match", target.Rule.StopAfterMatch, partial.Rule.StopAfterMatch)
		target.Rule.StopAfterMatch = partial.Rule.StopAfterMatch
	}
	if rt.Changed("condition") || rt.Changed("conditions") {
		diff = appendDiffIfChanged(diff, "conditions", target.Rule.Conditions, partial.Rule.Conditions)
		target.Rule.Conditions = partial.Rule.Conditions
	}
	if rt.Changed("action") || rt.Changed("actions") {
		diff = appendDiffIfChanged(diff, "actions", target.Rule.Actions, partial.Rule.Actions)
		target.Rule.Actions = partial.Rule.Actions
	}
	raw, err := encodeRuleSpec(&target)
	if err != nil {
		return nil, nil, nil, err
	}
	return &target, mergeRuleUpdateBody(rt, current, raw), diff, nil
}

func mergeRuleUpdateBody(rt *common.RuntimeContext, current mailRuleEnvelope, encoded map[string]any) map[string]any {
	raw := ruleRequestBodyFromEnvelope(current)
	for _, key := range []string{"name", "is_enable", "ignore_the_rest_of_rules"} {
		if v, ok := encoded[key]; ok {
			raw[key] = v
		}
	}
	conditionsChanged := rt.Changed("condition") || rt.Changed("conditions")
	if conditionsChanged {
		if currentCondition, ok := mapValue(raw["condition"]); ok && !rt.Changed("match") {
			merged := copyInterfaceMap(currentCondition)
			if encodedCondition, ok := encoded["condition"].(map[string]any); ok {
				merged["items"] = encodedCondition["items"]
			}
			raw["condition"] = merged
		} else {
			raw["condition"] = encoded["condition"]
		}
	} else if raw["condition"] == nil {
		raw["condition"] = encoded["condition"]
	} else if rt.Changed("match") {
		if currentCondition, ok := mapValue(raw["condition"]); ok {
			merged := copyInterfaceMap(currentCondition)
			if encodedCondition, ok := encoded["condition"].(map[string]any); ok {
				merged["match_type"] = encodedCondition["match_type"]
			}
			raw["condition"] = merged
		} else {
			raw["condition"] = encoded["condition"]
		}
	}
	if rt.Changed("action") || rt.Changed("actions") || raw["action"] == nil {
		raw["action"] = encoded["action"]
	}
	return raw
}

func ruleRequestBodyFromEnvelope(env mailRuleEnvelope) map[string]any {
	raw := map[string]any{}
	encoded := map[string]any{}
	if env.SemanticSpec != nil {
		if v, err := encodeRuleSpec(env.SemanticSpec); err == nil {
			encoded = v
		}
	}
	for _, key := range []string{"name", "is_enable", "ignore_the_rest_of_rules"} {
		if v, ok := env.Raw[key]; ok {
			raw[key] = v
			continue
		}
		if v, ok := encoded[key]; ok {
			raw[key] = v
		}
	}
	for _, key := range []string{"condition", "action"} {
		if v, ok := env.Raw[key]; ok {
			raw[key] = v
			continue
		}
		if v, ok := encoded[key]; ok {
			raw[key] = v
		}
	}
	return raw
}

func diffEntry(field string, before, after any) map[string]any {
	return map[string]any{"field": field, "before": before, "after": after}
}

func appendDiffIfChanged(diff []map[string]any, field string, before, after any) []map[string]any {
	if reflect.DeepEqual(before, after) {
		return diff
	}
	return append(diff, diffEntry(field, before, after))
}

func validateRuleReorderFlags(rt *common.RuntimeContext) error {
	full := len(rt.StrSlice("rule-ids")) > 0
	move := strings.TrimSpace(rt.Str("move-rule-id")) != ""
	if full == move {
		return mailValidationError("exactly one of --rule-ids or --move-rule-id is required")
	}
	targets := 0
	for _, set := range []bool{
		strings.TrimSpace(rt.Str("before-rule-id")) != "",
		strings.TrimSpace(rt.Str("after-rule-id")) != "",
		rt.Bool("to-top"),
		rt.Bool("to-bottom"),
	} {
		if set {
			targets++
		}
	}
	if full && targets > 0 {
		return mailValidationError("--rule-ids cannot be combined with move target flags")
	}
	if move && targets != 1 {
		return mailValidationError("move mode requires exactly one of --before-rule-id, --after-rule-id, --to-top, --to-bottom")
	}
	return nil
}

func buildRuleTargetOrder(rt *common.RuntimeContext, current []mailRuleEnvelope) ([]string, error) {
	currentIDs := envelopeRuleIDs(current)
	if ids := normalizeRuleIDs(rt.StrSlice("rule-ids")); len(ids) > 0 {
		if err := validateFullRuleOrder(ids, currentIDs); err != nil {
			return nil, err
		}
		return ids, nil
	}
	moveID := strings.TrimSpace(rt.Str("move-rule-id"))
	order := removeString(currentIDs, moveID)
	if len(order) == len(currentIDs) {
		return nil, mailValidationParamError("--move-rule-id", "--move-rule-id %s is not in current rule order", moveID)
	}
	switch {
	case rt.Bool("to-top"):
		return append([]string{moveID}, order...), nil
	case rt.Bool("to-bottom"):
		return append(order, moveID), nil
	case strings.TrimSpace(rt.Str("before-rule-id")) != "":
		return insertRelative(order, moveID, strings.TrimSpace(rt.Str("before-rule-id")), false)
	default:
		return insertRelative(order, moveID, strings.TrimSpace(rt.Str("after-rule-id")), true)
	}
}

func validateFullRuleOrder(target, current []string) error {
	if len(target) != len(current) {
		return mailValidationParamError("--rule-ids", "--rule-ids must contain every current rule id exactly once (got %d, want %d)", len(target), len(current))
	}
	want := make(map[string]int, len(current))
	for _, id := range current {
		want[id]++
	}
	for _, id := range target {
		want[id]--
	}
	for id, count := range want {
		if count != 0 {
			return mailValidationParamError("--rule-ids", "--rule-ids mismatch for %s; run +rule-list first and submit the complete order", id)
		}
	}
	return nil
}

func normalizeRuleIDs(ids []string) []string {
	var out []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func insertRelative(order []string, moveID, targetID string, after bool) ([]string, error) {
	if targetID == "" {
		return nil, mailValidationError("target rule id is required")
	}
	out := make([]string, 0, len(order)+1)
	inserted := false
	for _, id := range order {
		if !after && id == targetID {
			out = append(out, moveID)
			inserted = true
		}
		out = append(out, id)
		if after && id == targetID {
			out = append(out, moveID)
			inserted = true
		}
	}
	if !inserted {
		return nil, mailValidationError("target rule id %s is not in current rule order", targetID)
	}
	return out, nil
}

func removeString(values []string, remove string) []string {
	out := values[:0]
	for _, value := range values {
		if value != remove {
			out = append(out, value)
		}
	}
	return out
}

func envelopeRuleIDs(envelopes []mailRuleEnvelope) []string {
	ids := make([]string, 0, len(envelopes))
	for _, env := range envelopes {
		if env.RuleID != "" {
			ids = append(ids, env.RuleID)
		}
	}
	return ids
}

func filterMailRuleEnvelopes(envelopes []mailRuleEnvelope, filter string) []mailRuleEnvelope {
	out := envelopes[:0]
	for _, env := range envelopes {
		if strings.Contains(strings.ToLower(env.Name), filter) {
			out = append(out, env)
		}
	}
	return out
}

func printMailRuleTable(w io.Writer, envelopes []mailRuleEnvelope) {
	rows := make([]map[string]interface{}, 0, len(envelopes))
	for _, env := range envelopes {
		rows = append(rows, map[string]interface{}{
			"rule_id":     env.RuleID,
			"name":        env.Name,
			"enabled":     env.Enabled,
			"description": env.Description,
			"unknowns":    len(env.Unknowns),
		})
	}
	output.PrintTable(w, rows)
}

func copyInterfaceMap(in map[string]interface{}) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mapValue(v any) (map[string]interface{}, bool) {
	if m, ok := v.(map[string]interface{}); ok {
		return m, true
	}
	return nil, false
}

func intValue(v any) (int, bool) {
	switch typed := v.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		i, err := typed.Int64()
		return int(i), err == nil
	case string:
		i, err := strconv.Atoi(typed)
		return i, err == nil
	default:
		return 0, false
	}
}

func boolValueDefault(v any, fallback bool) bool {
	switch typed := v.(type) {
	case bool:
		return typed
	case string:
		if parsed, err := strconv.ParseBool(typed); err == nil {
			return parsed
		}
	case float64:
		return typed != 0
	}
	return fallback
}

func mailRuleFirstString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
			if n, ok := intValue(v); ok {
				return strconv.Itoa(n)
			}
		}
	}
	return ""
}

func firstPresent(m map[string]any, keys ...string) interface{} {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return nil
}
