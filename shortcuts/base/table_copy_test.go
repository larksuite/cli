// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestBaseTableCopyShortcutContract(t *testing.T) {
	if BaseTableCopy.Command != "+table-copy" {
		t.Fatalf("command = %q, want +table-copy", BaseTableCopy.Command)
	}
	if BaseTableCopy.Risk != "write" {
		t.Fatalf("risk = %q, want write", BaseTableCopy.Risk)
	}
	if !slices.Equal(BaseTableCopy.AuthTypes, authTypes()) {
		t.Fatalf("auth types = %#v, want standard Base auth types %#v", BaseTableCopy.AuthTypes, authTypes())
	}
	tips := strings.Join(BaseTableCopy.Tips, "\n")
	if !strings.Contains(tips, "--range all") || !strings.Contains(tips, "ID or name") {
		t.Fatalf("tips = %q, want range safety and table-ref guidance", tips)
	}

	flags := make(map[string]struct {
		defaultValue string
		required     bool
		enum         []string
	})
	for _, flag := range BaseTableCopy.Flags {
		flags[flag.Name] = struct {
			defaultValue string
			required     bool
			enum         []string
		}{flag.Default, flag.Required, flag.Enum}
	}
	for _, required := range []string{"base-token", "table-id", "name"} {
		if !flags[required].required {
			t.Fatalf("flag --%s must be required", required)
		}
	}
	if got := flags["range"]; got.required || got.defaultValue != "schema" || !slices.Equal(got.enum, []string{"schema", "all"}) {
		t.Fatalf("range flag = %#v, want optional schema default with schema/all enum", got)
	}

	cmd := &cobra.Command{Use: "+table-copy"}
	if BaseTableCopy.PostMount == nil {
		t.Fatal("table copy must register Cobra duration through PostMount")
	}
	BaseTableCopy.PostMount(cmd)
	timeoutFlag := cmd.Flags().Lookup("timeout")
	if timeoutFlag == nil {
		t.Fatal("--timeout was not registered")
	}
	if timeoutFlag.Value.Type() != "duration" || timeoutFlag.DefValue != (5*time.Minute).String() {
		t.Fatalf("timeout type/default = %q/%q, want duration/5m0s", timeoutFlag.Value.Type(), timeoutFlag.DefValue)
	}
	if !strings.Contains(timeoutFlag.Usage, "30m") {
		t.Fatalf("timeout usage = %q, want documented 30m maximum", timeoutFlag.Usage)
	}
}

func TestBaseTableCopyStatusShortcutContract(t *testing.T) {
	if BaseTableCopyStatus.Command != "+table-copy-status" {
		t.Fatalf("command = %q, want +table-copy-status", BaseTableCopyStatus.Command)
	}
	if BaseTableCopyStatus.Risk != "read" {
		t.Fatalf("risk = %q, want read", BaseTableCopyStatus.Risk)
	}
	if !slices.Equal(BaseTableCopyStatus.AuthTypes, authTypes()) {
		t.Fatalf("auth types = %#v, want standard Base auth types %#v", BaseTableCopyStatus.AuthTypes, authTypes())
	}
}

func TestBaseTableCopyShortcutsUseTableCreateScope(t *testing.T) {
	want := []string{"base:table:create"}
	if !slices.Equal(BaseTableCopy.Scopes, want) {
		t.Fatalf("table copy scopes = %#v, want %#v", BaseTableCopy.Scopes, want)
	}
	if !slices.Equal(BaseTableCopyStatus.Scopes, want) {
		t.Fatalf("table copy status scopes = %#v, want %#v", BaseTableCopyStatus.Scopes, want)
	}
}

func TestBaseTableCopyStatusRejectsInvalidTaskID(t *testing.T) {
	tests := []struct {
		name   string
		taskID string
	}{
		{name: "blank", taskID: "   "},
		{name: "too long", taskID: strings.Repeat("x", tableCopyTaskIDMax+1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			err := runShortcutWithAuthTypes(
				t,
				BaseTableCopyStatus,
				BaseTableCopyStatus.AuthTypes,
				[]string{"+table-copy-status", "--base-token", "app_x", "--task-id", test.taskID, "--as", "user"},
				factory,
				stdout,
			)
			if err == nil {
				t.Fatal("expected validation error")
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) || validationErr.Param != "--task-id" {
				t.Fatalf("error = %T %v, want validation --task-id", err, err)
			}
		})
	}
}

