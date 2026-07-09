// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func assertEnvVarBody(t *testing.T, req *http.Request, want map[string]interface{}) {
	t.Helper()
	if req.URL.RawQuery != "" {
		t.Fatalf("query should be empty, got %q", req.URL.RawQuery)
	}
	var got map[string]interface{}
	if err := json.NewDecoder(req.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("body = %#v, want %#v", got, want)
	}
}

func expectedEnvVarSceneJSON() float64 {
	return float64(defaultAppsEnvVarScene)
}

func decodeEnvVarEnvelopeData(t *testing.T, stdout string) map[string]interface{} {
	t.Helper()
	var envelope struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout)
	}
	if !envelope.OK {
		t.Fatalf("expected ok envelope, got %s", stdout)
	}
	return envelope.Data
}

func requireEnvVarValidationProblem(t *testing.T, err error, param string) {
	t.Helper()
	p := requireAppsProblem(t, err, errs.CategoryValidation)
	if p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("validation subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidArgument)
	}
	var validation *errs.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if validation.Param != param {
		t.Fatalf("validation param = %q, want %q", validation.Param, param)
	}
}

func TestAppsEnvVarList_DefaultsToDevAndHidesValues(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		OnMatch: func(req *http.Request) {
			assertEnvVarBody(t, req, map[string]interface{}{"env": "dev", "scene": expectedEnvVarSceneJSON()})
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"envVars": []interface{}{
					map[string]interface{}{"key": "SECRET_TOKEN", "value": "super-secret", "env": "dev"},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsEnvVarList,
		[]string{"+env-list", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	got := stdout.String()
	if strings.Contains(got, "super-secret") || strings.Contains(got, `"value"`) {
		t.Fatalf("stdout must not expose values by default: %s", got)
	}
	data := decodeEnvVarEnvelopeData(t, got)
	items, ok := data["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v, want one item", data["items"])
	}
	item, ok := items[0].(map[string]interface{})
	if !ok || item["key"] != "SECRET_TOKEN" {
		t.Fatalf("item = %#v, want SECRET_TOKEN", items[0])
	}
	if _, ok := item["value"]; ok {
		t.Fatalf("item must not contain value by default: %#v", item)
	}
}

func TestAppsEnvVarList_IncludeValuesAllowsValues(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		OnMatch: func(req *http.Request) {
			assertEnvVarBody(t, req, map[string]interface{}{"env": "online", "scene": expectedEnvVarSceneJSON()})
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"envVars": []interface{}{
					map[string]interface{}{"key": "SECRET_TOKEN", "value": "super-secret", "env": "online"},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsEnvVarList,
		[]string{"+env-list", "--app-id", "app_x", "--environment", "online", "--include-values", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "super-secret") {
		t.Fatalf("stdout should include values when requested: %s", got)
	}
}

func TestAppsEnvVarList_DoesNotAcceptEnvironmentShorthand(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsEnvVarList,
		[]string{"+env-list", "--app-id", "app_x", "-e", "online", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "unknown shorthand flag: 'e'") {
		t.Fatalf("expected unknown -e shorthand, got %v", err)
	}
}

func TestAppsEnvVarList_DryRunIncludesScene(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsEnvVarList, []string{
		"+env-list", "--app-id", "app_x", "--include-values", "--dry-run", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var dryRun struct {
		API []struct {
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &dryRun); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, stdout.String())
	}
	if got := dryRun.API[0].Body["scene"]; got != expectedEnvVarSceneJSON() {
		t.Fatalf("body.scene = %#v, want %v; stdout:\n%s", got, expectedEnvVarSceneJSON(), stdout.String())
	}
}

func TestAppsEnvVarList_PrettyDisplaysTable(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/env_vars",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"envVars": []interface{}{
					map[string]interface{}{"key": "API_HOST", "value": "https://example.com", "env": "online"},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsEnvVarList, []string{
		"+env-list", "--app-id", "app_x", "--environment", "online", "--include-values", "--format", "pretty", "--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.HasPrefix(got, "key") {
		t.Fatalf("pretty output should start with key column, got:\n%s", got)
	}
	for _, want := range []string{"API_HOST", "online", "https://example.com"} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"ok"`) || strings.Contains(got, `"data"`) {
		t.Fatalf("pretty output should not fall back to JSON envelope:\n%s", got)
	}
}

func TestAppsEnvVarSet_OnlineRequiresYesOutsideDryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsEnvVarSet,
		[]string{"+env-set", "--app-id", "app_x", "--environment", "online",
			"--key", "SECRET_TOKEN", "--value", "super-secret", "--as", "user"}, factory, stdout)

	p := requireAppsProblem(t, err, errs.CategoryConfirmation)
	if p.Subtype != errs.SubtypeConfirmationRequired {
		t.Fatalf("confirmation subtype = %q, want %q", p.Subtype, errs.SubtypeConfirmationRequired)
	}
	if !strings.Contains(p.Hint, "add --yes") {
		t.Fatalf("confirmation hint missing --yes guidance: %#v", p)
	}
}

func TestAppsEnvVarSet_OnlineDryRunDoesNotRequireYes(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsEnvVarSet,
		[]string{"+env-set", "--app-id", "app_x", "--environment", "online",
			"--key", "SECRET_TOKEN", "--value", "super-secret", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}

	got := stdout.String()
	if strings.Contains(got, "super-secret") {
		t.Fatalf("dry-run must redact value: %s", got)
	}
	for _, want := range []string{`"method": "POST"`, `/open-apis/spark/v1/apps/app_x/create_or_update_env_var`} {
		if !strings.Contains(got, want) {
			t.Fatalf("dry-run missing %q: %s", want, got)
		}
	}
	var dryRun struct {
		API []struct {
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal([]byte(got), &dryRun); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, got)
	}
	if len(dryRun.API) != 1 || dryRun.API[0].Body["value"] != "<redacted>" || dryRun.API[0].Body["key"] != "SECRET_TOKEN" {
		t.Fatalf("dry-run body = %#v, want redacted value and key", dryRun.API)
	}
}

func TestAppsEnvVarSet_ExecutesWithYesAndDoesNotEchoValue(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/create_or_update_env_var",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"action": "updated"}},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsEnvVarSet,
		[]string{"+env-set", "--app-id", "app_x", "--environment", "online",
			"--key", "SECRET_TOKEN", "--value", "super-secret", "--yes", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var sent map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &sent); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if sent["key"] != "SECRET_TOKEN" || sent["env"] != "online" || sent["value"] != "super-secret" {
		t.Fatalf("body = %#v, want real online value", sent)
	}
	got := stdout.String()
	if strings.Contains(got, "super-secret") || strings.Contains(got, `"value"`) {
		t.Fatalf("stdout must not echo value: %s", got)
	}
	for _, want := range []string{`"key": "SECRET_TOKEN"`, `"env": "online"`, `"action": "updated"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout missing %q: %s", want, got)
		}
	}
}

func TestAppsEnvVarDelete_IsHighRiskWrite(t *testing.T) {
	if AppsEnvVarDelete.Risk != "high-risk-write" {
		t.Fatalf("risk = %q, want high-risk-write", AppsEnvVarDelete.Risk)
	}
}

func TestAppsEnvVarDelete_BuildsDeleteBodyWithKeys(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/delete_env_vars",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"deleted_keys": []interface{}{"SECRET_ONE", "SECRET_TWO"}}},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsEnvVarDelete,
		[]string{"+env-delete", "--app-id", "app_x", "--environment", "online",
			"--key", "SECRET_ONE", "--key", "SECRET_TWO", "--yes", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var sent map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &sent); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if sent["env"] != "online" {
		t.Fatalf("body.env = %v, want online", sent["env"])
	}
	keys, ok := sent["keys"].([]interface{})
	if !ok || len(keys) != 2 || keys[0] != "SECRET_ONE" || keys[1] != "SECRET_TWO" {
		t.Fatalf("body.keys = %#v, want SECRET_ONE/SECRET_TWO", sent["keys"])
	}
	got := stdout.String()
	for _, want := range []string{`"env": "online"`, `"deleted_keys"`, `"SECRET_ONE"`, `"SECRET_TWO"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout missing %q: %s", want, got)
		}
	}
}

func TestAppsEnvVarDelete_NotModifiableHint(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/delete_env_vars",
		Body: map[string]interface{}{
			"code": 400000072,
			"msg":  "Invalid Request: env var (INTEGRATION_TOKEN) is not modifiable",
		},
	})

	err := runAppsShortcut(t, AppsEnvVarDelete,
		[]string{"+env-delete", "--app-id", "app_x", "--key", "INTEGRATION_TOKEN", "--yes", "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatalf("expected not modifiable error, got nil; stdout=%s", stdout.String())
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	if p.Code != 400000072 {
		t.Fatalf("code = %d, want 400000072", p.Code)
	}
	if !strings.Contains(p.Hint, "platform-managed") || !strings.Contains(p.Hint, "user-defined") {
		t.Fatalf("hint = %q, want platform-managed/user-defined guidance", p.Hint)
	}
	if strings.Contains(p.Hint, "apps +list") {
		t.Fatalf("hint should not point at app listing for protected env vars: %q", p.Hint)
	}
}

func TestAppsEnvVarDelete_OnlineDryRunDoesNotRequireYes(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsEnvVarDelete,
		[]string{"+env-delete", "--app-id", "app_x", "--environment", "online",
			"--key", "SECRET_ONE", "--key", "SECRET_TWO", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}

	var dryRun struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	got := stdout.String()
	if err := json.Unmarshal([]byte(got), &dryRun); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, got)
	}
	if len(dryRun.API) != 1 || dryRun.API[0].Method != "POST" || dryRun.API[0].URL != "/open-apis/spark/v1/apps/app_x/delete_env_vars" {
		t.Fatalf("dry-run api = %#v", dryRun.API)
	}
	if dryRun.API[0].Body["env"] != "online" {
		t.Fatalf("dry-run body.env = %v, want online", dryRun.API[0].Body["env"])
	}
	keys, ok := dryRun.API[0].Body["keys"].([]interface{})
	if !ok || len(keys) != 2 || keys[0] != "SECRET_ONE" || keys[1] != "SECRET_TWO" {
		t.Fatalf("dry-run body.keys = %#v, want SECRET_ONE/SECRET_TWO", dryRun.API[0].Body["keys"])
	}
}

func TestAppsEnvVarList_InvalidEnvTypedValidation(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsEnvVarList,
		[]string{"+env-list", "--app-id", "app_x", "--environment", "prod", "--as", "user"}, factory, stdout)
	requireEnvVarValidationProblem(t, err, "--environment")
}

func TestAppsEnvVarList_OldEnvFlagIsNotAlias(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsEnvVarList,
		[]string{"+env-list", "--app-id", "app_x", "--env", "online", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --env") {
		t.Fatalf("expected old --env to be rejected, got %v", err)
	}
}

func TestAppsEnvVarSet_InvalidKeyTypedValidation(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsEnvVarSet,
		[]string{"+env-set", "--app-id", "app_x", "--key", "bad-key",
			"--value", "super-secret", "--as", "user"}, factory, stdout)
	requireEnvVarValidationProblem(t, err, "--key")
}

func TestAppsEnvVarDelete_InvalidKeyTypedValidation(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsEnvVarDelete,
		[]string{"+env-delete", "--app-id", "app_x", "--key", "bad-key",
			"--yes", "--as", "user"}, factory, stdout)
	requireEnvVarValidationProblem(t, err, "--key")
}
