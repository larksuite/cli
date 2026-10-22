// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lingo

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func lingoTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	suffix := strings.NewReplacer("/", "-", " ", "-").Replace(strings.ToLower(t.Name()))
	return &core.CliConfig{
		AppID:     "test-lingo-" + suffix,
		AppSecret: "secret-lingo-" + suffix,
		Brand:     core.BrandFeishu,
	}
}

func runLingoShortcut(t *testing.T, s common.Shortcut, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "lingo"}
	s.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func decodeEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()
	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode output: %v\nraw=%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	if data == nil {
		t.Fatalf("missing data in output envelope: %#v", envelope)
	}
	return data
}

func TestLingoGetValidate(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantParam string
	}{
		{
			name:      "no target flags",
			args:      []string{"+get"},
			wantParam: "--word",
		},
		{
			name:      "provider without outer-id",
			args:      []string{"+get", "--provider", "myhr"},
			wantParam: "--provider",
		},
		{
			name:      "outer-id without provider",
			args:      []string{"+get", "--outer-id", "EMP-1"},
			wantParam: "--provider",
		},
		{
			name:      "word and entity-id mutually exclusive",
			args:      []string{"+get", "--word", "KYC", "--entity-id", "enterprise_1"},
			wantParam: "--word",
		},
		{
			name:      "repo-id without word",
			args:      []string{"+get", "--entity-id", "enterprise_1", "--repo-id", "repo_1"},
			wantParam: "--repo-id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
			err := runLingoShortcut(t, LingoGet, f, stdout, tc.args)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
			}
			if ve.Param != tc.wantParam {
				t.Fatalf("Param = %q, want %q (err=%v)", ve.Param, tc.wantParam, err)
			}
		})
	}
}

