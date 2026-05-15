// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package tracking

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/larksuite/cli/internal/build"
)

func TestLogDir_UsesValidatedLogDirEnv(t *testing.T) {
	base := t.TempDir()
	base, _ = filepath.EvalSymlinks(base)
	t.Setenv("LARKSUITE_CLI_LOG_DIR", filepath.Join(base, "logs", "..", "auth"))
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", "")

	got := logDir()
	want := filepath.Join(base, "auth")
	if got != want {
		t.Fatalf("logDir() = %q, want %q", got, want)
	}
}

func TestLogDir_InvalidLogDirFallsBackToConfigDir(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_LOG_DIR", "relative-logs")
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	got := logDir()
	want := filepath.Join(configDir, "logs")
	if got != want {
		t.Fatalf("logDir() = %q, want %q", got, want)
	}
}

func TestBuildRemoteAuthPayload_Response(t *testing.T) {
	event := authEvent{
		Kind:    authEventResponse,
		Time:    time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		Cmdline: "lark-cli auth login ...",
		Path:    "/open-apis/authen/v1/user_info",
		Status:  200,
		LogID:   "log-123",
	}

	payload, err := buildRemoteAuthPayload(event, "uuid-123")
	if err != nil {
		t.Fatalf("buildRemoteAuthPayload() error = %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("payload len = %d, want 1", len(payload))
	}
	item := payload[0]
	if item.User.UserUniqueID != "uuid-123" {
		t.Fatalf("user_unique_id = %q, want %q", item.User.UserUniqueID, "uuid-123")
	}
	if item.Header.AppName != "" {
		t.Fatalf("app_name = %q, want empty", item.Header.AppName)
	}
	if item.Caller != remoteCaller {
		t.Fatalf("caller = %q, want %q", item.Caller, remoteCaller)
	}
	if len(item.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(item.Events))
	}
	if item.Events[0].Event != "cli_auth_response" {
		t.Fatalf("event = %q, want %q", item.Events[0].Event, "cli_auth_response")
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(item.Events[0].Params), &params); err != nil {
		t.Fatalf("params is not valid JSON string: %v", err)
	}
	if params["path"] != event.Path {
		t.Fatalf("params.path = %v, want %q", params["path"], event.Path)
	}
	if int(params["status"].(float64)) != event.Status {
		t.Fatalf("params.status = %v, want %d", params["status"], event.Status)
	}
	if params["x_tt_logid"] != event.LogID {
		t.Fatalf("params.x_tt_logid = %v, want %q", params["x_tt_logid"], event.LogID)
	}
	if params["lark_cli_version"] != build.Version {
		t.Fatalf("params.lark_cli_version = %v, want %q", params["lark_cli_version"], build.Version)
	}
}

func TestCheckOfflineAccess(t *testing.T) {
	if OfflineAccessChecker != nil {
		t.Fatal("OfflineAccessChecker should be nil by default")
	}
	if checkOfflineAccess() {
		t.Fatal("checkOfflineAccess() = true when checker is nil, want false")
	}

	OfflineAccessChecker = func() bool { return true }
	if !checkOfflineAccess() {
		t.Fatal("checkOfflineAccess() = false, want true")
	}

	OfflineAccessChecker = func() bool { panic("boom") }
	if checkOfflineAccess() {
		t.Fatal("checkOfflineAccess() = true after panic, want false")
	}

	OfflineAccessChecker = nil
}

func TestCheckOfflineAccessForError(t *testing.T) {
	OfflineAccessChecker = func() bool { return true }

	if checkOfflineAccessForError(AuthComponentKeychain) {
		t.Fatal("checkOfflineAccessForError(keychain) = true, want false")
	}
	if !checkOfflineAccessForError(AuthComponentAuth) {
		t.Fatal("checkOfflineAccessForError(auth) = false, want true")
	}

	OfflineAccessChecker = nil
}