func TestBaseTableCopySupportsBotIdentity(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	err := runShortcutWithAuthTypes(
		t,
		BaseTableCopy,
		BaseTableCopy.AuthTypes,
		[]string{"+table-copy", "--base-token", "app_x", "--table-id", "tbl_x", "--name", "Copy", "--as", "bot", "--dry-run"},
		factory,
		stdout,
	)
	if err != nil {
		t.Fatalf("bot dry-run: %v", err)
	}
}

func TestBaseTableCopyRejectsInvalidFlagCombinations(t *testing.T) {
	tests := []struct {
		name      string
		extraArgs []string
		param     string
	}{
		{name: "schema wait", extraArgs: []string{"--wait"}, param: "--wait"},
		{name: "schema explicit timeout", extraArgs: []string{"--timeout", "1m"}, param: "--timeout"},
		{name: "all timeout without wait", extraArgs: []string{"--range", "all", "--timeout", "1m"}, param: "--timeout"},
		{name: "negative timeout", extraArgs: []string{"--range", "all", "--wait", "--timeout", "-1s"}, param: "--timeout"},
		{name: "zero timeout", extraArgs: []string{"--range", "all", "--wait", "--timeout", "0s"}, param: "--timeout"},
		{name: "timeout above limit", extraArgs: []string{"--range", "all", "--wait", "--timeout", "31m"}, param: "--timeout"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			args := []string{"+table-copy", "--base-token", "app_x", "--table-id", "tbl_x", "--name", "Copy", "--as", "user"}
			args = append(args, test.extraArgs...)
			err := runShortcutWithAuthTypes(t, BaseTableCopy, BaseTableCopy.AuthTypes, args, factory, stdout)
			if err == nil {
				t.Fatal("expected validation error")
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T %v, want validation error", err, err)
			}
			if validationErr.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != test.param {
				t.Fatalf("problem = %#v, want invalid_argument param %s", validationErr.Problem, test.param)
			}
		})
	}
}

func TestBaseTableCopyAcceptsMaximumTimeout(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	err := runShortcutWithAuthTypes(
		t,
		BaseTableCopy,
		BaseTableCopy.AuthTypes,
		[]string{"+table-copy", "--base-token", "app_x", "--table-id", "tbl_x", "--name", "Copy", "--range", "all", "--wait", "--timeout", "30m", "--dry-run", "--as", "user"},
		factory,
		stdout,
	)
	if err != nil {
		t.Fatalf("30m timeout must be accepted: %v", err)
	}
	data := decodeBaseEnvelope(t, stdout)
	if data["timeout"] != "30m0s" {
		t.Fatalf("timeout = %#v, want 30m0s", data["timeout"])
	}
}

func TestBaseTableCopyDefaultsToSchemaAndPassesTableNameDirectly(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	copyStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables/Tasks/copy",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"table": map[string]interface{}{"id": "tbl_target", "name": "Copy"},
				"state": "success",
			},
		},
	}
	reg.Register(copyStub)

	err := runShortcutWithAuthTypes(
		t,
		BaseTableCopy,
		BaseTableCopy.AuthTypes,
		[]string{"+table-copy", "--base-token", "app_x", "--table-id", "Tasks", "--name", "Copy", "--as", "user"},
		factory,
		stdout,
	)
	if err != nil {
		t.Fatalf("table copy: %v", err)
	}

	body := decodeCapturedJSONBody(t, copyStub)
	if body["name"] != "Copy" || body["range"] != tableCopyRangeSchema {
		t.Fatalf("copy body = %#v, want name Copy and default range schema", body)
	}
	data := decodeBaseEnvelope(t, stdout)
	if data["range"] != tableCopyRangeSchema || data["state"] != tableCopyStateSuccess || data["completed"] != true {
		t.Fatalf("copy output = %#v", data)
	}
	table := common.GetMap(data, "table")
	if common.GetString(table, "id") != "tbl_target" || common.GetString(table, "name") != "Copy" {
		t.Fatalf("table output = %#v", table)
	}
}