func TestLingoGetDryRun(t *testing.T) {
	t.Run("by entity-id escapes path", func(t *testing.T) {
		f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
		err := runLingoShortcut(t, LingoGet, f, stdout, []string{
			"+get", "--entity-id", "enterprise_123/456", "--dry-run",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := stdout.String()
		if !strings.Contains(out, "/open-apis/lingo/v1/entities/enterprise_123%2F456") {
			t.Fatalf("expected escaped entity_id in dry-run, got: %s", out)
		}
	})

	t.Run("by word shows match and get steps", func(t *testing.T) {
		f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
		err := runLingoShortcut(t, LingoGet, f, stdout, []string{
			"+get", "--word", "KYC", "--repo-id", "repo_99", "--dry-run",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := stdout.String()
		for _, want := range []string{
			"/open-apis/lingo/v1/entities/match",
			"repo_99",
			"KYC",
			"/open-apis/lingo/v1/entities/{entity_id}",
		} {
			if !strings.Contains(out, want) {
				t.Fatalf("dry-run missing %q, got: %s", want, out)
			}
		}
	})
}

func TestLingoGetExecute_ByEntityID(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, lingoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/lingo/v1/entities/enterprise_101",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"entity": map[string]interface{}{
					"id":          "enterprise_101",
					"main_keys":   []interface{}{map[string]interface{}{"key": "KYC"}},
					"aliases":     []interface{}{map[string]interface{}{"key": "Know Your Customer"}},
					"description": "Identity verification process",
				},
			},
		},
	})

	if err := runLingoShortcut(t, LingoGet, f, stdout, []string{"+get", "--entity-id", "enterprise_101"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	entity, _ := data["entity"].(map[string]interface{})
	if common.GetString(entity, "id") != "enterprise_101" {
		t.Fatalf("unexpected entity: %#v", entity)
	}
}

func TestLingoGetExecute_ByOuterID(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, lingoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/lingo/v1/entities/enterprise_0",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"entity": map[string]interface{}{
					"id":        "enterprise_202",
					"main_keys": []interface{}{map[string]interface{}{"key": "TCE"}},
				},
			},
		},
	})

	if err := runLingoShortcut(t, LingoGet, f, stdout, []string{
		"+get", "--provider", "bytetree", "--outer-id", "node_1",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	entity, _ := data["entity"].(map[string]interface{})
	if common.GetString(entity, "id") != "enterprise_202" {
		t.Fatalf("unexpected entity: %#v", entity)
	}
}

func TestLingoGetExecute_ByWordResolvesEntityDetails(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, lingoTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/lingo/v1/entities/match",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"results": []interface{}{
					map[string]interface{}{"entity_id": "enterprise_303", "type": 0},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/lingo/v1/entities/enterprise_303",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"entity": map[string]interface{}{
					"id":          "enterprise_303",
					"main_keys":   []interface{}{map[string]interface{}{"key": "PSM"}},
					"description": "Product Service Module",
				},
			},
		},
	})

	if err := runLingoShortcut(t, LingoGet, f, stdout, []string{"+get", "--word", "PSM"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	entity, _ := data["entity"].(map[string]interface{})
	if common.GetString(entity, "id") != "enterprise_303" {
		t.Fatalf("expected resolved entity enterprise_303, got: %#v", data)
	}
}

func TestLingoSearchValidateAndExecute(t *testing.T) {
	t.Run("blank query rejected", func(t *testing.T) {
		f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
		err := runLingoShortcut(t, LingoSearch, f, stdout, []string{"+search", "--query", "   "})
		if err == nil {
			t.Fatal("expected error for blank --query")
		}
		var ve *errs.ValidationError
		if !errors.As(err, &ve) || ve.Param != "--query" {
			t.Fatalf("expected ValidationError on --query, got %v", err)
		}
	})

	t.Run("page-size out of range rejected", func(t *testing.T) {
		f, stdout, _, _ := cmdutil.TestFactory(t, lingoTestConfig(t))
		err := runLingoShortcut(t, LingoSearch, f, stdout, []string{"+search", "--query", "PSM", "--page-size", "101"})
		if err == nil {
			t.Fatal("expected error for page-size > 100")
		}
		var ve *errs.ValidationError
		if !errors.As(err, &ve) || ve.Param != "--page-size" {
			t.Fatalf("expected ValidationError on --page-size, got %v", err)
		}
	})

	t.Run("execute returns entities and pagination", func(t *testing.T) {
		f, stdout, _, reg := cmdutil.TestFactory(t, lingoTestConfig(t))
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/lingo/v1/entities/search",
			Body: map[string]interface{}{
				"code": 0,
				"msg":  "ok",
				"data": map[string]interface{}{
					"entities": []interface{}{
						map[string]interface{}{
							"id":          "enterprise_505",
							"main_keys":   []interface{}{map[string]interface{}{"key": "AML"}},
							"description": "Anti-Money Laundering",
						},
					},
					"has_more":   true,
					"page_token": "next_tok",
				},
			},
		})

		if err := runLingoShortcut(t, LingoSearch, f, stdout, []string{
			"+search", "--query", "AML", "--repo-id", "repo_1", "--page-size", "10",
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data := decodeEnvelope(t, stdout)
		entities := common.GetSlice(data, "entities")
		if len(entities) != 1 || data["page_token"] != "next_tok" || data["has_more"] != true {
			t.Fatalf("unexpected search data: %#v", data)
		}
	})
}

func TestLingoDefaultBotIdentityWhenUserLoggedIn(t *testing.T) {
	cfg := lingoTestConfig(t)
	cfg.UserOpenId = "ou_logged_in_user"
	f, stdout, _, reg := cmdutil.TestFactory(t, cfg)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/lingo/v1/entities/search",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"entities": []interface{}{
					map[string]interface{}{
						"id":        "enterprise_777",
						"main_keys": []interface{}{map[string]interface{}{"key": "Lingo"}},
					},
				},
				"has_more": false,
			},
		},
	})

	if err := runLingoShortcut(t, LingoSearch, f, stdout, []string{"+search", "--query", "lingo"}); err != nil {
		t.Fatalf("expected default bot identity to succeed without user baike scope, got: %v", err)
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode output: %v", err)
	}
	if envelope["identity"] != "bot" {
		t.Fatalf("identity = %v, want bot", envelope["identity"])
	}
}