func TestBuildRemoteAuthPayload_HasOfflineAccess(t *testing.T) {
	event := authEvent{
		Kind:             authEventResponse,
		Time:             time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		Cmdline:          "lark-cli auth login ...",
		Path:             "/open-apis/authen/v2/oauth/token",
		Status:           200,
		LogID:            "log-789",
		HasOfflineAccess: true,
	}

	payload, err := buildRemoteAuthPayload(event, "uuid-123")
	if err != nil {
		t.Fatalf("buildRemoteAuthPayload() error = %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("payload len = %d, want 1", len(payload))
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(payload[0].Events[0].Params), &params); err != nil {
		t.Fatalf("params is not valid JSON string: %v", err)
	}
	got, ok := params["has_offline_access"]
	if !ok {
		t.Fatal("has_offline_access not found in params")
	}
	if got != true {
		t.Fatalf("has_offline_access = %v, want true", got)
	}
}

func TestPostRemoteAuthEvent_PostsExpectedPayload(t *testing.T) {
	event := authEvent{
		Kind:      authEventError,
		Time:      time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		Cmdline:   "lark-cli auth login ...",
		Component: AuthComponentAuth,
		Op:        AuthOpPermissionDenied,
		Error:     "reason=permission_denied",
	}

	var gotMethod, gotContentType string
	var gotPayload []remoteRequestItem
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	restore := SetAuthLogRemoteHooksForTest(server.Client(), "feishu", func() (string, error) {
		return "uuid-456", nil
	}, true)
	defer restore()

	if err := postRemoteAuthEvent(event, server.URL); err != nil {
		t.Fatalf("postRemoteAuthEvent() error = %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content-type = %q, want application/json", gotContentType)
	}
	if len(gotPayload) != 1 {
		t.Fatalf("payload len = %d, want 1", len(gotPayload))
	}
	if gotPayload[0].Header.AppName != "" {
		t.Fatalf("app_name = %q, want empty", gotPayload[0].Header.AppName)
	}
	if gotPayload[0].User.UserUniqueID != "uuid-456" {
		t.Fatalf("user_unique_id = %q, want %q", gotPayload[0].User.UserUniqueID, "uuid-456")
	}
	if gotPayload[0].Events[0].Event != "cli_auth_error" {
		t.Fatalf("event = %q, want %q", gotPayload[0].Events[0].Event, "cli_auth_error")
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(gotPayload[0].Events[0].Params), &params); err != nil {
		t.Fatalf("params is not valid JSON string: %v", err)
	}
	if params["lark_cli_version"] != build.Version {
		t.Fatalf("params.lark_cli_version = %v, want %q", params["lark_cli_version"], build.Version)
	}
}

func TestEmitRemoteAuthEvent_FailOpenOnTransportError(t *testing.T) {
	restore := SetAuthLogRemoteHooksForTest(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})}, "feishu", func() (string, error) {
		return "uuid-789", nil
	}, true)
	defer restore()

	emitRemoteAuthEvent(authEvent{Kind: authEventResponse, Time: time.Now(), Cmdline: "lark-cli auth status", Path: "/foo", Status: 200})
}

func TestPostRemoteAuthEvent_FallbackGeneratesUUIDv7(t *testing.T) {
	event := authEvent{
		Kind:    authEventResponse,
		Time:    time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		Cmdline: "lark-cli auth status",
		Path:    "/foo",
		Status:  200,
	}

	var gotPayload []remoteRequestItem
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	restore := SetAuthLogRemoteHooksForTest(server.Client(), "feishu", func() (string, error) {
		return "", errors.New("provider unavailable")
	}, true)
	defer restore()

	if err := postRemoteAuthEvent(event, server.URL); err != nil {
		t.Fatalf("postRemoteAuthEvent() error = %v", err)
	}
	if len(gotPayload) != 1 {
		t.Fatalf("payload len = %d, want 1", len(gotPayload))
	}
	parsed, err := uuid.Parse(gotPayload[0].User.UserUniqueID)
	if err != nil {
		t.Fatalf("uuid.Parse(user_unique_id) error = %v", err)
	}
	if parsed.Version() != 7 {
		t.Fatalf("uuid version = %d, want 7", parsed.Version())
	}
}

func TestEmitRemoteAuthEvent_SkipsWhenEndpointProviderDisablesReporting(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	restore := SetAuthLogRemoteHooksForTest(server.Client(), "lark", func() (string, error) {
		return "uuid-123", nil
	}, true)
	defer restore()

	emitRemoteAuthEvent(authEvent{Kind: authEventResponse, Time: time.Now(), Cmdline: "lark-cli auth status", Path: "/foo", Status: 200})
	if called {
		t.Fatal("remote endpoint called, want skipped")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestFormatCmdline_TruncatesExtraArgs(t *testing.T) {
	got := formatCmdline([]string{
		"lark-cli",
		"auth",
		"login",
		"--device-code", "device-code-secret",
		"--app-secret=top-secret",
		"--scope", "contact:read",
	})

	want := "lark-cli auth login ..."
	if got != want {
		t.Fatalf("formatCmdline() = %q, want %q", got, want)
	}
}

func TestResolveTelemetryEndpoint(t *testing.T) {
	if got := resolveTelemetryEndpoint("feishu"); got != "https://mcs-bd.feishu.cn/v1/list" {
		t.Errorf("resolveTelemetryEndpoint(feishu) = %q, want https://mcs-bd.feishu.cn/v1/list", got)
	}
	if got := resolveTelemetryEndpoint("lark"); got != "" {
		t.Errorf("resolveTelemetryEndpoint(lark) = %q, want empty", got)
	}
	if got := resolveTelemetryEndpoint(""); got != "" {
		t.Errorf("resolveTelemetryEndpoint(\"\") = %q, want empty", got)
	}
	if got := resolveTelemetryEndpoint("unknown"); got != "" {
		t.Errorf("resolveTelemetryEndpoint(unknown) = %q, want empty", got)
	}
}
