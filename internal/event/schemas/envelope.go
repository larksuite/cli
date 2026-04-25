// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schemas

import "encoding/json"

// v2Envelope is the Feishu V2 envelope JSON Schema; bump when upstream changes header shape.
var v2Envelope = map[string]interface{}{
	"type":        "object",
	"description": "飞书事件",
	"properties": map[string]interface{}{
		"schema": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"2.0"},
			"description": "飞书事件协议版本",
		},
		"header": map[string]interface{}{
			"type":        "object",
			"description": "事件头，所有事件结构一致",
			"properties": map[string]interface{}{
				"event_id":    map[string]string{"type": "string", "description": "事件唯一 ID"},
				"event_type":  map[string]string{"type": "string", "description": "事件类型，用于路由"},
				"create_time": map[string]string{"type": "string", "description": "事件创建时间，毫秒时间戳字符串"},
				"token":       map[string]string{"type": "string", "description": "回调校验 token"},
				"tenant_key":  map[string]string{"type": "string", "description": "租户唯一标识"},
				"app_id":      map[string]string{"type": "string", "description": "接收事件的应用 ID"},
			},
		},
	},
}

// WrapV2Envelope splices body into the `event` property; passes body through unchanged on parse fail.
func WrapV2Envelope(body json.RawMessage) json.RawMessage {
	var parsed interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return body
	}
	envelope := map[string]interface{}{}
	for k, v := range v2Envelope {
		envelope[k] = v
	}
	// Rebuild properties so we don't mutate the package-level template.
	props := map[string]interface{}{}
	for k, v := range v2Envelope["properties"].(map[string]interface{}) {
		props[k] = v
	}
	props["event"] = parsed
	envelope["properties"] = props
	data, err := json.Marshal(envelope)
	if err != nil {
		return body
	}
	return data
}
