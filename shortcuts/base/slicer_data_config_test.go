// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/stretchr/testify/require"
)

func TestBaseDashboardBlockSlicerRequests(t *testing.T) {
	const minimal = `{"table_name":"Orders","group_by":[{"field_name":"Region"}]}`
	const defaults = `{"table_name":"Orders","group_by":[{"field_name":"Region"}],"displayMode":"tiled","selectMode":"multiple","order":"asc"}`
	const complete = `{"table_name":"Orders","group_by":[{"field_name":"Region","mode":"enumerated"}],"filter":{"conjunction":"and","conditions":[{"field_name":"Status","operator":"is","value":["Done"]}]},"displayMode":"dropdown","selectMode":"single","order":"desc"}`
	for _, tc := range []struct {
		name       string
		create     bool
		noValidate bool
		input      string
		want       string
	}{
		{"minimal create", true, false, minimal, defaults},
		{"complete create", true, false, complete, complete},
		{"create bypass omits defaults", true, true, minimal, minimal},
		{"create bypass preserves invalid values", true, true, `{"order":1,"count_all":true}`, `{"order":1,"count_all":true}`},
		{"sort patch", false, false, `{"order":"desc"}`, `{"order":"desc"}`},
		{"display patch", false, false, `{"displayMode":"dropdown"}`, `{"displayMode":"dropdown"}`},
		{"selection patch", false, false, `{"selectMode":"single"}`, `{"selectMode":"single"}`},
		{"replace filter", false, false, `{"filter":{"conjunction":"or","conditions":[{"field_name":"Status","operator":"isEmpty"}]}}`, `{"filter":{"conjunction":"or","conditions":[{"field_name":"Status","operator":"isEmpty"}]}}`},
		{"clear filter", false, false, `{"filter":null}`, `{"filter":null}`},
		{"replace group", false, false, `{"group_by":[{"field_name":"Owner"}]}`, `{"group_by":[{"field_name":"Owner"}]}`},
		{"change table", false, false, `{"table_name":"Customers","group_by":[{"field_name":"City"}]}`, `{"table_name":"Customers","group_by":[{"field_name":"City"}]}`},
		{"same table resolved by server", false, false, `{"table_name":"Orders"}`, `{"table_name":"Orders"}`},
		{"empty patch", false, false, `{}`, `{}`},
		{"omitted config", false, false, "", ""},
		{"update bypass preserves null", false, true, `{"filter":null}`, `{"filter":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shortcut, method := BaseDashboardBlockUpdate, "PATCH"
			url := "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks"
			args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Region Filter"}
			want := map[string]interface{}{"name": "Region Filter"}
			if tc.create {
				shortcut, method = BaseDashboardBlockCreate, "POST"
				args[0] = "+dashboard-block-create"
				args = append(args, "--type", "slicer")
				want["type"] = "slicer"
			} else {
				url += "/blk_x"
				args = append(args, "--block-id", "blk_x")
			}
			if tc.input != "" {
				args = append(args, "--data-config", tc.input)
				want["data_config"] = decodeCapturedBody(t, []byte(tc.want))
			}
			if tc.noValidate {
				args = append(args, "--no-validate")
			}

			factory, stdout, reg := newExecuteFactory(t)
			returned := map[string]interface{}{"block_id": "blk_x", "type": "slicer", "data_config": decodeCapturedBody(t, []byte(complete))}
			stub := &httpmock.Stub{Method: method, URL: url, Body: map[string]interface{}{"code": 0, "data": returned}}
			reg.Register(stub)
			require.NoError(t, runShortcut(t, shortcut, args, factory, stdout))
			require.Equal(t, want, decodeCapturedBody(t, stub.CapturedBody))
			response := decodeCapturedBody(t, stdout.Bytes())
			require.Equal(t, returned, response["data"].(map[string]interface{})["block"])

			dryFactory, dryStdout, _ := newExecuteFactory(t)
			require.NoError(t, runShortcut(t, shortcut, append(args, "--dry-run"), dryFactory, dryStdout))
			body, call := dryRunPreviewBody(t, dryStdout.Bytes())
			require.Equal(t, want, body)
			require.Equal(t, method, call.Method)
			require.Equal(t, url, call.URL)
		})
	}
}

func TestBaseDashboardBlockSlicerCreateRejectsInvalidConfig(t *testing.T) {
	const prefix = `{"table_name":"Orders","group_by":[{"field_name":"Region"}],`
	tests := []struct{ name, input string }{
		{"missing config", ""},
		{"null config", `null`},
		{"array config", `[]`},
		{"empty config", `{}`},
		{"missing table", `{"group_by":[{"field_name":"Region"}]}`},
		{"blank table", `{"table_name":" ","group_by":[{"field_name":"Region"}]}`},
		{"missing group", `{"table_name":"Orders"}`},
		{"empty group", `{"table_name":"Orders","group_by":[]}`},
		{"multiple groups", `{"table_name":"Orders","group_by":[{"field_name":"Region"},{"field_name":"Owner"}]}`},
		{"nonarray group", `{"table_name":"Orders","group_by":{}}`},
		{"null group item", `{"table_name":"Orders","group_by":[null]}`},
		{"missing field", `{"table_name":"Orders","group_by":[{}]}`},
		{"group sort", `{"table_name":"Orders","group_by":[{"field_name":"Region","sort":{"type":"group"}}]}`},
		{"unknown group key", `{"table_name":"Orders","group_by":[{"field_name":"Region","field_id":"fld_x"}]}`},
		{"invalid mode", `{"table_name":"Orders","group_by":[{"field_name":"Region","mode":"unknown"}]}`},
		{"null mode", `{"table_name":"Orders","group_by":[{"field_name":"Region","mode":null}]}`},
		{"count all", prefix + `"count_all":true}`},
		{"series", prefix + `"series":[]}`},
		{"top sort", prefix + `"sort":{}}`},
		{"unknown key", prefix + `"range":false}`},
		{"range display", prefix + `"displayMode":"date_component","selectMode":"single","order":"asc"}`},
		{"null display", prefix + `"displayMode":null,"selectMode":"single","order":"asc"}`},
		{"invalid selection", prefix + `"displayMode":"tiled","selectMode":"all","order":"asc"}`},
		{"clear filter on create", prefix + `"filter":null}`},
		{"malformed filter", prefix + `"filter":false}`},
		{"empty conditions", prefix + `"filter":{"conjunction":"and","conditions":[]}}`},
		{"missing conjunction", prefix + `"filter":{"conditions":[{"field_name":"Status","operator":"isEmpty"}]}}`},
	}
	for mask := 1; mask < 7; mask++ {
		fields := []string{}
		for bit, field := range []string{`"displayMode":"dropdown"`, `"selectMode":"single"`, `"order":"desc"`} {
			if mask&(1<<bit) != 0 {
				fields = append(fields, field)
			}
		}
		input := prefix
		for i, field := range fields {
			if i > 0 {
				input += ","
			}
			input += field
		}
		tests = append(tests, struct{ name, input string }{fmt.Sprintf("partial display %d", mask), input + "}"})
	}
	for _, order := range []string{`1`, `2`, `null`, `false`, `"ASC"`, `"DESC"`, `"unknown"`} {
		tests = append(tests, struct{ name, input string }{"invalid order " + order, prefix + `"displayMode":"tiled","selectMode":"multiple","order":` + order + `}`})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Invalid", "--type", "slicer"}
			if tc.input != "" {
				args = append(args, "--data-config", tc.input)
			}
			err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout)
			var validation *errs.ValidationError
			require.ErrorAs(t, err, &validation)
			require.Equal(t, errs.CategoryValidation, validation.Category)
			require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
			require.Equal(t, "--data-config", validation.Param)
			require.Empty(t, stdout.String())
		})
	}
}

func TestBaseDashboardBlockSlicerRejectsNonObjectEvenWithNoValidate(t *testing.T) {
	for _, shortcut := range []common.Shortcut{BaseDashboardBlockCreate, BaseDashboardBlockUpdate} {
		for _, input := range []string{`null`, `false`, `0`, `""`, `[]`, `{`} {
			t.Run(shortcut.Command+input, func(t *testing.T) {
				factory, stdout, _ := newExecuteFactory(t)
				args := []string{shortcut.Command, "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Invalid", "--data-config", input, "--no-validate", "--dry-run"}
				if shortcut.Command == BaseDashboardBlockCreate.Command {
					args = append(args, "--type", "slicer")
				} else {
					args = append(args, "--block-id", "blk_x")
				}
				var validation *errs.ValidationError
				require.ErrorAs(t, runShortcut(t, shortcut, args, factory, stdout), &validation)
				require.Equal(t, errs.CategoryValidation, validation.Category)
				require.Empty(t, stdout.String())
			})
		}
	}
}

func TestBaseDashboardBlockSlicerGetPreservesConfig(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	block := map[string]interface{}{"block_id": "blk_x", "type": "slicer", "data_config": map[string]interface{}{"order": "desc", "displayMode": "dropdown", "selectMode": "single"}}
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_x", Body: map[string]interface{}{"code": 0, "data": block}})
	require.NoError(t, runShortcut(t, BaseDashboardBlockGet, []string{"+dashboard-block-get", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_x"}, factory, stdout))
	require.Equal(t, block, decodeCapturedBody(t, stdout.Bytes())["data"].(map[string]interface{})["block"])
}
