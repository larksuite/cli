// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var allowedAccessTargetTypes = map[string]bool{
	"user":       true,
	"department": true,
	"chat":       true,
}

// AppsAccessScopeSet sets the app's access scope (specific / public / tenant).
var AppsAccessScopeSet = common.Shortcut{
	Service:     appsService,
	Command:     "+access-scope-set",
	Description: "Set Miaoda app access scope (specific / public / tenant)",
	Risk:        "write",
	Scopes:      []string{"miaoda:app:write"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "scope", Desc: "scope: specific | public | tenant", Required: true, Enum: []string{"specific", "public", "tenant"}},
		{Name: "targets", Desc: `targets JSON array: [{"type":"user|department|chat","id":"..."}, ...]`},
		{Name: "apply-enabled", Type: "bool", Desc: "allow apply for access (scope=specific)"},
		{Name: "approver", Desc: "approver open_id (when --apply-enabled; server allows exactly one)"},
		{Name: "require-login", Type: "bool", Desc: "require login (scope=public)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return output.ErrValidation("--app-id is required")
		}
		return validateAccessScopeFlags(rctx)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildAccessScopeBody(rctx)
		return common.NewDryRunAPI().
			PUT(fmt.Sprintf("%s/apps/%s/access-scope", apiBasePath, validate.EncodePathSegment(rctx.Str("app-id")))).
			Desc("Set Miaoda app access scope").
			Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		body, err := buildAccessScopeBody(rctx)
		if err != nil {
			return err
		}
		path := fmt.Sprintf("%s/apps/%s/access-scope", apiBasePath, validate.EncodePathSegment(rctx.Str("app-id")))
		data, err := rctx.CallAPI("PUT", path, nil, body)
		if err != nil {
			return err
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "access-scope set: %s\n", rctx.Str("scope"))
		})
		return nil
	},
}

func validateAccessScopeFlags(rctx *common.RuntimeContext) error {
	scope := rctx.Str("scope")
	targets := strings.TrimSpace(rctx.Str("targets"))
	applyEnabled := rctx.Bool("apply-enabled")
	approver := strings.TrimSpace(rctx.Str("approver"))
	requireLogin := rctx.Bool("require-login")

	switch scope {
	case "specific":
		if targets == "" {
			return output.ErrValidation("--targets is required when --scope=specific")
		}
		if err := validateTargetsJSON(targets); err != nil {
			return err
		}
		if approver != "" && !applyEnabled {
			return output.ErrValidation("--approver requires --apply-enabled")
		}
		if requireLogin {
			return output.ErrValidation("--require-login is not allowed when --scope=specific")
		}
	case "public":
		if targets != "" {
			return output.ErrValidation("--targets is not allowed when --scope=public")
		}
		if applyEnabled {
			return output.ErrValidation("--apply-enabled is not allowed when --scope=public")
		}
		// H3 待对齐: bare --scope public without --require-login is currently accepted (sends require_login=false).
		// Concept design §5.1 says require-login should be required; revisit after BOE verification.
	case "tenant":
		if targets != "" || applyEnabled || approver != "" || requireLogin {
			return output.ErrValidation("no extra flags allowed when --scope=tenant")
		}
	default:
		return output.ErrValidation("--scope must be specific / public / tenant")
	}
	return nil
}

func validateTargetsJSON(targetsJSON string) error {
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(targetsJSON), &items); err != nil {
		return output.ErrValidation("--targets is not valid JSON: %v", err)
	}
	for i, t := range items {
		typ, _ := t["type"].(string)
		if !allowedAccessTargetTypes[typ] {
			return output.ErrValidation("--targets[%d].type %q must be one of: user / department / chat", i, typ)
		}
		if id, _ := t["id"].(string); strings.TrimSpace(id) == "" {
			return output.ErrValidation("--targets[%d].id is empty", i)
		}
	}
	return nil
}

func buildAccessScopeBody(rctx *common.RuntimeContext) (map[string]interface{}, error) {
	scope := rctx.Str("scope")
	body := map[string]interface{}{"scope": scope}

	switch scope {
	case "specific":
		var targets []map[string]interface{}
		if err := json.Unmarshal([]byte(rctx.Str("targets")), &targets); err != nil {
			return nil, output.ErrValidation("--targets is not valid JSON: %v", err)
		}
		body["targets"] = targets
		if rctx.Bool("apply-enabled") {
			applyConfig := map[string]interface{}{"enabled": true}
			if approver := strings.TrimSpace(rctx.Str("approver")); approver != "" {
				applyConfig["approvers"] = []string{approver}
			}
			body["apply_config"] = applyConfig
		}
	case "public":
		body["require_login"] = rctx.Bool("require-login")
	}
	return body, nil
}
