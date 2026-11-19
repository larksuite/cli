// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import "github.com/larksuite/cli/shortcuts/common"

const messageReactionReadScope = "im:message.reactions:read"

// ensureMessageReactionScope keeps reaction enrichment opt-in at the scope
// layer as well as the request layer. Pure builder tests construct partial
// runtime contexts, so only run the token preflight for mounted commands.
func ensureMessageReactionScope(runtime *common.RuntimeContext) error {
	if runtime.Bool("no-reactions") || runtime.Factory == nil || runtime.Factory.Credential == nil || runtime.Config == nil {
		return nil
	}
	return runtime.EnsureScopes([]string{messageReactionReadScope})
}
