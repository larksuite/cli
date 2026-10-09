// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseFieldManageSelectOptionsExecute(t *testing.T) {
	for _, withYes := range []bool{false, true} {
		t.Run(map[bool]string{false: "without yes", true: "with yes"}[withYes], func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			stub := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/Status/options",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"results": []interface{}{
						map[string]interface{}{"operation": "add", "name": "Done", "success": true},
						map[string]interface{}{"operation": "delete", "name": "Missing", "success": false, "error_message": "option not found"},
					}},
				},
			}
			reg.Register(stub)
			request := `{"operations":[{"operation":"add","name":"Done"},{"operation":"delete","name":"Missing"}]}`
			args := []string{
				"+manage-select-options", "--base-token", "app_x", "--table-id", "tbl_x", "--field-id", "Status", "--json", request,
			}
			if withYes {
				args = append(args, "--yes")
			}
			require.NoError(t, runShortcut(t, BaseFieldManageSelectOptions, args, factory, stdout))
			require.JSONEq(t, request, string(stub.CapturedBody))
			require.True(t, gjson.Get(stdout.String(), "data.results.0.success").Bool())
			require.False(t, gjson.Get(stdout.String(), "data.results.1.success").Bool())
			require.Equal(t, "option not found", gjson.Get(stdout.String(), "data.results.1.error_message").String())
		})
	}
}

func TestBaseFieldManageSelectOptionsYesIsHidden(t *testing.T) {
	parent := &cobra.Command{Use: "base"}
	BaseFieldManageSelectOptions.Mount(parent, &cmdutil.Factory{})
	flag := parent.Commands()[0].Flags().Lookup("yes")
	require.NotNil(t, flag)
	require.True(t, flag.Hidden)
	require.Equal(t, "write", BaseFieldManageSelectOptions.Risk)
}

func TestBaseFieldManageSelectOptionsAPIError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/Status/options",
		Body:   map[string]interface{}{"code": 190001, "msg": "dynamic options: update the source field"},
	})
	err := runShortcut(t, BaseFieldManageSelectOptions, []string{
		"+manage-select-options", "--base-token", "app_x", "--table-id", "tbl_x", "--field-id", "Status",
		"--json", `{"operations":[{"operation":"add","name":"Done"}]}`, "--yes",
	}, factory, stdout)
	require.Error(t, err)
	require.Contains(t, err.Error(), "source field")
	var output map[string]interface{}
	if stdout.Len() > 0 {
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &output))
		require.NotContains(t, output, "results")
	}
}
