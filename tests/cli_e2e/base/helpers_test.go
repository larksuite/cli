// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const cleanupTimeout = 30 * time.Second

func baseJSONPayload(t *testing.T, result *clie2e.Result) string {
	t.Helper()

	raw := strings.TrimSpace(result.Stdout)
	if raw == "" {
		raw = strings.TrimSpace(result.Stderr)
	}

	start := strings.LastIndex(raw, "\n{")
	if start >= 0 {
		start++
	} else {
		start = strings.Index(raw, "{")
	}
	require.NotEqualf(t, -1, start, "json payload not found:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)

	payload := raw[start:]
	require.Truef(t, gjson.Valid(payload), "invalid json payload:\n%s", payload)
	return payload
}

func skipIfBaseUnavailable(t *testing.T, result *clie2e.Result, reason string) {
	t.Helper()

	payload := baseJSONPayload(t, result)
	errType := gjson.Get(payload, "error.type").String()
	if errType == "config" && !runningInCI() {
		t.Skipf("%s: %s", reason, gjson.Get(payload, "error.message").String())
	}
}

func runningInCI() bool {
	return os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != ""
}

func reportCleanupFailure(parentT *testing.T, prefix string, result *clie2e.Result, err error) {
	parentT.Helper()

	if err != nil {
		parentT.Errorf("%s: %v", prefix, err)
		return
	}
	if result == nil {
		parentT.Errorf("%s: nil result", prefix)
		return
	}
	if isCleanupSuppressedResult(result) {
		return
	}

	parentT.Errorf("%s failed: exit=%d stdout=%s stderr=%s", prefix, result.ExitCode, result.Stdout, result.Stderr)
}

func cleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), cleanupTimeout)
}

func isCleanupSuppressedResult(result *clie2e.Result) bool {
	if result == nil {
		return false
	}

	raw := strings.TrimSpace(result.Stdout)
	if raw == "" {
		raw = strings.TrimSpace(result.Stderr)
	}
	if raw == "" {
		return false
	}

	start := strings.LastIndex(raw, "\n{")
	if start >= 0 {
		start++
	} else {
		start = strings.Index(raw, "{")
	}
	if start < 0 {
		return false
	}

	payload := raw[start:]
	if !gjson.Valid(payload) {
		return false
	}

	if gjson.Get(payload, "error.type").String() != "api_error" {
		return false
	}

	if gjson.Get(payload, "error.detail.type").String() == "not_found" ||
		strings.Contains(strings.ToLower(gjson.Get(payload, "error.message").String()), "not found") {
		return true
	}

	return gjson.Get(payload, "error.code").Int() == 800004135 ||
		strings.Contains(strings.ToLower(gjson.Get(payload, "error.message").String()), " limited")
}

func testSuffix() string {
	return time.Now().UTC().Format("20060102-150405")
}

func createBase(t *testing.T, ctx context.Context, name string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+base-create", "--name", name, "--time-zone", "Asia/Shanghai"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 {
		skipIfBaseUnavailable(t, result, "requires bot base create capability")
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	baseToken := gjson.Get(result.Stdout, "data.base.app_token").String()
	if baseToken == "" {
		baseToken = gjson.Get(result.Stdout, "data.base.base_token").String()
	}
	require.NotEmpty(t, baseToken, "stdout:\n%s", result.Stdout)
	return baseToken
}

func copyBase(t *testing.T, ctx context.Context, baseToken string, name string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+base-copy", "--base-token", baseToken, "--name", name, "--without-content", "--time-zone", "Asia/Shanghai"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 {
		skipIfBaseUnavailable(t, result, "requires bot base copy capability")
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	copiedToken := gjson.Get(result.Stdout, "data.base.app_token").String()
	if copiedToken == "" {
		copiedToken = gjson.Get(result.Stdout, "data.base.base_token").String()
	}
	require.NotEmpty(t, copiedToken, "stdout:\n%s", result.Stdout)
	return copiedToken
}

