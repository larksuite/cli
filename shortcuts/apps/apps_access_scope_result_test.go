// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// accessScopeResultBody builds a create response carrying data.access_scope_result.
func accessScopeResultBody(result map[string]interface{}) map[string]interface{} {
	data := map[string]interface{}{
		"app": map[string]interface{}{"app_id": "app_x", "name": "Demo"},
	}
	if result != nil {
		data["access_scope_result"] = result
	}
	return map[string]interface{}{"code": 0, "data": data}
}

// TestWarnAccessScopeResult covers the four shapes the server can produce, plus
// the two cases where the CLI must stay silent. The gate is what this CLI asked
// for: the result is only ever about a scope this invocation requested, so a run
// without --access-scope ignores the field entirely.
func TestWarnAccessScopeResult(t *testing.T) {
	cases := []struct {
		name       string
		requested  string
		result     map[string]interface{}
		wantQuiet  bool
		wantParts  []string
		wantAbsent []string
	}{
		{
			name: "no scope requested stays silent even if the server reports one",
			result: map[string]interface{}{
				"applied": false, "reason_code": "policy_denied",
			},
			wantQuiet: true,
		},
		{
			name:      "field absent stays silent",
			requested: "tenant",
			wantQuiet: true,
		},
		{
			name:      "applied stays silent",
			requested: "tenant",
			result:    map[string]interface{}{"applied": true},
			wantQuiet: true,
		},
		{
			name:      "policy_denied names the policy and routes to the settings page",
			requested: "tenant",
			result:    map[string]interface{}{"applied": false, "reason_code": "policy_denied"},
			wantParts: []string{
				"--access-scope tenant", "not applied", "policy", "visibility is unchanged",
				"sharing settings page", "Do not create the app again",
			},
			// Routing to an administrator is out of scope for this CLI: the
			// remedy the user can act on is the app's own settings page.
			wantAbsent: []string{"administrator"},
		},
		{
			name:      "downstream_error is reported as a platform failure",
			requested: "public",
			result:    map[string]interface{}{"applied": false, "reason_code": "downstream_error"},
			wantParts: []string{"--access-scope public", "not applied", "could not apply", "settings page"},
		},
		{
			name:       "unsupported_app_type names the app type and offers no next step",
			requested:  "self",
			result:     map[string]interface{}{"applied": false, "reason_code": "unsupported_app_type"},
			wantParts:  []string{"--access-scope self", "not applied", "app type"},
			wantAbsent: []string{"settings page"},
		},
		{
			name:      "unknown reason code degrades to the raw code",
			requested: "tenant",
			result:    map[string]interface{}{"applied": false, "reason_code": "brand_new_code"},
			wantParts: []string{"--access-scope tenant", "not applied", "brand_new_code", "settings page"},
		},
		{
			name:      "missing reason code still warns",
			requested: "tenant",
			result:    map[string]interface{}{"applied": false},
			wantParts: []string{"--access-scope tenant", "not applied", "settings page"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			body := accessScopeResultBody(tc.result)
			warnAccessScopeResult(&buf, tc.requested, body["data"].(map[string]interface{}))
			got := buf.String()
			if tc.wantQuiet {
				if got != "" {
					t.Fatalf("expected no warning, got %q", got)
				}
				return
			}
			for _, want := range tc.wantParts {
				if !strings.Contains(got, want) {
					t.Errorf("warning missing %q, got %q", want, got)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("warning must not offer %q here, got %q", absent, got)
				}
			}
		})
	}
}

