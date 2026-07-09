// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// AppsOpenAPIKeyDelete permanently deletes an open API key (irreversible).
var AppsOpenAPIKeyDelete = common.Shortcut{
	Service:     appsService,
	Command:     "+openapi-key-delete",
	Description: "Delete an open API key (irreversible; prefer +openapi-key-disable)",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +openapi-key-delete --app-id <app_id> --key-id <key_id> --yes",
		"Preview: add --dry-run to see the request without deleting",
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
		return common.NewDryRunAPI().DELETE(oapiKeyItemURL(rctx)).Desc("Delete open API key")
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		id := strings.TrimSpace(rctx.Str("key-id"))
		if _, err := rctx.CallAPITyped("DELETE", oapiKeyItemURL(rctx), nil, nil); err != nil {
			return withAppsHint(err, oapiKeyNotFoundHint(rctx))
		}
		out := map[string]interface{}{"api_key_id": id, "deleted": true}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "deleted API key ID: %s\n", id)
		})
		return nil
	},
}
