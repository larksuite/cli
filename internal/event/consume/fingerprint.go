// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/catalog"
)

// ComputeSubscriptionID delegates to the catalog's scope derivation so every
// layer (application decision, bus accounting, this host) computes the same
// identity from the same declaration.
func ComputeSubscriptionID(def *event.KeyDefinition, params map[string]string) string {
	return catalog.SubscriptionScope(def, params)
}
