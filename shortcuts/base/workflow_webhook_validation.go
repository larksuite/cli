// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"net/netip"
	"strings"
)

type workflowWebhookInput struct {
	IPs         json.RawMessage `json:"ip_allow_list"`
	Enabled     json.RawMessage `json:"enable_bearer_check"`
	Reset       json.RawMessage `json:"reset_bearer_token"`
	Output      json.RawMessage `json:"output_json"`
	URL         json.RawMessage `json:"webhook_url"`
	URLCamel    json.RawMessage `json:"webhookUrl"`
	Token       json.RawMessage `json:"webhook_token"`
	TokenCamel  json.RawMessage `json:"webhookToken"`
	Bearer      json.RawMessage `json:"bearer_token"`
	BearerCamel json.RawMessage `json:"bearerToken"`
}

// The public Go boundary runs before model conversion. In particular it rejects
// empty IP arrays and accepts reset only on update, even when reset is false.
func validateWorkflowWebhookInput(command, path string, raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage(`{}`)
	}
	data, err := workflowDecode[workflowWebhookInput](path, raw)
	if err != nil {
		return err
	}
	for _, field := range []struct {
		name string
		raw  json.RawMessage
	}{
		{"webhook_url", data.URL}, {"webhookUrl", data.URLCamel}, {"webhook_token", data.Token}, {"webhookToken", data.TokenCamel}, {"bearer_token", data.Bearer}, {"bearerToken", data.BearerCamel},
	} {
		if len(field.raw) > 0 {
			return baseFlagErrorf("%s.%s is server-managed and must not be provided", path, field.name)
		}
	}
	enabled := false
	if len(data.Enabled) > 0 {
		enabled, err = workflowDecode[bool](path+".enable_bearer_check", data.Enabled)
		if err != nil {
			return err
		}
	}
	if len(data.IPs) > 0 {
		ips, err := workflowDecode[[]string](path+".ip_allow_list", data.IPs)
		if err != nil {
			return err
		}
		if len(ips) == 0 || len(ips) > 10 {
			return baseFlagErrorf("%s.ip_allow_list must contain 1 to 10 public IPv4 addresses; omit to disable", path)
		}
		for i, text := range ips {
			ip, err := netip.ParseAddr(text)
			if err == nil {
				ip = ip.Unmap()
			}
			if err != nil || !ip.Is4() || ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() {
				return baseFlagErrorf("%s.ip_allow_list[%d] must be a public IPv4 address", path, i)
			}
		}
	}
	if len(data.Output) > 0 {
		value, err := workflowString(path+".output_json", data.Output, true)
		if err != nil {
			return err
		}
		if strings.TrimSpace(value) == "" || !json.Valid([]byte(value)) {
			return baseFlagErrorf("%s.output_json must be a valid non-empty JSON string", path)
		}
	}
	if len(data.Reset) > 0 {
		reset, err := workflowDecode[bool](path+".reset_bearer_token", data.Reset)
		if err != nil {
			return err
		}
		if command != "+workflow-update" {
			return baseFlagErrorf("%s.reset_bearer_token is only supported by +workflow-update", path)
		}
		if reset && !enabled {
			return baseFlagErrorf("%s.reset_bearer_token requires enable_bearer_check=true", path)
		}
	}
	return nil
}
