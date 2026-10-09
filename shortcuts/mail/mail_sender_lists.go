// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"io"
	netmail "net/mail"
	"regexp"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	mailSenderListBatchMax = 100
)

var mailSenderDomainRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

type mailSenderListKind struct {
	Name     string
	Segment  string
	Label    string
	ItemName string
}

type mailSenderEntry struct {
	Sender     string `json:"sender"`
	SenderType int    `json:"sender_type"`
}

type mailSenderInvalid struct {
	Input  string `json:"input"`
	Reason string `json:"reason"`
}

type mailSenderMutationInput struct {
	Kind     mailSenderListKind
	Mailbox  string
	Entries  []mailSenderEntry
	Invalids []mailSenderInvalid
}

type mailSenderMutationOutput struct {
	Type          string                 `json:"type"`
	MailboxID     string                 `json:"mailbox_id"`
	Action        string                 `json:"action"`
	Applied       []mailSenderEntry      `json:"applied_senders,omitempty"`
	Removed       []mailSenderEntry      `json:"removed_senders,omitempty"`
	Invalid       []mailSenderInvalid    `json:"invalid_senders,omitempty"`
	SuccessCount  int                    `json:"success_count"`
	FilteredCount int                    `json:"filtered_count"`
	Response      map[string]interface{} `json:"response,omitempty"`
}

type mailSenderListOutput struct {
	Type          string                 `json:"type"`
	MailboxID     string                 `json:"mailbox_id"`
	Keyword       string                 `json:"keyword,omitempty"`
	PageSize      int                    `json:"page_size,omitempty"`
	PageToken     string                 `json:"page_token,omitempty"`
	NextPageToken string                 `json:"next_page_token,omitempty"`
	HasMore       bool                   `json:"has_more,omitempty"`
	Senders       []interface{}          `json:"senders"`
	Total         int                    `json:"total"`
	Response      map[string]interface{} `json:"response,omitempty"`
}

var mailSenderListCommonFlags = []common.Flag{
	{Name: "type", Required: true, Enum: []string{"allow", "block"}, Desc: "Sender list type: allow for trusted senders, block for blocked senders."},
	{Name: "mailbox", Default: "me", Desc: "User mailbox ID or address that owns the sender list (default: me)."},
	{Name: "user-mailbox-id", Hidden: true, Desc: "Compatibility alias for --mailbox."},
}

var mailSenderListReadFlags = append([]common.Flag{}, append(mailSenderListCommonFlags,
	common.Flag{Name: "keyword", Desc: "Optional sender email address or domain prefix to search."},
	common.Flag{Name: "page-size", Type: "int", Default: "20", Desc: "Page size for the sender list request (1-100)."},
	common.Flag{Name: "page-token", Desc: "Cursor token returned as next_page_token by the previous page."},
)...)

var mailSenderListWriteFlags = append([]common.Flag{}, append(mailSenderListCommonFlags,
	common.Flag{Name: "sender", Type: "string_slice", Required: true, Desc: "Sender email address or domain. Comma-separated or repeat the flag (max 100 valid entries)."},
)...)

// MailSenderList lists one user mailbox allow/block sender list page.
var MailSenderList = common.Shortcut{
	Service:     "mail",
	Command:     "+sender-list",
	Description: "List trusted or blocked senders for a user mailbox. Use --type allow or --type block; supports page_token pagination.",
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox.message:readonly"},
	ConditionalScopes: []string{
		"mail:user_mailbox.message:modify",
	},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags:     mailSenderListReadFlags,
	Normalize: normalizeMailSenderMailboxCompatibility,
	Validate:  validateMailSenderListRead,
	DryRun:    dryRunMailSenderListRead,
	Execute:   executeMailSenderList,
}

// MailSenderSearch searches one user mailbox allow/block sender list by keyword.
var MailSenderSearch = common.Shortcut{
	Service:     "mail",
	Command:     "+sender-search",
	Description: "Search trusted or blocked senders for a user mailbox by sender email/domain prefix.",
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox.message:readonly"},
	ConditionalScopes: []string{
		"mail:user_mailbox.message:modify",
	},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags:     mailSenderListReadFlags,
	Normalize: normalizeMailSenderMailboxCompatibility,
	Validate: func(ctx context.Context, rt *common.RuntimeContext) error {
		if err := validateMailSenderListRead(ctx, rt); err != nil {
			return err
		}
		if strings.TrimSpace(rt.Str("keyword")) == "" {
			return mailValidationParamError("--keyword", "--keyword is required for +sender-search")
		}
		return nil
	},
	DryRun:  dryRunMailSenderListRead,
	Execute: executeMailSenderList,
}