func TestLingoScopesAndPrettyPaginationHint(t *testing.T) {
	for _, sc := range []common.Shortcut{LingoGet, LingoSearch} {
		botScopes := sc.DeclaredScopesForIdentity("bot")
		if len(botScopes) != 1 || botScopes[0] != "baike:entity:readonly" {
			t.Fatalf("%s bot scopes = %v, want [baike:entity:readonly]", sc.Command, botScopes)
		}
		if userScopes := sc.DeclaredScopesForIdentity("user"); len(userScopes) != 0 {
			t.Fatalf("%s user scopes = %v, want empty (baike:entity:readonly is tenant-only)", sc.Command, userScopes)
		}
	}

	stubBody := map[string]interface{}{
		"code": 0,
		"msg":  "ok",
		"data": map[string]interface{}{
			"entities": []interface{}{
				map[string]interface{}{
					"id":          "enterprise_505",
					"main_keys":   []interface{}{map[string]interface{}{"key": "AML"}},
					"description": "Anti-Money Laundering",
				},
			},
			"has_more":   true,
			"page_token": "next_tok",
		},
	}

	fPretty, stdoutPretty, _, regPretty := cmdutil.TestFactory(t, lingoTestConfig(t))
	regPretty.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/lingo/v1/entities/search", Body: stubBody})
	if err := runLingoShortcut(t, LingoSearch, fPretty, stdoutPretty, []string{
		"+search", "--query", "AML", "--format", "pretty",
	}); err != nil {
		t.Fatalf("pretty search failed: %v", err)
	}
	if !strings.Contains(stdoutPretty.String(), "(more available, page_token: next_tok)") {
		t.Fatalf("pretty output missing pagination hint: %s", stdoutPretty.String())
	}

	fNDJSON, stdoutNDJSON, _, regNDJSON := cmdutil.TestFactory(t, lingoTestConfig(t))
	regNDJSON.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/lingo/v1/entities/search", Body: stubBody})
	if err := runLingoShortcut(t, LingoSearch, fNDJSON, stdoutNDJSON, []string{
		"+search", "--query", "AML", "--format", "ndjson",
	}); err != nil {
		t.Fatalf("ndjson search failed: %v", err)
	}
	if strings.Contains(stdoutNDJSON.String(), "more available") {
		t.Fatalf("ndjson output should not contain human pagination hint: %s", stdoutNDJSON.String())
	}
}

func TestLingoGetByWordEmptyMatchAndI18nDescFallback(t *testing.T) {
	fEmpty, stdoutEmpty, _, regEmpty := cmdutil.TestFactory(t, lingoTestConfig(t))
	regEmpty.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/lingo/v1/entities/match",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	})
	if err := runLingoShortcut(t, LingoGet, fEmpty, stdoutEmpty, []string{"+get", "--word", "NoSuchTerm"}); err != nil {
		t.Fatalf("unexpected error on empty match: %v", err)
	}
	data := decodeEnvelope(t, stdoutEmpty)
	if results, ok := data["results"].([]interface{}); !ok || len(results) != 0 {
		t.Fatalf("results = %#v, want non-nil empty slice", data["results"])
	}

	fI18n, stdoutI18n, _, regI18n := cmdutil.TestFactory(t, lingoTestConfig(t))
	regI18n.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/lingo/v1/entities/enterprise_909",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"entity": map[string]interface{}{
					"id":        "enterprise_909",
					"main_keys": []interface{}{map[string]interface{}{"key": "I18nTerm"}},
					"i18n_descs": []interface{}{
						map[string]interface{}{"language": 1, "description": "Multilingual fallback description"},
					},
				},
			},
		},
	})
	if err := runLingoShortcut(t, LingoGet, fI18n, stdoutI18n, []string{
		"+get", "--entity-id", "enterprise_909", "--format", "pretty",
	}); err != nil {
		t.Fatalf("unexpected error on i18n pretty get: %v", err)
	}
	if !strings.Contains(stdoutI18n.String(), "Multilingual fallback description") {
		t.Fatalf("pretty output missing i18n_descs fallback description: %s", stdoutI18n.String())
	}
}
