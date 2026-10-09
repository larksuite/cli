// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExcelCompatRoundTripRunsSerialStagesAndCleansResources(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.xlsx")
	writeMinimalExcelCompatXLSX(t, source)
	pngPath := filepath.Join(dir, "valid.png")
	writeMinimalExcelCompatPNG(t, pngPath)
	fake := filepath.Join(dir, "fake-lark-cli")
	logFile := filepath.Join(dir, "commands.log")
	require.NoError(t, os.WriteFile(fake, []byte(`#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$FAKE_LOG"
printf 'progress\n' >&2
case "$*" in
  *"+import"*"source.xlsx"*) echo '{"ok":true,"data":{"ready":false,"ticket":"ticket_source","type":"sheet"}}' ;;
  *"+task_result"*"ticket_source"*) echo '{"ok":true,"data":{"ready":true,"token":"sht_source","type":"sheet","url":"https://example.feishu.cn/sheets/sht_source"}}' ;;
  *"+task_result"*"ticket_export"*) echo '{"ok":true,"data":{"ready":true,"file_token":"box_exported","file_name":"exported.xlsx","type":"sheet"}}' ;;
  *"+export-download"*"box_exported"*) mkdir -p "$FAKE_EXPORT_DIR"; cp "$FAKE_SOURCE" "$FAKE_EXPORT_DIR/exported.xlsx"; echo "{\"ok\":true,\"data\":{\"saved_path\":\"$FAKE_EXPORT_DIR/exported.xlsx\"}}" ;;
  *"drive +export "*"sht_source"*) echo '{"ok":true,"data":{"ready":false,"failed":false,"timed_out":true,"ticket":"ticket_export"}}' ;;
  *"+import"*"exported.xlsx"*) echo '{"ok":true,"data":{"ready":true,"token":"sht_reimport","url":"https://example.feishu.cn/sheets/sht_reimport"}}' ;;
  *"+delete"*"sht_source"*) echo '{"ok":true,"data":{"deleted":true}}' ;;
  *"+delete"*"sht_reimport"*) echo '{"ok":true,"data":{"deleted":true}}' ;;
  *"metas/batch_query"*) echo '{"ok":true,"data":{"metas":[]}}' ;;
  *) echo "unexpected: $*" >&2; exit 2 ;;
esac
`), 0o700))
	screenshot := filepath.Join(dir, "fake-shot")
	require.NoError(t, os.WriteFile(screenshot, []byte(`#!/bin/sh
set -eu
printf 'shot %s %s %s\n' "$1" "$2" "$3" >> "$FAKE_LOG"
mkdir -p "$3/$2"
cp "$FAKE_PNG" "$3/$2/000_Data.png"
printf '{"stage":"%s","token":"%s","provenance":{"kind":"lark_sheet"},"screenshots":[{"sheet_index":0,"sheet_name":"Data","stage":"%s","file":"%s/000_Data.png"}]}\n' "$2" "$1" "$2" "$2" > "$3/$2/manifest.json"
`), 0o700))
	renderer := filepath.Join(dir, "fake-render")
	require.NoError(t, os.WriteFile(renderer, []byte(`#!/bin/sh
set -eu
printf 'render %s %s %s\n' "$1" "$2" "$3" >> "$FAKE_LOG"
mkdir -p "$3/$2"
cp "$FAKE_PNG" "$3/$2/000_Data.png"
printf '{"stage":"%s","workbook":"%s","provenance":{"kind":"synthetic_preview"},"screenshots":[{"sheet_index":0,"sheet_name":"Data","stage":"%s","file":"%s/000_Data.png"}]}\n' "$2" "$1" "$2" "$2" > "$3/$2/manifest.json"
`), 0o700))

	result, err := runExcelCompatRoundTrip(context.Background(), excelCompatRoundTripOptions{
		CLI:               fake,
		Source:            source,
		OutputDir:         dir,
		Profile:           "e2e",
		As:                "bot",
		ScreenshotCommand: []string{screenshot, "{token}", "{stage}", "{output_dir}"},
		RenderCommand:     []string{renderer, "{workbook}", "{stage}", "{output_dir}"},
		Environment: map[string]string{
			"FAKE_LOG":        logFile,
			"FAKE_EXPORT_DIR": dir,
			"FAKE_SOURCE":     source,
			"FAKE_PNG":        pngPath,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "sht_source", result.ImportedToken)
	require.Equal(t, "sht_reimport", result.ReimportedToken)
	require.Equal(t, filepath.Join(dir, "exported.xlsx"), result.ExportedFile)

	lines := strings.Split(strings.TrimSpace(string(mustReadFile(t, logFile))), "\n")
	require.Len(t, lines, 14)
	require.Contains(t, lines[0], "render "+source+" source")
	require.Contains(t, lines[1], "drive +import")
	require.Contains(t, lines[2], "drive +task_result")
	require.Contains(t, lines[3], "shot sht_source imported")
	require.Contains(t, lines[4], "drive +export")
	require.Contains(t, lines[5], "drive +task_result")
	require.Contains(t, lines[6], "drive +export-download")
	require.Contains(t, lines[7], "render "+filepath.Join(dir, "exported.xlsx")+" exported")
	require.Contains(t, lines[8], "drive +import")
	require.Contains(t, lines[9], "shot sht_reimport reimported")
	require.Contains(t, lines[10], "drive +delete")
	require.Contains(t, lines[11], "metas/batch_query")
	require.Contains(t, lines[12], "drive +delete")
	require.Contains(t, lines[13], "metas/batch_query")
}

func TestExcelCompatRoundTripRequiresScreenshotAdapter(t *testing.T) {
	_, err := runExcelCompatRoundTrip(context.Background(), excelCompatRoundTripOptions{
		CLI:       "/tmp/lark-cli",
		Source:    "/tmp/source.xlsx",
		OutputDir: t.TempDir(),
	})
	require.ErrorContains(t, err, "screenshot command is required")
}

func TestExcelCompatRoundTripRequiresOfflineRenderer(t *testing.T) {
	_, err := runExcelCompatRoundTrip(context.Background(), excelCompatRoundTripOptions{
		CLI:               "/tmp/lark-cli",
		Source:            "/tmp/source.xlsx",
		OutputDir:         t.TempDir(),
		ScreenshotCommand: []string{"/tmp/screenshot", "{token}", "{stage}", "{output_dir}"},
	})
	require.ErrorContains(t, err, "render command is required")
}

func TestWaitExcelCompatImportKeepsOriginalTicketAcrossPendingResponses(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "fake-lark-cli")
	counter := filepath.Join(dir, "counter")
	require.NoError(t, os.WriteFile(fake, []byte(`#!/bin/sh
set -eu
count=0
if [ -f "$FAKE_COUNTER" ]; then count=$(cat "$FAKE_COUNTER"); fi
count=$((count + 1))
printf '%s' "$count" > "$FAKE_COUNTER"
case "$count" in
  1) echo '{"ok":true,"data":{"ready":false,"failed":false}}' ;;
  2) echo '{"ok":true,"data":{"ready":true,"failed":false,"token":"sht_ready"}}' ;;
  *) exit 2 ;;
esac
`), 0o700))

	var initial excelCompatEnvelope
	ok := true
	initial.OK = &ok
	initial.Data.Ticket = "ticket_original"
	token, err := waitExcelCompatImport(context.Background(), excelCompatRoundTripOptions{
		CLI: fake,
		As:  "bot",
		Environment: map[string]string{
			"FAKE_COUNTER": counter,
		},
	}, initial)

	require.NoError(t, err)
	require.Equal(t, "sht_ready", token)
	require.Equal(t, "2", string(mustReadFile(t, counter)))
}