func TestBaseTableCopySubmissionIsNeverRetried(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	requestCount := 0
	reg.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/base/v3/bases/app_x/tables/tbl_source/copy",
		Error:    context.DeadlineExceeded,
		Reusable: true,
		OnMatch: func(*http.Request) {
			requestCount++
		},
	})

	err := runShortcutWithAuthTypes(
		t,
		BaseTableCopy,
		BaseTableCopy.AuthTypes,
		[]string{"+table-copy", "--base-token", "app_x", "--table-id", "tbl_source", "--name", "Copy", "--range", "all", "--as", "user"},
		factory,
		stdout,
	)
	if err == nil {
		t.Fatal("expected submission error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkTimeout || problem.Retryable {
		t.Fatalf("error = %T %v, problem=%#v", err, err, problem)
	}
	if requestCount != 1 {
		t.Fatalf("submit request count = %d, want exactly 1", requestCount)
	}
}

func TestBaseTableCopyAllWithoutWaitReturnsTaskAndNextCommand(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	copyStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_source/copy",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"table":   map[string]interface{}{"id": "tbl_target", "name": "Copy"},
				"task_id": "ct1.token",
				"state":   "init",
			},
		},
	}
	reg.Register(copyStub)

	err := runShortcutWithAuthTypes(
		t,
		BaseTableCopy,
		BaseTableCopy.AuthTypes,
		[]string{"+table-copy", "--base-token", "app_x", "--table-id", "tbl_source", "--name", "Copy", "--range", "all", "--as", "user"},
		factory,
		stdout,
	)
	if err != nil {
		t.Fatalf("table copy: %v", err)
	}

	data := decodeBaseEnvelope(t, stdout)
	if data["range"] != tableCopyRangeAll || data["state"] != tableCopyStateInit || data["completed"] != false || data["task_id"] != "ct1.token" {
		t.Fatalf("copy output = %#v", data)
	}
	if data["next_action"] != "poll_status" {
		t.Fatalf("next_action = %#v", data["next_action"])
	}
	wantNext := "lark-cli base +table-copy-status --base-token app_x --task-id ct1.token --as user"
	if data["next_command"] != wantNext {
		t.Fatalf("next_command = %#v, want %q", data["next_command"], wantNext)
	}
}

func TestBaseTableCopyStatusProcessIgnoresLastErrorCode(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	statusStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/copy_table_state",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"table_id":        "tbl_target",
				"state":           "process",
				"last_error_code": 12345,
			},
		},
	}
	reg.Register(statusStub)

	err := runShortcutWithAuthTypes(
		t,
		BaseTableCopyStatus,
		BaseTableCopyStatus.AuthTypes,
		[]string{"+table-copy-status", "--base-token", "app_x", "--task-id", "ct1.token", "--as", "user"},
		factory,
		stdout,
	)
	if err != nil {
		t.Fatalf("table copy status: %v", err)
	}

	body := decodeCapturedJSONBody(t, statusStub)
	if body["task_id"] != "ct1.token" {
		t.Fatalf("status body = %#v", body)
	}
	data := decodeBaseEnvelope(t, stdout)
	if data["state"] != tableCopyStateProcess || data["completed"] != false || data["task_id"] != "ct1.token" {
		t.Fatalf("status output = %#v", data)
	}
	if _, ok := data["last_error_code"]; ok {
		t.Fatalf("process output must omit last_error_code: %#v", data)
	}
}

func TestProjectTableCopyStatusProcessIgnoresMalformedLastErrorCode(t *testing.T) {
	status, err := projectTableCopyStatus(map[string]interface{}{
		"table_id":        "tbl_target",
		"state":           "process",
		"last_error_code": "not-a-number",
	})
	if err != nil {
		t.Fatalf("project process status: %v", err)
	}
	if status.State != tableCopyStateProcess {
		t.Fatalf("status = %#v, want process", status)
	}
	if _, exists := reflect.TypeOf(status).FieldByName("LastErrorCode"); exists {
		t.Fatal("tableCopyStatus must not model last_error_code")
	}
	if _, exists := reflect.TypeOf(tableCopyOutput{}).FieldByName("LastErrorCode"); exists {
		t.Fatal("tableCopyOutput must not expose last_error_code")
	}
}

