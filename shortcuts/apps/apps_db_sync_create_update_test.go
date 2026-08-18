// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

const (
	dbSyncURL       = "/open-apis/spark/v1/apps/app_x/db/sync_create"
	dbSyncUpdateURL = "/open-apis/spark/v1/apps/app_x/db/sync_update"
)

const dbSyncPreviewConfig = `{
  "mode": "batch",
  "source": {"type": "base", "base_url": "https://base.example/base/CDfkbxg2Ra8aJusw53ib3bpWcMh?table=tblDhz3b9tOrrgMt", "table": {"name": "订单"}},
  "target": {"type": "postgresql", "table": {"name": "orders", "action": "create"}},
  "schema_only": true
}`

const dbSyncCommitConfig = `{
  "mode": "streaming",
  "source": {"type": "base", "base_url": "https://base.example/base/CDfkbxg2Ra8aJusw53ib3bpWcMh?table=tblDhz3b9tOrrgMt", "table": {"name": "订单"}},
  "target": {"type": "postgresql", "table": {"name": "orders", "action": "use_existing"}},
  "field_maps": [
    {"source_field": "Base 表记录 ID", "target_field": "record_id", "enabled": true}
  ]
}`

const dbSyncNoFieldMapsConfig = `{
  "mode": "streaming",
  "source": {"type": "base", "base_url": "https://base.example/base/CDfkbxg2Ra8aJusw53ib3bpWcMh?table=tblDhz3b9tOrrgMt", "table": {"name": "订单"}},
  "target": {"type": "postgresql", "table": {"name": "orders", "action": "use_existing"}}
}`

// base_url has only domain+token (no ?table=) and source.table.name is empty →
// the source table cannot be identified. +db-sync-create must reject this locally.
// field_maps present so commit mode reaches the source-table check (not blocked
// earlier on the field_maps requirement).
const dbSyncNoTableNoNameConfig = `{
  "mode": "streaming",
  "source": {"type": "base", "base_url": "https://base.example/base/CDfkbxg2Ra8aJusw53ib3bpWcMh", "table": {"name": ""}},
  "target": {"type": "postgresql", "table": {"name": "orders", "action": "create"}},
  "field_maps": [{"source_field": "Base 表记录 ID", "target_field": "record_id", "enabled": true}]
}`

// base_url carries ?table=, source.table.name empty → identifiable via url table param.
const dbSyncUrlTableNoNameConfig = `{
  "mode": "batch",
  "source": {"type": "base", "base_url": "https://base.example/base/CDfkbxg2Ra8aJusw53ib3bpWcMh?table=tblDhz3b9tOrrgMt", "table": {"name": ""}},
  "target": {"type": "postgresql", "table": {"name": "orders", "action": "create"}},
  "schema_only": true
}`

// base_url has no ?table= but source.table.name is set → identifiable via name.
const dbSyncNoTableWithNameConfig = `{
  "mode": "batch",
  "source": {"type": "base", "base_url": "https://base.example/base/CDfkbxg2Ra8aJusw53ib3bpWcMh", "table": {"name": "订单"}},
  "target": {"type": "postgresql", "table": {"name": "orders", "action": "create"}},
  "schema_only": true
}`

func TestAppsDBSyncCreatePreviewDryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBSyncCreate, []string{
		"+db-sync-create", "--app-id", "app_x", "--config", dbSyncPreviewConfig,
		"--preview", "--environment", "dev", "--dry-run", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}

	a := dbSyncFirstDryRunAPI(t, stdout.String())
	if a.Method != "POST" || a.URL != dbSyncURL {
		t.Fatalf("dry-run = %s %s", a.Method, a.URL)
	}
	if a.Body["env"] != "dev" {
		t.Fatalf("dry-run body.env = %v", a.Body["env"])
	}
	if _, ok := a.Params["env"]; ok {
		t.Fatalf("env must be in body, not query params: %v", a.Params)
	}
	if _, ok := a.Params["preview"]; ok {
		t.Fatalf("preview must be in body, not query params: %v", a.Params)
	}
	if a.Body["preview"] != true {
		t.Fatalf("dry-run body = %v", a.Body)
	}
	config := a.Body["config"].(map[string]interface{})
	if config["mode"] != "batch" {
		t.Fatalf("dry-run body.config = %v", config)
	}
	target := config["target"].(map[string]interface{})
	table := target["table"].(map[string]interface{})
	if table["action"] != "create" {
		t.Fatalf("target.table.action = %v", table["action"])
	}
}

func TestAppsDBSyncCreateCommitDryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBSyncCreate, []string{
		"+db-sync-create", "--app-id", "app_x", "--config", dbSyncCommitConfig,
		"--environment", "online", "--dry-run", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}

	a := dbSyncFirstDryRunAPI(t, stdout.String())
	if a.Method != "POST" || a.URL != dbSyncURL {
		t.Fatalf("dry-run = %s %s", a.Method, a.URL)
	}
	if a.Body["env"] != "online" {
		t.Fatalf("env missing from dry-run body: %v", a.Body["env"])
	}
	if _, ok := a.Params["env"]; ok {
		t.Fatalf("env must be in body, not query params: %v", a.Params)
	}
	if _, ok := a.Params["preview"]; ok {
		t.Fatalf("commit dry-run must not include preview param: %v", a.Params)
	}
	if a.Body["preview"] != false {
		t.Fatalf("dry-run body = %v", a.Body)
	}
	config := a.Body["config"].(map[string]interface{})
	if config["mode"] != "streaming" || config["field_maps"] == nil {
		t.Fatalf("dry-run body.config = %v", config)
	}
}

func TestAppsDBSyncCreateCommitWithoutFieldMapsDryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBSyncCreate, []string{
		"+db-sync-create", "--app-id", "app_x", "--config", dbSyncNoFieldMapsConfig,
		"--environment", "dev", "--dry-run", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}

	api := dbSyncFirstDryRunAPI(t, stdout.String())
	if api.Body["preview"] != false {
		t.Fatalf("dry-run body = %v", api.Body)
	}
	config := api.Body["config"].(map[string]interface{})
	if _, ok := config["field_maps"]; ok {
		t.Fatalf("CLI must not inject field_maps when omitted: %v", config)
	}
}

func TestAppsDBSyncCreatePreviewExecuteWritesConfigOnly(t *testing.T) {
	chdirTemp(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST", URL: dbSyncURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"config": map[string]interface{}{
				"mode": "batch",
				"target": map[string]interface{}{"table": map[string]interface{}{
					"name": "orders", "action": "create",
				}},
				"field_maps": []map[string]interface{}{{"source_field": "Name", "target_field": "name"}},
			},
			"syncable_source_fields": []map[string]interface{}{{"name": "Name"}},
			"summary": map[string]interface{}{
				"syncable_source_field_count": 1,
				"mapped_field_count":          1,
				"estimated_record_count":      42,
			},
		}},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsDBSyncCreate, []string{
		"+db-sync-create", "--app-id", "app_x", "--config", dbSyncPreviewConfig,
		"--preview", "--output", "resolved.json", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var req map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &req); err != nil {
		t.Fatalf("decode captured request body: %v", err)
	}
	if req["preview"] != true {
		t.Fatalf("captured request body = %v", req)
	}
	config := req["config"].(map[string]interface{})
	if config["mode"] != "batch" {
		t.Fatalf("captured request body.config = %v", config)
	}

	raw, err := os.ReadFile("resolved.json")
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var saved map[string]interface{}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("decode saved config: %v", err)
	}
	if saved["mode"] != "batch" || saved["summary"] != nil || saved["syncable_source_fields"] != nil {
		t.Fatalf("saved file must contain only data.config, got %v", saved)
	}
	data := dbSyncEnvelopeData(t, stdout.String())
	if data["config"] == nil || data["summary"] == nil {
		t.Fatalf("stdout should still contain preview data envelope: %v", data)
	}
}

