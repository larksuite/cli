// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const (
	roleVisibilityTimeout = 15 * time.Second
	roleVisibilityPoll    = 500 * time.Millisecond
)

type roleReadProbe func(*clie2e.Result) (observed string, ready bool)

func TestBase_RoleWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-base-role-"+clie2e.GenerateSuffix())
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+advperm-enable", "--base-token", baseToken},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	roleName := "Reviewer-" + clie2e.GenerateSuffix()
	roleID := createRole(t, ctx, baseToken, `{"role_name":"`+roleName+`","role_type":"custom_role"}`)
	createAcknowledgedAt := time.Now()
	t.Logf(
		"role visibility write acknowledged: operation=create role_id=%s acknowledged_at=%s",
		roleID,
		createAcknowledgedAt.UTC().Format(time.RFC3339Nano),
	)

	parentT.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"base", "+role-delete", "--base-token", baseToken, "--role-id", roleID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			reportCleanupFailure(parentT, "delete role "+roleID, deleteResult, deleteErr)
		}
	})

	t.Run("list as bot", func(t *testing.T) {
		waitForRoleRead(t, ctx, "create-list", roleID, createAcknowledgedAt, clie2e.Request{
			Args:      []string{"base", "+role-list", "--base-token", baseToken},
			DefaultAs: "bot",
		}, func(result *clie2e.Result) (string, bool) {
			return observeRoleInList(result, roleID, roleName)
		})
	})

	t.Run("get role as bot", func(t *testing.T) {
		waitForRoleRead(t, ctx, "create-get", roleID, createAcknowledgedAt, clie2e.Request{
			Args:      []string{"base", "+role-get", "--base-token", baseToken, "--role-id", roleID},
			DefaultAs: "bot",
		}, func(result *clie2e.Result) (string, bool) {
			return observeRoleGet(result, roleID, "")
		})
	})

	t.Run("update role as bot", func(t *testing.T) {
		updatedRoleName := roleName + " Updated"
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args:      []string{"base", "+role-update", "--base-token", baseToken, "--role-id", roleID, "--json", `{"role_name":"` + updatedRoleName + `","role_type":"custom_role"}`, "--yes"},
			DefaultAs: "bot",
		}, clie2e.RetryOptions{Attempts: 1})
		require.NoError(t, err)
		require.NotNil(t, result, "role update returned no CLI result")
		require.Equal(t, 0, result.ExitCode, "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
		statusObserved, statusOK := observeSuccessfulEnvelope(result)
		require.True(t, statusOK, "role update was not acknowledged: %s\nstdout:\n%s", statusObserved, result.Stdout)
		updateAcknowledgedAt := time.Now()
		t.Logf(
			"role visibility write acknowledged: operation=update role_id=%s acknowledged_at=%s",
			roleID,
			updateAcknowledgedAt.UTC().Format(time.RFC3339Nano),
		)

		waitForRoleRead(t, ctx, "update-get", roleID, updateAcknowledgedAt, clie2e.Request{
			Args:      []string{"base", "+role-get", "--base-token", baseToken, "--role-id", roleID},
			DefaultAs: "bot",
		}, func(result *clie2e.Result) (string, bool) {
			return observeRoleGet(result, roleID, updatedRoleName)
		})
	})

}

