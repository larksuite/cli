// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	mailSenderActionList   = "list"
	mailSenderActionAdd    = "add"
	mailSenderActionRemove = "remove"
	mailSenderTypeAddress  = 1
	mailSenderTypeDomain   = 2
	mailSenderBatchMax     = 100
)

type mailSenderListKind struct {
	Resource     string
	ListSegment  string
	Label        string
	OppositeName string
}

var (
	mailAllowSenderKind = mailSenderListKind{
		Resource:     "allow_senders",
		ListSegment:  "allow_senders",
		Label:        "allow",
		OppositeName: "blocked",
	}
	mailBlockedSenderKind = mailSenderListKind{
		Resource:     "blocked_senders",
		ListSegment:  "blocked_senders",
		Label:        "blocked",
		OppositeName: "allow",
	}
)

// MailAllowSendersList lists/searches a user's mailbox-level allow sender list.
var MailAllowSendersList = newMailSenderListShortcut(mailAllowSenderKind, "+allow-senders-list")

// MailAllowSendersAdd adds entries to a user's mailbox-level allow sender list.
var MailAllowSendersAdd = newMailSenderWriteShortcut(mailAllowSenderKind, "+allow-senders-add", mailSenderActionAdd)

// MailAllowSendersRemove removes entries from a user's mailbox-level allow sender list.
var MailAllowSendersRemove = newMailSenderWriteShortcut(mailAllowSenderKind, "+allow-senders-remove", mailSenderActionRemove)

// MailBlockedSendersList lists/searches a user's mailbox-level blocked sender list.
var MailBlockedSendersList = newMailSenderListShortcut(mailBlockedSenderKind, "+blocked-senders-list")

// MailBlockedSendersAdd adds entries to a user's mailbox-level blocked sender list.
var MailBlockedSendersAdd = newMailSenderWriteShortcut(mailBlockedSenderKind, "+blocked-senders-add", mailSenderActionAdd)

// MailBlockedSendersRemove removes entries from a user's mailbox-level blocked sender list.
var MailBlockedSendersRemove = newMailSenderWriteShortcut(mailBlockedSenderKind, "+blocked-senders-remove", mailSenderActionRemove)

func newMailSenderListShortcut(kind mailSenderListKind, command string) common.Shortcut {
	return common.Shortcut{
		Service:     "mail",
		Command:     command,
		Description: fmt.Sprintf("List/search user mailbox %s sender entries.", kind.Label),
		Risk:        "read",
		Scopes:      []string{"mail:user_mailbox.message:readonly"},
		AuthTypes:   []string{"user"},
		HasFormat:   true,
		Flags: []common.Flag{
			{Name: "mailbox", Default: "me", Desc: "Mailbox ID, mailbox email address, open_id, or me (default: me)."},
			{Name: "keyword", Desc: "Search keyword for list; matches sender address or domain prefix."},
			{Name: "page-size", Type: "int", Default: "20", Desc: "List page size (1-100)."},
			{Name: "page-token", Desc: "Pagination token returned by a previous list call."},
		},
		Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
			_, err := buildMailSenderInput(runtime)
			return err
		},
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			return dryRunMailSender(kind, runtime)
		},
		Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
			return executeMailSender(kind, runtime)
		},
	}
}

func newMailSenderWriteShortcut(kind mailSenderListKind, command, action string) common.Shortcut {
	return common.Shortcut{
		Service:     "mail",
		Command:     command,
		Description: fmt.Sprintf("%s entries in a user mailbox %s sender list.", mailSenderWriteVerb(action), kind.Label),
		Risk:        "write",
		Scopes:      []string{"mail:user_mailbox.message:modify"},
		AuthTypes:   []string{"user"},
		HasFormat:   true,
		Flags: []common.Flag{
			{Name: "mailbox", Default: "me", Desc: "Mailbox ID, mailbox email address, open_id, or me (default: me)."},
			{Name: "sender", Type: "string_slice", Required: true, Desc: "Sender email address or domain; comma-separated or repeat the flag."},
		},
		Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
			_, err := buildMailSenderInputForAction(runtime, action)
			return err
		},
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			return dryRunMailSenderAction(kind, runtime, action)
		},
		Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
			return executeMailSenderAction(kind, runtime, action)
		},
	}
}

func mailSenderWriteVerb(action string) string {
	if action == mailSenderActionRemove {
		return "Remove"
	}
	return "Add"
}

type mailSenderInput struct {
	Action    string
	MailboxID string
	Senders   []string
	Keyword   string
	PageSize  int
	PageToken string
}

func buildMailSenderInput(runtime *common.RuntimeContext) (mailSenderInput, error) {
	return buildMailSenderInputForAction(runtime, mailSenderActionList)
}