func createTable(t *testing.T, parentT *testing.T, ctx context.Context, baseToken string, name string, fieldsJSON string, viewJSON string) (tableID string, primaryFieldID string, primaryViewID string) {
	t.Helper()

	args := []string{"base", "+table-create", "--base-token", baseToken, "--name", name}
	if fieldsJSON != "" {
		args = append(args, "--fields", fieldsJSON)
	}
	if viewJSON != "" {
		args = append(args, "--view", viewJSON)
	}

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      args,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 {
		skipIfBaseUnavailable(t, result, "requires bot table create capability")
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	tableID = gjson.Get(result.Stdout, "data.table.id").String()
	if tableID == "" {
		tableID = gjson.Get(result.Stdout, "data.table.table_id").String()
	}
	require.NotEmpty(t, tableID, "stdout:\n%s", result.Stdout)

	primaryFieldID = gjson.Get(result.Stdout, "data.fields.0.id").String()
	if primaryFieldID == "" {
		primaryFieldID = gjson.Get(result.Stdout, "data.fields.0.field_id").String()
	}

	primaryViewID = gjson.Get(result.Stdout, "data.views.0.id").String()
	if primaryViewID == "" {
		primaryViewID = gjson.Get(result.Stdout, "data.views.0.view_id").String()
	}

	parentT.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"base", "+table-delete", "--base-token", baseToken, "--table-id", tableID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			reportCleanupFailure(parentT, "delete table "+tableID, deleteResult, deleteErr)
		}
	})

	return tableID, primaryFieldID, primaryViewID
}

func createField(t *testing.T, parentT *testing.T, ctx context.Context, baseToken string, tableID string, body string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+field-create", "--base-token", baseToken, "--table-id", tableID, "--json", body},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 {
		skipIfBaseUnavailable(t, result, "requires bot field create capability")
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	fieldID := gjson.Get(result.Stdout, "data.field.id").String()
	if fieldID == "" {
		fieldID = gjson.Get(result.Stdout, "data.field.field_id").String()
	}
	require.NotEmpty(t, fieldID, "stdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"base", "+field-delete", "--base-token", baseToken, "--table-id", tableID, "--field-id", fieldID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			reportCleanupFailure(parentT, "delete field "+fieldID, deleteResult, deleteErr)
		}
	})

	return fieldID
}

func createRecord(t *testing.T, parentT *testing.T, ctx context.Context, baseToken string, tableID string, body string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+record-upsert", "--base-token", baseToken, "--table-id", tableID, "--json", body},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 {
		skipIfBaseUnavailable(t, result, "requires bot record create capability")
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	recordID := gjson.Get(result.Stdout, "data.record.record_id").String()
	if recordID == "" {
		recordID = gjson.Get(result.Stdout, "data.record.record_id_list.0").String()
	}
	require.NotEmpty(t, recordID, "stdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"base", "+record-delete", "--base-token", baseToken, "--table-id", tableID, "--record-id", recordID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			reportCleanupFailure(parentT, "delete record "+recordID, deleteResult, deleteErr)
		}
	})

	return recordID
}

func createView(t *testing.T, parentT *testing.T, ctx context.Context, baseToken string, tableID string, body string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+view-create", "--base-token", baseToken, "--table-id", tableID, "--json", body},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 {
		skipIfBaseUnavailable(t, result, "requires bot view create capability")
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	viewID := gjson.Get(result.Stdout, "data.views.0.id").String()
	if viewID == "" {
		viewID = gjson.Get(result.Stdout, "data.views.0.view_id").String()
	}
	require.NotEmpty(t, viewID, "stdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"base", "+view-delete", "--base-token", baseToken, "--table-id", tableID, "--view-id", viewID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			reportCleanupFailure(parentT, "delete view "+viewID, deleteResult, deleteErr)
		}
	})

	return viewID
}

