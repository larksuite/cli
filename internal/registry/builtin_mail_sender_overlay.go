// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/meta"
)

const (
	mailUserMailboxMessageReadonly = "mail:user_mailbox.message:readonly"
	mailUserMailboxMessageModify   = "mail:user_mailbox.message:modify"
)

func applyBuiltInServiceOverlays() {
	mergeServiceOverlay(mailUserSenderOverlay())
}

func mergeServiceOverlay(overlay meta.Service) {
	if overlay.Name == "" {
		return
	}
	base, ok := mergedServices[overlay.Name]
	if !ok {
		mergedServices[overlay.Name] = overlay
		return
	}
	if base.Version == "" {
		base.Version = overlay.Version
	}
	if base.Title == "" {
		base.Title = overlay.Title
	}
	if base.Description == "" {
		base.Description = overlay.Description
	}
	if base.ServicePath == "" {
		base.ServicePath = overlay.ServicePath
	}
	if base.Resources == nil {
		base.Resources = map[string]meta.Resource{}
	}
	for name, resource := range overlay.Resources {
		base.Resources[name] = mergeResourceOverlay(base.Resources[name], resource)
	}
	mergedServices[overlay.Name] = base
}

func mergeResourceOverlay(base, overlay meta.Resource) meta.Resource {
	if base.Methods == nil {
		base.Methods = map[string]meta.Method{}
	}
	for name, method := range overlay.Methods {
		if _, exists := base.Methods[name]; !exists {
			base.Methods[name] = method
		}
	}
	if base.Resources == nil && len(overlay.Resources) > 0 {
		base.Resources = map[string]meta.Resource{}
	}
	for name, resource := range overlay.Resources {
		base.Resources[name] = mergeResourceOverlay(base.Resources[name], resource)
	}
	return base
}

func mailUserSenderOverlay() meta.Service {
	return meta.Service{
		Name:        "mail",
		Version:     "v1",
		Title:       "Mail",
		Description: "Mail API",
		ServicePath: "/open-apis/mail/v1",
		Resources: map[string]meta.Resource{
			"user_mailbox.allow_senders":   mailSenderResource("allow_senders"),
			"user_mailbox.blocked_senders": mailSenderResource("blocked_senders"),
		},
	}
}

func mailSenderResource(segment string) meta.Resource {
	methods := map[string]meta.Method{
		"list":         mailSenderListMethod(segment),
		"batch_create": mailSenderBatchMethod(segment, "batch_create"),
		"batch_remove": mailSenderBatchMethod(segment, "batch_remove"),
	}
	return meta.Resource{Methods: methods}
}

func mailSenderListMethod(segment string) meta.Method {
	scope := mailUserMailboxMessageReadonly
	return meta.Method{
		Path:           "user_mailboxes/{user_mailbox_id}/" + segment,
		HTTPMethod:     "GET",
		Description:    "List user mailbox " + segment + ".",
		Risk:           core.RiskRead,
		Scopes:         []string{scope},
		RequiredScopes: []string{scope},
		AccessTokens:   []meta.Token{meta.TokenUser},
		Parameters: map[string]meta.Field{
			"user_mailbox_id": {
				Type:        "string",
				Location:    "path",
				Required:    true,
				Description: "User mailbox ID. Use me for the current user.",
			},
			"query": {
				Type:        "string",
				Location:    "query",
				Description: "Search keyword for sender address.",
			},
			"page_size": {
				Type:        "integer",
				Location:    "query",
				Description: "Page size.",
			},
			"page_token": {
				Type:        "string",
				Location:    "query",
				Description: "Page token.",
			},
		},
		ResponseBody: map[string]meta.Field{
			"items": {
				Type: "array",
				Properties: map[string]meta.Field{
					"address": {Type: "string", Description: "Sender address."},
				},
			},
			"next_page_token": {Type: "string"},
		},
	}
}

func mailSenderBatchMethod(segment, action string) meta.Method {
	scope := mailUserMailboxMessageModify
	return meta.Method{
		Path:           "user_mailboxes/{user_mailbox_id}/" + segment + "/" + action,
		HTTPMethod:     "POST",
		Description:    action + " user mailbox " + segment + ".",
		Risk:           core.RiskWrite,
		Scopes:         []string{scope},
		RequiredScopes: []string{scope},
		AccessTokens:   []meta.Token{meta.TokenUser},
		Parameters: map[string]meta.Field{
			"user_mailbox_id": {
				Type:        "string",
				Location:    "path",
				Required:    true,
				Description: "User mailbox ID. Use me for the current user.",
			},
		},
		RequestBody: map[string]meta.Field{
			"addresses": {
				Type:        "array",
				Required:    true,
				Description: "Sender addresses.",
			},
		},
		ResponseBody: map[string]meta.Field{
			"submitted_count":    {Type: "integer"},
			"deduplicated_count": {Type: "integer"},
		},
	}
}