func TestLoadExcelCompatStageManifestRejectsSyntheticOnlineEvidence(t *testing.T) {
	dir := t.TempDir()
	writeExcelCompatManifest(t, dir, "imported", "synthetic_preview", "Data")

	_, err := loadExcelCompatStageManifest(dir, "imported", "sht_actual", "", true)

	require.ErrorContains(t, err, "real screenshot provenance")
}

func TestLoadExcelCompatStageManifestRejectsEmptyScreenshotList(t *testing.T) {
	dir := t.TempDir()
	stageDir := filepath.Join(dir, "source")
	require.NoError(t, os.MkdirAll(stageDir, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(stageDir, "manifest.json"),
		[]byte(`{"stage":"source","provenance":{"kind":"synthetic_preview"},"screenshots":[]}`),
		0o600,
	))

	_, err := loadExcelCompatStageManifest(
		dir, "source", "", filepath.Join(dir, "workbook.xlsx"), false,
	)

	require.ErrorContains(t, err, "no screenshots")
}

func TestValidateExcelCompatStageSetRejectsSheetMismatch(t *testing.T) {
	dir := t.TempDir()
	writeExcelCompatManifest(t, dir, "source", "synthetic_preview", "Data")
	writeExcelCompatManifest(t, dir, "imported", "lark_sheet", "Renamed")
	writeExcelCompatManifest(t, dir, "exported", "synthetic_preview", "Data")
	writeExcelCompatManifest(t, dir, "reimported", "lark_sheet", "Data")

	err := validateExcelCompatStageSet(
		dir,
		filepath.Join(dir, "workbook.xlsx"),
		"sht_actual",
		filepath.Join(dir, "workbook.xlsx"),
		"sht_actual",
	)

	require.ErrorContains(t, err, "workbook requires")
}