func TestBaseTableCopyStatusRejectsFailedSuccessEnvelope(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/copy_table_state",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"table_id":        "tbl_target",
				"state":           "failed",
				"last_error_code": 12345,
			},
		},
	})

	err := runShortcutWithAuthTypes(
		t,
		BaseTableCopyStatus,
		BaseTableCopyStatus.AuthTypes,
		[]string{"+table-copy-status", "--base-token", "app_x", "--task-id", "ct1.token", "--as", "user"},
		factory,
		stdout,
	)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("error = %T %v, problem=%#v", err, err, problem)
	}
	if stdout.Len() != 0 {
		t.Fatalf("failed success envelope must not produce status output: %s", stdout.String())
	}
}

func TestBaseTableCopyDryRunDefaultsToSchemaWithoutNetwork(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	err := runShortcutWithAuthTypes(
		t,
		BaseTableCopy,
		BaseTableCopy.AuthTypes,
		[]string{"+table-copy", "--base-token", "app_x", "--table-id", "Tasks", "--name", "Copy", "--dry-run", "--as", "user"},
		factory,
		stdout,
	)
	if err != nil {
		t.Fatalf("table copy dry-run: %v", err)
	}

	data := decodeBaseEnvelope(t, stdout)
	api, _ := data["api"].([]interface{})
	if len(api) != 1 {
		t.Fatalf("api calls = %#v, want one submit", data["api"])
	}
	call, _ := api[0].(map[string]interface{})
	if call["method"] != "POST" || call["url"] != "/open-apis/base/v3/bases/app_x/tables/Tasks/copy" {
		t.Fatalf("submit call = %#v", call)
	}
	body, _ := call["body"].(map[string]interface{})
	if body["name"] != "Copy" || body["range"] != tableCopyRangeSchema {
		t.Fatalf("submit body = %#v", body)
	}
	if data["wait"] != false {
		t.Fatalf("wait metadata = %#v", data["wait"])
	}
}

func TestBaseTableCopyDryRunWaitShowsSymbolicStatusStep(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	err := runShortcutWithAuthTypes(
		t,
		BaseTableCopy,
		BaseTableCopy.AuthTypes,
		[]string{"+table-copy", "--base-token", "app_x", "--table-id", "tbl_x", "--name", "Copy", "--range", "all", "--wait", "--timeout", "2m", "--dry-run", "--as", "user"},
		factory,
		stdout,
	)
	if err != nil {
		t.Fatalf("table copy wait dry-run: %v", err)
	}

	data := decodeBaseEnvelope(t, stdout)
	api, _ := data["api"].([]interface{})
	if len(api) != 2 {
		t.Fatalf("api calls = %#v, want submit and symbolic status", data["api"])
	}
	statusCall, _ := api[1].(map[string]interface{})
	statusBody, _ := statusCall["body"].(map[string]interface{})
	if statusCall["url"] != "/open-apis/base/v3/bases/app_x/copy_table_state" || statusBody["task_id"] != "<task_id_from_step_1>" {
		t.Fatalf("status call = %#v", statusCall)
	}
	if data["wait"] != true || data["timeout"] != "2m0s" {
		encoded, _ := json.Marshal(data)
		t.Fatalf("orchestration metadata = %s", encoded)
	}
}

func TestBaseTableCopyAllWaitsForSuccess(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stderr := factory.IOStreams.ErrOut.(interface{ String() string })
	statusRequestHasDeadline := false
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_source/copy",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"table":   map[string]interface{}{"id": "tbl_target", "name": "Copy"},
				"task_id": "ct1.token",
				"state":   "init",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/copy_table_state",
		OnMatch: func(req *http.Request) {
			_, statusRequestHasDeadline = req.Context().Deadline()
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"table_id": "tbl_target",
				"state":    "success",
			},
		},
	})

	clock := &advancingTableCopyClock{now: time.Unix(0, 0)}
	shortcut := BaseTableCopy
	shortcut.Execute = func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTableCopyWithClock(ctx, runtime, clock)
	}
	err := runShortcutWithAuthTypes(
		t,
		shortcut,
		shortcut.AuthTypes,
		[]string{"+table-copy", "--base-token", "app_x", "--table-id", "tbl_source", "--name", "Copy", "--range", "all", "--wait", "--as", "user"},
		factory,
		stdout,
	)
	if err != nil {
		t.Fatalf("table copy wait: %v", err)
	}

	data := decodeBaseEnvelope(t, stdout)
	if data["state"] != tableCopyStateSuccess || data["completed"] != true || data["task_id"] != "ct1.token" {
		t.Fatalf("wait output = %#v", data)
	}
	if _, ok := data["last_error_code"]; ok {
		t.Fatalf("success output must omit last_error_code: %#v", data)
	}
	if !strings.Contains(stderr.String(), "Table copy status: success") {
		t.Fatalf("stderr = %q, want status progress", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Table copy submitted: init, task_id=ct1.token") {
		t.Fatalf("stderr = %q, want recoverable submit progress", stderr.String())
	}
	if !statusRequestHasDeadline {
		t.Fatal("status API request context must inherit the remaining poll deadline")
	}
}

