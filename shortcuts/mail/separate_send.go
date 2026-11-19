// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import "github.com/larksuite/cli/shortcuts/common"

var separateSendFlag = common.Flag{Name: "send-separately", Type: "bool", Desc: "Send separately using the existing mail service. Pass --send-separately=false to cancel. Omit to preserve an existing draft setting; new drafts default to ordinary sending. Use +draft-edit to change saved drafts before +draft-send."}

func separateSendSetting(runtime *common.RuntimeContext) *bool {
	if !runtime.Changed("send-separately") {
		return nil
	}
	value := runtime.Bool("send-separately")
	return &value
}

func withSeparateSendBody(runtime *common.RuntimeContext, body map[string]interface{}) map[string]interface{} {
	if value := separateSendSetting(runtime); value != nil {
		body["is_send_separately"] = *value
	}
	return body
}
