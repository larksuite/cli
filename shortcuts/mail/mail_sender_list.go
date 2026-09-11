// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	senderListReadScope  = "mail:user_mailbox.message:readonly"
	senderListWriteScope = "mail:user_mailbox.message:modify"
)

// MailSenderList manages a user's allowed and blocked senders through the
// published Mail OpenAPI. The list kind is always explicit so a command cannot
// accidentally write to the opposite list.
var MailSenderList = common.Shortcut{
	Service:     "mail",
	Command:     "+sender-list",
	Description: "List allowed or blocked senders for a mailbox. Use --kind allow or block.",
	Risk:        "read",
	Scopes:      []string{senderListReadScope},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox address (default: me)."},
		{Name: "kind", Required: true, Enum: []string{"allow", "block"}, Desc: "Sender list kind: allow or block."},
		{Name: "page-size", Type: "int", Default: "50", Desc: "Maximum senders to return (1-100, default: 50)."},
		{Name: "page-token", Desc: "Pagination token from a previous response."},
	},
	Validate: validateSenderList,
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().GET(senderListPath(resolveMailboxID(rt), rt.Str("kind"))).
			Params(senderListDryRunParams(rt))
	},
	Execute: executeSenderList,
}

var MailSenderSearch = common.Shortcut{
	Service:     "mail",
	Command:     "+sender-search",
	Description: "Search allowed or blocked senders returned by one Mail OpenAPI page. Use --kind allow or block.",
	Risk:        "read",
	Scopes:      []string{senderListReadScope},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox address (default: me)."},
		{Name: "kind", Required: true, Enum: []string{"allow", "block"}, Desc: "Sender list kind: allow or block."},
		{Name: "query", Required: true, Desc: "Case-insensitive sender address substring to match."},
		{Name: "page-size", Type: "int", Default: "100", Desc: "Maximum senders to inspect (1-100, default: 100)."},
		{Name: "page-token", Desc: "Pagination token from a previous response."},
	},
	Validate: func(ctx context.Context, rt *common.RuntimeContext) error {
		if err := validateSenderList(ctx, rt); err != nil {
			return err
		}
		if strings.TrimSpace(rt.Str("query")) == "" {
			return mailValidationParamError("--query", "--query is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().Desc("List one sender page then filter it locally by --query.").
			GET(senderListPath(resolveMailboxID(rt), rt.Str("kind"))).Params(senderListDryRunParams(rt)).
			Set("query", rt.Str("query"))
	},
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		data, err := fetchSenderList(rt)
		if err != nil {
			return err
		}
		query := strings.ToLower(strings.TrimSpace(rt.Str("query")))
		items := senderItems(data["items"])
		matched := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if strings.Contains(strings.ToLower(senderAddress(item)), query) {
				matched = append(matched, item)
			}
		}
		data["items"] = matched
		data["total"] = len(matched)
		rt.OutFormat(data, &output.Meta{Count: len(matched)}, nil)
		return nil
	},
}

var MailSenderSet = common.Shortcut{
	Service:     "mail",
	Command:     "+sender-set",
	Description: "Add sender addresses to an allowed or blocked sender list. Use --kind allow or block.",
	Risk:        "write",
	Scopes:      []string{senderListWriteScope},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox address (default: me)."},
		{Name: "kind", Required: true, Enum: []string{"allow", "block"}, Desc: "Sender list kind: allow or block."},
		{Name: "senders", Type: "string_slice", Required: true, Desc: "Sender email addresses to add; comma-separated or repeated."},
	},
	Validate: validateSenderMutation,
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().POST(senderMutationPath(resolveMailboxID(rt), rt.Str("kind"), "batch_create")).Body(senderBody(rt))
	},
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		return executeSenderMutation(rt, "batch_create", "added")
	},
}

var MailSenderDelete = common.Shortcut{
	Service:     "mail",
	Command:     "+sender-delete",
	Description: "Remove sender addresses from an allowed or blocked sender list. Use --kind allow or block.",
	Risk:        "delete",
	Scopes:      []string{senderListWriteScope},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags:       MailSenderSet.Flags,
	Validate:    validateSenderMutation,
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().POST(senderMutationPath(resolveMailboxID(rt), rt.Str("kind"), "batch_remove")).Body(senderBody(rt))
	},
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		return executeSenderMutation(rt, "batch_remove", "removed")
	},
}

