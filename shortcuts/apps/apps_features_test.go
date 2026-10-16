// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/shortcuts/common"
)

const testKey = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

func featureStatusWire(enabled bool) map[string]any {
	return map[string]any{"name": "database", "enabled": enabled}
}

func featuresResponse(feat map[string]any, op map[string]any) map[string]any {
	data := map[string]any{
		"app_id":   "app_x",
		"features": []any{feat},
	}
	if op != nil {
		data["operation"] = op
	}
	return map[string]any{"code": 0, "data": data}
}

func enableResponse(feat map[string]any, op map[string]any) map[string]any {
	data := map[string]any{
		"app_id":  "app_x",
		"feature": feat,
	}
	if op != nil {
		data["operation"] = op
	}
	return map[string]any{"code": 0, "data": data}
}

// echoKeyStub answers with a body built from the request's idempotent_key,
// so responses can echo the key the CLI generated.
func echoKeyStub(t *testing.T, method, url string, build func(key string) map[string]any) *httpmock.Stub {
	t.Helper()
	stub := &httpmock.Stub{Method: method, URL: url}
	stub.BodyFilter = func(body []byte) bool {
		stub.Body = build(sentKey(t, body))
		return true
	}
	return stub
}

// forbidFeaturesGET fails the test if the command reads the features endpoint.
func forbidFeaturesGET(t *testing.T, reg *httpmock.Registry, why string) {
	t.Helper()
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/features", Reusable: true, Optional: true,
		OnMatch: func(*http.Request) { t.Errorf("unexpected GET features: %s", why) }})
}

func forbidEnablePOST(t *testing.T, reg *httpmock.Registry, why string) {
	t.Helper()
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/features/database/enable", Reusable: true, Optional: true,
		OnMatch: func(*http.Request) { t.Errorf("unexpected POST enable: %s", why) }})
}

func sentKey(t *testing.T, body []byte) string {
	t.Helper()
	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode request body %q: %v", body, err)
	}
	key, _ := req["idempotent_key"].(string)
	return key
}

func decodeBody(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("decode body %q: %v", body, err)
	}
	return m
}

func decodeSuccessData(t *testing.T, stdout *bytes.Buffer) map[string]any {
	t.Helper()
	var env struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), err)
	}
	if !env.OK || env.Data == nil {
		t.Fatalf("stdout is not a success envelope: %s", stdout.String())
	}
	return env.Data
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fastFeatureTiming(t *testing.T) {
	t.Helper()
	backoff, poll, wait := featureRetryBackoff, featureEnablePollInterval, featureEnableMaxWait
	featureRetryBackoff = []time.Duration{0, 0}
	featureEnablePollInterval = time.Millisecond
	t.Cleanup(func() {
		featureRetryBackoff, featureEnablePollInterval, featureEnableMaxWait = backoff, poll, wait
	})
}

func requireExit(t *testing.T, err error, want int) *errs.Problem {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error with exit %d", want)
	}
	if got := output.ExitCodeOf(err); got != want {
		t.Fatalf("exit code = %d, want %d (err=%T %v)", got, want, err, err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed error, got %T %v", err, err)
	}
	return p
}

// ── metadata ──

func TestAppsFeatures_MetadataIsReadOnly(t *testing.T) {
	if AppsFeatures.Risk != "read" || !reflect.DeepEqual(AppsFeatures.Scopes, []string{"spark:app:read"}) {
		t.Fatalf("+features risk/scopes = %q/%v, want read/[spark:app:read]", AppsFeatures.Risk, AppsFeatures.Scopes)
	}
}

func TestAppsFeatureEnable_MetadataDeclaresReadAndWriteScopes(t *testing.T) {
	if AppsFeatureEnable.Risk != "write" {
		t.Fatalf("+feature-enable risk = %q, want write", AppsFeatureEnable.Risk)
	}
	if !reflect.DeepEqual(AppsFeatureEnable.Scopes, []string{"spark:app:read", "spark:app:write"}) {
		t.Fatalf("+feature-enable scopes = %v", AppsFeatureEnable.Scopes)
	}
	if featureEnableWaitLimit != 60*time.Second {
		t.Fatalf("wait limit = %s, want 60s", featureEnableWaitLimit)
	}
}

// ── +features ──

func TestAppsFeatures_OutputsOnlyNameAndEnabled(t *testing.T) {
	// A server still sending the removed resource/readiness/revision fields
	// must not leak them into the CLI output.
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/features",
		RawBody: []byte(`{"code":0,"data":{"app_id":"app_x","feature_protocol":1,"config_revision":"7560123456789012345",` +
			`"features":[{"name":"database","enabled":false,"resource_status":"absent","db_env_type":"none","available_environments":[],` +
			`"local_development_ready":false,"can_enable":true,"reason":"","next_action":""}]}}`),
	})

	if err := runAppsShortcut(t, AppsFeatures, []string{"+features", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	data := decodeSuccessData(t, stdout)
	if got := sortedKeys(data); !reflect.DeepEqual(got, []string{"app_id", "features"}) {
		t.Fatalf("data keys = %v, want [app_id features]", got)
	}
	features, _ := data["features"].([]any)
	if len(features) != 1 {
		t.Fatalf("features = %v", data["features"])
	}
	feat, _ := features[0].(map[string]any)
	if !reflect.DeepEqual(feat, map[string]any{"name": "database", "enabled": false}) {
		t.Fatalf("feature = %v, want exactly name/enabled with enabled=false kept", feat)
	}
	reg.Verify(t)
}

func TestAppsFeatures_PrettyShowsNameAndEnabled(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/features", Body: featuresResponse(featureStatusWire(true), nil)})
	if err := runAppsShortcut(t, AppsFeatures, []string{"+features", "--app-id", "app_x", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); got != "app: app_x\ndatabase: enabled=true\n" {
		t.Fatalf("pretty output = %q", got)
	}
}

func TestAppsFeatures_ForwardsOperationRef(t *testing.T) {
	// The ref is sent in the lower-case form the server stores, whatever the
	// caller typed.
	for _, ref := range []string{testKey, strings.ToUpper(testKey)} {
		t.Run(ref, func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/spark/v1/apps/app_x/features?operation_ref=" + testKey,
				Body:   featuresResponse(featureStatusWire(true), map[string]any{"ref": testKey, "state": "succeeded"}),
			})
			if err := runAppsShortcut(t, AppsFeatures, []string{"+features", "--app-id", "app_x", "--operation-ref", ref, "--as", "user"}, factory, stdout); err != nil {
				t.Fatalf("execute err=%v", err)
			}
			data := decodeSuccessData(t, stdout)
			op, _ := data["operation"].(map[string]any)
			if op["ref"] != testKey || op["state"] != "succeeded" {
				t.Fatalf("operation = %v", data["operation"])
			}
			reg.Verify(t)
		})
	}
}