func TestAppsDBSyncCreatePreviewRejectsMissingConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		data map[string]interface{}
	}{
		{"no config key", map[string]interface{}{}},
		{"config not an object", map[string]interface{}{"config": "oops"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chdirTemp(t)
			factory, stdout, reg := newAppsExecuteFactory(t)
			reg.Register(&httpmock.Stub{
				Method: "POST", URL: dbSyncURL,
				Body: map[string]interface{}{"code": 0, "data": tc.data},
			})

			err := runAppsShortcut(t, AppsDBSyncCreate, []string{
				"+db-sync-create", "--app-id", "app_x", "--config", dbSyncPreviewConfig,
				"--preview", "--output", "resolved.json", "--as", "user",
			}, factory, stdout)

			var ie *errs.InternalError
			if !errors.As(err, &ie) {
				t.Fatalf("err = %T %v, want internal invalid_response", err, err)
			}
			if ie.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("subtype = %q, want %q", ie.Subtype, errs.SubtypeInvalidResponse)
			}
			if _, statErr := os.Stat("resolved.json"); !os.IsNotExist(statErr) {
				t.Fatalf("must not write output file on invalid config, stat err = %v", statErr)
			}
		})
	}
}

func TestAppsDBSyncCreateCommitExecuteOutputsTask(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST", URL: dbSyncURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"task_id": "streaming_101", "mode": "streaming", "status": "active",
		}},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsDBSyncCreate, []string{
		"+db-sync-create", "--app-id", "app_x", "--config", dbSyncCommitConfig, "--yes", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	var req map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &req); err != nil {
		t.Fatalf("decode captured request body: %v", err)
	}
	if req["preview"] != false {
		t.Fatalf("captured request body = %v", req)
	}
	config := req["config"].(map[string]interface{})
	if config["mode"] != "streaming" || config["field_maps"] == nil {
		t.Fatalf("captured request body.config = %v", config)
	}
	data := dbSyncEnvelopeData(t, stdout.String())
	for _, key := range []string{"task_id", "mode", "status"} {
		if data[key] == "" {
			t.Fatalf("output missing %s: %v", key, data)
		}
	}
}

func TestAppsDBSyncCreateTargetTableNotFoundHint(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: dbSyncURL,
		Body: map[string]interface{}{"code": 400002483, "msg": "target table not found"},
	})

	err := runAppsShortcut(t, AppsDBSyncCreate, []string{
		"+db-sync-create", "--app-id", "app_x", "--config", dbSyncCommitConfig, "--yes", "--as", "user",
	}, factory, stdout)
	p := requireAppsAPIProblem(t, err)
	if !strings.Contains(p.Hint, "target.table.action") {
		t.Fatalf("hint = %q, want target.table.action guidance", p.Hint)
	}
}

func TestAppsDBSyncCreateRejectsUnsafeOutput(t *testing.T) {
	for _, out := range []string{"/tmp/resolved.json", "../resolved.json"} {
		t.Run(out, func(t *testing.T) {
			factory, stdout, _ := newAppsExecuteFactory(t)
			err := runAppsShortcut(t, AppsDBSyncCreate, []string{
				"+db-sync-create", "--app-id", "app_x", "--config", dbSyncPreviewConfig,
				"--preview", "--output", out, "--as", "user",
			}, factory, stdout)
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %T %v, want validation", err, err)
			}
			if ve.Param != "--output" {
				t.Fatalf("Param = %q, want --output", ve.Param)
			}
		})
	}
}

