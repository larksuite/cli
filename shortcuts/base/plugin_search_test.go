// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func pluginSearchTestRuntime(t *testing.T, keyword string, pluginTypes []string, limit int) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("keyword", "", "")
	cmd.Flags().StringSlice("plugin-types", nil, "")
	cmd.Flags().Int("limit", pluginSearchDefaultLimit, "")
	if err := cmd.Flags().Set("keyword", keyword); err != nil {
		t.Fatal(err)
	}
	if len(pluginTypes) > 0 {
		if err := cmd.Flags().Set("plugin-types", strings.Join(pluginTypes, ",")); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.Flags().Set("limit", strconv.Itoa(limit)); err != nil {
		t.Fatal(err)
	}
	return &common.RuntimeContext{Cmd: cmd, Config: &core.CliConfig{UserOpenId: "ou_test"}}
}

func TestPluginSearchValidate(t *testing.T) {
	tests := []struct {
		name        string
		keyword     string
		pluginTypes []string
		limit       int
		wantParam   string
	}{
		{name: "blank keyword", keyword: " \t", limit: 10, wantParam: "--keyword"},
		{name: "keyword too long", keyword: strings.Repeat("界", 201), limit: 10, wantParam: "--keyword"},
		{name: "unknown plugin type", keyword: "jira", pluginTypes: []string{"unknown"}, limit: 10, wantParam: "--plugin-types"},
		{name: "limit below range", keyword: "jira", limit: 0, wantParam: "--limit"},
		{name: "limit above range", keyword: "jira", limit: 21, wantParam: "--limit"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := pluginSearchTestRuntime(t, test.keyword, test.pluginTypes, test.limit)
			err := BasePluginSearch.Validate(context.Background(), runtime)
			assertInvalidArgumentValidation(t, err, test.wantParam, nil, "")
		})
	}
	t.Run("explicit empty plugin types", func(t *testing.T) {
		runtime := pluginSearchTestRuntime(t, "jira", nil, 10)
		if err := runtime.Cmd.Flags().Set("plugin-types", ""); err != nil {
			t.Fatal(err)
		}
		err := BasePluginSearch.Validate(context.Background(), runtime)
		assertInvalidArgumentValidation(t, err, "--plugin-types", nil, "")
	})

	runtime := pluginSearchTestRuntime(
		t,
		strings.Repeat("界", 200),
		[]string{"side_view", "data_connector", "dashboard_widget", "field_shortcut", "workflow", "base_app"},
		20,
	)
	if err := BasePluginSearch.Validate(context.Background(), runtime); err != nil {
		t.Fatalf("valid search rejected: %v", err)
	}
}

func TestPluginSearchDryRunRequest(t *testing.T) {
	runtime := pluginSearchTestRuntime(t, " Jira issue sync ", []string{"data_connector", "workflow"}, 7)
	dryRun, err := json.Marshal(BasePluginSearch.DryRun(context.Background(), runtime))
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(dryRun, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.API) != 1 || payload.API[0].Method != "POST" || payload.API[0].URL != pluginSearchPath {
		t.Fatalf("dry-run API=%#v", payload.API)
	}
	body := payload.API[0].Body
	if body["query"] != " Jira issue sync " || body["limit"] != float64(7) {
		t.Fatalf("dry-run body=%#v", body)
	}
	types, _ := body["plugin_types"].([]interface{})
	if len(types) != 2 || types[0] != "data_connector" || types[1] != "workflow" {
		t.Fatalf("dry-run plugin_types=%#v", body["plugin_types"])
	}
}

func TestPluginSearchExecutePreservesFusedResponse(t *testing.T) {
	factory, stdout, registry := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    pluginSearchPath,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"partial": true,
				"items": []interface{}{
					map[string]interface{}{
						"plugin_id":   "pack_jira",
						"plugin_type": "data_connector",
						"name":        "Jira Connector",
						"detail_url":  "https://example.com/plugins/pack_jira",
					},
				},
			},
		},
	}
	registry.Register(stub)
	err := runShortcutWithAuthTypes(
		t,
		BasePluginSearch,
		[]string{"user"},
		[]string{
			"+plugin-search",
			"--keyword", "Jira issue sync",
			"--plugin-types", "data_connector,workflow",
			"--as", "user",
		},
		factory,
		stdout,
	)
	if err != nil {
		t.Fatalf("execute plugin search: %v", err)
	}

	var requestBody map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &requestBody); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if requestBody["query"] != "Jira issue sync" || requestBody["limit"] != float64(10) {
		t.Fatalf("request body=%#v", requestBody)
	}
	pluginTypes, _ := requestBody["plugin_types"].([]interface{})
	if len(pluginTypes) != 2 || pluginTypes[0] != "data_connector" || pluginTypes[1] != "workflow" {
		t.Fatalf("request plugin_types=%#v", requestBody["plugin_types"])
	}

	data := decodeBaseEnvelope(t, stdout)
	if data["partial"] != true {
		t.Fatalf("response data=%#v", data)
	}
	items, _ := data["items"].([]interface{})
	if len(items) != 1 || items[0].(map[string]interface{})["plugin_id"] != "pack_jira" {
		t.Fatalf("response items=%#v", items)
	}
	if items[0].(map[string]interface{})["detail_url"] != "https://example.com/plugins/pack_jira" {
		t.Fatalf("response detail_url=%#v", items[0].(map[string]interface{})["detail_url"])
	}
}

func TestPluginSearchSupportsUserAndBotIdentities(t *testing.T) {
	if !slices.Equal(BasePluginSearch.AuthTypes, authTypes()) {
		t.Fatalf("auth types=%v, want %v", BasePluginSearch.AuthTypes, authTypes())
	}
}

func TestPluginSearchKeywordGuidesAgentQueryRewrite(t *testing.T) {
	var description string
	for _, flag := range BasePluginSearch.Flags {
		if flag.Name == "keyword" {
			description = flag.Desc
			break
		}
	}
	for _, guidance := range []string{
		"concise retrieval query",
		"rewritten from the user's request",
		"resolve references",
		"1-2 high-signal terms",
		"synonyms, common names, or alternate phrasing",
		"1-200 characters",
	} {
		if !strings.Contains(description, guidance) {
			t.Fatalf("--keyword description %q must contain %q", description, guidance)
		}
	}
}

func TestPluginSearchTipsExplainCommonRewriteTechniques(t *testing.T) {
	tips := strings.Join(BasePluginSearch.Tips, "\n")
	for _, guidance := range []string{
		"conversational references",
		"1-2 high-signal terms",
		"do not send the full request sentence",
		"data synchronization",
		"external source product name",
		"MySQL, Jira, or Salesforce",
		"omit generic terms such as sync, connector, or Base",
		"small set of same-intent queries",
		"alternative terms or common names",
		"one rewritten query per call",
		"do not concatenate multiple queries",
		"hard constraints only when needed to distinguish candidates",
		"never add capabilities",
		"non-sensitive business concepts, field meanings, or redacted examples",
		"raw record values, bulk data, credentials, user data",
		"full conversation",
	} {
		if !strings.Contains(tips, guidance) {
			t.Fatalf("plugin search tips %q must contain %q", tips, guidance)
		}
	}
}