// TestWarnAccessScopeResult_ServerMessageStaysOutOfTheTerminal: the server-side
// Message is localized for one brand, so it rides in the JSON envelope for the
// caller to use and never reaches stderr, which this repo keeps English.
func TestWarnAccessScopeResult_ServerMessageStaysOutOfTheTerminal(t *testing.T) {
	var buf bytes.Buffer
	body := accessScopeResultBody(map[string]interface{}{
		"applied": false, "reason_code": "policy_denied", "message": "该应用受管控策略限制",
	})
	warnAccessScopeResult(&buf, "tenant", body["data"].(map[string]interface{}))
	if strings.Contains(buf.String(), "该应用") {
		t.Errorf("server message must not be echoed to stderr, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "not applied") {
		t.Errorf("warning still required, got %q", buf.String())
	}
}

// TestAppsCreate_AccessScopeResultWarnsAndPassesThrough wires the helper into
// +create: stderr carries the warning and the untouched result stays in the
// envelope so an agent can read reason_code and message programmatically.
func TestAppsCreate_AccessScopeResultWarnsAndPassesThrough(t *testing.T) {
	factory, stdout, stderr, reg := newAppsExecuteFactoryWithStderr(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps",
		Body: accessScopeResultBody(map[string]interface{}{
			"applied": false, "reason_code": "policy_denied", "message": "受管控策略限制",
		}),
	})

	if err := runAppsShortcut(t, AppsCreate,
		[]string{"+create", "--name", "Demo", "--app-type", "html", "--access-scope", "tenant", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("a refused scope must not fail the create: %v", err)
	}
	if !strings.Contains(stderr.String(), "not applied") {
		t.Errorf("stderr must warn, got %q", stderr.String())
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			AccessScopeResult struct {
				Applied    bool   `json:"applied"`
				ReasonCode string `json:"reason_code"`
				Message    string `json:"message"`
			} `json:"access_scope_result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v (raw=%q)", err, stdout.String())
	}
	if !env.OK {
		t.Errorf("create must still report ok=true, got %s", stdout.String())
	}
	if env.Data.AccessScopeResult.Applied || env.Data.AccessScopeResult.ReasonCode != "policy_denied" ||
		env.Data.AccessScopeResult.Message != "受管控策略限制" {
		t.Errorf("access_scope_result must pass through untouched, got %s", stdout.String())
	}
}

// TestHTMLDeploy_AccessScopeResultOnCreateBranch: the bare-HTML publish builds
// its own envelope, so the create endpoint's report has to be carried into it
// explicitly. The publish itself must still succeed.
func TestHTMLDeploy_AccessScopeResultOnCreateBranch(t *testing.T) {
	chdirHTMLPayload(t, map[string]string{"report.html": "<h1>hi</h1>"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	factory, stdout, stderr, reg := newAppsExecuteFactoryWithStderr(t)
	stubHasHTMLAppCreated(reg, map[string]interface{}{"exists": false})
	reg.Register(&httpmock.Stub{
		Method:     "POST",
		URL:        "/open-apis/spark/v1/apps",
		BodyFilter: func(b []byte) bool { return bytes.Contains(b, []byte(`"app_type"`)) },
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"app":                 map[string]interface{}{"app_id": "app_new"},
			"access_scope_result": map[string]interface{}{"applied": false, "reason_code": "policy_denied"},
		}},
	})
	stubPreRelease(reg, "app_new", srv.URL, nil)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_new/releases",
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"release_id": "rel_9", "status": "publishing",
		}},
	})

	args := []string{"+deploy", "--file-path", "report.html", "--access-scope", "tenant", "--as", "user"}
	if err := runAppsShortcut(t, AppsDeploy, args, factory, stdout); err != nil {
		t.Fatalf("a refused scope must not fail the publish: %v", err)
	}
	if !strings.Contains(stderr.String(), "not applied") {
		t.Errorf("stderr must warn, got %q", stderr.String())
	}
	data := parseEnvelopeData(t, stdout)
	res, ok := data["access_scope_result"].(map[string]interface{})
	if !ok {
		t.Fatalf("access_scope_result must reach the envelope, got %v", data)
	}
	if res["applied"] != false || res["reason_code"] != "policy_denied" {
		t.Errorf("access_scope_result must pass through untouched, got %v", res)
	}
	if data["release_id"] != "rel_9" {
		t.Errorf("publish must still complete, got %v", data)
	}
}

// TestHTMLDeploy_LookupHitCarriesNoAccessScopeResult: when the idempotency
// lookup finds an existing app nothing was created, so there is no result to
// report and the envelope must not invent one.
func TestHTMLDeploy_LookupHitCarriesNoAccessScopeResult(t *testing.T) {
	chdirHTMLPayload(t, map[string]string{"report.html": "<h1>hi</h1>"})
	srv := newTOSTLSServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	factory, stdout, _, reg := newAppsExecuteFactoryWithStderr(t)
	stubHasHTMLAppCreated(reg, map[string]interface{}{"exists": true, "app_id": "app_hit"})
	stubPreRelease(reg, "app_hit", srv.URL, nil)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_hit/releases",
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"release_id": "rel_10", "status": "publishing",
		}},
	})

	args := []string{"+deploy", "--file-path", "report.html", "--access-scope", "tenant", "--as", "user"}
	if err := runAppsShortcut(t, AppsDeploy, args, factory, stdout); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	data := parseEnvelopeData(t, stdout)
	if _, present := data["access_scope_result"]; present {
		t.Errorf("no app was created, so no result may be reported, got %v", data)
	}
}
