// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

// app_open_api_key_status enum: 0=DISABLE, 1=ENABLE.
const (
	keyStatusDisable = 0
	keyStatusEnable  = 1
)

// AppsOpenAPIKeyEnable enables (status=1) an open API key.
var AppsOpenAPIKeyEnable = common.Shortcut{
	Service:     appsService,
	Command:     "+openapi-key-enable",
	Description: "Enable an open API key",
	Risk:        "write",
	Tips:        []string{"Example: lark-cli apps +openapi-key-enable --app-id <app_id> --key-id <key_id>"},
	Scopes:      []string{"spark:app:write"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "key-id", Desc: "API key ID", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error { return oapiKeyValidateKeyID(rctx) },
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().PATCH(oapiKeyItemURL(rctx)).Desc("Enable open API key").Body(openAPIKeyStatusBody(keyStatusEnable))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return execOpenAPIKeyStatus(rctx, keyStatusEnable)
	},
}

// openAPIKeyStatusBody builds the PATCH body for a status change.
func openAPIKeyStatusBody(status int) map[string]interface{} {
	return map[string]interface{}{"status": status}
}

// execOpenAPIKeyStatus PATCHes status and prints the redacted info.
func execOpenAPIKeyStatus(rctx *common.RuntimeContext, status int) error {
	data, err := rctx.CallAPITyped("PATCH", oapiKeyItemURL(rctx), nil, openAPIKeyStatusBody(status))
	if err != nil {
		return withAppsHint(err, oapiKeyNotFoundHint(rctx))
	}
	return outputRedactedInfo(rctx, data)
}