func TestExcelCompatCommandEnvOverridesInheritedValuesOnce(t *testing.T) {
	t.Setenv("EXCEL_COMPAT_OVERRIDE", "inherited")

	env, err := excelCompatCommandEnv(map[string]string{
		"EXCEL_COMPAT_OVERRIDE": "isolated",
	})

	require.NoError(t, err)
	var matches []string
	for _, value := range env {
		if strings.HasPrefix(value, "EXCEL_COMPAT_OVERRIDE=") {
			matches = append(matches, value)
		}
	}
	require.Equal(t, []string{"EXCEL_COMPAT_OVERRIDE=isolated"}, matches)
}

func TestExcelCompatCLICommandEnvStripsInheritedCredentialProviders(t *testing.T) {
	for key, value := range map[string]string{
		"LARKSUITE_CLI_APP_ID":                     "global-app",
		"LARKSUITE_CLI_APP_SECRET":                 "global-secret",
		"LARKSUITE_CLI_USER_ACCESS_TOKEN":          "global-uat",
		"LARKSUITE_CLI_TENANT_ACCESS_TOKEN":        "global-tat",
		"LARKSUITE_CLI_TENANT_ACCESS_TOKEN_SOURCE": "credential-store",
		"LARKSUITE_CLI_PROFILE":                    "global-profile",
		"LARKSUITE_CLI_AUTH_PROXY":                 "http://global-proxy",
		"LARKSUITE_CLI_PROXY_KEY":                  "global-proxy-key",
		"TEST_BOT1_APP_ID":                         "global-test-app",
		"TEST_TENANT_ACCESS_TOKEN":                 "global-test-tat",
		"HTTP_PROXY":                               "http://global-proxy",
		"LARKSUITE_CLI_PROXY_ENABLE":               "1",
		"LARKSUITE_CLI_PROXY_ADDRESS":              "global-proxy-address",
		"LARKSUITE_CLI_CA_PATH":                    "/global/ca.pem",
		"LARKSUITE_CLI_STRICT_MODE":                "user",
		"LARKSUITE_CLI_DATA_DIR":                   "/global/data",
	} {
		t.Setenv(key, value)
	}

	env, err := excelCompatCLICommandEnv(
		map[string]string{"FAKE_LOG": "/tmp/log"},
		map[string]string{
			"HOME":                              "/isolated/home",
			"LARKSUITE_CLI_CONFIG_DIR":          "/isolated/config",
			"LARKSUITE_CLI_APP_ID":              "selected-app",
			"LARKSUITE_CLI_TENANT_ACCESS_TOKEN": "selected-tat",
		},
	)

	require.NoError(t, err)
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, forbidden := range []string{
		"global-app", "global-secret", "global-uat", "global-tat",
		"credential-store", "global-profile", "global-proxy",
		"global-proxy-key", "global-test-app", "global-test-tat",
		"global-proxy-address", "/global/ca.pem", "/global/data",
	} {
		require.NotContains(t, joined, forbidden)
	}
	require.Contains(t, joined, "\nLARKSUITE_CLI_APP_ID=selected-app\n")
	require.Contains(t, joined, "\nLARKSUITE_CLI_TENANT_ACCESS_TOKEN=selected-tat\n")
	require.Contains(t, joined, "\nFAKE_LOG=/tmp/log\n")
}

