// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsOpenAPIKeyGet returns one open API key's detail (redacted).
var AppsOpenAPIKeyGet = common.Shortcut{
	Service:     appsService,
	Command:     "+openapi-key-get",
	Description: "Get an open API key detail (secret redacted)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +openapi-key-get --app-id <app_id> --key-id <key_id>",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "key-id", Desc: "API key ID", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return oapiKeyValidateKeyID(rctx)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET(oapiKeyItemURL(rctx)).
			Desc("Get open API key detail")
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPITyped("GET", oapiKeyItemURL(rctx), nil, nil)
		if err != nil {
			return withAppsHint(err, oapiKeyNotFoundHint(rctx))
		}
		return outputRedactedInfo(rctx, data)
	},
}

// oapiKeyItemURL builds the per-key item path from --app-id / --key-id.
func oapiKeyItemURL(rctx *common.RuntimeContext) string {
	return fmt.Sprintf(oapiKeyItemPath,
		validate.EncodePathSegment(strings.TrimSpace(rctx.Str("app-id"))),
		validate.EncodePathSegment(strings.TrimSpace(rctx.Str("key-id"))))
}

// oapiKeyNotFoundHint points a failed per-key call at +openapi-key-list.
func oapiKeyNotFoundHint(rctx *common.RuntimeContext) string {
	return "verify --key-id; list keys with `lark-cli apps +openapi-key-list --app-id " +
		strings.TrimSpace(rctx.Str("app-id")) + "`"
}

// outputRedactedInfo emits {info: <redacted>} for get/update/enable/disable.
func outputRedactedInfo(rctx *common.RuntimeContext, data map[string]interface{}) error {
	info := common.GetMap(data, "info")
	red := redactKeyInfo(info)
	out := map[string]interface{}{"info": red}
	rctx.OutFormat(out, nil, func(w io.Writer) {
		fmt.Fprintf(w, "API key ID: %v\nname: %v\nstatus: %v\nkey_preview: %v\n",
			red["api_key_id"], red["name"], red["status"], red["key_preview"])
	})
	return nil
}
