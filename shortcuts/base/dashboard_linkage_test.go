// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func dashboardLinkageArgs() []string {
	return []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "chtcnA"}
}

func TestDashboardLinkageRequest(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"preserve omitted", []string{"--name", "Renamed"}, `{"name":"Renamed"}`},
		{"replace targets", []string{"--linkage", `{"target_block_ids":["chtcnC","chtcnB"]}`}, `{"linkage":{"target_block_ids":["chtcnC","chtcnB"]}}`},
		{"clear", []string{"--linkage", `{"target_block_ids":[]}`}, `{"linkage":{"target_block_ids":[]}}`},
		{"no validate", []string{"--linkage", `{"target_block_ids":["chtcnB"]}`, "--no-validate"}, `{"linkage":{"target_block_ids":["chtcnB"]}}`},
		{"combined update", []string{"--name", "Regions", "--data-config", `{"count_all":true}`, "--position", `{"x":0,"y":0,"w":6,"h":4}`, "--linkage", `{"target_block_ids":["chtcnB"]}`}, `{"name":"Regions","data_config":{"count_all":true},"position":{"x":0,"y":0,"w":6,"h":4},"fine_grid":true,"linkage":{"target_block_ids":["chtcnB"]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			stub := &httpmock.Stub{
				Method: "PATCH", URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/chtcnA",
				Body: json.RawMessage(`{"code":0,"data":{"block_id":"chtcnA"}}`),
			}
			reg.Register(stub)
			require.NoError(t, runShortcut(t, BaseDashboardBlockUpdate, append(dashboardLinkageArgs(), tc.args...), factory, stdout))
			require.JSONEq(t, tc.want, string(stub.CapturedBody))
			require.True(t, gjson.Get(stdout.String(), "data.updated").Bool())
		})
	}
}

func TestDashboardLinkageRejectsUnsupportedInput(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	tooMany := make([]string, 101)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("chtcnTarget%d", i)
	}
	encoded, err := json.Marshal(dashboardChartLinkage{TargetBlockIDs: tooMany})
	require.NoError(t, err)
	for _, input := range []string{
		"", " ", "null", "[]", "{}", `{"target_block_ids":null}`, `{"target_block_ids":"chtcnB"}`,
		`{"target_block_ids":[1]}`, `{"target_block_ids":[null]}`, `{"target_block_ids":[""]}`,
		`{"target_block_ids":[" "]}`, `{"target_block_ids":["chtcnA"]}`, `{"target_block_ids":["chtcnB","chtcnB"]}`,
		`{"target_block_ids":[],"field_mappings":[]}`, `{"target_block_ids":[],"unknown":true}`,
		`{"target_block_ids":["chtcnB"]`, `{"target_block_ids":[]} {}`, `{"target_block_ids":[]} invalid`,
		`{"target_block_ids":["` + strings.Repeat("x", 4097) + `"]}`, string(encoded),
	} {
		for _, dryRun := range []bool{false, true} {
			for _, noValidate := range []bool{false, true} {
				t.Run(fmt.Sprintf("case_%d/dry=%t/noValidate=%t", len(input), dryRun, noValidate), func(t *testing.T) {
					factory, stdout, _ := newExecuteFactory(t)
					args := append(dashboardLinkageArgs(), "--linkage", input)
					if dryRun {
						args = append(args, "--dry-run")
					}
					if noValidate {
						args = append(args, "--no-validate")
					}
					err := runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout)
					assertInvalidArgumentValidation(t, err, "--linkage", nil, "")
					require.Empty(t, stdout.String())
				})
			}
		}
	}
}

func TestDashboardLinkageDecodePreservesCause(t *testing.T) {
	_, err := parseDashboardChartLinkage(nil, `{"target_block_ids":[2]}`, "chtcnA")
	var typeErr *json.UnmarshalTypeError
	require.True(t, errors.As(err, &typeErr))
	_, err = parseDashboardChartLinkage(nil, `{"target_block_ids":`, "chtcnA")
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

func TestDashboardLinkagePropagatesBackendRejection(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PATCH", URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/chtcnA",
		Body: json.RawMessage(`{"code":1254001,"msg":"linkage requires identical datasources"}`),
	})
	err := runShortcut(t, BaseDashboardBlockUpdate,
		append(dashboardLinkageArgs(), "--linkage", `{"target_block_ids":["chtcnB"]}`), factory, stdout)
	assertProblemCode(t, err, 1254001, "linkage requires identical datasources")
	require.Empty(t, stdout.String(), "a rejected update must not emit success data")
}

func TestDashboardLinkageFileReadOnce(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, dryRun := range []bool{false, true} {
		t.Run(fmt.Sprintf("dry=%t", dryRun), func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			fio := &changingJSONFileIO{contents: []string{`{"target_block_ids":["chtcnB"]}`, `{"target_block_ids":["chtcnC"]}`}}
			factory.FileIOProvider = changingJSONProvider{fio: fio}
			stub := &httpmock.Stub{
				Method: "PATCH", URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/chtcnA",
				Body: json.RawMessage(`{"code":0,"data":{"block_id":"chtcnA"}}`),
			}
			args := append(dashboardLinkageArgs(), "--linkage", "@linkage.json", "--no-validate")
			if dryRun {
				args = append(args, "--dry-run")
			} else {
				reg.Register(stub)
			}
			require.NoError(t, runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout))
			require.Equal(t, 1, fio.opens)
			if dryRun {
				require.Contains(t, stdout.String(), "chtcnB")
				require.NotContains(t, stdout.String(), "chtcnC")
				require.Empty(t, stub.CapturedBody)
			} else {
				require.JSONEq(t, `{"linkage":{"target_block_ids":["chtcnB"]}}`, string(stub.CapturedBody))
			}
		})
	}
}

func TestDashboardLinkageReadback(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name     string
		shortcut common.Shortcut
		method   string
		args     []string
	}{
		{"get", BaseDashboardBlockGet, "GET", []string{"+dashboard-block-get", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "chtcnA"}},
		{"update", BaseDashboardBlockUpdate, "PATCH", append(dashboardLinkageArgs(), "--name", "Renamed")},
	} {
		for _, linkage := range []string{"", `,"linkage":{"target_block_ids":["chtcnB"]}`, `,"linkage":{"target_block_ids":["chtcnB"],"field_mappings":[{"source_table_name":"Orders","target_table_name":"Budget","fields":[{"source_field_name":"Region","target_field_name":"Area"}]}]}`} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, len(linkage)), func(t *testing.T) {
				factory, stdout, reg := newExecuteFactory(t)
				block := `{"block_id":"chtcnA","name":"Regions","type":"column"` + linkage + `}`
				reg.Register(&httpmock.Stub{Method: tc.method, URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/chtcnA", Body: json.RawMessage(`{"code":0,"data":` + block + `}`)})
				require.NoError(t, runShortcut(t, tc.shortcut, tc.args, factory, stdout))
				require.JSONEq(t, block, gjson.Get(stdout.String(), "data.block").Raw)
			})
		}
	}
}