func TestBaseTableCopyAllWaitTimeoutReturnsContinuation(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_source/copy",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"table":   map[string]interface{}{"id": "tbl_target", "name": "Copy"},
				"task_id": "ct1.token",
				"state":   "init",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/base/v3/bases/app_x/copy_table_state",
		Reusable: true,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"table_id":        "tbl_target",
				"state":           "process",
				"last_error_code": 12345,
			},
		},
	})

	clock := &advancingTableCopyClock{now: time.Unix(0, 0)}
	shortcut := BaseTableCopy
	shortcut.Execute = func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTableCopyWithClock(ctx, runtime, clock)
	}
	err := runShortcutWithAuthTypes(
		t,
		shortcut,
		shortcut.AuthTypes,
		[]string{"+table-copy", "--base-token", "app_x", "--table-id", "tbl_source", "--name", "Copy", "--range", "all", "--wait", "--timeout", "10s", "--as", "user"},
		factory,
		stdout,
	)
	if err != nil {
		t.Fatalf("table copy wait timeout: %v", err)
	}

	data := decodeBaseEnvelope(t, stdout)
	if data["state"] != tableCopyStateProcess || data["completed"] != false || data["timed_out"] != true {
		t.Fatalf("timeout output = %#v", data)
	}
	if _, ok := data["last_error_code"]; ok {
		t.Fatalf("process timeout output must omit last_error_code: %#v", data)
	}
	if data["next_action"] != "poll_status" || data["next_command"] == "" {
		t.Fatalf("timeout continuation = %#v", data)
	}
}

func TestBaseTableCopyAllWaitTimeoutBeforeFirstStatusReturnsUnknown(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_source/copy",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"table":   map[string]interface{}{"id": "tbl_target", "name": "Copy"},
				"task_id": "ct1.token",
				"state":   "init",
			},
		},
	})

	clock := &advancingTableCopyClock{now: time.Unix(0, 0)}
	shortcut := BaseTableCopy
	shortcut.Execute = func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTableCopyWithClock(ctx, runtime, clock)
	}
	err := runShortcutWithAuthTypes(
		t,
		shortcut,
		shortcut.AuthTypes,
		[]string{"+table-copy", "--base-token", "app_x", "--table-id", "tbl_source", "--name", "Copy", "--range", "all", "--wait", "--timeout", "1s", "--as", "user"},
		factory,
		stdout,
	)
	if err != nil {
		t.Fatalf("table copy early timeout: %v", err)
	}

	data := decodeBaseEnvelope(t, stdout)
	if data["state"] != tableCopyStateUnknown || data["timed_out"] != true || data["completed"] != false {
		t.Fatalf("early timeout output = %#v", data)
	}
}

func TestBaseTableCopyAllWaitUsesTopLevelTaskError(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_source/copy",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"table":   map[string]interface{}{"id": "tbl_target", "name": "Copy"},
				"task_id": "ct1.token",
				"state":   "init",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/copy_table_state",
		Body: map[string]interface{}{
			"code": 800070111,
			"msg":  "table copy task failed",
		},
	})

	clock := &advancingTableCopyClock{now: time.Unix(0, 0)}
	shortcut := BaseTableCopy
	shortcut.Execute = func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTableCopyWithClock(ctx, runtime, clock)
	}
	err := runShortcutWithAuthTypes(
		t,
		shortcut,
		shortcut.AuthTypes,
		[]string{"+table-copy", "--base-token", "app_x", "--table-id", "tbl_source", "--name", "Copy", "--range", "all", "--wait", "--as", "user"},
		factory,
		stdout,
	)
	if err == nil {
		t.Fatal("expected typed task failure")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Code != 800070111 || problem.Category != errs.CategoryAPI || problem.Subtype != errs.SubtypeUnknown {
		t.Fatalf("error = %T %v, problem=%#v", err, err, problem)
	}
}

