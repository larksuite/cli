// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package clie2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLIStdinRegression_SuccessCases(t *testing.T) {
	setDryRunConfigEnv(t)

	tests := []struct {
		name       string
		req        Request
		assertions func(*testing.T, *Result)
	}{
		{
			name: "api reads params from stdin",
			req: Request{
				Args:  []string{"api", "GET", "/open-apis/test", "--params", "-", "--dry-run"},
				Stdin: []byte(`{"a":"1","b":"2"}` + "\n"),
			},
			assertions: func(t *testing.T, result *Result) {
				entry := firstDryRunRequest(t, result.Stdout)
				assert.Equal(t, "GET", entry["method"])
				assert.Equal(t, "/open-apis/test", entry["url"])
				assert.Equal(t, map[string]any{"a": "1", "b": "2"}, entry["params"])
			},
		},
		{
			name: "api reads data from stdin",
			req: Request{
				Args:  []string{"api", "POST", "/open-apis/test", "--data", "-", "--dry-run"},
				Stdin: []byte(`{"text":"hello"}` + "\n"),
			},
			assertions: func(t *testing.T, result *Result) {
				entry := firstDryRunRequest(t, result.Stdout)
				assert.Equal(t, "POST", entry["method"])
				assert.Equal(t, map[string]any{"text": "hello"}, entry["body"])
			},
		},
		{
			name: "api strips single quoted json",
			req: Request{
				Args: []string{"api", "GET", "/open-apis/test", "--params", `'{"a":"1"}'`, "--dry-run"},
			},
			assertions: func(t *testing.T, result *Result) {
				entry := firstDryRunRequest(t, result.Stdout)
				assert.Equal(t, map[string]any{"a": "1"}, entry["params"])
			},
		},
		{
			name: "service reads params from stdin",
			req: Request{
				Args: []string{
					"calendar", "events", "instance_view",
					"--as", "bot",
					"--params", "-",
					"--dry-run",
				},
				Stdin: []byte(`{"calendar_id":"primary","start_time":"1700000000","end_time":"1700003600"}` + "\n"),
			},
			assertions: func(t *testing.T, result *Result) {
				entry := firstDryRunRequest(t, result.Stdout)
				assert.Equal(t, "GET", entry["method"])
				assert.Equal(t, "/open-apis/calendar/v4/calendars/primary/events/instance_view", entry["url"])
				assert.Equal(t, map[string]any{
					"start_time": "1700000000",
					"end_time":   "1700003600",
				}, entry["params"])
			},
		},
		{
			name: "service reads data from stdin",
			req: Request{
				Args: []string{
					"task", "tasks", "create",
					"--as", "bot",
					"--data", "-",
					"--dry-run",
				},
				Stdin: []byte(`{"summary":"stdin regression"}` + "\n"),
			},
			assertions: func(t *testing.T, result *Result) {
				entry := firstDryRunRequest(t, result.Stdout)
				assert.Equal(t, "POST", entry["method"])
				assert.Equal(t, "/open-apis/task/v2/tasks", entry["url"])
				assert.Equal(t, map[string]any{"summary": "stdin regression"}, entry["body"])
			},
		},
		{
			name: "service strips single quoted json",
			req: Request{
				Args: []string{
					"task", "tasks", "create",
					"--as", "bot",
					"--data", `'{"summary":"single quote"}'`,
					"--dry-run",
				},
			},
			assertions: func(t *testing.T, result *Result) {
				entry := firstDryRunRequest(t, result.Stdout)
				assert.Equal(t, map[string]any{"summary": "single quote"}, entry["body"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RunCmd(context.Background(), tt.req)
			require.NoError(t, err)
			require.NoError(t, result.RunErr, "stderr:\n%s", result.Stderr)
			result.AssertExitCode(t, 0)
			tt.assertions(t, result)
		})
	}
}

func TestCLIStdinRegression_ErrorCases(t *testing.T) {
	setDryRunConfigEnv(t)

	tests := []struct {
		name        string
		req         Request
		wantMessage string
	}{
		{
			name: "api rejects empty stdin",
			req: Request{
				Args:  []string{"api", "GET", "/open-apis/test", "--params", "-", "--dry-run"},
				Stdin: []byte{},
			},
			wantMessage: "--params: stdin is empty (did you forget to pipe input?)",
		},
		{
			name: "api rejects double stdin",
			req: Request{
				Args:  []string{"api", "POST", "/open-apis/test", "--params", "-", "--data", "-", "--dry-run"},
				Stdin: []byte(`{"x":1}` + "\n"),
			},
			wantMessage: "--params and --data cannot both read from stdin (-)",
		},
		{
			name: "service rejects empty stdin",
			req: Request{
				Args: []string{
					"calendar", "events", "instance_view",
					"--as", "bot",
					"--params", "-",
					"--dry-run",
				},
				Stdin: []byte{},
			},
			wantMessage: "--params: stdin is empty (did you forget to pipe input?)",
		},
		{
			name: "service rejects double stdin",
			req: Request{
				Args: []string{
					"task", "tasks", "create",
					"--as", "bot",
					"--params", "-",
					"--data", "-",
					"--dry-run",
				},
				Stdin: []byte(`{"summary":"stdin regression"}` + "\n"),
			},
			wantMessage: "--params and --data cannot both read from stdin (-)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RunCmd(context.Background(), tt.req)
			require.NoError(t, err)
			assert.Error(t, result.RunErr)
			result.AssertExitCode(t, 2)

			envelope, ok := result.StderrJSON(t).(map[string]any)
			require.True(t, ok)
			assert.Equal(t, false, envelope["ok"])

			errDetail, ok := envelope["error"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "validation", errDetail["type"])
			assert.Equal(t, tt.wantMessage, errDetail["message"])
		})
	}
}

func setDryRunConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

func firstDryRunRequest(t *testing.T, stdout string) map[string]any {
	t.Helper()

	const prefix = "=== Dry Run ===\n"
	if !strings.HasPrefix(stdout, prefix) {
		t.Fatalf("expected dry-run prefix, got:\n%s", stdout)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(stdout, prefix)), &payload); err != nil {
		t.Fatalf("parse dry-run payload: %v\nstdout:\n%s", err, stdout)
	}

	apiEntries, ok := payload["api"].([]any)
	require.True(t, ok, "payload missing api array: %#v", payload)
	require.Len(t, apiEntries, 1)

	entry, ok := apiEntries[0].(map[string]any)
	require.True(t, ok, "api entry is not an object: %#v", apiEntries[0])
	return entry
}