func buildMailSenderInputForAction(runtime *common.RuntimeContext, action string) (mailSenderInput, error) {
	input := mailSenderInput{
		Action:    strings.TrimSpace(action),
		MailboxID: resolveMailboxID(runtime),
		Keyword:   strings.TrimSpace(runtime.Str("keyword")),
		PageSize:  runtime.Int("page-size"),
		PageToken: strings.TrimSpace(runtime.Str("page-token")),
	}
	if input.PageSize == 0 {
		input.PageSize = 20
	}
	switch input.Action {
	case mailSenderActionList:
		if err := validateMailSenderListInput(input); err != nil {
			return mailSenderInput{}, err
		}
	case mailSenderActionAdd:
		senders, err := normalizeMailSenderAddItems(runtime.StrSlice("sender"))
		if err != nil {
			return mailSenderInput{}, err
		}
		input.Senders = senders
	case mailSenderActionRemove:
		senders, err := normalizeMailSenderRemoveItems(runtime.StrSlice("sender"))
		if err != nil {
			return mailSenderInput{}, err
		}
		input.Senders = senders
	default:
		return mailSenderInput{}, mailValidationParamError("--action", "unsupported action %q; expected list, add, or remove", input.Action)
	}
	return input, nil
}

func validateMailSenderListInput(input mailSenderInput) error {
	if input.PageSize < 1 || input.PageSize > 100 {
		return mailValidationParamError("--page-size", "--page-size must be between 1 and 100")
	}
	return nil
}

func normalizeMailSenderAddItems(values []string) ([]string, error) {
	senders := normalizeMailSenderItems(values, true)
	if len(senders) == 0 {
		return nil, mailValidationParamError("--sender", "--sender is required for add")
	}
	if len(senders) > mailSenderBatchMax {
		return nil, mailValidationParamError("--sender", "at most %d senders may be added at once", mailSenderBatchMax)
	}
	return senders, nil
}

func normalizeMailSenderRemoveItems(values []string) ([]string, error) {
	senders := normalizeMailSenderItems(values, false)
	if len(senders) == 0 {
		return nil, mailValidationParamError("--sender", "--sender is required for remove")
	}
	if len(senders) > mailSenderBatchMax {
		return nil, mailValidationParamError("--sender", "at most %d senders may be removed at once", mailSenderBatchMax)
	}
	return senders, nil
}

func normalizeMailSenderItems(values []string, lower bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range values {
		for _, part := range common.SplitCSV(raw) {
			sender := strings.TrimSpace(part)
			if sender == "" {
				continue
			}
			if lower {
				sender = strings.ToLower(sender)
			}
			key := strings.ToLower(sender)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, sender)
		}
	}
	return out
}

func mailSenderListPath(kind mailSenderListKind, mailboxID string) string {
	return mailboxPath(mailboxID, kind.ListSegment)
}

func mailSenderBatchCreatePath(kind mailSenderListKind, mailboxID string) string {
	return mailboxPath(mailboxID, kind.ListSegment, "batch_create")
}

func mailSenderBatchRemovePath(kind mailSenderListKind, mailboxID string) string {
	return mailboxPath(mailboxID, kind.ListSegment, "batch_remove")
}

func dryRunMailSender(kind mailSenderListKind, runtime *common.RuntimeContext) *common.DryRunAPI {
	input, err := buildMailSenderInput(runtime)
	if err != nil {
		return common.NewDryRunAPI().Set("error", err.Error())
	}
	return dryRunMailSenderInput(kind, input)
}

func dryRunMailSenderAction(kind mailSenderListKind, runtime *common.RuntimeContext, action string) *common.DryRunAPI {
	input, err := buildMailSenderInputForAction(runtime, action)
	if err != nil {
		return common.NewDryRunAPI().Set("error", err.Error())
	}
	return dryRunMailSenderInput(kind, input)
}

func dryRunMailSenderInput(kind mailSenderListKind, input mailSenderInput) *common.DryRunAPI {
	switch input.Action {
	case mailSenderActionAdd:
		return common.NewDryRunAPI().
			POST(mailSenderBatchCreatePath(kind, input.MailboxID)).
			Body(map[string]interface{}{"items": buildMailSenderCreateItems(input.Senders)}).
			Desc(fmt.Sprintf("add entries to the user mailbox %s sender list; entries conflicting with the %s list are resolved by the server", kind.Label, kind.OppositeName))
	case mailSenderActionRemove:
		return common.NewDryRunAPI().
			POST(mailSenderBatchRemovePath(kind, input.MailboxID)).
			Body(map[string]interface{}{"senders": input.Senders}).
			Desc("remove entries by literal sender value; server also handles historical mixed-case records")
	default:
		return common.NewDryRunAPI().
			GET(mailSenderListPath(kind, input.MailboxID)).
			Params(mailSenderListParams(input)).
			Desc(fmt.Sprintf("list/search the user mailbox %s sender list", kind.Label))
	}
}

func executeMailSender(kind mailSenderListKind, runtime *common.RuntimeContext) error {
	input, err := buildMailSenderInput(runtime)
	if err != nil {
		return err
	}
	switch input.Action {
	case mailSenderActionAdd:
		return executeMailSenderAdd(kind, runtime, input)
	case mailSenderActionRemove:
		return executeMailSenderRemove(kind, runtime, input)
	default:
		return executeMailSenderList(kind, runtime, input)
	}
}