// MailSenderSet adds sender entries to the selected allow/block list.
var MailSenderSet = common.Shortcut{
	Service:     "mail",
	Command:     "+sender-set",
	Description: "Add email addresses or domains to a user mailbox allow/block sender list. Inputs are trimmed, validated, and lowercased before writing.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags:       mailSenderListWriteFlags,
	Normalize:   normalizeMailSenderMailboxCompatibility,
	Validate: func(ctx context.Context, rt *common.RuntimeContext) error {
		_, err := buildMailSenderMutationInput(rt, false)
		return err
	},
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		input, _ := buildMailSenderMutationInput(rt, false)
		return common.NewDryRunAPI().
			Desc("Add valid sender entries; invalid inputs are filtered when at least one valid sender remains").
			Set("type", input.Kind.Name).
			Set("mailbox_id", input.Mailbox).
			Set("invalid_senders", input.Invalids).
			POST(mailSenderListPath(input.Mailbox, input.Kind, "batch_create")).
			Body(map[string]interface{}{"senders": input.Entries})
	},
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		return executeMailSenderMutation(rt, false)
	},
}

// MailSenderDelete removes sender entries from the selected allow/block list.
var MailSenderDelete = common.Shortcut{
	Service:     "mail",
	Command:     "+sender-delete",
	Description: "Remove email addresses or domains from a user mailbox allow/block sender list. Mixed-case input also submits lowercase variants for historical entries.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags:       mailSenderListWriteFlags,
	Normalize:   normalizeMailSenderMailboxCompatibility,
	Validate: func(ctx context.Context, rt *common.RuntimeContext) error {
		_, err := buildMailSenderMutationInput(rt, true)
		return err
	},
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		input, _ := buildMailSenderMutationInput(rt, true)
		return common.NewDryRunAPI().
			Desc("Remove valid sender entries; mixed-case inputs also include lowercase variants for compatibility").
			Set("type", input.Kind.Name).
			Set("mailbox_id", input.Mailbox).
			Set("invalid_senders", input.Invalids).
			POST(mailSenderListPath(input.Mailbox, input.Kind, "batch_remove")).
			Body(map[string]interface{}{"senders": input.Entries})
	},
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		return executeMailSenderMutation(rt, true)
	},
}

func normalizeMailSenderMailboxCompatibility(ctx context.Context, flags *common.FlagContext) error {
	if flags.Changed("mailbox") && flags.Changed("user-mailbox-id") {
		return mailValidationParamError("--user-mailbox-id", "--user-mailbox-id is a compatibility alias for --mailbox; pass only one of them")
	}
	if !flags.Changed("user-mailbox-id") {
		return nil
	}
	value := strings.TrimSpace(flags.Str("user-mailbox-id"))
	if value == "" {
		return mailValidationParamError("--user-mailbox-id", "--user-mailbox-id must not be empty")
	}
	return flags.SetCanonicalFrom("user-mailbox-id", "mailbox", value)
}

func validateMailSenderListRead(ctx context.Context, rt *common.RuntimeContext) error {
	if _, err := resolveMailSenderListKind(rt.Str("type")); err != nil {
		return err
	}
	if strings.TrimSpace(resolveMailboxID(rt)) == "" {
		return mailValidationParamError("--mailbox", "--mailbox must not be empty")
	}
	pageSize := rt.Int("page-size")
	if pageSize < 1 || pageSize > mailSenderListBatchMax {
		return mailValidationParamError("--page-size", "--page-size must be between 1 and 100")
	}
	return nil
}

func dryRunMailSenderListRead(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
	kind, _ := resolveMailSenderListKind(rt.Str("type"))
	mailboxID := resolveMailboxID(rt)
	params := mailSenderListParams(rt)
	return common.NewDryRunAPI().
		Desc("List or search one page of user mailbox sender-list entries").
		Set("type", kind.Name).
		Set("mailbox_id", mailboxID).
		GET(mailSenderListPath(mailboxID, kind, "")).
		Params(params)
}

