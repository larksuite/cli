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
	switch errType {
	case "config", "missing_scope", "permission", "auth", "auth_error", "security_policy":
		t.Skipf("%s: %s", reason, gjson.Get(payload, "error.message").String())
	}
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
		deleteResult, deleteErr := clie2e.RunCmd(context.Background(), clie2e.Request{
			Args:      []string{"base", "+table-delete", "--base-token", baseToken, "--table-id", tableID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			parentT.Logf("best-effort table cleanup skipped: table=%s err=%v stdout=%s stderr=%s", tableID, deleteErr, deleteResult.Stdout, deleteResult.Stderr)
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
		deleteResult, deleteErr := clie2e.RunCmd(context.Background(), clie2e.Request{
			Args:      []string{"base", "+field-delete", "--base-token", baseToken, "--table-id", tableID, "--field-id", fieldID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			parentT.Logf("best-effort field cleanup skipped: field=%s err=%v stdout=%s stderr=%s", fieldID, deleteErr, deleteResult.Stdout, deleteResult.Stderr)
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
	require.NotEmpty(t, recordID, "stdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		deleteResult, deleteErr := clie2e.RunCmd(context.Background(), clie2e.Request{
			Args:      []string{"base", "+record-delete", "--base-token", baseToken, "--table-id", tableID, "--record-id", recordID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			parentT.Logf("best-effort record cleanup skipped: record=%s err=%v stdout=%s stderr=%s", recordID, deleteErr, deleteResult.Stdout, deleteResult.Stderr)
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
		deleteResult, deleteErr := clie2e.RunCmd(context.Background(), clie2e.Request{
			Args:      []string{"base", "+view-delete", "--base-token", baseToken, "--table-id", tableID, "--view-id", viewID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			parentT.Logf("best-effort view cleanup skipped: view=%s err=%v stdout=%s stderr=%s", viewID, deleteErr, deleteResult.Stdout, deleteResult.Stderr)
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
		deleteResult, deleteErr := clie2e.RunCmd(context.Background(), clie2e.Request{
			Args:      []string{"base", "+dashboard-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			parentT.Logf("best-effort dashboard cleanup skipped: dashboard=%s err=%v stdout=%s stderr=%s", dashboardID, deleteErr, deleteResult.Stdout, deleteResult.Stderr)
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
		deleteResult, deleteErr := clie2e.RunCmd(context.Background(), clie2e.Request{
			Args:      []string{"base", "+dashboard-block-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			parentT.Logf("best-effort block cleanup skipped: block=%s err=%v stdout=%s stderr=%s", blockID, deleteErr, deleteResult.Stdout, deleteResult.Stderr)
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
		deleteResult, deleteErr := clie2e.RunCmd(context.Background(), clie2e.Request{
			Args:      []string{"base", "+form-delete", "--base-token", baseToken, "--table-id", tableID, "--form-id", formID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			parentT.Logf("best-effort form cleanup skipped: form=%s err=%v stdout=%s stderr=%s", formID, deleteErr, deleteResult.Stdout, deleteResult.Stderr)
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
	require.NotEmpty(t, roleID, "stdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		deleteResult, deleteErr := clie2e.RunCmd(context.Background(), clie2e.Request{
			Args:      []string{"base", "+role-delete", "--base-token", baseToken, "--role-id", roleID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			parentT.Logf("best-effort role cleanup skipped: role=%s err=%v stdout=%s stderr=%s", roleID, deleteErr, deleteResult.Stdout, deleteResult.Stderr)
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

	path := filepath.Join(t.TempDir(), "attachment.txt")
	err := os.WriteFile(path, []byte(content), 0o644)
	require.NoError(t, err)
	return path
}

func uniqueName(prefix string) string {
	return prefix + "-" + time.Now().UTC().Format("20060102-150405")
}