func TestExcelCompatAdapterCommandEnvUsesMinimalBase(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_TENANT_ACCESS_TOKEN", "global-tat")
	t.Setenv("HTTPS_PROXY", "http://global-proxy")
	t.Setenv("PATH", "/safe/path")
	env, err := excelCompatAdapterCommandEnv(map[string]string{
		"FAKE_LOG": "/tmp/adapter.log",
	})
	require.NoError(t, err)
	joined := "\n" + strings.Join(env, "\n") + "\n"
	require.Contains(t, joined, "\nPATH=/safe/path\n")
	require.Contains(t, joined, "\nFAKE_LOG=/tmp/adapter.log\n")
	require.NotContains(t, joined, "global-tat")
	require.NotContains(t, joined, "global-proxy")
}

func TestValidateExcelCompatRepositoryBinaryRejectsGlobalFallback(t *testing.T) {
	repository := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(repository, "go.mod"),
		[]byte("module github.com/larksuite/cli\n"),
		0o600,
	))
	localBinary := filepath.Join(repository, "lark-cli")
	require.NoError(t, os.WriteFile(localBinary, []byte("local"), 0o700))
	globalBinary := filepath.Join(t.TempDir(), "lark-cli")
	require.NoError(t, os.WriteFile(globalBinary, []byte("global"), 0o700))

	err := validateExcelCompatRepositoryBinary(globalBinary, repository)

	require.ErrorContains(t, err, "repository-local")
	require.NoError(t, validateExcelCompatRepositoryBinary(localBinary, repository))

	symlink := filepath.Join(repository, "symlink-cli")
	require.NoError(t, os.Symlink(globalBinary, symlink))
	require.ErrorContains(t, validateExcelCompatRepositoryBinary(
		symlink,
		filepath.Dir(symlink),
	), "symlink")
}

func TestExcelCompatLiveCLIEnvironmentNeverFallsBackToDefaultProfile(t *testing.T) {
	values := map[string]string{}
	_, err := excelCompatLiveCLIEnvironment(
		"",
		t.TempDir(),
		t.TempDir(),
		func(name string) string { return values[name] },
	)
	require.ErrorContains(t, err, "explicit bot")

	values["LARKSUITE_CLI_APP_ID"] = "cli_test"
	values["LARKSUITE_CLI_TENANT_ACCESS_TOKEN"] = "tat_test"
	environment, err := excelCompatLiveCLIEnvironment(
		"",
		t.TempDir(),
		t.TempDir(),
		func(name string) string { return values[name] },
	)
	require.NoError(t, err)
	require.Equal(t, "cli_test", environment["LARKSUITE_CLI_APP_ID"])
	require.Equal(t, "tat_test", environment["LARKSUITE_CLI_TENANT_ACCESS_TOKEN"])
}

func TestExcelCompatLiveCLIEnvironmentRequiresIsolatedProfileDirs(t *testing.T) {
	_, err := excelCompatLiveCLIEnvironment(
		"boe",
		t.TempDir(),
		t.TempDir(),
		func(string) string { return "" },
	)
	require.ErrorContains(t, err, excelCompatHomeEnv)

	home := t.TempDir()
	configDir := t.TempDir()
	values := map[string]string{
		excelCompatHomeEnv:      home,
		excelCompatConfigDirEnv: configDir,
	}
	environment, err := excelCompatLiveCLIEnvironment(
		"boe",
		t.TempDir(),
		t.TempDir(),
		func(name string) string { return values[name] },
	)
	require.NoError(t, err)
	require.Equal(t, home, environment["HOME"])
	require.Equal(t, configDir, environment["LARKSUITE_CLI_CONFIG_DIR"])
}

func TestRunExcelCompatCLIRejectsLegacyOrContradictoryEnvelope(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "fake-lark-cli")
	require.NoError(t, os.WriteFile(fake, []byte(`#!/bin/sh
set -eu
echo '{"status":"success","ok":false,"data":{}}'
`), 0o700))

	_, err := runExcelCompatCLI(context.Background(), excelCompatRoundTripOptions{
		CLI: fake,
		As:  "bot",
	}, "whoami")

	require.ErrorContains(t, err, "ok=true")
}