func createDashboard(t *testing.T, parentT *testing.T, ctx context.Context, baseToken string, name string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+dashboard-create", "--base-token", baseToken, "--name", name},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 {
		skipIfBaseUnavailable(t, result, "requires bot dashboard create capability")
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	dashboardID := gjson.Get(result.Stdout, "data.dashboard.dashboard_id").String()
	require.NotEmpty(t, dashboardID, "stdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"base", "+dashboard-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			reportCleanupFailure(parentT, "delete dashboard "+dashboardID, deleteResult, deleteErr)
		}
	})

	return dashboardID
}

func createBlock(t *testing.T, parentT *testing.T, ctx context.Context, baseToken string, dashboardID string, name string, blockType string, dataConfig string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+dashboard-block-create", "--base-token", baseToken, "--dashboard-id", dashboardID, "--name", name, "--type", blockType, "--data-config", dataConfig},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 {
		skipIfBaseUnavailable(t, result, "requires bot dashboard block create capability")
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	blockID := gjson.Get(result.Stdout, "data.block.block_id").String()
	require.NotEmpty(t, blockID, "stdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"base", "+dashboard-block-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			reportCleanupFailure(parentT, "delete dashboard block "+blockID, deleteResult, deleteErr)
		}
	})

	return blockID
}

func createForm(t *testing.T, parentT *testing.T, ctx context.Context, baseToken string, tableID string, name string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+form-create", "--base-token", baseToken, "--table-id", tableID, "--name", name},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 {
		skipIfBaseUnavailable(t, result, "requires bot form create capability")
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	formID := gjson.Get(result.Stdout, "data.id").String()
	require.NotEmpty(t, formID, "stdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"base", "+form-delete", "--base-token", baseToken, "--table-id", tableID, "--form-id", formID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			reportCleanupFailure(parentT, "delete form "+formID, deleteResult, deleteErr)
		}
	})

	return formID
}

func createRole(t *testing.T, parentT *testing.T, ctx context.Context, baseToken string, body string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+role-create", "--base-token", baseToken, "--json", body},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 {
		skipIfBaseUnavailable(t, result, "requires bot role create capability")
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	roleID := gjson.Get(result.Stdout, "data.role_id").String()
	if roleID == "" {
		roleName := gjson.Get(body, "role_name").String()
		require.NotEmpty(t, roleName, "role_name is required to resolve role id from list")

		listResult, listErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+role-list", "--base-token", baseToken},
			DefaultAs: "bot",
		})
		require.NoError(t, listErr)
		if listResult.ExitCode != 0 {
			skipIfBaseUnavailable(t, listResult, "requires bot role list capability")
		}
		listResult.AssertExitCode(t, 0)
		listResult.AssertStdoutStatus(t, true)

		roleListPayload := gjson.Get(listResult.Stdout, "data.data").String()
		require.NotEmpty(t, roleListPayload, "stdout:\n%s", listResult.Stdout)
		require.True(t, gjson.Valid(roleListPayload), "stdout:\n%s", listResult.Stdout)

		for _, item := range gjson.Get(roleListPayload, "base_roles").Array() {
			rolePayload := item.String()
			if !gjson.Valid(rolePayload) {
				continue
			}
			if gjson.Get(rolePayload, "role_name").String() == roleName {
				roleID = gjson.Get(rolePayload, "role_id").String()
				break
			}
		}
	}
	require.NotEmpty(t, roleID, "stdout:\n%s", result.Stdout)

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

	return roleID
}

func createWorkflow(t *testing.T, ctx context.Context, baseToken string, body string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+workflow-create", "--base-token", baseToken, "--json", body},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 {
		skipIfBaseUnavailable(t, result, "requires bot workflow create capability")
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	workflowID := gjson.Get(result.Stdout, "data.workflow_id").String()
	require.NotEmpty(t, workflowID, "stdout:\n%s", result.Stdout)
	return workflowID
}

func writeTempAttachment(t *testing.T, content string) string {
	t.Helper()

	wd, err := os.Getwd()
	require.NoError(t, err)

	path := filepath.Join(wd, "attachment-"+testSuffix()+".txt")
	err = os.WriteFile(path, []byte(content), 0o644)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Remove(path)
	})
	return "./" + filepath.Base(path)
}