func executeMailSenderList(ctx context.Context, rt *common.RuntimeContext) error {
	kind, err := resolveMailSenderListKind(rt.Str("type"))
	if err != nil {
		return err
	}
	mailboxID := resolveMailboxID(rt)
	params := mailSenderListParams(rt)
	data, err := rt.CallAPITyped("GET", mailSenderListPath(mailboxID, kind, ""), params, nil)
	if err != nil {
		return mailDecorateProblemMessage(err, "list %s senders failed", kind.Name)
	}
	items := mailSenderListItems(data, kind)
	out := mailSenderListOutput{
		Type:          kind.Name,
		MailboxID:     mailboxID,
		Keyword:       firstMailSenderString(params, "keyword"),
		PageSize:      rt.Int("page-size"),
		PageToken:     strings.TrimSpace(rt.Str("page-token")),
		NextPageToken: firstMailSenderString(data, "next_page_token", "page_token"),
		HasMore:       firstMailSenderBool(data, "has_more"),
		Senders:       items,
		Total:         len(items),
		Response:      data,
	}
	rt.OutFormat(out, &output.Meta{
		Count: len(items),
		Pagination: &output.PaginationMeta{
			Complete:  !out.HasMore && out.NextPageToken == "",
			Pages:     1,
			Items:     len(items),
			NextToken: out.NextPageToken,
		},
	}, func(w io.Writer) {
		printMailSenderList(w, out)
	})
	return nil
}

func executeMailSenderMutation(rt *common.RuntimeContext, remove bool) error {
	input, err := buildMailSenderMutationInput(rt, remove)
	if err != nil {
		return err
	}
	method := "batch_create"
	action := "set"
	if remove {
		method = "batch_remove"
		action = "delete"
	}
	data, err := rt.CallAPITyped("POST", mailSenderListPath(input.Mailbox, input.Kind, method), nil, map[string]interface{}{"senders": input.Entries})
	if err != nil {
		return mailDecorateProblemMessage(err, "%s %s senders failed", action, input.Kind.Name)
	}
	out := mailSenderMutationOutput{
		Type:          input.Kind.Name,
		MailboxID:     input.Mailbox,
		Action:        action,
		Invalid:       input.Invalids,
		SuccessCount:  len(input.Entries),
		FilteredCount: len(input.Invalids),
		Response:      data,
	}
	if remove {
		out.Removed = input.Entries
	} else {
		out.Applied = input.Entries
	}
	rt.OutFormat(out, &output.Meta{Count: len(input.Entries)}, func(w io.Writer) {
		printMailSenderMutation(w, out)
	})
	return nil
}

func buildMailSenderMutationInput(rt *common.RuntimeContext, remove bool) (mailSenderMutationInput, error) {
	kind, err := resolveMailSenderListKind(rt.Str("type"))
	if err != nil {
		return mailSenderMutationInput{}, err
	}
	mailboxID := resolveMailboxID(rt)
	if strings.TrimSpace(mailboxID) == "" {
		return mailSenderMutationInput{}, mailValidationParamError("--mailbox", "--mailbox must not be empty")
	}
	entries, invalids := normalizeMailSenderEntries(rt.StrSlice("sender"), remove)
	if len(entries) == 0 {
		if len(invalids) == 0 {
			return mailSenderMutationInput{}, mailValidationParamError("--sender", "--sender is required")
		}
		return mailSenderMutationInput{}, mailValidationParamError("--sender", "no valid sender entries; first invalid input %q: %s", invalids[0].Input, invalids[0].Reason)
	}
	if len(entries) > mailSenderListBatchMax {
		return mailSenderMutationInput{}, mailValidationParamError("--sender", "too many valid sender entries: %d > %d", len(entries), mailSenderListBatchMax)
	}
	return mailSenderMutationInput{
		Kind:     kind,
		Mailbox:  mailboxID,
		Entries:  entries,
		Invalids: invalids,
	}, nil
}

func normalizeMailSenderEntries(raw []string, remove bool) ([]mailSenderEntry, []mailSenderInvalid) {
	var entries []mailSenderEntry
	var invalids []mailSenderInvalid
	seen := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		trimmed := strings.TrimSpace(value)
		entry, reason, ok := normalizeMailSenderEntry(trimmed)
		if !ok {
			invalids = append(invalids, mailSenderInvalid{Input: value, Reason: reason})
			continue
		}
		addMailSenderEntry(&entries, seen, entry)
		if remove && trimmed != entry.Sender {
			addMailSenderEntry(&entries, seen, mailSenderEntry{Sender: trimmed, SenderType: entry.SenderType})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].SenderType != entries[j].SenderType {
			return entries[i].SenderType < entries[j].SenderType
		}
		return entries[i].Sender < entries[j].Sender
	})
	return entries, invalids
}