func TestRunExcelCompatRenderClearsOldStageAndBindsWorkbook(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.xlsx")
	writeMinimalExcelCompatXLSX(t, source)
	stageDir := filepath.Join(dir, "source")
	require.NoError(t, os.MkdirAll(stageDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(stageDir, "stale.png"), []byte("stale"), 0o600))
	renderer := filepath.Join(dir, "renderer")
	require.NoError(t, os.WriteFile(renderer, []byte(`#!/bin/sh
set -eu
test ! -e "$3/$2/stale.png"
mkdir -p "$3/$2"
cp "$FAKE_PNG" "$3/$2/000.png"
printf '{"stage":"%s","workbook":"%s","provenance":{"kind":"synthetic_preview"},"screenshots":[{"sheet_index":0,"sheet_name":"Data","stage":"%s","file":"%s/000.png"}]}' "$2" "$1" "$2" "$2" > "$3/$2/manifest.json"
`), 0o700))

	err := runExcelCompatRender(context.Background(), excelCompatRoundTripOptions{
		OutputDir:     dir,
		RenderCommand: []string{renderer, "{workbook}", "{stage}", "{output_dir}"},
		Environment:   map[string]string{"FAKE_PNG": func() string { p := filepath.Join(dir, "valid.png"); writeMinimalExcelCompatPNG(t, p); return p }()},
	}, source, "source")

	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(stageDir, "stale.png"))
}

func TestResetExcelCompatOwnedArtifactsClearsPriorRunEvidence(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"source/old.png",
		"imported/old.png",
		"exported/old.png",
		"reimported/old.png",
		"exported.xlsx",
		"roundtrip.json",
	} {
		target := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o700))
		require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))
	}
	unowned := filepath.Join(dir, "keep.txt")
	require.NoError(t, os.WriteFile(unowned, []byte("keep"), 0o600))

	require.NoError(t, resetExcelCompatOwnedArtifacts(dir))

	for _, name := range []string{
		"source", "imported", "exported", "reimported",
		"exported.xlsx", "roundtrip.json",
	} {
		_, err := os.Stat(filepath.Join(dir, name))
		require.Error(t, err)
		require.True(t, os.IsNotExist(err))
	}
	require.FileExists(t, unowned)
}

func TestLoadExcelCompatStageManifestRejectsWrongSourceBinding(t *testing.T) {
	dir := t.TempDir()
	writeExcelCompatManifest(t, dir, "imported", "lark_sheet", "Data")

	_, err := loadExcelCompatStageManifest(
		dir, "imported", "sht_expected", "", true,
	)

	require.ErrorContains(t, err, "token")
}

func TestLoadExcelCompatStageManifestRejectsSymlinkAndDuplicateImage(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	dir := t.TempDir()
	stageDir := filepath.Join(dir, "source")
	require.NoError(t, os.MkdirAll(stageDir, 0o700))
	writeMinimalExcelCompatXLSX(t, filepath.Join(dir, "source.xlsx"))
	outside := filepath.Join(dir, "outside.png")
	require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(stageDir, "linked.png")))
	manifest := map[string]interface{}{
		"stage":    "source",
		"workbook": filepath.Join(dir, "source.xlsx"),
		"provenance": map[string]interface{}{
			"kind": "synthetic_preview",
		},
		"screenshots": []map[string]interface{}{
			{"sheet_index": 0, "sheet_name": "Data", "stage": "source", "file": "source/linked.png"},
		},
	}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(stageDir, "manifest.json"), data, 0o600))

	_, err = loadExcelCompatStageManifest(
		dir, "source", "", filepath.Join(dir, "source.xlsx"), false,
	)
	require.ErrorContains(t, err, "symlink")

	require.NoError(t, os.Remove(filepath.Join(stageDir, "linked.png")))
	writeMinimalExcelCompatPNG(t, filepath.Join(stageDir, "shared.png"))
	manifest["screenshots"] = []map[string]interface{}{
		{"sheet_index": 0, "sheet_name": "Data", "stage": "source", "file": "source/shared.png"},
		{"sheet_index": 1, "sheet_name": "Chart", "stage": "source", "file": "source/shared.png"},
	}
	data, err = json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(stageDir, "manifest.json"), data, 0o600))

	_, err = loadExcelCompatStageManifest(
		dir, "source", "", filepath.Join(dir, "source.xlsx"), false,
	)
	require.ErrorContains(t, err, "duplicate screenshot file")
}

