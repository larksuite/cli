// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

type restoreRoundTripFunc func(*http.Request) (*http.Response, error)

func (f restoreRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type recordingKeychain struct {
	setCalls int
	account  string
	value    string
}

func (k *recordingKeychain) Get(_, _ string) (string, error) { return "", nil }
func (k *recordingKeychain) Remove(_, _ string) error        { return nil }
func (k *recordingKeychain) Set(_, account, value string) error {
	k.setCalls++
	k.account = account
	k.value = value
	return nil
}

func TestConfigInitRestoreUpdatesSelectedProfile(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARK_CLI_NO_PROXY", "")
	original := &core.MultiAppConfig{
		CurrentApp: "other",
		Apps: []core.AppConfig{
			{Name: "target", AppId: "cli_target", AppSecret: core.PlainSecret("old-secret"), Brand: core.BrandFeishu,
				Users: []core.AppUser{{UserOpenId: "ou_target", UserName: "Target"}}},
			{Name: "other", AppId: "cli_other", AppSecret: core.PlainSecret("other-secret"), Brand: core.BrandLark},
		},
	}
	if err := core.SaveMultiAppConfig(original); err != nil {
		t.Fatal(err)
	}

	var beginAppID string
	replaceRestoreDefaultTransport(t, restoreRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.ParseForm(); err != nil {
			t.Fatal(err)
		}
		switch req.Form.Get("action") {
		case "begin":
			beginAppID = req.Form.Get("app_id")
			return restoreJSONResponse(`{"device_code":"device","user_code":"TEST-CODE","expire_in":30,"interval":0}`), nil
		case "poll":
			return restoreJSONResponse(`{"client_id":"cli_target","client_secret":"restored-secret"}`), nil
		default:
			t.Fatalf("unexpected action %q", req.Form.Get("action"))
			return nil, nil
		}
	}))

	kc := &recordingKeychain{}
	rt := &fakeRT{}
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	f.Keychain = kc
	f.Invocation = cmdutil.InvocationContext{Profile: "target", ProfileSource: core.ProfileFromFlag}
	f.HttpClient = func() (*http.Client, error) { return &http.Client{Transport: rt}, nil }
	cmd := NewCmdConfigInit(f, nil)
	cmd.SetArgs([]string{"--restore"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	if beginAppID != "cli_target" {
		t.Fatalf("begin app_id = %q, want cli_target", beginAppID)
	}
	if kc.setCalls != 1 || kc.account != "appsecret:cli_target" || kc.value != "restored-secret" {
		t.Fatalf("keychain set = calls:%d account:%q value:%q", kc.setCalls, kc.account, kc.value)
	}
	after, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	if after.Apps[0].AppSecret.Ref == nil || after.Apps[0].AppSecret.Ref.ID != "appsecret:cli_target" {
		t.Fatalf("target secret = %#v", after.Apps[0].AppSecret)
	}
	if !reflect.DeepEqual(after.Apps[0].Users, original.Apps[0].Users) || !reflect.DeepEqual(after.Apps[1], original.Apps[1]) {
		t.Fatalf("restore changed unrelated config: %#v", after)
	}
	var output map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	if output["appSecret"] != "****" || strings.Contains(stdout.String(), "restored-secret") {
		t.Fatalf("stdout leaked restored secret: %q", stdout.String())
	}
}

func TestConfigInitRestoreRejectsMismatchedAppIDWithoutPersisting(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARK_CLI_NO_PROXY", "")
	original := &core.MultiAppConfig{Apps: []core.AppConfig{{
		AppId: "cli_target", AppSecret: core.PlainSecret("old-secret"), Brand: core.BrandFeishu,
	}}}
	if err := core.SaveMultiAppConfig(original); err != nil {
		t.Fatal(err)
	}
	replaceRestoreDefaultTransport(t, restoreRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if req.Form.Get("action") == "begin" {
			return restoreJSONResponse(`{"device_code":"device","user_code":"TEST-CODE","expire_in":30,"interval":0}`), nil
		}
		return restoreJSONResponse(`{"client_id":"cli_other","client_secret":"returned-secret"}`), nil
	}))

	kc := &recordingKeychain{}
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	f.Keychain = kc
	cmd := NewCmdConfigInit(f, nil)
	cmd.SetArgs([]string{"--restore"})
	err := cmd.ExecuteContext(context.Background())
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryConfig || problem.Subtype != errs.SubtypeInvalidClient {
		t.Fatalf("problem = %#v, err = %v", problem, err)
	}
	if kc.setCalls != 0 || stdout.Len() != 0 {
		t.Fatalf("restore persisted mismatched credentials: keychain calls=%d stdout=%q", kc.setCalls, stdout.String())
	}
	after, loadErr := core.LoadMultiAppConfig()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if !reflect.DeepEqual(after, original) {
		t.Fatalf("config changed: got %#v, want %#v", after, original)
	}
}

func restoreJSONResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func replaceRestoreDefaultTransport(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = rt
	t.Cleanup(func() { http.DefaultTransport = original })
}