func validateSenderList(ctx context.Context, rt *common.RuntimeContext) error {
	if err := validateSenderKind(rt); err != nil {
		return err
	}
	if size := rt.Int("page-size"); size < 1 || size > 100 {
		return mailValidationParamError("--page-size", "--page-size must be between 1 and 100")
	}
	return nil
}

func validateSenderMutation(ctx context.Context, rt *common.RuntimeContext) error {
	if err := validateSenderKind(rt); err != nil {
		return err
	}
	if len(normalizeSenders(rt.StrSlice("senders"))) == 0 {
		return mailValidationParamError("--senders", "--senders must contain at least one sender address")
	}
	return nil
}

func validateSenderKind(rt *common.RuntimeContext) error {
	if rt.Str("kind") != "allow" && rt.Str("kind") != "block" {
		return mailValidationParamError("--kind", "--kind must be allow or block")
	}
	return nil
}

func senderListPath(mailbox, kind string) string {
	if kind == "block" {
		return mailboxPath(mailbox, "blocked_senders")
	}
	return mailboxPath(mailbox, "allow_senders")
}

func senderMutationPath(mailbox, kind, action string) string {
	return senderListPath(mailbox, kind) + "/" + action
}

func senderListQuery(rt *common.RuntimeContext) larkcore.QueryParams {
	query := larkcore.QueryParams{}
	query.Set("page_size", fmt.Sprintf("%d", rt.Int("page-size")))
	if token := strings.TrimSpace(rt.Str("page-token")); token != "" {
		query.Set("page_token", token)
	}
	return query
}

func senderListDryRunParams(rt *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{"page_size": rt.Int("page-size")}
	if token := strings.TrimSpace(rt.Str("page-token")); token != "" {
		params["page_token"] = token
	}
	return params
}

func senderBody(rt *common.RuntimeContext) map[string]any {
	return map[string]any{"senders": normalizeSenders(rt.StrSlice("senders"))}
}

func normalizeSenders(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			address := strings.TrimSpace(part)
			if address != "" && !seen[address] {
				seen[address] = true
				result = append(result, address)
			}
		}
	}
	return result
}

func fetchSenderList(rt *common.RuntimeContext) (map[string]any, error) {
	data, err := rt.DoAPIJSONTyped("GET", senderListPath(resolveMailboxID(rt), rt.Str("kind")), senderListQuery(rt), nil)
	if err != nil {
		return nil, mailDecorateProblemMessage(err, "list %s senders failed", rt.Str("kind"))
	}
	data["total"] = len(senderItems(data["items"]))
	return data, nil
}

func executeSenderList(ctx context.Context, rt *common.RuntimeContext) error {
	data, err := fetchSenderList(rt)
	if err != nil {
		return err
	}
	rt.OutFormat(data, &output.Meta{Count: len(senderItems(data["items"]))}, nil)
	return nil
}

func executeSenderMutation(rt *common.RuntimeContext, action, result string) error {
	body := senderBody(rt)
	data, err := rt.DoAPIJSONTyped("POST", senderMutationPath(resolveMailboxID(rt), rt.Str("kind"), action), nil, body)
	if err != nil {
		return mailDecorateProblemMessage(err, "%s %s senders failed", result, rt.Str("kind"))
	}
	data["kind"] = rt.Str("kind")
	data["senders"] = body["senders"]
	data["result"] = result
	rt.OutFormat(data, &output.Meta{Count: len(body["senders"].([]string))}, nil)
	return nil
}

func senderItems(value any) []map[string]any {
	items, _ := value.([]any)
	result := make([]map[string]any, 0, len(items))
	for _, raw := range items {
		if item, ok := raw.(map[string]any); ok {
			result = append(result, item)
		}
	}
	return result
}

func senderAddress(item map[string]any) string {
	for _, key := range []string{"sender", "email", "email_address", "address"} {
		if value, ok := item[key].(string); ok {
			return value
		}
	}
	return ""
}