func executeMailSenderAction(kind mailSenderListKind, runtime *common.RuntimeContext, action string) error {
	input, err := buildMailSenderInputForAction(runtime, action)
	if err != nil {
		return err
	}
	switch input.Action {
	case mailSenderActionAdd:
		return executeMailSenderAdd(kind, runtime, input)
	case mailSenderActionRemove:
		return executeMailSenderRemove(kind, runtime, input)
	default:
		return mailValidationParamError("--action", "unsupported action %q; expected add or remove", input.Action)
	}
}

func executeMailSenderList(kind mailSenderListKind, runtime *common.RuntimeContext, input mailSenderInput) error {
	data, err := doJSONAPI(runtime, &larkcore.ApiReq{
		HttpMethod:  http.MethodGet,
		ApiPath:     mailSenderListPath(kind, input.MailboxID),
		QueryParams: toQueryParams(mailSenderListParams(input)),
	}, fmt.Sprintf("list %s senders", kind.Label))
	if err != nil {
		return err
	}
	items := normalizeMailSenderResponseItems(data["items"])
	result := map[string]interface{}{
		"mailbox_id": input.MailboxID,
		"list":       kind.Label,
		"items":      items,
		"count":      len(items),
		"has_more":   data["has_more"],
		"page_token": data["page_token"],
		"keyword":    input.Keyword,
		"page_size":  input.PageSize,
		"raw":        data,
	}
	runtime.Out(result, nil)
	return nil
}

func executeMailSenderAdd(kind mailSenderListKind, runtime *common.RuntimeContext, input mailSenderInput) error {
	if err := validateMailSenderModifyScope(runtime); err != nil {
		return err
	}
	data, err := runtime.CallAPITyped("POST", mailSenderBatchCreatePath(kind, input.MailboxID), nil,
		map[string]interface{}{"items": buildMailSenderCreateItems(input.Senders)})
	if err != nil {
		return mailDecorateProblemMessage(err, "add %s senders", kind.Label)
	}
	failed := normalizeMailSenderFailureItems(data["failed_items"])
	runtime.Out(map[string]interface{}{
		"mailbox_id":    input.MailboxID,
		"list":          kind.Label,
		"requested":     input.Senders,
		"requested_cnt": len(input.Senders),
		"failed_items":  failed,
		"failed_count":  len(failed),
		"raw":           data,
	}, nil)
	return nil
}

func executeMailSenderRemove(kind mailSenderListKind, runtime *common.RuntimeContext, input mailSenderInput) error {
	if err := validateMailSenderModifyScope(runtime); err != nil {
		return err
	}
	data, err := runtime.CallAPITyped("POST", mailSenderBatchRemovePath(kind, input.MailboxID), nil,
		map[string]interface{}{"senders": input.Senders})
	if err != nil {
		return mailDecorateProblemMessage(err, "remove %s senders", kind.Label)
	}
	runtime.Out(map[string]interface{}{
		"mailbox_id":      input.MailboxID,
		"list":            kind.Label,
		"requested":       input.Senders,
		"requested_cnt":   len(input.Senders),
		"deleted_count":   data["deleted_count"],
		"mixed_case_note": "server matches literal sender values and handles historical mixed-case records",
		"raw":             data,
	}, nil)
	return nil
}

func validateMailSenderModifyScope(runtime *common.RuntimeContext) error {
	appID := runtime.Config.AppID
	userOpenID := runtime.UserOpenId()
	if appID == "" || userOpenID == "" {
		return nil
	}
	stored, _ := auth.GetStoredToken(appID, userOpenID)
	if stored == nil {
		return nil
	}
	required := []string{"mail:user_mailbox.message:modify"}
	if missing := auth.MissingScopes(stored.Scope, required); len(missing) > 0 {
		return errs.NewPermissionError(errs.SubtypeMissingScope,
			"sender list updates require scope: %s", strings.Join(missing, ", ")).
			WithMissingScopes(missing...).
			WithIdentity("user")
	}
	return nil
}

func mailSenderListParams(input mailSenderInput) map[string]interface{} {
	params := map[string]interface{}{"page_size": input.PageSize}
	if input.Keyword != "" {
		params["keyword"] = input.Keyword
	}
	if input.PageToken != "" {
		params["page_token"] = input.PageToken
	}
	return params
}

func buildMailSenderCreateItems(senders []string) []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(senders))
	for _, sender := range senders {
		items = append(items, map[string]interface{}{
			"sender":      sender,
			"sender_type": inferMailSenderType(sender),
		})
	}
	return items
}

func inferMailSenderType(sender string) int {
	if strings.Contains(sender, "@") {
		return mailSenderTypeAddress
	}
	return mailSenderTypeDomain
}

func normalizeMailSenderResponseItems(raw interface{}) []map[string]interface{} {
	rawItems, ok := raw.([]interface{})
	if !ok {
		return []map[string]interface{}{}
	}
	items := make([]map[string]interface{}, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, ok := rawItem.(map[string]interface{})
		if !ok {
			continue
		}
		items = append(items, item)
	}
	return items
}

func normalizeMailSenderFailureItems(raw interface{}) []map[string]interface{} {
	return normalizeMailSenderResponseItems(raw)
}
