// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/core"
)

// consoleScopeGrantURL builds the Feishu/Lark developer-console deep link
// that opens the "apply & grant scopes" dialog for an app. It mirrors the
// URL the OAPI embeds in 99991672-family errors so a CLI-side hint reads
// the same as a server-side one to the user.
//
// Shape (verified against a real 99991672 response):
//
//	https://open.feishu.cn/app/:app_id/auth?q=<scope1>,<scope2>&op_from=openapi&token_type=tenant
//
// Scopes are comma-joined without URL-encoding — Feishu's console serves
// this shape and encoded commas/colons render as the noisy "%2C" / "%3A"
// without any behavior benefit.
func consoleScopeGrantURL(brand core.LarkBrand, appID string, scopes []string) string {
	host := core.ResolveEndpoints(brand).Open
	return fmt.Sprintf("%s/app/%s/auth?q=%s&op_from=openapi&token_type=tenant",
		host, appID, strings.Join(scopes, ","))
}

// consoleEventSubscriptionURL points at the app's event subscription page
// in the developer console. Unlike the scope grant URL this one doesn't
// encode the missing items — there is no "apply for event" dialog, the
// developer has to tick the checkboxes on the page. Path verified against
// the live console (2026-04-21).
func consoleEventSubscriptionURL(brand core.LarkBrand, appID string) string {
	host := core.ResolveEndpoints(brand).Open
	return fmt.Sprintf("%s/app/%s/event", host, appID)
}
