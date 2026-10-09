// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

const (
	excelCompatSourceEnv     = "LARK_CLI_E2E_EXCEL_COMPAT_SOURCE"
	excelCompatOutputDirEnv  = "LARK_CLI_E2E_EXCEL_COMPAT_OUTPUT_DIR"
	excelCompatScreenshotEnv = "LARK_CLI_E2E_EXCEL_COMPAT_SCREENSHOT_COMMAND"
	excelCompatRendererEnv   = "LARK_CLI_E2E_EXCEL_COMPAT_RENDER_COMMAND"
	excelCompatProfileEnv    = "LARK_CLI_E2E_EXCEL_COMPAT_PROFILE"
	excelCompatHomeEnv       = "LARK_CLI_E2E_EXCEL_COMPAT_HOME"
	excelCompatConfigDirEnv  = "LARK_CLI_E2E_EXCEL_COMPAT_CONFIG_DIR"
)

// TestSheets_ExcelCompatRoundTripWorkflow executes:
// local XLSX -> Lark Sheet -> exported XLSX -> Lark Sheet.
//
// It is opt-in because it creates two temporary spreadsheets and requires an
// external real-sheet screenshot adapter. Offline source/exported renders and
// online imported/reimported screenshots must both emit stage/manifest.json.
func TestSheets_ExcelCompatRoundTripWorkflow(t *testing.T) {
	source := strings.TrimSpace(os.Getenv(excelCompatSourceEnv))
	if source == "" {
		t.Skipf("set %s to an absolute XLSX fixture path", excelCompatSourceEnv)
	}

	outputDir := strings.TrimSpace(os.Getenv(excelCompatOutputDirEnv))
	if outputDir == "" {
		outputDir = filepath.Join(t.TempDir(), "excel-compat")
	}
	screenshotCommand := parseExcelCompatCommandEnv(t, excelCompatScreenshotEnv)
	renderCommand := parseExcelCompatCommandEnv(t, excelCompatRendererEnv)
	binary, err := clie2e.ResolveBinaryPath(clie2e.Request{})
	require.NoError(t, err)
	repositoryRoot, err := findExcelCompatRepositoryRoot()
	require.NoError(t, err)
	require.NoError(t, validateExcelCompatRepositoryBinary(binary, repositoryRoot))
	profile := strings.TrimSpace(os.Getenv(excelCompatProfileEnv))
	cliEnvironment, err := excelCompatLiveCLIEnvironment(
		profile,
		t.TempDir(),
		t.TempDir(),
		os.Getenv,
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	t.Cleanup(cancel)
	result, err := runExcelCompatRoundTrip(ctx, excelCompatRoundTripOptions{
		CLI:               binary,
		Source:            source,
		OutputDir:         outputDir,
		Profile:           profile,
		As:                "bot",
		ScreenshotCommand: screenshotCommand,
		RenderCommand:     renderCommand,
		CLIEnvironment:    cliEnvironment,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.ImportedToken)
	require.NotEmpty(t, result.ReimportedToken)
	require.FileExists(t, result.ExportedFile)

	manifestPath := filepath.Join(outputDir, "roundtrip.json")
	data, err := json.MarshalIndent(result, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, append(data, '\n'), 0o600))
}

func parseExcelCompatCommandEnv(t *testing.T, name string) []string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Skipf("set %s to a JSON argv array", name)
	}
	var command []string
	require.NoError(t, json.Unmarshal([]byte(value), &command))
	require.NotEmpty(t, command)
	require.True(t, filepath.IsAbs(command[0]), "%s executable must be absolute", name)
	return command
}

func excelCompatLiveCLIEnvironment(
	profile string,
	temporaryHome string,
	temporaryConfig string,
	getenv func(string) string,
) (map[string]string, error) {
	if profile != "" {
		home := strings.TrimSpace(getenv(excelCompatHomeEnv))
		configDir := strings.TrimSpace(getenv(excelCompatConfigDirEnv))
		if !filepath.IsAbs(home) || !filepath.IsAbs(configDir) {
			return nil, fmt.Errorf(
				"profile %q requires absolute %s and %s",
				profile, excelCompatHomeEnv, excelCompatConfigDirEnv,
			)
		}
		for name, path := range map[string]string{
			excelCompatHomeEnv:      home,
			excelCompatConfigDirEnv: configDir,
		} {
			info, err := os.Stat(path)
			if err != nil || !info.IsDir() {
				return nil, fmt.Errorf("%s must point to an existing directory", name)
			}
		}
		return map[string]string{
			"HOME":                     home,
			"LARKSUITE_CLI_CONFIG_DIR": configDir,
		}, nil
	}

	appID := strings.TrimSpace(getenv("LARKSUITE_CLI_APP_ID"))
	if appID == "" {
		appID = strings.TrimSpace(getenv("TEST_BOT1_APP_ID"))
	}
	token := strings.TrimSpace(getenv("LARKSUITE_CLI_TENANT_ACCESS_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(getenv("TEST_TENANT_ACCESS_TOKEN"))
	}
	if appID == "" || token == "" {
		return nil, errors.New(
			"Excel compatibility live workflow requires explicit bot app ID and tenant access token; default lark-cli profiles are not consulted",
		)
	}
	return map[string]string{
		"HOME":                              temporaryHome,
		"LARKSUITE_CLI_CONFIG_DIR":          temporaryConfig,
		"LARKSUITE_CLI_APP_ID":              appID,
		"LARKSUITE_CLI_TENANT_ACCESS_TOKEN": token,
	}, nil
}