func TestBaseTableCopyWaitErrorEmitsRecoverableTaskOnProgressAndStdout(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stderr := factory.IOStreams.ErrOut.(interface{ String() string })
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_source/copy",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"table":   map[string]interface{}{"id": "tbl_target", "name": "Copy"},
				"task_id": "ct1.token",
				"state":   "init",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/copy_table_state",
		Body: map[string]interface{}{
			"code": 800010109,
			"msg":  "invalid task",
		},
	})

	clock := &advancingTableCopyClock{now: time.Unix(0, 0)}
	shortcut := BaseTableCopy
	shortcut.Execute = func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTableCopyWithClock(ctx, runtime, clock)
	}
	err := runShortcutWithAuthTypes(
		t,
		shortcut,
		shortcut.AuthTypes,
		[]string{"+table-copy", "--base-token", "app_x", "--table-id", "tbl_source", "--name", "Copy", "--range", "all", "--wait", "--as", "user"},
		factory,
		stdout,
	)
	if err == nil {
		t.Fatal("expected status error after successful submit")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Code != 800010109 {
		t.Fatalf("error = %T %v, problem=%#v", err, err, problem)
	}
	if strings.Contains(problem.Hint, "ct1.token") {
		t.Fatalf("error hint must remain reusable and not embed one task ID: %q", problem.Hint)
	}
	if !strings.Contains(stderr.String(), "Table copy submitted: init, task_id=ct1.token") {
		t.Fatalf("stderr = %q, want recoverable submit progress", stderr.String())
	}

	var envelope struct {
		OK   bool            `json:"ok"`
		Data tableCopyOutput `json:"data"`
	}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &envelope); decodeErr != nil {
		t.Fatalf("decode recovery stdout: %v\nraw=%s", decodeErr, stdout.String())
	}
	if envelope.OK || envelope.Data.TaskID != "ct1.token" || envelope.Data.NextAction != "poll_status" || !strings.Contains(envelope.Data.NextCommand, "ct1.token") {
		t.Fatalf("recovery envelope = %#v", envelope)
	}
}

type advancingTableCopyClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (c *advancingTableCopyClock) Now() time.Time { return c.now }

func (c *advancingTableCopyClock) NewTimer(duration time.Duration) tableCopyTimer {
	c.sleeps = append(c.sleeps, duration)
	c.now = c.now.Add(duration)
	ch := make(chan time.Time, 1)
	ch <- c.now
	return &advancingTableCopyTimer{ch: ch}
}

type advancingTableCopyTimer struct {
	ch chan time.Time
}

func (t *advancingTableCopyTimer) C() <-chan time.Time { return t.ch }
func (t *advancingTableCopyTimer) Stop() bool          { return true }

func TestPollTableCopyUsesBackoffAndStateOnly(t *testing.T) {
	statuses := []tableCopyStatus{
		{TableID: "tbl_target", State: tableCopyStateProcess},
		{TableID: "tbl_target", State: tableCopyStateSuccess},
	}
	clock := &advancingTableCopyClock{now: time.Unix(0, 0)}
	fetches := 0

	status, timedOut, err := pollTableCopy(
		context.Background(),
		30*time.Second,
		clock,
		func(context.Context) (tableCopyStatus, error) {
			status := statuses[fetches]
			fetches++
			return status, nil
		},
	)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if timedOut || status.State != tableCopyStateSuccess {
		t.Fatalf("status=%#v timedOut=%v", status, timedOut)
	}
	if !slices.Equal(clock.sleeps, []time.Duration{3 * time.Second, 6 * time.Second}) {
		t.Fatalf("sleeps = %#v", clock.sleeps)
	}
}