func TestRoleReadObservations(t *testing.T) {
	t.Run("created role ID", func(t *testing.T) {
		tests := []struct {
			name   string
			stdout string
			want   string
		}{
			{
				name:   "direct envelope data",
				stdout: `{"ok":true,"data":{"role_id":"rol_direct"}}`,
				want:   "rol_direct",
			},
			{
				name:   "nested object data",
				stdout: `{"ok":true,"data":{"data":{"role_id":"rol_nested"}}}`,
				want:   "rol_nested",
			},
			{
				name:   "double encoded data",
				stdout: `{"ok":true,"data":{"data":"{\"role_id\":\"rol_encoded\"}"}}`,
				want:   "rol_encoded",
			},
			{
				name:   "double encoded nested data",
				stdout: `{"ok":true,"data":"{\"data\":{\"role_id\":\"rol_inner\"}}"}`,
				want:   "rol_inner",
			},
			{
				name:   "missing ID",
				stdout: `{"ok":true,"data":{"success":true}}`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				require.Equal(t, tt.want, createdRoleID(tt.stdout))
			})
		}
	})

	t.Run("envelope", func(t *testing.T) {
		tests := []struct {
			name          string
			result        *clie2e.Result
			wantObserved  string
			wantSucceeded bool
		}{
			{name: "nil result", wantObserved: "nil-result"},
			{
				name:         "nonzero exit",
				result:       &clie2e.Result{ExitCode: 3, Stdout: `{"ok":true}`},
				wantObserved: "exit-code-3",
			},
			{
				name:         "missing status",
				result:       &clie2e.Result{Stdout: `{}`},
				wantObserved: "envelope-status-missing",
			},
			{
				name:         "failed status",
				result:       &clie2e.Result{Stdout: `{"ok":false}`},
				wantObserved: "envelope-ok=false",
			},
			{
				name:          "successful status",
				result:        &clie2e.Result{Stdout: `{"ok":true}`},
				wantObserved:  "envelope-ok",
				wantSucceeded: true,
			},
			{
				name:         "failed code",
				result:       &clie2e.Result{Stdout: `{"code":7}`},
				wantObserved: "envelope-code-7",
			},
			{
				name:          "successful code",
				result:        &clie2e.Result{Stdout: `{"code":0}`},
				wantObserved:  "envelope-code-0",
				wantSucceeded: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				observed, succeeded := observeSuccessfulEnvelope(tt.result)
				require.Equal(t, tt.wantObserved, observed)
				require.Equal(t, tt.wantSucceeded, succeeded)
			})
		}
	})

	t.Run("list", func(t *testing.T) {
		tests := []struct {
			name         string
			payload      string
			wantObserved string
			wantReady    bool
		}{
			{
				name:         "invalid payload",
				payload:      "not-json",
				wantObserved: "data.data-invalid-json",
			},
			{
				name:         "role missing",
				payload:      `{"base_roles":[]}`,
				wantObserved: "missing",
			},
			{
				name:         "same ID has old name",
				payload:      `{"base_roles":[{"role_id":"role-1","role_name":"Old"}]}`,
				wantObserved: "Old",
			},
			{
				name:         "same ID has new name",
				payload:      `{"base_roles":[{"role_id":"role-1","role_name":"New"}]}`,
				wantObserved: "New",
				wantReady:    true,
			},
			{
				name:         "same name belongs to another ID",
				payload:      `{"base_roles":[{"role_id":"other","role_name":"New"}]}`,
				wantObserved: "missing",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := successfulRoleReadResult(tt.payload)
				observed, ready := observeRoleRead(result, func(result *clie2e.Result) (string, bool) {
					return observeRoleInList(result, "role-1", "New")
				})
				require.Equal(t, tt.wantObserved, observed)
				require.Equal(t, tt.wantReady, ready)
			})
		}
	})

	t.Run("get", func(t *testing.T) {
		tests := []struct {
			name             string
			payload          string
			expectedRoleName string
			wantObserved     string
			wantReady        bool
		}{
			{
				name:         "invalid payload",
				payload:      "not-json",
				wantObserved: "data.data-invalid-json",
			},
			{
				name:         "same ID missing name",
				payload:      `{"role_id":"role-1"}`,
				wantObserved: "role_name-missing",
			},
			{
				name:             "different ID has new name",
				payload:          `{"role_id":"other","role_name":"New"}`,
				expectedRoleName: "New",
				wantObserved:     "role_id=other role_name=New",
			},
			{
				name:             "same ID has old name",
				payload:          `{"role_id":"role-1","role_name":"Old"}`,
				expectedRoleName: "New",
				wantObserved:     "Old",
			},
			{
				name:             "same ID has new name",
				payload:          `{"role_id":"role-1","role_name":"New"}`,
				expectedRoleName: "New",
				wantObserved:     "New",
				wantReady:        true,
			},
			{
				name:         "same ID is enough without expected name",
				payload:      `{"role_id":"role-1","role_name":"Current"}`,
				wantObserved: "Current",
				wantReady:    true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := successfulRoleReadResult(tt.payload)
				observed, ready := observeRoleRead(result, func(result *clie2e.Result) (string, bool) {
					return observeRoleGet(result, "role-1", tt.expectedRoleName)
				})
				require.Equal(t, tt.wantObserved, observed)
				require.Equal(t, tt.wantReady, ready)
			})
		}
	})
}

func successfulRoleReadResult(payload string) *clie2e.Result {
	return &clie2e.Result{
		Stdout: fmt.Sprintf(`{"ok":true,"data":{"data":%q}}`, payload),
	}
}

