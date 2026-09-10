// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

type mailSenderRecord struct {
	Sender   string `json:"sender"`
	ListType string `json:"list_type"`
}

type mailSenderPage struct {
	Items []struct {
		Sender string `json:"sender"`
	} `json:"items"`
	HasMore   *bool  `json:"has_more"`
	PageToken string `json:"page_token"`
}

type mailSenderWriteResponse struct {
	FailedItems []struct {
		Sender     string `json:"sender"`
		ReasonCode int    `json:"reason_code"`
	} `json:"failed_items"`
	LogID string `json:"log_id"`
}

var MailSenderList = common.Shortcut{
	Service: "mail", Command: "+sender-list",
	Description: "List the current user's allowed and blocked senders, automatically following all pages and labeling each record with its list type.",
	Risk:        "read", Scopes: []string{"mail:user_mailbox.message:readonly"}, AuthTypes: []string{"user"}, HasFormat: true,
	Flags: []common.Flag{
		{Name: "type", Default: "all", Desc: "Sender list: allow, block, or all."},
		{Name: "page-size", Type: "int", Default: "50", Desc: "Records per API page (1–100); all pages are fetched."},
	},
	Validate: func(ctx context.Context, rt *common.RuntimeContext) error {
		if err := validateMailSenderType(rt, true); err != nil {
			return err
		}
		if rt.Int("page-size") < 1 || rt.Int("page-size") > 100 {
			return mailValidationParamError("--page-size", "--page-size must be between 1 and 100")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return dryRunMailSenderRead(rt.Str("type"), rt.Int("page-size"))
	},
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		items, err := listMailSenders(rt, rt.Str("type"), rt.Int("page-size"))
		if err != nil {
			return err
		}
		rt.OutFormat(struct {
			Items []mailSenderRecord `json:"items"`
		}{items}, &output.Meta{Count: len(items)}, nil)
		return nil
	},
}

var MailSenderGet = common.Shortcut{
	Service: "mail", Command: "+sender-get",
	Description: "Find an exact sender in both of the current user's lists. Returns found=false only after every page succeeds.",
	Risk:        "read", Scopes: []string{"mail:user_mailbox.message:readonly"}, AuthTypes: []string{"user"}, HasFormat: true,
	Flags:    []common.Flag{{Name: "sender", Required: true, Desc: "Sender email address or domain; matched exactly after trimming whitespace, ignoring case."}},
	Validate: validateMailSender,
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return dryRunMailSenderRead("all", 100).Set("exact_sender", strings.TrimSpace(rt.Str("sender")))
	},
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		items, err := listMailSenders(rt, "all", 100)
		if err != nil {
			return err
		}
		sender := strings.TrimSpace(rt.Str("sender"))
		result := struct {
			Found    bool   `json:"found"`
			Sender   string `json:"sender"`
			ListType string `json:"list_type,omitempty"`
		}{Sender: sender}
		for _, item := range items {
			if !strings.EqualFold(strings.TrimSpace(item.Sender), sender) {
				continue
			}
			if result.Found && result.ListType != item.ListType {
				return errs.NewAPIError(errs.SubtypeConflict, "sender exists in both allow and block lists").WithHint("Resolve the conflicting sender configuration in the mail client, then query again.")
			}
			result.Found = true
			result.Sender = item.Sender
			result.ListType = item.ListType
		}
		rt.OutFormat(result, nil, nil)
		return nil
	},
}

var MailSenderSet = common.Shortcut{
	Service: "mail", Command: "+sender-set",
	Description: "Set one sender in the current user's allow or block list. The server handles duplicate settings and switches lists atomically.",
	Risk:        "write", Scopes: []string{"mail:user_mailbox.message:modify"}, AuthTypes: []string{"user"}, HasFormat: true,
	Flags: mailSenderWriteFlags(), Validate: validateMailSenderWrite,
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().POST(mailSenderPath(rt.Str("type"), "batch_create")).Body(mailSenderSetBody(rt))
	},
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		return executeMailSenderWrite(rt, "batch_create", "configured", mailSenderSetBody(rt))
	},
}

var MailSenderDelete = common.Shortcut{
	Service: "mail", Command: "+sender-delete",
	Description: "Remove one sender from the specified current-user list. A missing record is already absent; the other list is unchanged.",
	Risk:        "high-risk-write", Scopes: []string{"mail:user_mailbox.message:modify"}, AuthTypes: []string{"user"}, HasFormat: true,
	Flags: mailSenderWriteFlags(), Validate: validateMailSenderWrite,
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().POST(mailSenderPath(rt.Str("type"), "batch_remove")).Body(mailSenderDeleteBody(rt))
	},
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		return executeMailSenderWrite(rt, "batch_remove", "absent", mailSenderDeleteBody(rt))
	},
}