func TestPollTableCopyBoundsEachFetchByRemainingTimeout(t *testing.T) {
	clock := &advancingTableCopyClock{now: time.Unix(0, 0)}
	fetches := 0
	_, timedOut, err := pollTableCopy(
		context.Background(),
		30*time.Second,
		clock,
		func(ctx context.Context) (tableCopyStatus, error) {
			fetches++
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("fetch context has no deadline")
			}
			remaining := time.Until(deadline)
			if remaining <= 0 || remaining > 27*time.Second {
				t.Fatalf("fetch deadline remaining = %s, want within remaining 27s budget", remaining)
			}
			return tableCopyStatus{TableID: "tbl_target", State: tableCopyStateSuccess}, nil
		},
	)
	if err != nil || timedOut || fetches != 1 {
		t.Fatalf("timedOut=%v fetches=%d err=%v", timedOut, fetches, err)
	}
}

func TestPollTableCopyDoesNotAcceptResponseAfterDeadline(t *testing.T) {
	clock := &advancingTableCopyClock{now: time.Unix(0, 0)}
	status, timedOut, err := pollTableCopy(
		context.Background(),
		30*time.Second,
		clock,
		func(context.Context) (tableCopyStatus, error) {
			clock.now = clock.now.Add(28 * time.Second)
			return tableCopyStatus{TableID: "tbl_target", State: tableCopyStateSuccess}, nil
		},
	)
	if err != nil || !timedOut || status.State != "" {
		t.Fatalf("status=%#v timedOut=%v err=%v, want timed-out without accepting late response", status, timedOut, err)
	}
}

func TestPollTableCopyCapsSleepAtDeadline(t *testing.T) {
	clock := &advancingTableCopyClock{now: time.Unix(0, 0)}
	fetches := 0
	status, timedOut, err := pollTableCopy(
		context.Background(),
		10*time.Second,
		clock,
		func(context.Context) (tableCopyStatus, error) {
			fetches++
			return tableCopyStatus{TableID: "tbl_target", State: tableCopyStateProcess}, nil
		},
	)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if !timedOut || status.State != tableCopyStateProcess || fetches != 2 {
		t.Fatalf("status=%#v timedOut=%v fetches=%d", status, timedOut, fetches)
	}
	if !slices.Equal(clock.sleeps, []time.Duration{3 * time.Second, 6 * time.Second, time.Second}) {
		t.Fatalf("sleeps = %#v", clock.sleeps)
	}
}

func TestPollTableCopyRetriesTransientNetworkError(t *testing.T) {
	clock := &advancingTableCopyClock{now: time.Unix(0, 0)}
	fetches := 0
	status, timedOut, err := pollTableCopy(
		context.Background(),
		30*time.Second,
		clock,
		func(context.Context) (tableCopyStatus, error) {
			fetches++
			if fetches == 1 {
				return tableCopyStatus{}, errs.NewNetworkError(errs.SubtypeNetworkTimeout, "temporary timeout")
			}
			return tableCopyStatus{TableID: "tbl_target", State: tableCopyStateSuccess}, nil
		},
	)
	if err != nil || timedOut || status.State != tableCopyStateSuccess {
		t.Fatalf("status=%#v timedOut=%v err=%v", status, timedOut, err)
	}
	if fetches != 2 {
		t.Fatalf("fetches=%d, want 2", fetches)
	}
}

func TestPollTableCopyReturnsLastErrorWhenNoStatusSucceeded(t *testing.T) {
	clock := &advancingTableCopyClock{now: time.Unix(0, 0)}
	wantErr := errs.NewNetworkError(errs.SubtypeNetworkServer, "temporary server error")
	status, timedOut, err := pollTableCopy(
		context.Background(),
		4*time.Second,
		clock,
		func(context.Context) (tableCopyStatus, error) {
			return tableCopyStatus{}, wantErr
		},
	)
	if !errors.Is(err, wantErr) || timedOut || status.State != "" {
		t.Fatalf("status=%#v timedOut=%v err=%v, want last error", status, timedOut, err)
	}
}

func TestPollTableCopyDoesNotRetryTLSEvenWhenFlaggedRetryable(t *testing.T) {
	clock := &advancingTableCopyClock{now: time.Unix(0, 0)}
	wantErr := errs.NewNetworkError(errs.SubtypeNetworkTLS, "certificate failure").WithRetryable()
	fetches := 0
	_, timedOut, err := pollTableCopy(
		context.Background(),
		30*time.Second,
		clock,
		func(context.Context) (tableCopyStatus, error) {
			fetches++
			return tableCopyStatus{}, wantErr
		},
	)
	if !errors.Is(err, wantErr) || timedOut || fetches != 1 {
		t.Fatalf("timedOut=%v fetches=%d err=%v", timedOut, fetches, err)
	}
}

