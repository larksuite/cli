// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFieldGroupCreate = common.Shortcut{
	Service:     "base",
	Command:     "+field-group-create",
	Description: "Create field groups on a table",
	Risk:        "write",
	Scopes:      []string{"base:field_group:create"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "json", Desc: `field group JSON object with a "field_groups" array, or a bare array of groups; supports @file`, Required: true},
	},
	Tips: []string{
		`Example: lark-cli base +field-group-create --base-token <base_token> --table-id <table_id> --json '{"field_groups":[{"name":"Customer Info","children":[{"type":"field","id":"fldXXXXXX"}]}]}'`,
		"Field groups bundle fields in the table UI; they are distinct from +view-set-group, which groups records inside a view.",
		"Each field can belong to only one field group; a conflict fails with 1254122 (Child belongs to multiple field groups).",
		"The platform exposes create only: there is no list, update, or delete field group API, so a 1254122 retry cannot check-then-create. Pick unused field IDs before retrying.",
		"Requires scope base:field_group:create; a full-domain auth login may omit it, request it explicitly with lark-cli auth login --scope base:field_group:create.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := parseFieldGroupBodies(newParseCtx(runtime), runtime.Str("json"))
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		groups, err := parseFieldGroupBodies(newParseCtx(runtime), runtime.Str("json"))
		if err != nil {
			return common.NewDryRunAPI().Desc(fmt.Sprintf("dry-run validation failed: %v", err))
		}
		return common.NewDryRunAPI().
			POST("/open-apis/bitable/v1/apps/:base_token/tables/:table_id/field_groups").
			Body(map[string]interface{}{"field_groups": groups}).
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", baseTableID(runtime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		groups, err := parseFieldGroupBodies(newParseCtx(runtime), runtime.Str("json"))
		if err != nil {
			return err
		}

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodPost,
			ApiPath: fmt.Sprintf(
				"/open-apis/bitable/v1/apps/%s/tables/%s/field_groups",
				validate.EncodePathSegment(runtime.Str("base-token")),
				validate.EncodePathSegment(baseTableID(runtime)),
			),
			Body: map[string]interface{}{"field_groups": groups},
		})
		if err != nil {
			return err
		}

		return handleRoleAPIResponse(runtime, apiResp, "create field groups failed")
	},
}

// parseFieldGroupBodies parses --json into the field_groups array, accepting
// either the API body shape {"field_groups":[...]} or a bare array of groups.
func parseFieldGroupBodies(pc *parseCtx, raw string) ([]interface{}, error) {
	resolved, err := loadJSONInput(pc, raw, "json")
	if err != nil {
		return nil, err
	}
	resolved = strings.TrimSpace(resolved)

	var groups []interface{}
	switch {
	case strings.HasPrefix(resolved, "["):
		if err := common.ParseJSON([]byte(resolved), &groups); err != nil {
			return nil, formatJSONError("json", "array", err)
		}
	case strings.HasPrefix(resolved, "{"):
		var body map[string]interface{}
		if err := common.ParseJSON([]byte(resolved), &body); err != nil {
			return nil, formatJSONError("json", "object", err)
		}
		rawGroups, ok := body["field_groups"]
		if !ok {
			return nil, baseFlagErrorf(`--json object must contain a "field_groups" array`)
		}
		arr, ok := rawGroups.([]interface{})
		if !ok {
			return nil, baseFlagErrorf(`--json "field_groups" must be an array`)
		}
		groups = arr
	default:
		return nil, baseFlagErrorf("--json must be a JSON array of field groups or an object with a field_groups array")
	}

	return validateFieldGroupBodies(groups)
}

// validateFieldGroupBodies enforces the API's group contract plus the
// one-field-one-group rule client-side, because the create-only API gives no
// way to check membership before retrying a 1254122 conflict.
func validateFieldGroupBodies(groups []interface{}) ([]interface{}, error) {
	if len(groups) == 0 {
		return nil, baseFlagErrorf("--json must contain at least one field group")
	}

	seen := map[string]string{}
	for i, g := range groups {
		group, ok := g.(map[string]interface{})
		if !ok {
			return nil, baseFlagErrorf("field group %d must be an object", i+1)
		}
		name := strings.TrimSpace(common.GetString(group, "name"))
		if name == "" {
			return nil, baseFlagErrorf("field group %d needs a non-empty name", i+1)
		}
		rawChildren, ok := group["children"].([]interface{})
		if !ok || len(rawChildren) == 0 {
			return nil, baseFlagErrorf("field group %q needs at least one child", name)
		}
		for j, c := range rawChildren {
			child, ok := c.(map[string]interface{})
			if !ok {
				return nil, baseFlagErrorf("field group %q child %d must be an object", name, j+1)
			}
			childType := strings.TrimSpace(common.GetString(child, "type"))
			childID := strings.TrimSpace(common.GetString(child, "id"))
			if childType == "" || childID == "" {
				return nil, baseFlagErrorf("field group %q child %d needs type and id", name, j+1)
			}
			if prev, dup := seen[childID]; dup {
				return nil, baseFlagErrorf("field %q is listed in both %q and %q; each field can belong to only one field group", childID, prev, name)
			}
			seen[childID] = name
		}
	}
	return groups, nil
}
