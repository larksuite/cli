// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"fmt"

	"github.com/larksuite/cli/shortcuts/common"
)

const imPageSizeMinimum = 1

// imPageSizeLimits is the single source of truth for shortcut page-size
// declarations and local validation in the IM domain.
//
// Verified against the corresponding OpenAPI contract or a read-only request:
//   - GET /open-apis/im/v1/messages: 50
//   - POST /open-apis/im/v1/messages/search: 50
//   - GET /open-apis/im/v1/flags: 50
//   - GET /open-apis/im/v1/groups: 50
//   - POST /open-apis/im/v2/chats/search: 100
//   - GET /open-apis/im/v1/chats: 100
//   - GET /open-apis/im/v1/chats/:chat_id/members/list: 100
//
// GET /open-apis/im/v1/groups/:group_id/list_item has no public specification.
// Its limit was established by probing the endpoint: page_size 51 and above
// returns code 230001 "param is invalid", 50 succeeds.
var imPageSizeLimits = map[string]int{
	"+threads-messages-list": 50,
	"+chat-messages-list":    50,
	"+messages-search":       50,
	"+flag-list":             50,
	"+feed-group-list":       50,
	"+feed-group-list-item":  50,
	"+chat-search":           100,
	"+chat-list":             100,
	"+chat-members-list":     100,
}

func imPageSizeLimit(command string) int {
	limit, ok := imPageSizeLimits[command]
	if !ok {
		panic(fmt.Sprintf("missing IM page-size limit for %s", command))
	}
	return limit
}

func imPageSizeDescription(command string) string {
	return fmt.Sprintf("page size (1-%d)", imPageSizeLimit(command))
}

func validateIMPageSize(runtime *common.RuntimeContext, command string, defaultValue int) (int, error) {
	return validateIMPageSizeFlag(runtime, command, "page-size", defaultValue)
}

func validateIMPageSizeFlag(runtime *common.RuntimeContext, command, flagName string, defaultValue int) (int, error) {
	return common.ValidatePageSizeTyped(
		runtime,
		flagName,
		defaultValue,
		imPageSizeMinimum,
		imPageSizeLimit(command),
	)
}
