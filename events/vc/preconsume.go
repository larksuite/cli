// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"

	"github.com/larksuite/cli/events/internal/subscribeprep"
	"github.com/larksuite/cli/internal/event"
)

// subscriptionPreConsume registers a VC event type with the server so this
// tenant starts receiving it, and hands back the matching unregister.
//
// Every VC EventKey subscribes this way: the subscription is per event type
// (not per meeting), so the first consumer of a key registers it and the last
// one to exit unregisters it. The register/unregister pair itself is shared
// with the other domains that follow the same OAPI convention.
func subscriptionPreConsume(eventType, subscribePath, unsubscribePath string) func(context.Context, event.APIClient, map[string]string) (func() error, error) {
	return subscribeprep.Hook(eventType, subscribePath, unsubscribePath)
}