func mailSenderWriteFlags() []common.Flag {
	return []common.Flag{
		{Name: "sender", Required: true, Desc: "One sender email address or domain; surrounding whitespace is trimmed."},
		{Name: "type", Required: true, Desc: "Target sender list: allow or block."},
	}
}

func validateMailSender(ctx context.Context, rt *common.RuntimeContext) error {
	if strings.TrimSpace(rt.Str("sender")) == "" {
		return mailValidationParamError("--sender", "--sender must not be empty")
	}
	return nil
}

func validateMailSenderType(rt *common.RuntimeContext, allowAll bool) error {
	switch rt.Str("type") {
	case "allow", "block":
		return nil
	case "all":
		if allowAll {
			return nil
		}
	}
	if allowAll {
		return mailValidationParamError("--type", "--type must be allow, block, or all")
	}
	return mailValidationParamError("--type", "--type must be allow or block")
}

func validateMailSenderWrite(ctx context.Context, rt *common.RuntimeContext) error {
	if err := validateMailSender(ctx, rt); err != nil {
		return err
	}
	return validateMailSenderType(rt, false)
}

func mailSenderTypes(listType string) []string {
	if listType == "all" {
		return []string{"allow", "block"}
	}
	return []string{listType}
}

func mailSenderPath(listType string, action string) string {
	resource := "allow_senders"
	if listType == "block" {
		resource = "blocked_senders"
	}
	return mailboxPath("me", resource, action)
}

func dryRunMailSenderRead(listType string, pageSize int) *common.DryRunAPI {
	api := common.NewDryRunAPI().Desc("Follow all pages of each selected list without keyword filtering; stop on any API or pagination error.")
	for _, typ := range mailSenderTypes(listType) {
		api.GET(mailSenderPath(typ, "")).Params(map[string]interface{}{"page_size": pageSize})
	}
	return api
}

func listMailSenders(rt *common.RuntimeContext, listType string, pageSize int) ([]mailSenderRecord, error) {
	items := []mailSenderRecord{}
	for _, typ := range mailSenderTypes(listType) {
		token := ""
		seen := map[string]bool{}
		for {
			query := larkcore.QueryParams{"page_size": []string{strconv.Itoa(pageSize)}}
			if token != "" {
				query["page_token"] = []string{token}
			}
			data, err := rt.DoAPIJSONTyped("GET", mailSenderPath(typ, ""), query, nil)
			if err != nil {
				return nil, err
			}
			var page mailSenderPage
			if err := decodeMailSenderResponse(data, &page); err != nil {
				return nil, err
			}
			if page.HasMore == nil {
				return nil, mailInvalidResponseError("sender list response is missing has_more")
			}
			for _, item := range page.Items {
				if strings.TrimSpace(item.Sender) == "" {
					return nil, mailInvalidResponseError("sender list contains an empty sender")
				}
				items = append(items, mailSenderRecord{Sender: item.Sender, ListType: typ})
			}
			if !*page.HasMore {
				break
			}
			if strings.TrimSpace(page.PageToken) == "" || seen[page.PageToken] {
				return nil, mailInvalidResponseError("sender list returned a missing or repeated page token").WithHint("Retry the query from the beginning; if it persists, report the pagination error.")
			}
			seen[page.PageToken] = true
			token = page.PageToken
		}
	}
	return items, nil
}

func decodeMailSenderResponse(data map[string]any, target any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return mailInvalidResponseError("cannot encode sender response").WithCause(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return mailInvalidResponseError("invalid sender response").WithCause(err)
	}
	return nil
}

func mailSenderSetBody(rt *common.RuntimeContext) map[string]any {
	return map[string]any{"items": []map[string]string{{"sender": strings.TrimSpace(rt.Str("sender"))}}}
}

func mailSenderDeleteBody(rt *common.RuntimeContext) map[string]any {
	return map[string]any{"senders": []string{strings.TrimSpace(rt.Str("sender"))}}
}

func executeMailSenderWrite(rt *common.RuntimeContext, action, status string, body map[string]any) error {
	data, err := rt.DoAPIJSONTyped("POST", mailSenderPath(rt.Str("type"), action), nil, body)
	if err != nil {
		return err
	}
	var response mailSenderWriteResponse
	if err := decodeMailSenderResponse(data, &response); err != nil {
		return err
	}
	if len(response.FailedItems) > 0 {
		item := response.FailedItems[0]
		return errs.NewAPIError(errs.SubtypeUnknown, "sender operation failed: sender=%s reason_code=%d", item.Sender, item.ReasonCode).
			WithCode(item.ReasonCode).WithLogID(response.LogID).
			WithHint("Check the sender address/domain and mailbox restrictions or quota, then retry. No successful configuration is reported for failed_items.")
	}
	result := struct {
		Sender   string `json:"sender"`
		ListType string `json:"list_type"`
		Status   string `json:"status"`
	}{strings.TrimSpace(rt.Str("sender")), rt.Str("type"), status}
	rt.OutFormat(result, nil, nil)
	return nil
}
