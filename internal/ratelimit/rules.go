// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package ratelimit

import (
	"strings"
	"time"
)

type Scope string

const ScopeApp Scope = "app"

const (
	tier3Window       = time.Minute
	tier3Limit        = 100
	tier4MinuteWindow = time.Minute
	tier4MinuteLimit  = 1000
	tier4SecondWindow = time.Second
	tier4SecondLimit  = 50
	tier7Window       = time.Second
	tier7Limit        = 10
)

type Rule struct {
	Method        string
	CanonicalPath string
	Window        time.Duration
	Limit         int
	Scope         Scope
}

var builtinRules = []Rule{
	// Online mail API YAML rateLimit tiers:
	// tier 3 = 100/min, tier 4 = 1000/min and 50/s, tier 7 = 10/s.
	{
		Method:        "GET",
		CanonicalPath: "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages/:message_id",
		Window:        tier3Window,
		Limit:         tier3Limit,
		Scope:         ScopeApp,
	},
	{
		Method:        "POST",
		CanonicalPath: "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages/batch_get",
		Window:        tier7Window,
		Limit:         tier7Limit,
		Scope:         ScopeApp,
	},
	{
		Method:        "GET",
		CanonicalPath: "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/messages",
		Window:        tier7Window,
		Limit:         tier7Limit,
		Scope:         ScopeApp,
	},
	{
		Method:        "POST",
		CanonicalPath: "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/search",
		Window:        tier4MinuteWindow,
		Limit:         tier4MinuteLimit,
		Scope:         ScopeApp,
	},
	{
		Method:        "POST",
		CanonicalPath: "/open-apis/mail/v1/user_mailboxes/:user_mailbox_id/search",
		Window:        tier4SecondWindow,
		Limit:         tier4SecondLimit,
		Scope:         ScopeApp,
	},
}

func maxRuleWindow(rules []Rule) time.Duration {
	var max time.Duration
	for _, rule := range rules {
		if rule.Window > max {
			max = rule.Window
		}
	}
	return max
}

func normalizeMethod(method string) string {
	return strings.ToUpper(strings.TrimSpace(method))
}