func waitForRoleRead(
	t *testing.T,
	ctx context.Context,
	operation string,
	roleID string,
	acknowledgedAt time.Time,
	request clie2e.Request,
	probe roleReadProbe,
) *clie2e.Result {
	t.Helper()

	require.NotEmpty(t, operation, "role read operation is required")
	require.NotEmpty(t, roleID, "role ID is required")
	require.NotNil(t, probe, "role read probe is required")

	attempts := 0
	staleReads := 0
	lastObserved := "not-attempted"
	var lastResult *clie2e.Result

	pollCtx, cancel := context.WithTimeout(ctx, roleVisibilityTimeout)
	defer cancel()

	timeoutError := func() error {
		lastStdout := ""
		if lastResult != nil {
			lastStdout = lastResult.Stdout
		}
		return fmt.Errorf(
			"role visibility did not converge: operation=%s role_id=%s attempts=%d last_observed=%q\nlast stdout:\n%s",
			operation,
			roleID,
			attempts,
			lastObserved,
			lastStdout,
		)
	}

	err := clie2e.WaitForCondition(pollCtx, clie2e.WaitOptions{
		Timeout:      roleVisibilityTimeout,
		Interval:     roleVisibilityPoll,
		TimeoutError: timeoutError,
	}, func() (bool, error) {
		result, runErr := clie2e.RunCmdWithRetry(
			pollCtx,
			request,
			clie2e.RetryOptions{Attempts: 1},
		)
		if runErr != nil {
			return false, runErr
		}

		attempts++
		lastResult = result
		exitCode := -1
		if result != nil {
			exitCode = result.ExitCode
		}

		observed, ready := observeRoleRead(result, probe)
		lastObserved = observed
		if !ready {
			staleReads++
		}
		t.Logf(
			"role visibility observation: operation=%s role_id=%s attempt=%d since_ack=%s exit_code=%d observed=%q ready=%t",
			operation,
			roleID,
			attempts,
			time.Since(acknowledgedAt).Round(time.Millisecond),
			exitCode,
			observed,
			ready,
		)
		return ready, nil
	})
	if errors.Is(err, context.DeadlineExceeded) &&
		errors.Is(pollCtx.Err(), context.DeadlineExceeded) &&
		ctx.Err() == nil {
		err = timeoutError()
	}
	require.NoError(t, err)
	require.NotNil(t, lastResult, "role visibility converged without a CLI result")

	t.Logf(
		"role visibility converged: operation=%s role_id=%s attempts=%d stale_reads=%d converged_after=%s",
		operation,
		roleID,
		attempts,
		staleReads,
		time.Since(acknowledgedAt).Round(time.Millisecond),
	)
	return lastResult
}

func observeRoleRead(result *clie2e.Result, probe roleReadProbe) (string, bool) {
	statusObserved, statusOK := observeSuccessfulEnvelope(result)
	if !statusOK {
		return statusObserved, false
	}
	return probe(result)
}

func observeSuccessfulEnvelope(result *clie2e.Result) (string, bool) {
	if result == nil {
		return "nil-result", false
	}
	if result.ExitCode != 0 {
		return fmt.Sprintf("exit-code-%d", result.ExitCode), false
	}

	if okResult := gjson.Get(result.Stdout, "ok"); okResult.Exists() {
		if okResult.Bool() {
			return "envelope-ok", true
		}
		return "envelope-ok=false", false
	}
	if codeResult := gjson.Get(result.Stdout, "code"); codeResult.Exists() {
		if codeResult.Int() == 0 {
			return "envelope-code-0", true
		}
		return fmt.Sprintf("envelope-code-%d", codeResult.Int()), false
	}
	return "envelope-status-missing", false
}

func observeRoleInList(result *clie2e.Result, roleID string, expectedRoleName string) (string, bool) {
	roleListPayload := gjson.Get(result.Stdout, "data.data").String()
	if roleListPayload == "" {
		return "data.data-missing", false
	}
	if !gjson.Valid(roleListPayload) {
		return "data.data-invalid-json", false
	}

	for _, item := range gjson.Get(roleListPayload, "base_roles").Array() {
		if item.Get("role_id").String() != roleID {
			continue
		}
		roleName := item.Get("role_name").String()
		if roleName == "" {
			return "role_name-missing", false
		}
		return roleName, roleName == expectedRoleName
	}
	return "missing", false
}

func observeRoleGet(result *clie2e.Result, roleID string, expectedRoleName string) (string, bool) {
	rolePayload := gjson.Get(result.Stdout, "data.data").String()
	if rolePayload == "" {
		return "data.data-missing", false
	}
	if !gjson.Valid(rolePayload) {
		return "data.data-invalid-json", false
	}

	observedRoleID := gjson.Get(rolePayload, "role_id").String()
	observedRoleName := gjson.Get(rolePayload, "role_name").String()
	if observedRoleID != roleID {
		return fmt.Sprintf("role_id=%s role_name=%s", observedRoleID, observedRoleName), false
	}
	if observedRoleName == "" {
		return "role_name-missing", false
	}
	if expectedRoleName != "" && observedRoleName != expectedRoleName {
		return observedRoleName, false
	}
	return observedRoleName, true
}