func TestAppsDBSyncCreateRequiresIdentifiableSourceTable(t *testing.T) {
	// base_url without ?table= and empty source.table.name → rejected locally,
	// for both preview and commit. Hint points at base +table-list.
	for _, preview := range []bool{true, false} {
		name := "commit"
		args := []string{"+db-sync-create", "--app-id", "app_x", "--config", dbSyncNoTableNoNameConfig, "--as", "user"}
		if preview {
			name = "preview"
			args = append(args, "--preview")
		} else {
			args = append(args, "--yes")
		}
		t.Run(name+" rejects unidentifiable source table", func(t *testing.T) {
			factory, stdout, _ := newAppsExecuteFactory(t)
			err := runAppsShortcut(t, AppsDBSyncCreate, args, factory, stdout)
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %T %v, want validation", err, err)
			}
			if ve.Param != "--config" {
				t.Fatalf("Param = %q, want --config", ve.Param)
			}
			if !strings.Contains(ve.Hint, "base +table-list") {
				t.Fatalf("hint = %q, want base +table-list guidance", ve.Hint)
			}
		})
	}

	// base_url carries ?table= → identifiable even with empty name.
	t.Run("url table param is identifiable", func(t *testing.T) {
		factory, stdout, _ := newAppsExecuteFactory(t)
		err := runAppsShortcut(t, AppsDBSyncCreate, []string{
			"+db-sync-create", "--app-id", "app_x", "--config", dbSyncUrlTableNoNameConfig,
			"--preview", "--dry-run", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("url-table config must pass validation, got %v", err)
		}
	})

	// source.table.name set → identifiable even without a url table param.
	t.Run("source table name is identifiable", func(t *testing.T) {
		factory, stdout, _ := newAppsExecuteFactory(t)
		err := runAppsShortcut(t, AppsDBSyncCreate, []string{
			"+db-sync-create", "--app-id", "app_x", "--config", dbSyncNoTableWithNameConfig,
			"--preview", "--dry-run", "--as", "user",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("named-table config must pass validation, got %v", err)
		}
	})
}

// +db-sync-update must not inherit the create-only source-table preflight: an
// update config with no url table and no name is legal (server reuses the
// original task's source).
func TestAppsDBSyncUpdateSkipsSourceTablePreflight(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	// add field_maps so parseDBSyncConfigFlag's update requirement is satisfied.
	cfg := `{
  "mode": "streaming",
  "source": {"type": "base", "base_url": "https://base.example/base/CDfkbxg2Ra8aJusw53ib3bpWcMh", "table": {"name": ""}},
  "target": {"type": "postgresql", "table": {"name": "orders", "action": "use_existing"}},
  "field_maps": [{"source_field": "Base 表记录 ID", "target_field": "record_id", "enabled": true}]
}`
	err := runAppsShortcut(t, AppsDBSyncUpdate, []string{
		"+db-sync-update", "--app-id", "app_x", "--task-id", "streaming_1",
		"--config", cfg, "--environment", "dev", "--dry-run", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("update must not apply create-only source-table preflight, got %v", err)
	}
	if strings.Contains(stdout.String(), "base +table-list") {
		t.Fatalf("update output must not carry create-only preflight hint: %s", stdout.String())
	}
}

func TestAppsDBSyncCreateConfirmation(t *testing.T) {
	t.Run("commit requires yes", func(t *testing.T) {
		factory, stdout, _ := newAppsExecuteFactory(t)
		err := runAppsShortcut(t, AppsDBSyncCreate, []string{
			"+db-sync-create", "--app-id", "app_x", "--config", dbSyncCommitConfig, "--as", "user",
		}, factory, stdout)
		p := requireAppsProblem(t, err, errs.CategoryConfirmation)
		if p.Subtype != errs.SubtypeConfirmationRequired {
			t.Fatalf("subtype = %q", p.Subtype)
		}
	})

	t.Run("preview does not require yes", func(t *testing.T) {
		factory, stdout, reg := newAppsExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST", URL: dbSyncURL,
			Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
				"config": map[string]interface{}{"mode": "batch"},
			}},
		})
		if err := runAppsShortcut(t, AppsDBSyncCreate, []string{
			"+db-sync-create", "--app-id", "app_x", "--config", dbSyncPreviewConfig, "--preview", "--as", "user",
		}, factory, stdout); err != nil {
			t.Fatalf("preview execute err=%v", err)
		}
	})
}

func TestAppsDBSyncUpdateDryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBSyncUpdate, []string{
		"+db-sync-update", "--app-id", "app_x", "--task-id", "streaming 1/prod",
		"--config", dbSyncCommitConfig, "--environment", "dev", "--dry-run", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}

	a := dbSyncFirstDryRunAPI(t, stdout.String())
	if a.Method != "PUT" || a.URL != dbSyncUpdateURL {
		t.Fatalf("dry-run = %s %s", a.Method, a.URL)
	}
	if a.Body["task_id"] != "streaming 1/prod" {
		t.Fatalf("dry-run body.task_id = %v", a.Body["task_id"])
	}
	if _, ok := a.Params["task_id"]; ok {
		t.Fatalf("task_id must be in body, not query params: %v", a.Params)
	}
	if a.Body["env"] != "dev" {
		t.Fatalf("dry-run body.env = %v", a.Body["env"])
	}
	if _, ok := a.Params["env"]; ok {
		t.Fatalf("env must be in body, not query params: %v", a.Params)
	}
	config := a.Body["config"].(map[string]interface{})
	if config["mode"] != "streaming" || config["field_maps"] == nil {
		t.Fatalf("dry-run body = %v", a.Body)
	}
}

func TestAppsDBSyncUpdateExecuteSuccess(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PUT", URL: dbSyncUpdateURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"task_id": "streaming_1", "mode": "streaming", "status": "active",
		}},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsDBSyncUpdate, []string{
		"+db-sync-update", "--app-id", "app_x", "--task-id", "streaming_1",
		"--config", dbSyncCommitConfig, "--yes", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	var req map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &req); err != nil {
		t.Fatalf("decode captured request body: %v", err)
	}
	config := req["config"].(map[string]interface{})
	if config["mode"] != "streaming" || config["field_maps"] == nil {
		t.Fatalf("captured request body = %v", req)
	}
	if req["task_id"] != "streaming_1" {
		t.Fatalf("captured request body.task_id = %v", req["task_id"])
	}
	data := dbSyncEnvelopeData(t, stdout.String())
	if data["task_id"] != "streaming_1" || data["mode"] != "streaming" || data["status"] != "active" {
		t.Fatalf("output data = %v", data)
	}
}

func TestAppsDBSyncUpdateSourceTableErrorHintStaysCommandNeutral(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT", URL: dbSyncUpdateURL,
		Body: map[string]interface{}{"code": 400002482, "msg": "invalid source table"},
	})

	err := runAppsShortcut(t, AppsDBSyncUpdate, []string{
		"+db-sync-update", "--app-id", "app_x", "--task-id", "streaming_1",
		"--config", dbSyncCommitConfig, "--yes", "--as", "user",
	}, factory, stdout)
	p := requireAppsAPIProblem(t, err)
	// Update must not be steered into a create-only recovery path.
	if !strings.Contains(p.Hint, "+db-sync-update") {
		t.Fatalf("update hint = %q, want +db-sync-update guidance", p.Hint)
	}
}

func TestAppsDBSyncUpdateMissingFieldMapsValidation(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBSyncUpdate, []string{
		"+db-sync-update", "--app-id", "app_x", "--task-id", "streaming_1",
		"--config", dbSyncNoFieldMapsConfig, "--yes", "--as", "user",
	}, factory, stdout)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want validation", err, err)
	}
	if ve.Param != "--config" {
		t.Fatalf("Param = %q, want --config", ve.Param)
	}
}

func TestAppsDBSyncUpdateConfirmation(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBSyncUpdate, []string{
		"+db-sync-update", "--app-id", "app_x", "--task-id", "streaming_1",
		"--config", dbSyncCommitConfig, "--as", "user",
	}, factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryConfirmation)
	if p.Subtype != errs.SubtypeConfirmationRequired {
		t.Fatalf("subtype = %q", p.Subtype)
	}
}

func dbSyncFirstDryRunAPI(t *testing.T, stdout string) dryRunAPICall {
	t.Helper()
	var env dryRunAPIEnvelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("decode dry-run: %v (raw=%s)", err, stdout)
	}
	if len(env.API) != 1 {
		t.Fatalf("dry-run API calls = %d, want 1; stdout=%s", len(env.API), stdout)
	}
	return env.API[0]
}

func dbSyncEnvelopeData(t *testing.T, stdout string) map[string]interface{} {
	t.Helper()
	var env struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("decode envelope: %v (raw=%s)", err, stdout)
	}
	if env.Data == nil {
		t.Fatalf("envelope data is nil: %s", stdout)
	}
	return env.Data
}