func TestAppsFeatures_RejectsMalformedAnswers(t *testing.T) {
	cases := map[string]string{
		"missing enabled":  `{"code":0,"data":{"app_id":"app_x","features":[{"name":"database"}]}}`,
		"enabled string":   `{"code":0,"data":{"app_id":"app_x","features":[{"name":"database","enabled":"true"}]}}`,
		"missing name":     `{"code":0,"data":{"app_id":"app_x","features":[{"enabled":true}]}}`,
		"missing features": `{"code":0,"data":{"app_id":"app_x"}}`,
		"missing app_id":   `{"code":0,"data":{"features":[]}}`,
		"bad state":        `{"code":0,"data":{"app_id":"app_x","features":[],"operation":{"ref":"k","state":"done"}}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/features", RawBody: []byte(body)})
			err := runAppsShortcut(t, AppsFeatures, []string{"+features", "--app-id", "app_x", "--as", "user"}, factory, stdout)
			p := requireExit(t, err, output.ExitInternal)
			if p.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("subtype = %q, want invalid_response", p.Subtype)
			}
			if stdout.Len() != 0 {
				t.Fatalf("no success output expected, got %s", stdout.String())
			}
		})
	}
}

func TestAppsFeatures_DryRunSendsNothing(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	// No stubs: any request would fail the command.
	if err := runAppsShortcut(t, AppsFeatures, []string{"+features", "--app-id", "app_x", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode dry-run: %v", err)
	}
	if len(env.API) != 1 || env.API[0].Method != "GET" || env.API[0].URL != "/open-apis/spark/v1/apps/app_x/features" {
		t.Fatalf("dry-run plan = %+v", env.API)
	}
	reg.Verify(t)
}

func TestAppsFeatures_RejectsPositionalArgument(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsFeatures, []string{"+features", "database", "--app-id", "app_x", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "positional arguments are not supported") {
		t.Fatalf("err = %v, want positional rejection", err)
	}
}

// ── +feature-enable ──

func TestAppsFeatureEnable_PostsOnlyTheIdempotentKey(t *testing.T) {
	fastFeatureTiming(t)
	t.Setenv(envKeyMiaodaAuthnCode, "")
	factory, stdout, reg := newAppsExecuteFactory(t)
	forbidFeaturesGET(t, reg, "a new request needs no read before the enable")
	post := echoKeyStub(t, "POST", "/open-apis/spark/v1/apps/app_x/features/database/enable", func(key string) map[string]any {
		return enableResponse(featureStatusWire(true), map[string]any{"ref": key, "state": "succeeded"})
	})
	reg.Register(post)

	if err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	body := decodeBody(t, post.CapturedBody)
	if got := sortedKeys(body); !reflect.DeepEqual(got, []string{"idempotent_key"}) {
		t.Fatalf("POST body keys = %v, want only idempotent_key", got)
	}
	key, _ := body["idempotent_key"].(string)
	if !idempotentKeyPattern.MatchString(key) {
		t.Fatalf("idempotent_key %q is not a UUID", key)
	}
	data := decodeSuccessData(t, stdout)
	if got := sortedKeys(data); !reflect.DeepEqual(got, []string{"app_id", "feature", "idempotent_key", "next_steps", "operation"}) {
		t.Fatalf("output keys = %v", got)
	}
	if data["idempotent_key"] != key {
		t.Fatalf("output key = %v, want %s", data["idempotent_key"], key)
	}
	if !reflect.DeepEqual(data["feature"], map[string]any{"name": "database", "enabled": true}) {
		t.Fatalf("feature = %v", data["feature"])
	}
	steps, _ := json.Marshal(data["next_steps"])
	for _, want := range []string{"+env-pull --app-id app_x", "restart", "nothing restarts it automatically", "+db-env-create --app-id app_x --environment dev --dry-run"} {
		if !strings.Contains(string(steps), want) {
			t.Fatalf("next_steps missing %q: %s", want, steps)
		}
	}
	reg.Verify(t)
}

// In a Miaoda sandbox the platform applies the new database and the sandbox
// build has no +env-pull, so success must not send the caller to local steps.
func TestAppsFeatureEnable_SandboxNextStepsLeaveTheDatabaseToThePlatform(t *testing.T) {
	fastFeatureTiming(t)
	t.Setenv(envKeyMiaodaAuthnCode, "sandbox-authn-code")
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(echoKeyStub(t, "POST", "/features/database/enable", func(key string) map[string]any {
		return enableResponse(featureStatusWire(true), map[string]any{"ref": key, "state": "succeeded"})
	}))

	if err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	steps, _ := json.Marshal(decodeSuccessData(t, stdout)["next_steps"])
	for _, want := range []string{"platform applies the new database settings", "verify one real database read/write"} {
		if !strings.Contains(string(steps), want) {
			t.Fatalf("next_steps missing %q: %s", want, steps)
		}
	}
	for _, forbid := range []string{"+env-pull", "+db-env-create", "npm run dev"} {
		if strings.Contains(string(steps), forbid) {
			t.Fatalf("sandbox next_steps must not suggest %q: %s", forbid, steps)
		}
	}
	reg.Verify(t)
}

func TestAppsFeatureEnable_ReadyWithoutOperationSucceeds(t *testing.T) {
	fastFeatureTiming(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/features/database/enable", Body: enableResponse(featureStatusWire(true), nil)})

	if err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	data := decodeSuccessData(t, stdout)
	if _, present := data["operation"]; present {
		t.Fatalf("no operation expected for a no-change enable: %v", data["operation"])
	}
	reg.Verify(t)
}

func TestAppsFeatureEnable_SuccessWithoutEnabledIsNotReportedAsSuccess(t *testing.T) {
	cases := map[string]map[string]any{
		"enabled=false":  featureStatusWire(false),
		"enabled absent": {"name": "database"},
	}
	for name, feat := range cases {
		t.Run(name, func(t *testing.T) {
			fastFeatureTiming(t)
			factory, stdout, reg := newAppsExecuteFactory(t)
			post := echoKeyStub(t, "POST", "/features/database/enable", func(key string) map[string]any {
				return enableResponse(feat, map[string]any{"ref": key, "state": "succeeded"})
			})
			reg.Register(post)
			err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--as", "user"}, factory, stdout)
			p := requireExit(t, err, output.ExitInternal)
			if !strings.Contains(p.Hint, "--operation-ref "+sentKey(t, post.CapturedBody)) {
				t.Fatalf("hint must point to the read-only query for the same key: %q", p.Hint)
			}
			if stdout.Len() != 0 {
				t.Fatalf("no success output expected: %s", stdout.String())
			}
		})
	}
}

func TestAppsFeatureEnable_ServerCodesAreNotRetriedWithANewKey(t *testing.T) {
	cases := []struct {
		name string
		code int
		// wantQuery: only an outcome that may still change points at the
		// read-only query of the same key.
		wantQuery  bool
		wantHint   string
		forbidHint string
	}{
		{name: "key conflict", code: featureCodeKeyConflict, wantHint: "different, superseded or still-running", forbidHint: "--operation-ref"},
		{name: "result unknown", code: featureCodeResultUnknown, wantQuery: true, wantHint: "do not start a new request with another key", forbidHint: "--idempotent-key"},
		{name: "server step failure", code: 50001, wantQuery: true, wantHint: "to resume the same request run"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fastFeatureTiming(t)
			factory, stdout, reg := newAppsExecuteFactory(t)
			post := &httpmock.Stub{Method: "POST", URL: "/features/database/enable", Reusable: true,
				Body: map[string]any{"code": tc.code, "msg": tc.name}}
			reg.Register(post)
			// A 40913 makes the command look up its own key once; this key was
			// never kept, so the conflict itself is reported.
			reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=", Reusable: true, Optional: true,
				Body: map[string]any{"code": featureGatewayCodeBase + featureCodeOperationUnavailable, "msg": "operation_unavailable"}})

			err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--as", "user"}, factory, stdout)
			p := requireExit(t, err, output.ExitAPI)
			if len(post.CapturedBodies) != 1 {
				t.Fatalf("POST sent %d times, want exactly 1", len(post.CapturedBodies))
			}
			key := sentKey(t, post.CapturedBody)
			wants := []string{key, tc.wantHint}
			if tc.wantQuery {
				wants = append(wants, "+features --app-id app_x --operation-ref "+key)
			}
			for _, want := range wants {
				if !strings.Contains(p.Hint, want) {
					t.Fatalf("hint missing %q: %q", want, p.Hint)
				}
			}
			if tc.forbidHint != "" && strings.Contains(p.Hint, tc.forbidHint) {
				t.Fatalf("hint must not suggest %q: %q", tc.forbidHint, p.Hint)
			}
			if stdout.Len() != 0 {
				t.Fatalf("no success output expected: %s", stdout.String())
			}
		})
	}
}

// Studio rejects some enables for good: the app, the key or the request
// itself is the problem. Such answers must say so in plain words, carry a
// specific subtype, and never send the caller to the operation query, which
// cannot change them. The gateway adds 3300000 to every Studio code.
func TestAppsFeatureEnable_ServerRejectionsAreExplainedWithoutTheOperationQuery(t *testing.T) {
	cases := []struct {
		name        string
		code        int
		msg         string
		wantSub     errs.Subtype
		wantMessage []string
		wantHint    []string
		// wantSameKey: the request was not acted on, so re-sending it under
		// the same key is the recovery.
		wantSameKey bool
	}{
		{
			name: "not a new-contract app behind the gateway label", code: 3340914, msg: "feature enable unsupported for this app",
			wantSub:     errs.SubtypeFailedPrecondition,
			wantMessage: []string{"app app_x does not accept enabling its database", "not a new-contract app", "server: feature enable unsupported for this app"},
			wantHint:    []string{"Miaoda cloud development", "will not help"},
		},
		{
			name: "not a new-contract app with the server reason", code: featureCodeEnableUnsupported, msg: "new_contract_required, log_id=20261014000000",
			wantSub:     errs.SubtypeFailedPrecondition,
			wantMessage: []string{"not a new-contract app", "new_contract_required, log_id=20261014000000"},
			wantHint:    []string{"Miaoda cloud development"},
		},
		{
			name: "app identity missing", code: 3340914, msg: "app_identity_missing, log_id=20261014000000",
			wantSub:     errs.SubtypeFailedPrecondition,
			wantMessage: []string{"no resolvable tenant or owner", "app_identity_missing"},
			wantHint:    []string{"report the log_id"},
		},
		{
			name: "feature not offered", code: 3340009, msg: "feature_name unsupported",
			wantSub:     errs.SubtypeFeatureNotAvailable,
			wantMessage: []string{"does not offer", "feature_name unsupported"},
			wantHint:    []string{"+features --app-id app_x"},
		},
		{
			name: "parameters rejected", code: 3340001, msg: "idempotent_key must be a UUID",
			wantSub:     errs.SubtypeInvalidParameters,
			wantMessage: []string{"idempotent_key must be a UUID"},
			wantHint:    []string{"changed nothing"},
		},
		{
			name: "configuration changed", code: 3340912, msg: "config_revision_changed",
			wantSub:  errs.SubtypeConflict,
			wantHint: []string{"configuration changed", "+features --app-id app_x", "decide again"},
		},
		{
			name: "another enable running", code: 3340913, msg: "operation_in_progress, operation_ref=bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			wantSub:  errs.SubtypeConflict,
			wantHint: []string{"still running", "do not send a new request"},
		},
		{
			name: "superseded", code: 3340913, msg: "operation_superseded",
			wantSub:  errs.SubtypeConflict,
			wantHint: []string{"newer enable request replaced"},
		},
		{
			name: "conflict behind the gateway label", code: 3340913, msg: "key_conflict",
			wantSub:  errs.SubtypeConflict,
			wantHint: []string{"different, superseded or still-running", "without --idempotent-key"},
		},
		{
			name: "key no longer usable", code: 3340915, msg: "operation_unavailable",
			wantSub:  errs.SubtypeFailedPrecondition,
			wantHint: []string{"no longer usable", "+features --app-id app_x", "without --idempotent-key", "do not retry with the same key"},
		},
		{
			name: "rate limited", code: 3342900, msg: "rate limit exceeded",
			wantSub:     errs.SubtypeRateLimit,
			wantHint:    []string{"rate limited", "did not act on this request"},
			wantSameKey: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fastFeatureTiming(t)
			factory, stdout, reg := newAppsExecuteFactory(t)
			post := &httpmock.Stub{Method: "POST", URL: "/features/database/enable", Reusable: true, Status: 400,
				Body: map[string]any{"code": tc.code, "msg": tc.msg}}
			reg.Register(post)
			reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=", Reusable: true, Optional: true,
				Body: map[string]any{"code": featureGatewayCodeBase + featureCodeOperationUnavailable, "msg": "operation_unavailable"}})

			err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--as", "user"}, factory, stdout)
			p := requireExit(t, err, output.ExitAPI)
			if len(post.CapturedBodies) != 1 {
				t.Fatalf("POST sent %d times, want exactly 1", len(post.CapturedBodies))
			}
			if p.Subtype != tc.wantSub {
				t.Fatalf("subtype = %q, want %q", p.Subtype, tc.wantSub)
			}
			if p.Code != tc.code {
				t.Fatalf("code = %d, want the wire code %d", p.Code, tc.code)
			}
			if p.Retryable != tc.wantSameKey {
				t.Fatalf("retryable = %t, want %t", p.Retryable, tc.wantSameKey)
			}
			for _, want := range tc.wantMessage {
				if !strings.Contains(p.Message, want) {
					t.Fatalf("message missing %q: %q", want, p.Message)
				}
			}
			for _, want := range tc.wantHint {
				if !strings.Contains(p.Hint, want) {
					t.Fatalf("hint missing %q: %q", want, p.Hint)
				}
			}
			if strings.Contains(p.Hint, "--operation-ref") {
				t.Fatalf("a final answer must not point at the operation query: %q", p.Hint)
			}
			resend := "--idempotent-key " + sentKey(t, post.CapturedBody)
			if got := strings.Contains(p.Hint, resend); got != tc.wantSameKey {
				t.Fatalf("hint names the same-key resend = %t, want %t: %q", got, tc.wantSameKey, p.Hint)
			}
			if stdout.Len() != 0 {
				t.Fatalf("no success output expected: %s", stdout.String())
			}
		})
	}
}

func TestAppsFeatureEnable_NetworkRetryKeepsTheSameKey(t *testing.T) {
	fastFeatureTiming(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	failed := &httpmock.Stub{Method: "POST", URL: "/features/database/enable", Error: errors.New("connection reset by peer")}
	reg.Register(failed)
	ok := echoKeyStub(t, "POST", "/features/database/enable", func(key string) map[string]any {
		return enableResponse(featureStatusWire(true), map[string]any{"ref": key, "state": "succeeded"})
	})
	reg.Register(ok)

	if err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	first, second := decodeBody(t, failed.CapturedBody), decodeBody(t, ok.CapturedBody)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("replayed body differs:\nfirst  %v\nsecond %v", first, second)
	}
}

func TestAppsFeatureEnable_NetworkFailureReportsKeyAndQueryWithoutConnection(t *testing.T) {
	fastFeatureTiming(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	// --idempotent-key resumes: the kept operation is read first; none is kept yet.
	reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey, Body: featuresResponse(featureStatusWire(false), nil)})
	post := &httpmock.Stub{Method: "POST", URL: "/features/database/enable", Reusable: true, Error: errors.New("i/o timeout")}
	reg.Register(post)

	err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", testKey, "--as", "user"}, factory, stdout)
	p := requireExit(t, err, output.ExitNetwork)
	if len(post.CapturedBodies) != 1+len(featureRetryBackoff) {
		t.Fatalf("POST attempts = %d, want %d", len(post.CapturedBodies), 1+len(featureRetryBackoff))
	}
	for _, b := range post.CapturedBodies {
		if got := sentKey(t, b); got != testKey {
			t.Fatalf("attempt used key %q, want the explicit --idempotent-key %q", got, testKey)
		}
	}
	for _, want := range []string{testKey, "+features --app-id app_x --operation-ref " + testKey, "--idempotent-key " + testKey} {
		if !strings.Contains(p.Hint, want) {
			t.Fatalf("hint missing %q: %q", want, p.Hint)
		}
	}
	if strings.Contains(p.Hint, "postgres://") || strings.Contains(p.Message, "postgres://") {
		t.Fatalf("error must not carry a connection string: %v / %q", p.Message, p.Hint)
	}
}

// The server stores keys in lower case and echoes that form as operation.ref.
// A key typed in upper case must still be sent, queried and matched as the
// same key instead of failing the answer as a mismatch.
func TestAppsFeatureEnable_UpperCaseKeyIsSentInTheServerForm(t *testing.T) {
	fastFeatureTiming(t)
	t.Setenv(envKeyMiaodaAuthnCode, "")
	factory, stdout, reg := newAppsExecuteFactory(t)
	get := &httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey, Body: featuresResponse(featureStatusWire(false), nil)}
	reg.Register(get)
	post := echoKeyStub(t, "POST", "/features/database/enable", func(key string) map[string]any {
		return enableResponse(featureStatusWire(true), map[string]any{"ref": key, "state": "succeeded"})
	})
	reg.Register(post)

	err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", strings.ToUpper(testKey), "--as", "user"}, factory, stdout)
	if err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := sentKey(t, post.CapturedBody); got != testKey {
		t.Fatalf("POST idempotent_key = %q, want the lower-case %q", got, testKey)
	}
	if got := decodeSuccessData(t, stdout)["idempotent_key"]; got != testKey {
		t.Fatalf("output idempotent_key = %v, want %q", got, testKey)
	}
	reg.Verify(t)
}

func TestAppsFeatureEnable_RejectsMalformedIdempotentKeyWithoutNetwork(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", "retry-1", "--as", "user"}, factory, stdout)
	p := requireExit(t, err, output.ExitValidation)
	if !strings.Contains(p.Message, "--idempotent-key") {
		t.Fatalf("message = %q", p.Message)
	}
	reg.Verify(t)
}

// The feature is a declared --feature flag: Required and Enum come from the
// shared Flag declaration, so help, completion and machine metadata carry it.
func TestAppsFeatureEnable_DeclaresRequiredFeatureFlagLimitedToDatabase(t *testing.T) {
	var feature *common.Flag
	for i := range AppsFeatureEnable.Flags {
		if AppsFeatureEnable.Flags[i].Name == "feature" {
			feature = &AppsFeatureEnable.Flags[i]
		}
	}
	if feature == nil || !feature.Required || !reflect.DeepEqual(feature.Enum, []string{"database"}) {
		t.Fatalf("--feature = %#v, want a required flag limited to database", feature)
	}

	factory, _, _ := newAppsExecuteFactory(t)
	parent := &cobra.Command{Use: "apps"}
	AppsFeatureEnable.Mount(parent, factory)
	cmd := parent.Commands()[0]
	if cmd.Use != "+feature-enable" {
		t.Fatalf("Use = %q, want +feature-enable with no positional placeholder", cmd.Use)
	}
	fl := cmd.Flags().Lookup("feature")
	if fl == nil {
		t.Fatal("missing --feature flag")
	}
	if !strings.Contains(fl.Usage, "(database)") {
		t.Fatalf("--feature help = %q, want the allowed value (database)", fl.Usage)
	}
	if got := fl.Annotations[cobra.BashCompOneRequiredFlag]; !reflect.DeepEqual(got, []string{"true"}) {
		t.Fatalf("--feature required annotation = %v, want [true]", got)
	}
	if help := cmd.Flags().FlagUsages(); !strings.Contains(help, "--feature") {
		t.Fatalf("--help does not list --feature:\n%s", help)
	}
}

// Every malformed call stops before identity-backed requests: unsupported or
// empty values fail the shared enum/Validate path, a missing flag fails cobra's
// required-flag check, and positional words (such as
// `+feature-enable database`) hit the framework's positional rejection.
func TestAppsFeatureEnable_RejectsMalformedFeatureBeforeNetwork(t *testing.T) {
	typed := map[string][]string{
		"unsupported": {"+feature-enable", "--feature", "redis", "--app-id", "app_x", "--as", "user"},
		"engine name": {"+feature-enable", "--feature", "pg", "--app-id", "app_x", "--as", "user"},
		"untrimmed":   {"+feature-enable", "--feature", " database", "--app-id", "app_x", "--as", "user"},
		"empty":       {"+feature-enable", "--feature", "", "--app-id", "app_x", "--as", "user"},
		"dry-run":     {"+feature-enable", "--feature", "redis", "--app-id", "app_x", "--dry-run", "--as", "user"},
	}
	for name, args := range typed {
		t.Run(name, func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			forbidFeaturesGET(t, reg, "a malformed --feature must fail before any request")
			reg.Register(&httpmock.Stub{Method: "POST", URL: "/enable", Reusable: true, Optional: true,
				OnMatch: func(*http.Request) { t.Errorf("unexpected POST enable for a malformed --feature") }})
			err := runAppsShortcut(t, AppsFeatureEnable, args, factory, stdout)
			requireExit(t, err, output.ExitValidation)
			var verr *errs.ValidationError
			if !errors.As(err, &verr) || verr.Param != "--feature" {
				t.Fatalf("err = %T %v, want a --feature validation error", err, err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want no dry-run plan for a rejected call", stdout.String())
			}
			reg.Verify(t)
		})
	}

	usage := map[string]struct {
		args []string
		want string
	}{
		"missing":         {[]string{"+feature-enable", "--app-id", "app_x", "--as", "user"}, `required flag(s) "feature" not set`},
		"missing dry-run": {[]string{"+feature-enable", "--app-id", "app_x", "--dry-run", "--as", "user"}, `required flag(s) "feature" not set`},
		"extra argument":  {[]string{"+feature-enable", "--feature", "database", "redis", "--app-id", "app_x", "--as", "user"}, "positional arguments are not supported"},
		"positional form": {[]string{"+feature-enable", "database", "--app-id", "app_x", "--as", "user"}, "positional arguments are not supported"},
	}
	for name, tc := range usage {
		t.Run(name, func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			forbidFeaturesGET(t, reg, "a malformed call must fail before any request")
			reg.Register(&httpmock.Stub{Method: "POST", URL: "/enable", Reusable: true, Optional: true,
				OnMatch: func(*http.Request) { t.Errorf("unexpected POST enable for a malformed call") }})
			err := runAppsShortcut(t, AppsFeatureEnable, tc.args, factory, stdout)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			reg.Verify(t)
		})
	}
}

func TestAppsFeatureEnable_DryRunSendsNothing(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode dry-run: %v", err)
	}
	if len(env.API) != 1 || env.API[0].Method != "POST" || env.API[0].URL != "/open-apis/spark/v1/apps/app_x/features/database/enable" {
		t.Fatalf("dry-run plan without a key = %+v, want only the enable POST", env.API)
	}
	body := env.API[0].Body
	if got := sortedKeys(body); !reflect.DeepEqual(got, []string{"idempotent_key"}) {
		t.Fatalf("dry-run body keys = %v, want only idempotent_key", got)
	}
	if key, _ := body["idempotent_key"].(string); idempotentKeyPattern.MatchString(key) {
		t.Fatalf("dry-run must not invent a key, got %q", key)
	}
	reg.Verify(t)

	stdout.Reset()
	if err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", testKey, "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	env = dryRunAPIEnvelope{}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode dry-run: %v", err)
	}
	if len(env.API) != 2 || env.API[0].Method != "GET" || env.API[0].Params["operation_ref"] != testKey ||
		env.API[1].Method != "POST" || env.API[1].Body["idempotent_key"] != testKey {
		t.Fatalf("dry-run plan with a key = %+v, want the kept-operation read then the same-key POST", env.API)
	}
}

func TestAppsFeatureEnable_WaitsOnlyWhileRunning(t *testing.T) {
	fastFeatureTiming(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey, Body: featuresResponse(featureStatusWire(false), nil)})
	post := &httpmock.Stub{Method: "POST", URL: "/features/database/enable",
		Body: enableResponse(featureStatusWire(false), map[string]any{"ref": testKey, "state": "running"})}
	reg.Register(post)
	reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey,
		Body: featuresResponse(featureStatusWire(false), map[string]any{"ref": testKey, "state": "running"})})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey,
		Body: featuresResponse(featureStatusWire(true), map[string]any{"ref": testKey, "state": "succeeded"})})

	if err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", testKey, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	data := decodeSuccessData(t, stdout)
	op, _ := data["operation"].(map[string]any)
	if op["state"] != "succeeded" || !reflect.DeepEqual(data["feature"], map[string]any{"name": "database", "enabled": true}) {
		t.Fatalf("output = %v, want the polled succeeded state", data)
	}
	if len(post.CapturedBodies) != 1 {
		t.Fatalf("waiting must not resend the write, POST count = %d", len(post.CapturedBodies))
	}
	reg.Verify(t)
}

// The server answers 40913 while any enable holds the app. When a network
// replay races this key's own first attempt, the operation kept for the key is
// the real answer and the command follows it instead of reporting a conflict.
func TestAppsFeatureEnable_ConflictAfterReplayFollowsTheOwnOperation(t *testing.T) {
	fastFeatureTiming(t)
	t.Setenv(envKeyMiaodaAuthnCode, "")
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey, Body: featuresResponse(featureStatusWire(false), nil)})
	lost := &httpmock.Stub{Method: "POST", URL: "/features/database/enable", Error: errors.New("connection reset by peer")}
	reg.Register(lost)
	raced := &httpmock.Stub{Method: "POST", URL: "/features/database/enable", Status: 400,
		Body: map[string]any{"code": featureGatewayCodeBase + featureCodeKeyConflict, "msg": "operation conflict"}}
	reg.Register(raced)
	reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey,
		Body: featuresResponse(featureStatusWire(false), map[string]any{"ref": testKey, "state": "running"})})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey,
		Body: featuresResponse(featureStatusWire(true), map[string]any{"ref": testKey, "state": "succeeded"})})
	forbidEnablePOST(t, reg, "the kept operation must be followed, not re-sent")

	if err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", testKey, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	data := decodeSuccessData(t, stdout)
	op, _ := data["operation"].(map[string]any)
	if op["state"] != "succeeded" || data["idempotent_key"] != testKey {
		t.Fatalf("output = %v, want the followed succeeded operation", data)
	}
	if sentKey(t, lost.CapturedBody) != testKey || sentKey(t, raced.CapturedBody) != testKey {
		t.Fatalf("the replay must carry the same key")
	}
	reg.Verify(t)
}

// While polling, the operation is already running on the server, so a failed
// read is an unknown outcome: the hint keeps the caller on the same key and
// never claims the request was rejected or left unapplied.
func TestAppsFeatureEnable_PollFailureKeepsTheOutcomeUnknown(t *testing.T) {
	for name, code := range map[string]int{
		"rate limited":       featureGatewayCodeBase + featureCodeRateLimited,
		"parameter rejected": featureGatewayCodeBase + featureCodeInvalidArgument,
	} {
		t.Run(name, func(t *testing.T) {
			fastFeatureTiming(t)
			factory, stdout, reg := newAppsExecuteFactory(t)
			// --idempotent-key reads the kept operation first: none is kept yet.
			reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey, Body: featuresResponse(featureStatusWire(false), nil)})
			reg.Register(&httpmock.Stub{Method: "POST", URL: "/features/database/enable",
				Body: enableResponse(featureStatusWire(false), map[string]any{"ref": testKey, "state": "running"})})
			reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey, Status: 400,
				Body: map[string]any{"code": code, "msg": name}})

			err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", testKey, "--as", "user"}, factory, stdout)
			p := requireExit(t, err, output.ExitAPI)
			for _, want := range []string{"outcome is unknown", "--idempotent-key " + testKey, "--operation-ref " + testKey} {
				if !strings.Contains(p.Hint, want) {
					t.Fatalf("hint missing %q: %q", want, p.Hint)
				}
			}
			for _, forbid := range []string{"did not act on this request", "changed nothing", "without --idempotent-key"} {
				if strings.Contains(p.Hint, forbid) {
					t.Fatalf("a poll failure must not suggest %q: %q", forbid, p.Hint)
				}
			}
		})
	}
}

// gatewayInternalStub answers like the OpenAPI gateway when the upstream call
// fails without a business answer, which is all a server without the app
// features API can give.
func gatewayInternalStub(method, url string) *httpmock.Stub {
	return &httpmock.Stub{Method: method, URL: url, Status: 500, Reusable: true,
		Body: map[string]any{"code": featureCodeGatewayInternal, "msg": "Internal Error"}}
}

func requireFeatureAPIMissing(t *testing.T, err error) *errs.Problem {
	t.Helper()
	p := requireExit(t, err, output.ExitAPI)
	if p.Subtype != errs.SubtypeFeatureNotAvailable || p.Retryable {
		t.Fatalf("subtype/retryable = %q/%t, want %q/false", p.Subtype, p.Retryable, errs.SubtypeFeatureNotAvailable)
	}
	if !strings.Contains(p.Message, "did not run the app features API") {
		t.Fatalf("message = %q", p.Message)
	}
	for _, want := range []string{"do not retry", "upgraded", "Miaoda cloud development"} {
		if !strings.Contains(p.Hint, want) {
			t.Fatalf("hint missing %q: %q", want, p.Hint)
		}
	}
	for _, forbid := range []string{"--operation-ref", "--idempotent-key "} {
		if strings.Contains(p.Hint, forbid) {
			t.Fatalf("hint must not send the caller back to %q: %q", forbid, p.Hint)
		}
	}
	return p
}

// A server without the app features API answers the gateway's internal error
// to every call. Confirmed by a read-only re-check, that is final: the command
// must not be marked retryable or suggest resuming the same key.
func TestAppsFeatureEnable_ServerWithoutFeatureAPIIsFinal(t *testing.T) {
	t.Run("new request", func(t *testing.T) {
		fastFeatureTiming(t)
		factory, stdout, reg := newAppsExecuteFactory(t)
		post := gatewayInternalStub("POST", "/features/database/enable")
		reg.Register(post)
		get := gatewayInternalStub("GET", "/open-apis/spark/v1/apps/app_x/features")
		reg.Register(get)

		err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--as", "user"}, factory, stdout)
		requireFeatureAPIMissing(t, err)
		if len(post.CapturedBodies) != 1 || len(get.CapturedBodies) != 1 {
			t.Fatalf("POST/GET = %d/%d, want one enable and one read-only re-check", len(post.CapturedBodies), len(get.CapturedBodies))
		}
	})
	t.Run("resume", func(t *testing.T) {
		fastFeatureTiming(t)
		factory, stdout, reg := newAppsExecuteFactory(t)
		get := gatewayInternalStub("GET", "/open-apis/spark/v1/apps/app_x/features")
		reg.Register(get)
		forbidEnablePOST(t, reg, "a server without the API must not get the write")

		err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", testKey, "--as", "user"}, factory, stdout)
		requireFeatureAPIMissing(t, err)
		if len(get.CapturedBodies) != 2 {
			t.Fatalf("GET = %d, want the kept-operation read and one re-check", len(get.CapturedBodies))
		}
	})
}

// The same gateway error from a server that does serve the API is a passing
// failure: the read-only re-check succeeds, and the enable keeps its
// same-key resume guidance.
func TestAppsFeatureEnable_PassingGatewayFailureKeepsTheResumeHint(t *testing.T) {
	fastFeatureTiming(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	post := gatewayInternalStub("POST", "/features/database/enable")
	reg.Register(post)
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/features", Body: featuresResponse(featureStatusWire(false), nil)})

	err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--as", "user"}, factory, stdout)
	p := requireExit(t, err, output.ExitAPI)
	if p.Subtype != errs.SubtypeServerError || !p.Retryable {
		t.Fatalf("subtype/retryable = %q/%t, want the classifier's %q/true", p.Subtype, p.Retryable, errs.SubtypeServerError)
	}
	key := sentKey(t, post.CapturedBody)
	for _, want := range []string{"to resume the same request run", "--idempotent-key " + key, "--operation-ref " + key} {
		if !strings.Contains(p.Hint, want) {
			t.Fatalf("hint missing %q: %q", want, p.Hint)
		}
	}
	reg.Verify(t)
}

func TestAppsFeatures_ServerWithoutFeatureAPIIsFinal(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	get := gatewayInternalStub("GET", "/open-apis/spark/v1/apps/app_x/features")
	reg.Register(get)

	err := runAppsShortcut(t, AppsFeatures, []string{"+features", "--app-id", "app_x", "--as", "user"}, factory, stdout)
	requireFeatureAPIMissing(t, err)
	if len(get.CapturedBodies) != 2 {
		t.Fatalf("GET = %d, want the read and one re-check", len(get.CapturedBodies))
	}
}

func TestAppsFeatures_PassingGatewayFailureIsReadAgain(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/features", Status: 500,
		Body: map[string]any{"code": featureCodeGatewayInternal, "msg": "Internal Error"}})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/features", Body: featuresResponse(featureStatusWire(true), nil)})

	if err := runAppsShortcut(t, AppsFeatures, []string{"+features", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := decodeSuccessData(t, stdout)["features"]; !reflect.DeepEqual(got, []any{map[string]any{"name": "database", "enabled": true}}) {
		t.Fatalf("features = %v", got)
	}
	reg.Verify(t)
}

// requireFeatureReauthorization checks that an authentication failure keeps
// the CLI's own re-authorization guidance and never sends the caller round
// the same key: no key can succeed until the user authorizes again.
func requireFeatureReauthorization(t *testing.T, err error, code int) {
	t.Helper()
	p := requireExit(t, err, output.ExitAuth)
	if p.Category != errs.CategoryAuthentication || p.Retryable || p.Code != code {
		t.Fatalf("problem = %+v, want a non-retryable authentication error with code %d", p, code)
	}
	if want := recovery.UserAuthorization().String(); p.Hint != want {
		t.Fatalf("hint = %q, want the CLI's re-authorization guidance %q", p.Hint, want)
	}
	for _, forbid := range []string{"--idempotent-key", "--operation-ref", "resume"} {
		if strings.Contains(p.Hint, forbid) {
			t.Fatalf("hint must not suggest %q: %q", forbid, p.Hint)
		}
	}
}

func TestAppsFeatures_AuthenticationFailureAsksForReauthorization(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   map[string]any
		code   int
	}{
		// What the Miaoda sandbox proxy answers once the user's credential can
		// no longer be refreshed.
		{name: "Miaoda credential expired", code: 221801,
			body: map[string]any{"code": 221801, "msg": "miaoda UAT expired and cannot refresh, need reactivate"}},
		{name: "user token expired", code: 99991677,
			body: map[string]any{"code": 99991677, "msg": "user access token expired"}},
		// What the Miaoda sandbox proxy answers for a credential it cannot
		// accept: a bare HTTP 401 without an API envelope.
		{name: "credential rejected by the proxy", status: 401, code: 401,
			body: map[string]any{"lang_id": 1033, "status_code": "k_gw_ec_000032", "error_msg": "Invalid Parameters"}},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/enable", func(t *testing.T) {
			fastFeatureTiming(t)
			factory, stdout, reg := newAppsExecuteFactory(t)
			post := &httpmock.Stub{Method: "POST", URL: "/features/database/enable", Reusable: true, Status: tc.status, Body: tc.body}
			reg.Register(post)
			forbidFeaturesGET(t, reg, "an authentication failure needs no operation lookup")

			err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--as", "user"}, factory, stdout)
			requireFeatureReauthorization(t, err, tc.code)
			if len(post.CapturedBodies) != 1 {
				t.Fatalf("POST sent %d times, want exactly 1", len(post.CapturedBodies))
			}
		})
		t.Run(tc.name+"/resume read", func(t *testing.T) {
			fastFeatureTiming(t)
			factory, stdout, reg := newAppsExecuteFactory(t)
			reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey, Status: tc.status, Body: tc.body})
			forbidEnablePOST(t, reg, "an authentication failure must not reach the write")

			err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", testKey, "--as", "user"}, factory, stdout)
			requireFeatureReauthorization(t, err, tc.code)
		})
		for _, extra := range [][]string{nil, {"--operation-ref", testKey}} {
			t.Run(tc.name+"/features"+strings.Join(extra, "="), func(t *testing.T) {
				factory, stdout, reg := newAppsExecuteFactory(t)
				reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/features", Status: tc.status, Body: tc.body})

				args := append([]string{"+features", "--app-id", "app_x", "--as", "user"}, extra...)
				err := runAppsShortcut(t, AppsFeatures, args, factory, stdout)
				requireFeatureReauthorization(t, err, tc.code)
			})
		}
	}
}

func TestAppsFeatureEnable_WaitTimeoutKeepsKeyAndDoesNotClaimRollback(t *testing.T) {
	fastFeatureTiming(t)
	featureEnableMaxWait = 20 * time.Millisecond
	factory, stdout, reg := newAppsExecuteFactory(t)
	running := map[string]any{"ref": testKey, "state": "running"}
	reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey, Body: featuresResponse(featureStatusWire(false), nil)})
	post := &httpmock.Stub{Method: "POST", URL: "/features/database/enable", Reusable: true,
		Body: enableResponse(featureStatusWire(false), running)}
	reg.Register(post)
	reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey, Reusable: true, Optional: true,
		Body: featuresResponse(featureStatusWire(false), running)})

	err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", testKey, "--as", "user"}, factory, stdout)
	p := requireExit(t, err, output.ExitNetwork)
	if p.Subtype != errs.SubtypeNetworkTimeout {
		t.Fatalf("subtype = %q, want timeout", p.Subtype)
	}
	if !strings.Contains(p.Message, testKey) || !strings.Contains(p.Hint, "not cancelled or rolled back") || !strings.Contains(p.Hint, "--operation-ref "+testKey) {
		t.Fatalf("timeout must keep the key and the query: %q / %q", p.Message, p.Hint)
	}
	if len(post.CapturedBodies) != 1 {
		t.Fatalf("POST count = %d, want 1", len(post.CapturedBodies))
	}
}

func TestAppsFeatureEnable_TerminalOperationStatesAreNotSuccessAndNotWaited(t *testing.T) {
	cases := []struct {
		state      string
		wantSub    errs.Subtype
		forbidHint string
	}{
		{state: "failed", wantSub: errs.SubtypeUnknown},
		{state: "unknown", wantSub: errs.SubtypeUnknown, forbidHint: "--idempotent-key"},
		{state: "conflicted", wantSub: errs.SubtypeConflict},
	}
	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			fastFeatureTiming(t)
			factory, stdout, reg := newAppsExecuteFactory(t)
			forbidFeaturesGET(t, reg, "a terminal state must not be polled")
			post := echoKeyStub(t, "POST", "/features/database/enable", func(key string) map[string]any {
				return enableResponse(featureStatusWire(false), map[string]any{"ref": key, "state": tc.state, "error_reason": "resource_request_unknown"})
			})
			reg.Register(post)
			err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--as", "user"}, factory, stdout)
			p := requireExit(t, err, output.ExitAPI)
			if p.Subtype != tc.wantSub {
				t.Fatalf("subtype = %q, want %q", p.Subtype, tc.wantSub)
			}
			if tc.forbidHint != "" && strings.Contains(p.Hint, tc.forbidHint) {
				t.Fatalf("%s hint must not suggest %q: %q", tc.state, tc.forbidHint, p.Hint)
			}
			if !strings.Contains(p.Hint, "idempotent_key="+sentKey(t, post.CapturedBody)) {
				t.Fatalf("hint must name the key: %q", p.Hint)
			}
			if stdout.Len() != 0 {
				t.Fatalf("no success output for %s: %s", tc.state, stdout.String())
			}
		})
	}
}

func TestAppsFeatureEnable_ConflictedWhileWaitingStopsImmediately(t *testing.T) {
	fastFeatureTiming(t)
	featureEnableMaxWait = time.Hour // a terminal state must end the wait, not the deadline
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey, Body: featuresResponse(featureStatusWire(false), nil)})
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/features/database/enable",
		Body: enableResponse(featureStatusWire(false), map[string]any{"ref": testKey, "state": "running"})})
	poll := &httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey, Reusable: true,
		Body: featuresResponse(featureStatusWire(false), map[string]any{"ref": testKey, "state": "conflicted", "error_reason": "superseded"})}
	reg.Register(poll)

	err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", testKey, "--as", "user"}, factory, stdout)
	p := requireExit(t, err, output.ExitAPI)
	if p.Subtype != errs.SubtypeConflict {
		t.Fatalf("subtype = %q, want conflict", p.Subtype)
	}
	if len(poll.CapturedBodies) != 1 {
		t.Fatalf("polled %d times after conflicted, want 1", len(poll.CapturedBodies))
	}
}

func TestAppsFeatureEnable_ResumeUsesTheKeptOperation(t *testing.T) {
	cases := []struct {
		name     string
		op       map[string]any
		enabled  bool
		wantExit int
		wantPost bool
	}{
		{name: "succeeded is reported", op: map[string]any{"ref": testKey, "state": "succeeded"}, enabled: true},
		{name: "unknown is reported", op: map[string]any{"ref": testKey, "state": "unknown"}, wantExit: output.ExitAPI},
		{name: "conflicted is reported", op: map[string]any{"ref": testKey, "state": "conflicted"}, wantExit: output.ExitAPI},
		{name: "non-retryable failure is reported", op: map[string]any{"ref": testKey, "state": "failed", "retryable": false}, wantExit: output.ExitAPI},
		{name: "retryable failure resumes", op: map[string]any{"ref": testKey, "state": "failed", "retryable": true}, wantPost: true},
		{name: "no kept operation sends", op: nil, wantPost: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fastFeatureTiming(t)
			factory, stdout, reg := newAppsExecuteFactory(t)
			reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/features?operation_ref=" + testKey,
				Body: featuresResponse(featureStatusWire(tc.enabled), tc.op)})
			post := &httpmock.Stub{Method: "POST", URL: "/features/database/enable", Optional: !tc.wantPost,
				Body: enableResponse(featureStatusWire(true), map[string]any{"ref": testKey, "state": "succeeded"})}
			reg.Register(post)

			err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", testKey, "--as", "user"}, factory, stdout)
			if tc.wantExit != 0 {
				requireExit(t, err, tc.wantExit)
				if stdout.Len() != 0 {
					t.Fatalf("no success output expected: %s", stdout.String())
				}
			} else if err != nil {
				t.Fatalf("execute err=%v", err)
			}
			if sent := len(post.CapturedBodies); (sent > 0) != tc.wantPost {
				t.Fatalf("POST sent %d times, wantPost=%t", sent, tc.wantPost)
			}
			if tc.wantPost {
				body := decodeBody(t, post.CapturedBody)
				if !reflect.DeepEqual(body, map[string]any{"idempotent_key": testKey}) {
					t.Fatalf("resumed POST body = %v, want the same key only", body)
				}
			}
			reg.Verify(t)
		})
	}
}

func TestAppsFeatureEnable_ResumeOfARunningOperationWaitsWithoutResending(t *testing.T) {
	fastFeatureTiming(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey,
		Body: featuresResponse(featureStatusWire(false), map[string]any{"ref": testKey, "state": "running"})})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey,
		Body: featuresResponse(featureStatusWire(true), map[string]any{"ref": testKey, "state": "succeeded"})})
	forbidEnablePOST(t, reg, "a running operation is waited on, not re-sent")

	if err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", testKey, "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if data := decodeSuccessData(t, stdout); data["idempotent_key"] != testKey {
		t.Fatalf("output key = %v", data["idempotent_key"])
	}
	reg.Verify(t)
}

// featureKeyNotKeptHintParts is what every "key not kept" answer must tell the
// caller: look at the current state, start over without the key if still
// needed, and never retry the same key.
func featureKeyNotKeptHintParts(key string) []string {
	return []string{"idempotent_key=" + key, "no longer usable", "+features --app-id app_x", "without --idempotent-key", "do not retry with the same key"}
}

func TestAppsFeatureEnable_ResumeOfAnUnavailableOperationDoesNotSend(t *testing.T) {
	cases := []struct {
		name    string
		code    int
		status  int
		wantSub errs.Subtype
	}{
		// Studio's own code for operation_unavailable; the gateway passes the
		// message through and adds its 33 prefix.
		{name: "40915", code: featureCodeOperationUnavailable, status: 409, wantSub: errs.SubtypeFailedPrecondition},
		{name: "40915 behind the gateway", code: featureGatewayCodeBase + featureCodeOperationUnavailable, status: 409, wantSub: errs.SubtypeFailedPrecondition},
		// A 40913 led by the same reason keeps its conflict classification.
		{name: "40913 led by the reason", code: featureCodeKeyConflict, wantSub: errs.SubtypeConflict},
		{name: "40913 behind the gateway", code: featureGatewayCodeBase + featureCodeKeyConflict, wantSub: errs.SubtypeConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fastFeatureTiming(t)
			factory, stdout, reg := newAppsExecuteFactory(t)
			// The server's own message for a ref it does not keep: reason, then the ref.
			msg := featureReasonOperationUnavailable + ", operation_ref=" + testKey
			get := &httpmock.Stub{Method: "GET", URL: "operation_ref=" + testKey, Status: tc.status,
				Body: map[string]any{"code": tc.code, "msg": msg}}
			reg.Register(get)
			forbidEnablePOST(t, reg, "a key the server does not keep must not be re-sent")

			err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", testKey, "--as", "user"}, factory, stdout)
			p := requireExit(t, err, output.ExitAPI)
			if len(get.CapturedBodies) != 1 {
				t.Fatalf("GET sent %d times, want exactly 1", len(get.CapturedBodies))
			}
			if p.Message != msg {
				t.Fatalf("message = %q, want the server's %q", p.Message, msg)
			}
			if p.Subtype != tc.wantSub {
				t.Fatalf("subtype = %q, want %q", p.Subtype, tc.wantSub)
			}
			if p.Retryable {
				t.Fatalf("a key that is not kept must not be reported as retryable")
			}
			for _, want := range featureKeyNotKeptHintParts(testKey) {
				if !strings.Contains(p.Hint, want) {
					t.Fatalf("hint missing %q: %q", want, p.Hint)
				}
			}
			if strings.Contains(p.Hint, "--idempotent-key "+testKey) {
				t.Fatalf("hint must not resend the same key: %q", p.Hint)
			}
			reg.Verify(t)
		})
	}
}

// +features --operation-ref reads the same operation; a key the server no
// longer keeps gets the same final explanation there.
func TestAppsFeatures_UnavailableOperationRefIsExplained(t *testing.T) {
	for name, code := range map[string]int{
		"studio code":  featureCodeOperationUnavailable,
		"gateway code": featureGatewayCodeBase + featureCodeOperationUnavailable,
	} {
		t.Run(name, func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			msg := featureReasonOperationUnavailable + ", operation_ref=" + testKey
			reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/features?operation_ref=" + testKey, Status: 409,
				Body: map[string]any{"code": code, "msg": msg}})

			err := runAppsShortcut(t, AppsFeatures, []string{"+features", "--app-id", "app_x", "--operation-ref", testKey, "--as", "user"}, factory, stdout)
			p := requireExit(t, err, output.ExitAPI)
			if p.Message != msg {
				t.Fatalf("message = %q, want the server's %q", p.Message, msg)
			}
			if p.Subtype != errs.SubtypeFailedPrecondition || p.Retryable {
				t.Fatalf("subtype/retryable = %q/%t, want %q/false", p.Subtype, p.Retryable, errs.SubtypeFailedPrecondition)
			}
			for _, want := range featureKeyNotKeptHintParts(testKey) {
				if !strings.Contains(p.Hint, want) {
					t.Fatalf("hint missing %q: %q", want, p.Hint)
				}
			}
			if stdout.Len() != 0 {
				t.Fatalf("no success output expected: %s", stdout.String())
			}
			reg.Verify(t)
		})
	}
}

// A failed read of the kept operation does not tell whether the key is kept,
// so it must keep the real cause and the same key: no POST, no new key, no
// second query, and no hint toward a new attempt.
func TestAppsFeatureEnable_ResumeReadFailureKeepsTheCauseAndTheKey(t *testing.T) {
	cases := []struct {
		name        string
		stub        httpmock.Stub
		wantExit    int
		wantGETs    int
		wantMessage string
		// ownGuidance: authorization failures keep the classifier's own hint.
		ownGuidance bool
	}{
		{
			name:        "server internal error",
			stub:        httpmock.Stub{Status: 500, Body: map[string]any{"code": 50001, "msg": "upstream_internal"}},
			wantExit:    output.ExitAPI,
			wantGETs:    1,
			wantMessage: "upstream_internal",
		},
		{
			name:        "gateway forbidden page",
			stub:        httpmock.Stub{Status: 403, Headers: http.Header{"Content-Type": []string{"text/plain"}}, RawBody: []byte("forbidden")},
			wantExit:    output.ExitAPI,
			wantGETs:    1,
			wantMessage: "HTTP 403: forbidden",
		},
		{
			name: "operation conflict that is not unavailable",
			stub: httpmock.Stub{Body: map[string]any{"code": featureCodeKeyConflict,
				"msg": "operation_in_progress, operation_ref=" + testKey}},
			wantExit:    output.ExitAPI,
			wantGETs:    1,
			wantMessage: "operation_in_progress, operation_ref=" + testKey,
		},
		{
			name:     "network failure after replays",
			stub:     httpmock.Stub{Error: errors.New("connection reset by peer")},
			wantExit: output.ExitNetwork,
			wantGETs: 1 + len(featureRetryBackoff),
		},
		{
			name: "missing scope",
			stub: httpmock.Stub{Body: map[string]any{"code": 99991679, "msg": "scope missing",
				"error": map[string]any{"permission_violations": []any{map[string]any{"subject": "spark:app:read"}}}}},
			wantExit:    output.ExitAuth,
			wantGETs:    1,
			ownGuidance: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fastFeatureTiming(t)
			factory, stdout, reg := newAppsExecuteFactory(t)
			get := tc.stub
			get.Method, get.URL, get.Reusable = "GET", "operation_ref="+testKey, true
			reg.Register(&get)
			forbidEnablePOST(t, reg, "the outcome of the kept operation is unknown")

			err := runAppsShortcut(t, AppsFeatureEnable, []string{"+feature-enable", "--feature", "database", "--app-id", "app_x", "--idempotent-key", testKey, "--as", "user"}, factory, stdout)
			p := requireExit(t, err, tc.wantExit)
			if len(get.CapturedBodies) != tc.wantGETs {
				t.Fatalf("GET sent %d times, want %d", len(get.CapturedBodies), tc.wantGETs)
			}
			if tc.wantMessage != "" && p.Message != tc.wantMessage {
				t.Fatalf("message = %q, want the real cause %q", p.Message, tc.wantMessage)
			}
			for _, forbid := range []string{"new attempt", "without --idempotent-key", "not kept"} {
				if strings.Contains(p.Hint, forbid) {
					t.Fatalf("hint must not suggest %q: %q", forbid, p.Hint)
				}
			}
			if !tc.ownGuidance {
				for _, want := range []string{"outcome is unknown", "--idempotent-key " + testKey, "+features --app-id app_x --operation-ref " + testKey} {
					if !strings.Contains(p.Hint, want) {
						t.Fatalf("hint missing %q: %q", want, p.Hint)
					}
				}
			}
			if stdout.Len() != 0 {
				t.Fatalf("no success output expected: %s", stdout.String())
			}
		})
	}
}