func TestLoadExcelCompatStageManifestRejectsInvalidPNG(t *testing.T) {
	dir := t.TempDir()
	writeExcelCompatManifest(t, dir, "source", "synthetic_preview", "Data")
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "source", "000.png"),
		[]byte("not a png"),
		0o600,
	))

	_, err := loadExcelCompatStageManifest(
		dir, "source", "", filepath.Join(dir, "workbook.xlsx"), false,
	)

	require.ErrorContains(t, err, "not a valid PNG")
}

func TestValidateExcelCompatManifestSheetsRejectsMissingWorkbookSheet(t *testing.T) {
	var manifest excelCompatStageManifest
	manifest.Stage = "source"
	manifest.Screenshots = append(manifest.Screenshots, struct {
		SheetIndex int    `json:"sheet_index"`
		SheetName  string `json:"sheet_name"`
		Stage      string `json:"stage"`
		File       string `json:"file"`
	}{
		SheetIndex: 0,
		SheetName:  "Data",
		Stage:      "source",
		File:       "source/000.png",
	})

	err := validateExcelCompatManifestSheets("source", manifest, []excelCompatSheetIdentity{
		{Index: 0, Name: "Data"},
		{Index: 1, Name: "Chart"},
	})

	require.ErrorContains(t, err, "workbook has 2")
}

func TestValidateExcelCompatSourceRejectsFakeXLSX(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake.xlsx")
	require.NoError(t, os.WriteFile(path, []byte("not a zip"), 0o600))

	require.Error(t, validateExcelCompatSource(path))
}

func TestExcelCompatWorkbookSheetsAcceptsStrictOOXML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strict.xlsx")
	writeMinimalExcelCompatXLSXWithNamespaces(
		t,
		path,
		excelCompatSpreadsheetStrictNS,
		excelCompatRelationshipsStrict,
	)

	sheets, err := excelCompatWorkbookSheets(path)

	require.NoError(t, err)
	require.Equal(t, []excelCompatSheetIdentity{{Index: 0, Name: "Data"}}, sheets)
}

func TestLoadExcelCompatStageManifestRejectsTruncatedPNG(t *testing.T) {
	dir := t.TempDir()
	writeExcelCompatManifest(t, dir, "source", "synthetic_preview", "Data")
	imagePath := filepath.Join(dir, "source", "000.png")
	data := mustReadFile(t, imagePath)
	idat := bytes.Index(data, []byte("IDAT"))
	require.Greater(t, idat, 12)
	require.NoError(t, os.WriteFile(imagePath, data[:idat-4], 0o600))

	_, err := loadExcelCompatStageManifest(
		dir, "source", "", filepath.Join(dir, "workbook.xlsx"), false,
	)

	require.ErrorContains(t, err, "truncated or corrupt")
}

func TestCleanupExcelCompatResourcesRecoversTokenFromTicket(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "fake-lark-cli")
	logFile := filepath.Join(dir, "commands.log")
	require.NoError(t, os.WriteFile(fake, []byte(`#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$FAKE_LOG"
case "$*" in
  *"+task_result"*"ticket_late"*) echo '{"ok":true,"data":{"ready":true,"token":"sht_late"}}' ;;
  *"+delete"*"sht_late"*) echo '{"ok":true,"data":{"deleted":true}}' ;;
  *"metas/batch_query"*) echo '{"ok":true,"data":{"metas":[]}}' ;;
  *) exit 2 ;;
esac
`), 0o700))

	err := cleanupExcelCompatResources(context.Background(), excelCompatRoundTripOptions{
		CLI: fake,
		As:  "bot",
		Environment: map[string]string{
			"FAKE_LOG": logFile,
		},
	}, []excelCompatImportResource{{ticket: "ticket_late"}})

	require.NoError(t, err)
	log := string(mustReadFile(t, logFile))
	require.Contains(t, log, "+task_result")
	require.Contains(t, log, "+delete")
	require.Contains(t, log, "metas/batch_query")
}

