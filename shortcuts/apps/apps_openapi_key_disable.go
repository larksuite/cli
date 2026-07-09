// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

// AppsOpenAPIKeyDisable disables (status=0) an open API key — the minimal safety brake.
var AppsOpenAPIKeyDisable = common.Shortcut{
	Service:     appsService,
	Command:     "+openapi-key-disable",
	Description: "Disable an open API key (minimal safety brake)",
	Risk:        "write",
	Tips:        []string{"Example: lark-cli apps +openapi-key-disable --app-id <app_id> --key-id <key_id>"},
	Scopes:      []string{"spark:app:write"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "key-id", Desc: "API key ID", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error { return oapiKeyValidateKeyID(rctx) },
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().PATCH(oapiKeyItemURL(rctx)).Desc("Disable open API key").Body(openAPIKeyStatusBody(keyStatusDisable))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return execOpenAPIKeyStatus(rctx, keyStatusDisable)
	},
}
