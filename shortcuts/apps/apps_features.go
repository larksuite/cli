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

// AppsFeatures reads an app's business feature switches (database in this release):
// each feature's name and whether it is enabled. It is read-only: it never
// prepares resources, issues credentials, or returns connection strings, and
// enabled=true says nothing about local development readiness.
var AppsFeatures = common.Shortcut{
	Service:     appsService,
	Command:     "+features",
	Description: "List an app's business features (database) and whether each is enabled",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +features --app-id <app_id>",
		"Example: lark-cli apps +features --app-id <app_id> --operation-ref b3d93913-e777-4968-a00c-afd0a1c7ce44",
		"Tip: name/enabled is the app's business switch; enabled=true does not mean local development is ready (locally run +env-pull and restart; in a Miaoda sandbox the platform applies it).",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "operation-ref", Desc: "idempotent_key of an earlier +feature-enable, to read that operation's state"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		if appID == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		return validateRealAppID(appID)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		dr := common.NewDryRunAPI().GET(appFeaturesPath(appID)).Desc("Read app features")
		if params := featuresQueryParams(rctx); params != nil {
			dr.Params(params)
		}
		return dr
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		data, err := rctx.CallAPITyped("GET", appFeaturesPath(appID), featuresQueryParams(rctx), nil)
		if isFeatureGatewayInternal(err) {
			// A passing upstream failure clears on a second read; a server
			// without app features answers the same way again.
			data, err = rctx.CallAPITyped("GET", appFeaturesPath(appID), featuresQueryParams(rctx), nil)
			if isFeatureGatewayInternal(err) {
				return explainFeatureAPIMissing(err, appID)
			}
		}
		if err != nil {
			if authErr, ok := featureAuthFailure(err); ok {
				return withFeatureAuthRecovery(authErr)
			}
			if ref := canonicalIdempotentKey(rctx.Str("operation-ref")); ref != "" && explainFeatureKeyNotKept(err, appID, ref) {
				return err
			}
			return withAppsHint(err, appIDListHint)
		}
		result, err := parseFeaturesResult(data)
		if err != nil {
			return err
		}
		rctx.OutFormat(result, nil, func(w io.Writer) {
			writeFeaturesPretty(w, result)
		})
		return nil
	},
}

func featuresQueryParams(rctx *common.RuntimeContext) map[string]any {
	if ref := canonicalIdempotentKey(rctx.Str("operation-ref")); ref != "" {
		return map[string]any{"operation_ref": ref}
	}
	return nil
}

func writeFeaturesPretty(w io.Writer, result *appFeaturesResult) {
	fmt.Fprintf(w, "app: %s\n", result.AppID)
	for _, f := range result.Features {
		writeFeatureStatusPretty(w, f)
	}
	if op := result.Operation; op != nil {
		fmt.Fprintf(w, "operation %s: %s", op.Ref, op.State)
		if op.ErrorReason != "" {
			fmt.Fprintf(w, " (%s)", op.ErrorReason)
		}
		fmt.Fprintln(w)
	}
}

func writeFeatureStatusPretty(w io.Writer, f appFeatureStatus) {
	fmt.Fprintf(w, "%s: enabled=%t\n", f.Name, f.Enabled)
}
