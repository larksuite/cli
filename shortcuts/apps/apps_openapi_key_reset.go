// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsOpenAPIKeyReset rotates (refreshes) an open API key, returning a new raw secret ONCE.
var AppsOpenAPIKeyReset = common.Shortcut{
	Service:     appsService,
	Command:     "+openapi-key-reset",
	Description: "Reset (rotate) an open API key; returns a new raw secret once",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +openapi-key-reset --app-id <app_id> --key-id <key_id> --yes",
		"Preview: add --dry-run to see the request without rotating",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "key-id", Desc: "API key ID", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error { return oapiKeyValidateKeyID(rctx) },
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().POST(oapiKeyRefreshURL(rctx)).Desc("Reset (rotate) open API key")
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPITyped("POST", oapiKeyRefreshURL(rctx), nil, nil)
		if err != nil {
			return withAppsHint(err, oapiKeyNotFoundHint(rctx))
		}
		return outputIssuedKey(rctx, data)
	},
}

// oapiKeyRefreshURL builds the refresh path from --app-id / --key-id.
func oapiKeyRefreshURL(rctx *common.RuntimeContext) string {
	return fmt.Sprintf(oapiKeyRefreshPath,
		validate.EncodePathSegment(strings.TrimSpace(rctx.Str("app-id"))),
		validate.EncodePathSegment(strings.TrimSpace(rctx.Str("key-id"))))
}