func TestIsExcelCompatSheetDeletedRequestsURLAndValidatesIdentity(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "fake-lark-cli")
	require.NoError(t, os.WriteFile(fake, []byte(`#!/bin/sh
set -eu
case "$*" in
  *"metas/batch_query"*"\"with_url\":true"*)
    echo '{"ok":true,"data":{"metas":[{"doc_token":"sht_live","doc_type":"sheet","url":"https://example.test/sheets/sht_live"}]}}'
    ;;
  *) echo "missing with_url: $*" >&2; exit 2 ;;
esac
`), 0o700))

	deleted, err := isExcelCompatSheetDeleted(context.Background(), excelCompatRoundTripOptions{
		CLI: fake,
		As:  "bot",
	}, "sht_live")
	require.NoError(t, err)
	require.False(t, deleted)

	require.NoError(t, os.WriteFile(fake, []byte(`#!/bin/sh
set -eu
echo '{"ok":true,"data":{"metas":[{"doc_token":"other","doc_type":"sheet","url":"https://example.test/sheets/other"}]}}'
`), 0o700))
	_, err = isExcelCompatSheetDeleted(context.Background(), excelCompatRoundTripOptions{
		CLI: fake,
		As:  "bot",
	}, "sht_live")
	require.ErrorContains(t, err, "identity mismatch")
}

func writeExcelCompatManifest(
	t *testing.T,
	outputDir string,
	stage string,
	provenance string,
	sheetName string,
) {
	t.Helper()
	stageDir := filepath.Join(outputDir, stage)
	require.NoError(t, os.MkdirAll(stageDir, 0o700))
	workbookPath := filepath.Join(outputDir, "workbook.xlsx")
	if _, err := os.Stat(workbookPath); os.IsNotExist(err) {
		writeMinimalExcelCompatXLSX(t, workbookPath)
	}
	imagePath := filepath.Join(stageDir, "000.png")
	writeMinimalExcelCompatPNG(t, imagePath)
	manifest := map[string]interface{}{
		"stage":    stage,
		"token":    "sht_actual",
		"workbook": workbookPath,
		"provenance": map[string]interface{}{
			"kind": provenance,
		},
		"screenshots": []map[string]interface{}{{
			"sheet_index": 0,
			"sheet_name":  sheetName,
			"stage":       stage,
			"file":        filepath.ToSlash(filepath.Join(stage, "000.png")),
		}},
	}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(stageDir, "manifest.json"), data, 0o600))
}

func writeMinimalExcelCompatXLSX(t *testing.T, path string) {
	writeMinimalExcelCompatXLSXWithNamespaces(
		t,
		path,
		excelCompatSpreadsheetTransitionalNS,
		excelCompatRelationshipsTransitional,
	)
}

func writeMinimalExcelCompatXLSXWithNamespaces(
	t *testing.T,
	path string,
	spreadsheetNamespace string,
	relationshipNamespace string,
) {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	parts := map[string]string{
		"[Content_Types].xml":        `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
		"_rels/.rels":                `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            fmt.Sprintf(`<?xml version="1.0"?><workbook xmlns="%s" xmlns:r="%s"><sheets><sheet name="Data" sheetId="1" r:id="rId1"/></sheets></workbook>`, spreadsheetNamespace, relationshipNamespace),
		"xl/_rels/workbook.xml.rels": fmt.Sprintf(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="%s/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`, relationshipNamespace),
		"xl/worksheets/sheet1.xml":   fmt.Sprintf(`<?xml version="1.0"?><worksheet xmlns="%s"><sheetData/></worksheet>`, spreadsheetNamespace),
	}
	for name, content := range parts {
		part, err := writer.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(part, content)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(path, buffer.Bytes(), 0o600))
}

func writeMinimalExcelCompatPNG(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	require.NoError(t, err)
	canvas := image.NewRGBA(image.Rect(0, 0, 1, 1))
	canvas.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	require.NoError(t, png.Encode(file, canvas))
	require.NoError(t, file.Close())
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
