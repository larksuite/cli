// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestContactFieldEnrichmentShortcutMetadata(t *testing.T) {
	if BaseContactFieldEnrichmentList.Risk != "read" || !reflect.DeepEqual(BaseContactFieldEnrichmentList.Scopes, []string{"base:field:read"}) {
		t.Fatalf("list risk/scopes = %q/%v", BaseContactFieldEnrichmentList.Risk, BaseContactFieldEnrichmentList.Scopes)
	}
	if BaseContactFieldEnrichmentCreate.Risk != "write" || !reflect.DeepEqual(BaseContactFieldEnrichmentCreate.Scopes, []string{"base:field:create"}) {
		t.Fatalf("create risk/scopes = %q/%v", BaseContactFieldEnrichmentCreate.Risk, BaseContactFieldEnrichmentCreate.Scopes)
	}

	parent := &cobra.Command{Use: "base"}
	BaseContactFieldEnrichmentCreate.Mount(parent, &cmdutil.Factory{})
	jsonFlag := parent.Commands()[0].Flags().Lookup("json")
	if jsonFlag == nil || !strings.Contains(jsonFlag.Usage, contactFieldEnrichmentReference) {
		t.Fatalf("--json help must route agents to %s; flag=%v", contactFieldEnrichmentReference, jsonFlag)
	}

	registered := map[string]bool{}
	for _, shortcut := range Shortcuts() {
		registered[shortcut.Command] = true
	}
	for _, command := range []string{BaseContactFieldEnrichmentList.Command, BaseContactFieldEnrichmentCreate.Command} {
		if !registered[command] {
			t.Fatalf("shortcut %q is not registered", command)
		}
	}
}

func TestValidateContactFieldEnrichmentCreate(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "valid batch",
			body: `{"enrichments":[{"source_field":"People","info_type":"department"},{"source_field":"fld_people","info_type":"work_email"}]}`,
		},
		{name: "invalid json", body: `{`, wantErr: "invalid JSON contact field enrichment object"},
		{name: "array", body: `[]`, wantErr: "cannot unmarshal array"},
		{name: "empty", body: `{"enrichments":[]}`, wantErr: "--json.enrichments must contain at least one item"},
		{name: "unknown top-level property", body: `{"enrichments":[{"source_field":"People","info_type":"department"}],"name":"x"}`, wantErr: `unknown field "name"`},
		{name: "unknown item property", body: `{"enrichments":[{"source_field":"People","info_type":"department","name":"x"}]}`, wantErr: `unknown field "name"`},
		{name: "blank source", body: `{"enrichments":[{"source_field":" ","info_type":"department"}]}`, wantErr: "source_field must not be blank"},
		{name: "unsupported info type", body: `{"enrichments":[{"source_field":"People","info_type":"personal_email"}]}`, wantErr: "must be one of department"},
		{name: "trailing value", body: `{"enrichments":[{"source_field":"People","info_type":"department"}]} {}`, wantErr: "must contain exactly one JSON object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := newBaseTestRuntime(map[string]string{
				"base-token": "app_x",
				"table-id":   "tbl_x",
				"json":       tt.body,
			}, nil, nil)
			err := BaseContactFieldEnrichmentCreate.Validate(context.Background(), runtime)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() err=%v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() err=%v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestContactFieldEnrichmentDryRun(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		runtime := newBaseTestRuntime(map[string]string{"base-token": "app_x", "table-id": "tbl_x"}, nil, nil)
		assertContactFieldEnrichmentDryRun(t, dryRunContactFieldEnrichmentList(context.Background(), runtime), "GET", nil)
	})

	t.Run("create", func(t *testing.T) {
		runtime := newBaseTestRuntime(map[string]string{
			"base-token": "app_x",
			"table-id":   "tbl_x",
			"json":       `{"enrichments":[{"source_field":"People","info_type":"direct_manager"}]}`,
		}, nil, nil)
		assertContactFieldEnrichmentDryRun(t, dryRunContactFieldEnrichmentCreate(context.Background(), runtime), "POST", map[string]interface{}{
			"enrichments": []interface{}{map[string]interface{}{"source_field": "People", "info_type": "direct_manager"}},
		})
	})
}

func assertContactFieldEnrichmentDryRun(t *testing.T, dryRun *common.DryRunAPI, method string, body map[string]interface{}) {
	t.Helper()
	raw, err := json.Marshal(dryRun)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	var value struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode dry-run: %v", err)
	}
	if len(value.API) != 1 {
		t.Fatalf("dry-run calls=%d, want 1: %s", len(value.API), raw)
	}
	if value.API[0].Method != method {
		t.Fatalf("method=%q, want %q", value.API[0].Method, method)
	}
	const wantURL = "/open-apis/base/v3/bases/app_x/tables/tbl_x/contact_field_enrichments"
	if value.API[0].URL != wantURL {
		t.Fatalf("url=%q, want %q", value.API[0].URL, wantURL)
	}
	if !reflect.DeepEqual(value.API[0].Body, body) {
		t.Fatalf("body=%#v, want %#v", value.API[0].Body, body)
	}
}

func TestContactFieldEnrichmentExecuteShortcuts(t *testing.T) {
	const path = "/open-apis/base/v3/bases/app_x/tables/tbl_x/contact_field_enrichments"
	snapshot := map[string]interface{}{
		"enrichments": []interface{}{
			map[string]interface{}{
				"field":        map[string]interface{}{"id": "fld_department", "name": "People.Department", "type": "text"},
				"source_field": map[string]interface{}{"id": "fld_people", "name": "People", "type": "user"},
				"info_type":    "department",
			},
		},
		"total": float64(1),
	}

	t.Run("list preserves snapshot", func(t *testing.T) {
		factory, stdout, registry := newExecuteFactory(t)
		registry.Register(&httpmock.Stub{Method: "GET", URL: path, Body: map[string]interface{}{"code": 0, "data": snapshot}})
		if err := runShortcut(t, BaseContactFieldEnrichmentList, []string{
			"+contact-field-enrichment-list", "--base-token", "app_x", "--table-id", "tbl_x",
		}, factory, stdout); err != nil {
			t.Fatalf("execute list: %v", err)
		}
		if got := decodeBaseEnvelope(t, stdout); !reflect.DeepEqual(got, snapshot) {
			t.Fatalf("data=%#v, want %#v", got, snapshot)
		}
	})

	t.Run("create sends body and preserves snapshot", func(t *testing.T) {
		factory, stdout, registry := newExecuteFactory(t)
		stub := &httpmock.Stub{Method: "POST", URL: path, Body: map[string]interface{}{"code": 0, "data": snapshot}}
		registry.Register(stub)
		if err := runShortcut(t, BaseContactFieldEnrichmentCreate, []string{
			"+contact-field-enrichment-create",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--json", `{"enrichments":[{"source_field":"People","info_type":"department"}]}`,
		}, factory, stdout); err != nil {
			t.Fatalf("execute create: %v", err)
		}
		body := decodeCapturedJSONBody(t, stub)
		wantBody := map[string]interface{}{
			"enrichments": []interface{}{map[string]interface{}{"source_field": "People", "info_type": "department"}},
		}
		if !reflect.DeepEqual(body, wantBody) {
			t.Fatalf("request body=%#v, want %#v", body, wantBody)
		}
		if got := decodeBaseEnvelope(t, stdout); !reflect.DeepEqual(got, snapshot) {
			t.Fatalf("data=%#v, want %#v", got, snapshot)
		}
	})
}