func addMailSenderEntry(entries *[]mailSenderEntry, seen map[string]struct{}, entry mailSenderEntry) {
	key := fmt.Sprintf("%d:%s", entry.SenderType, entry.Sender)
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*entries = append(*entries, entry)
}

func normalizeMailSenderEntry(value string) (mailSenderEntry, string, bool) {
	if value == "" {
		return mailSenderEntry{}, "empty value", false
	}
	if strings.ContainsAny(value, "\r\n\t ") {
		return mailSenderEntry{}, "must not contain whitespace", false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f || common.IsDangerousUnicode(r) {
			return mailSenderEntry{}, "contains unsafe character", false
		}
	}
	lower := strings.ToLower(value)
	if strings.Contains(lower, "@") {
		addr, err := netmail.ParseAddress(lower)
		if err != nil || addr.Address != lower || strings.Count(lower, "@") != 1 {
			return mailSenderEntry{}, "invalid email address", false
		}
		domain := strings.TrimPrefix(lower[strings.LastIndex(lower, "@"):], "@")
		if !mailSenderDomainRE.MatchString(domain) {
			return mailSenderEntry{}, "invalid email domain", false
		}
		return mailSenderEntry{Sender: lower, SenderType: 1}, "", true
	}
	if !mailSenderDomainRE.MatchString(lower) {
		return mailSenderEntry{}, "invalid domain", false
	}
	return mailSenderEntry{Sender: lower, SenderType: 2}, "", true
}

func resolveMailSenderListKind(value string) (mailSenderListKind, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "allow":
		return mailSenderListKind{Name: "allow", Segment: "allow_senders", Label: "trusted senders", ItemName: "allow_sender"}, nil
	case "block", "blocked":
		return mailSenderListKind{Name: "block", Segment: "blocked_senders", Label: "blocked senders", ItemName: "blocked_sender"}, nil
	default:
		return mailSenderListKind{}, mailValidationParamError("--type", "--type must be allow or block")
	}
}

func mailSenderListPath(mailboxID string, kind mailSenderListKind, method string) string {
	if method == "" {
		return mailboxPath(mailboxID, kind.Segment)
	}
	return mailboxPath(mailboxID, kind.Segment, method)
}

func mailSenderListParams(rt *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{}
	if pageSize := rt.Int("page-size"); pageSize > 0 {
		params["page_size"] = pageSize
	}
	if pageToken := strings.TrimSpace(rt.Str("page-token")); pageToken != "" {
		params["page_token"] = pageToken
	}
	if keyword := strings.ToLower(strings.TrimSpace(rt.Str("keyword"))); keyword != "" {
		params["keyword"] = keyword
	}
	return params
}

func mailSenderListItems(data map[string]interface{}, kind mailSenderListKind) []interface{} {
	for _, key := range []string{"senders", "items", kind.Segment, kind.ItemName + "s"} {
		if items, ok := data[key].([]interface{}); ok {
			return items
		}
	}
	return []interface{}{}
}

func firstMailSenderString(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok {
			return value
		}
	}
	return ""
}

func firstMailSenderBool(data map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		if value, ok := data[key].(bool); ok {
			return value
		}
	}
	return false
}

func printMailSenderList(w io.Writer, out mailSenderListOutput) {
	fmt.Fprintf(w, "%s for %s: %d\n", out.Type, out.MailboxID, out.Total)
	for _, item := range out.Senders {
		if m, ok := item.(map[string]interface{}); ok {
			fmt.Fprintf(w, "- %s\n", firstMailSenderString(m, "sender", "email", "domain", "mail_address"))
			continue
		}
		fmt.Fprintf(w, "- %v\n", item)
	}
	if out.NextPageToken != "" {
		fmt.Fprintf(w, "next_page_token: %s\n", out.NextPageToken)
	}
}

func printMailSenderMutation(w io.Writer, out mailSenderMutationOutput) {
	fmt.Fprintf(w, "%s %s senders for %s: %d applied", out.Action, out.Type, out.MailboxID, out.SuccessCount)
	if out.FilteredCount > 0 {
		fmt.Fprintf(w, ", %d invalid filtered", out.FilteredCount)
	}
	fmt.Fprintln(w)
}