func TestTableCopySubmissionErrorMarksOutcomeUnknown(t *testing.T) {
	cause := errors.New("read timeout")
	original := errs.NewNetworkError(errs.SubtypeNetworkTimeout, "request failed").WithCause(cause).WithRetryable()
	err := tableCopySubmissionError(original)
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T %v, want typed error", err, err)
	}
	if problem.Subtype != errs.SubtypeNetworkTimeout || problem.Retryable {
		t.Fatalf("problem = %#v, want non-retryable network timeout", problem)
	}
	if !strings.Contains(problem.Message, "outcome is unknown") || !strings.Contains(problem.Hint, "Do not retry") {
		t.Fatalf("problem = %#v, want unknown-outcome guidance", problem)
	}
	if !errors.Is(err, cause) {
		t.Fatal("submission error must preserve its cause")
	}
}

func TestProjectTableCopySubmitRejectsOversizedTaskID(t *testing.T) {
	_, err := projectTableCopySubmit(map[string]interface{}{
		"table":   map[string]interface{}{"id": "tbl_target"},
		"task_id": strings.Repeat("x", tableCopyTaskIDMax+1),
		"state":   tableCopyStateInit,
	})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("error = %T %v, problem=%#v", err, err, problem)
	}
}

func TestProjectTableCopyStatusRejectsFailedWithoutInspectingLastErrorCode(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
	}{
		{name: "missing", data: map[string]interface{}{"table_id": "tbl_target", "state": "failed"}},
		{name: "positive", data: map[string]interface{}{"table_id": "tbl_target", "state": "failed", "last_error_code": 12345}},
		{name: "malformed", data: map[string]interface{}{"table_id": "tbl_target", "state": "failed", "last_error_code": "ignore-me"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := projectTableCopyStatus(test.data)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("error = %T %v, problem=%#v", err, err, problem)
			}
		})
	}
}

func TestTableCopyWaitErrorAddsContinuationWithoutReclassification(t *testing.T) {
	original := errs.NewNetworkError(errs.SubtypeNetworkServer, "status unavailable").WithCode(503)
	err := tableCopyWaitError(original)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkServer || problem.Code != 503 {
		t.Fatalf("problem = %#v", problem)
	}
	if !strings.Contains(problem.Hint, "+table-copy-status") {
		t.Fatalf("hint = %q, want continuation command", problem.Hint)
	}
	for _, sensitive := range []string{"ct1.token", "app_x", "--task-id"} {
		if strings.Contains(problem.Hint, sensitive) {
			t.Fatalf("hint = %q, must not contain %q", problem.Hint, sensitive)
		}
	}
}

func TestTableCopyWaitErrorClassifiesCancellation(t *testing.T) {
	err := tableCopyWaitError(context.Canceled)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("problem = %#v", problem)
	}
	if !errors.Is(err, context.Canceled) || !strings.Contains(problem.Hint, "+table-copy-status") {
		t.Fatalf("error=%v problem=%#v", err, problem)
	}
}

func TestTableCopyStatusErrorClassifiesTaskTokenCodes(t *testing.T) {
	upstream := errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid task").
		WithCode(800010109).
		WithHint("Submit a new copy request.")
	err := tableCopyStatusError(upstream)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want validation error", err, err)
	}
	if validationErr.Subtype != errs.SubtypeInvalidArgument || validationErr.Code != 800010109 || validationErr.Param != "--task-id" {
		t.Fatalf("problem = %#v param=%q", validationErr.Problem, validationErr.Param)
	}
	if validationErr.Hint != "Submit a new copy request." {
		t.Fatalf("hint = %q", validationErr.Hint)
	}
}

func TestTableCopyStatusErrorPreservesStatusUnavailableClassification(t *testing.T) {
	upstream := errs.NewAPIError(errs.SubtypeNotFound, "status unavailable").
		WithCode(800030110).
		WithHint("Submit a new copy request.")
	err := tableCopyStatusError(upstream)
	if err != upstream {
		t.Fatalf("error = %T %v, want original error", err, err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryAPI || problem.Subtype != errs.SubtypeNotFound || problem.Hint != "Submit a new copy request." {
		t.Fatalf("problem = %#v", problem)
	}
}
