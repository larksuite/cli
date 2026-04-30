// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/core"
)

// consoleScopeGrantURL builds the developer-console "apply & grant scopes" deep link; scopes are comma-joined without URL encoding.
func consoleScopeGrantURL(brand core.LarkBrand, appID string, scopes []string) string {
	host := core.ResolveEndpoints(brand).Open
	return fmt.Sprintf("%s/app/%s/auth?q=%s&op_from=openapi&token_type=tenant",
		host, appID, strings.Join(scopes, ","))
}

// consoleEventSubscriptionURL points at the app's event subscription console page.
func consoleEventSubscriptionURL(brand core.LarkBrand, appID string) string {
	host := core.ResolveEndpoints(brand).Open
	return fmt.Sprintf("%s/app/%s/event", host, appID)
}
