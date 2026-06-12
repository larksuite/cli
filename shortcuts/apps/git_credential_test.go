// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/apps/gitcred"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestAppsGitCredentialInitDryRunRequestShape(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsGitCredentialInit,
		[]string{"+git-credential-init", "--app-id", "app_xxx", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var payload struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Params map[string]interface{} `json:"params"`
			Body   interface{}            `json:"body"`
		} `json:"api"`
		Mode         string   `json:"mode"`
		Action       string   `json:"action"`
		AppID        string   `json:"app_id"`
		MetadataFile string   `json:"metadata_file"`
		LocalEffects []string `json:"local_effects"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &payload); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if len(payload.API) != 1 {
		t.Fatalf("api len = %d, want 1", len(payload.API))
	}
	call := payload.API[0]
	if call.Method != "GET" {
		t.Fatalf("method = %q, want GET", call.Method)
	}
	if call.URL != "/open-apis/spark/v1/apps/app_xxx/git_info" {
		t.Fatalf("url = %q", call.URL)
	}
	if call.Params["app_id"] != "app_xxx" {
		t.Fatalf("app_id param = %v", call.Params["app_id"])
	}
	if call.Body != nil {
		t.Fatalf("body = %#v, want nil", call.Body)
	}
	if payload.Mode != "api-plus-local-setup" {
		t.Fatalf("mode = %q", payload.Mode)
	}
	if payload.Action != "initialize_local_git_credential" {
		t.Fatalf("action = %q", payload.Action)
	}
	if payload.AppID != "app_xxx" {
		t.Fatalf("app_id = %q", payload.AppID)
	}
	if !strings.HasSuffix(payload.MetadataFile, filepath.Join("spark", "app_xxx", "git.json")) {
		t.Fatalf("metadata_file = %q", payload.MetadataFile)
	}
	assertStringSliceEqual(t, payload.LocalEffects, []string{
		"save the issued PAT in the local system credential store",
		"write app-scoped git credential metadata",
		"configure a URL-scoped Git credential helper in global git config when possible",
	})
}

func TestAppsGitCredentialListDryRunDescribesLocalReads(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsGitCredentialList,
		[]string{"+git-credential-list", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var payload struct {
		Description string        `json:"description"`
		API         []interface{} `json:"api"`
		Mode        string        `json:"mode"`
		Action      string        `json:"action"`
		StorageRoot string        `json:"storage_root"`
		Reads       []string      `json:"reads"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &payload); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if payload.Description != "Preview local Git credential listing (no API call, read-only local state)." {
		t.Fatalf("description = %q", payload.Description)
	}
	if len(payload.API) != 0 {
		t.Fatalf("api len = %d, want 0", len(payload.API))
	}
	if payload.Mode != "local-read-only" {
		t.Fatalf("mode = %q", payload.Mode)
	}
	if payload.Action != "list_local_git_credentials" {
		t.Fatalf("action = %q", payload.Action)
	}
	if !strings.HasSuffix(payload.StorageRoot, filepath.Join("spark")) {
		t.Fatalf("storage_root = %q", payload.StorageRoot)
	}
	assertStringSliceEqual(t, payload.Reads, []string{
		"scan app-scoped git credential metadata under the CLI config directory",
		"derive per-app repository URLs and local credential status from local metadata",
	})
}

func TestAppsGitCredentialRemoveDryRunDescribesLocalCleanup(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsGitCredentialRemove,
		[]string{"+git-credential-remove", "--app-id", "app_xxx", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var payload struct {
		Description  string        `json:"description"`
		API          []interface{} `json:"api"`
		Mode         string        `json:"mode"`
		Action       string        `json:"action"`
		AppID        string        `json:"app_id"`
		MetadataFile string        `json:"metadata_file"`
		Effects      []string      `json:"effects"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &payload); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	if payload.Description != "Preview local Git credential cleanup (no API call; would clean up local-only state)." {
		t.Fatalf("description = %q", payload.Description)
	}
	if len(payload.API) != 0 {
		t.Fatalf("api len = %d, want 0", len(payload.API))
	}
	if payload.Mode != "local-cleanup-only" {
		t.Fatalf("mode = %q", payload.Mode)
	}
	if payload.Action != "remove_local_git_credential" {
		t.Fatalf("action = %q", payload.Action)
	}
	if payload.AppID != "app_xxx" {
		t.Fatalf("app_id = %q", payload.AppID)
	}
	if !strings.HasSuffix(payload.MetadataFile, filepath.Join("spark", "app_xxx", "git.json")) {
		t.Fatalf("metadata_file = %q", payload.MetadataFile)
	}
	assertStringSliceEqual(t, payload.Effects, []string{
		"read app-scoped git credential metadata",
		"remove the saved PAT from the local system credential store",
		"remove the app-scoped Git helper from global git config when present",
		"delete the local metadata record after cleanup succeeds",
	})
}

func TestAppsGitCredentialInitRequiresAppID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsGitCredentialInit, []string{"+git-credential-init", "--app-id", " ", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "--app-id is required") {
		t.Fatalf("expected --app-id validation error, got %v", err)
	}
}

func TestIssuedFromDataAcceptsBackendGetAppGitInfoFields(t *testing.T) {
	expiresAt := time.Now().Add(24 * time.Hour).Unix()
	issued, err := issuedFromData("app_xxx", map[string]interface{}{
		"gitURL":      "https://example.com/git/u/app.git",
		"username":    "x-access-token",
		"token":       "pat-token",
		"expiredTime": float64(expiresAt),
	})
	if err != nil {
		t.Fatalf("issuedFromData returned error: %v", err)
	}
	if issued.GitHTTPURL != "https://example.com/git/u/app.git" {
		t.Fatalf("GitHTTPURL = %q", issued.GitHTTPURL)
	}
	if issued.PAT != "pat-token" {
		t.Fatalf("PAT = %q", issued.PAT)
	}
	if issued.ExpiresAt != expiresAt {
		t.Fatalf("ExpiresAt = %d", issued.ExpiresAt)
	}
}

func TestParseIssueCredentialDataAcceptsDirectBaseRespShape(t *testing.T) {
	data, err := parseIssueCredentialData(&larkcore.ApiResp{
		StatusCode: http.StatusOK,
		RawBody: []byte(`{
			"gitURL":"https://example.com/git/u/app.git",
			"username":"x-access-token",
			"token":"pat-token",
			"expiredTime":1780050600,
			"BaseResp":{"StatusCode":0,"StatusMessage":"ok"}
		}`),
	}, nil, errclass.ClassifyContext{})
	if err != nil {
		t.Fatalf("parseIssueCredentialData returned error: %v", err)
	}
	if data["gitURL"] != "https://example.com/git/u/app.git" {
		t.Fatalf("gitURL = %v", data["gitURL"])
	}
	if data["token"] != "pat-token" {
		t.Fatalf("token = %v", data["token"])
	}
}

func TestAppsGitCredentialInitExecutesAndRefreshes(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	kc := newAppsTestKeychain()
	factory.Keychain = kc
	installAppsFakeGit(t, 0)
	expiresAt := time.Now().Add(24 * time.Hour).Unix()
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_xxx/git_info",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"gitURL":      "https://example.com/git/u/app.git",
				"username":    "x-access-token",
				"token":       "pat-token",
				"expiredTime": float64(expiresAt),
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_xxx/git_info",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"gitURL":      "https://example.com/git/u/app.git",
				"username":    "x-access-token",
				"token":       "newer-token",
				"expiredTime": float64(expiresAt + 20000),
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_xxx/git_info",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"gitURL":      "https://example.com/git/u/app.git",
				"username":    "x-access-token",
				"token":       "new-token",
				"expiredTime": float64(expiresAt + 10000),
			},
		},
	})

	if err := runAppsShortcut(t, AppsGitCredentialInit, []string{"+git-credential-init", "--app-id", "app_xxx", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute init err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"status": "initialized"`) || !strings.Contains(got, `"repository_url": "https://example.com/git/u/app.git"`) {
		t.Fatalf("init stdout = %s", got)
	}
	meta, err := Read("app_xxx", gitcred.MetadataFilename)
	if err != nil {
		t.Fatalf("read app-scoped metadata: %v", err)
	}
	if !strings.Contains(string(meta), `"git_http_url": "https://example.com/git/u/app.git"`) {
		t.Fatalf("metadata missing git url: %s", meta)
	}
	if strings.Contains(string(meta), "pat-token") || strings.Contains(string(meta), `"credentials"`) {
		t.Fatalf("metadata should be app-scoped and must not contain PAT: %s", meta)
	}
	if len(kc.values) != 1 {
		t.Fatalf("keychain entries = %#v, want one PAT entry", kc.values)
	}
	for ref, pat := range kc.values {
		if ref == "" {
			t.Fatal("keychain ref is empty")
		}
		if pat != "pat-token" {
			t.Fatalf("keychain PAT = %q, want pat-token", pat)
		}
	}
	if err := runAppsShortcut(t, AppsGitCredentialInit, []string{"+git-credential-init", "--app-id", "app_xxx", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute refresh err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"status": "refreshed"`) {
		t.Fatalf("refresh stdout = %s", got)
	}
	if err := runAppsShortcut(t, AppsGitCredentialInit, []string{"+git-credential-init", "--app-id", "app_xxx", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute pretty refresh err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "Git credential refreshed") || !strings.Contains(got, "git clone https://example.com/git/u/app.git") {
		t.Fatalf("pretty refresh stdout = %s", got)
	}
}

func TestAppsGitCredentialInitPrettyWithGitConfigWarning(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	factory.Keychain = newAppsTestKeychain()
	installAppsFakeGit(t, 7)
	expiresAt := time.Now().Add(24 * time.Hour).Unix()
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_xxx/git_info",
		Body: map[string]interface{}{
			"gitURL":      "https://example.com/git/u/app.git",
			"username":    "x-access-token",
			"token":       "pat-token",
			"expiredTime": float64(expiresAt),
			"BaseResp": map[string]interface{}{
				"StatusCode":    0,
				"StatusMessage": "ok",
			},
		},
	})

	if err := runAppsShortcut(t, AppsGitCredentialInit, []string{"+git-credential-init", "--app-id", "app_xxx", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute init err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{
		"Git credential initialized",
		"Status: initialized",
		"Repository URL: https://example.com/git/u/app.git",
		"Git credential saved, but Git helper was not configured",
		"Next step: lark-cli apps +git-credential-init --app-id app_xxx",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty stdout missing %q in:\n%s", want, got)
		}
	}
}

func TestAppsGitCredentialInitAPIError(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	factory.Keychain = newAppsTestKeychain()
	installAppsFakeGit(t, 0)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_xxx/git_info",
		Status: http.StatusBadRequest,
		Body:   map[string]interface{}{"msg": "permission denied"},
	})
	err := runAppsShortcut(t, AppsGitCredentialInit, []string{"+git-credential-init", "--app-id", "app_xxx", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected API error, got %v", err)
	}
}

func TestAppsGitCredentialInitHooksDirectly(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("app-id", "", "")
	if err := cmd.Flags().Set("app-id", " "); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	rctx := &common.RuntimeContext{Cmd: cmd}
	if err := AppsGitCredentialInit.Validate(context.Background(), rctx); err == nil {
		t.Fatal("Validate returned nil for blank app-id")
	}
	if err := cmd.Flags().Set("app-id", " app_xxx "); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if AppsGitCredentialInit.DryRun(context.Background(), rctx) == nil {
		t.Fatal("DryRun returned nil")
	}
}

func TestAppsGitCredentialRemove(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	factory.Keychain = newAppsTestKeychain()
	installAppsFakeGit(t, 0)
	expiresAt := time.Now().Add(24 * time.Hour).Unix()
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_xxx/git_info",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"gitURL":      "https://example.com/git/u/app.git",
				"token":       "pat-token",
				"expiredTime": float64(expiresAt),
			},
		},
	})
	if err := runAppsShortcut(t, AppsGitCredentialInit, []string{"+git-credential-init", "--app-id", "app_xxx", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute init err=%v", err)
	}
	if err := runAppsShortcut(t, AppsGitCredentialRemove, []string{"+git-credential-remove", "--app-id", "app_xxx", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute remove err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "Git credential removed") || !strings.Contains(got, "Status: removed") {
		t.Fatalf("remove stdout = %s", got)
	}
	if err := runAppsShortcut(t, AppsGitCredentialRemove, []string{"+git-credential-remove", "--app-id", "app_xxx", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute remove missing err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "No local Git credential found") {
		t.Fatalf("remove missing stdout = %s", got)
	}
}

func TestAppsGitCredentialListScansAllLocalAppStorage(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	factory.Keychain = newAppsTestKeychain()
	installAppsFakeGit(t, 0)
	expiresA := time.Now().Add(24 * time.Hour).Unix()
	expiresB := time.Now().Add(48 * time.Hour).Unix()
	for _, tc := range []struct {
		appID     string
		url       string
		token     string
		expiresAt int64
	}{
		{appID: "app_b", url: "https://example.com/git/u/b.git", token: "pat-b", expiresAt: expiresB},
		{appID: "app_a", url: "https://example.com/git/u/a.git", token: "pat-a", expiresAt: expiresA},
	} {
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/spark/v1/apps/" + tc.appID + "/git_info",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"gitURL":      tc.url,
					"token":       tc.token,
					"expiredTime": float64(tc.expiresAt),
				},
			},
		})
		if err := runAppsShortcut(t, AppsGitCredentialInit, []string{"+git-credential-init", "--app-id", tc.appID, "--as", "user"}, factory, stdout); err != nil {
			t.Fatalf("execute init %s err=%v", tc.appID, err)
		}
	}

	if err := runAppsShortcut(t, AppsGitCredentialList, []string{"+git-credential-list", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute list pretty err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{
		"App ID",
		"Repository URL",
		"app_a",
		"https://example.com/git/u/a.git",
		"app_b",
		"https://example.com/git/u/b.git",
		gitcred.ListStatusValid,
		"Profile switches do not remove old URL-scoped Git helpers automatically.",
		"Cleanup: lark-cli apps +git-credential-remove --app-id <app_id>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("list pretty stdout missing %q in:\n%s", want, got)
		}
	}
	for _, hidden := range []string{"Expires At", "expires_at", "expired", time.Unix(expiresA, 0).UTC().Format(time.RFC3339), time.Unix(expiresB, 0).UTC().Format(time.RFC3339)} {
		if strings.Contains(got, hidden) {
			t.Fatalf("list pretty stdout should not expose %q in:\n%s", hidden, got)
		}
	}
	if strings.Index(got, "app_a") > strings.Index(got, "app_b") {
		t.Fatalf("list should be sorted by app_id, got:\n%s", got)
	}

	if err := runAppsShortcut(t, AppsGitCredentialList, []string{"+git-credential-list", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute list json err=%v", err)
	}
	var envelope struct {
		Data struct {
			Count       int `json:"count"`
			Credentials []struct {
				AppID         string `json:"app_id"`
				RepositoryURL string `json:"repository_url"`
				Status        string `json:"status"`
			} `json:"credentials"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil {
		t.Fatalf("decode list output: %v\n%s", err, stdout.String())
	}
	payload := envelope.Data
	if payload.Count != 2 || len(payload.Credentials) != 2 {
		t.Fatalf("payload count = %d records=%#v\n%s", payload.Count, payload.Credentials, stdout.String())
	}
	if payload.Credentials[0].AppID != "app_a" || payload.Credentials[0].RepositoryURL != "https://example.com/git/u/a.git" || payload.Credentials[0].Status != gitcred.ListStatusValid {
		t.Fatalf("first credential = %#v", payload.Credentials[0])
	}
	if strings.Contains(stdout.String(), "expires_at") || strings.Contains(stdout.String(), "expires_at_iso") || strings.Contains(stdout.String(), strconv.FormatInt(expiresA, 10)) || strings.Contains(stdout.String(), strconv.FormatInt(expiresB, 10)) {
		t.Fatalf("list json should not expose expiry fields or values:\n%s", stdout.String())
	}
}

func TestAppsGitCredentialListEmpty(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	factory.Keychain = newAppsTestKeychain()

	if err := runAppsShortcut(t, AppsGitCredentialList, []string{"+git-credential-list", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute list pretty err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "No Git credentials initialized") || !strings.Contains(got, "+git-credential-init --app-id <app_id>") {
		t.Fatalf("empty list stdout = %s", got)
	}
}

func TestGitCredentialAppStorageListAppIDsSkipsNonCredentialAppDirs(t *testing.T) {
	newAppsExecuteFactory(t)
	if err := Write("app/a", gitcred.MetadataFilename, []byte("{}")); err != nil {
		t.Fatalf("Write escaped app metadata: %v", err)
	}
	if err := Write("app_b", gitcred.MetadataFilename, []byte("{}")); err != nil {
		t.Fatalf("Write app_b metadata: %v", err)
	}
	root := filepath.Join(core.GetConfigDir(), "spark")
	if err := os.WriteFile(filepath.Join(root, "not-an-app-dir"), []byte("x"), 0600); err != nil {
		t.Fatalf("write non-dir: %v", err)
	}
	for _, name := range []string{"%zz", "app%2F..%2Fb"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}

	appIDs, err := gitCredentialAppStorage{}.ListAppIDs()
	if err != nil {
		t.Fatalf("ListAppIDs: %v", err)
	}
	got := map[string]bool{}
	for _, appID := range appIDs {
		got[appID] = true
	}
	if len(got) != 2 || !got["app/a"] || !got["app_b"] {
		t.Fatalf("appIDs = %v, want app/a and app_b only", appIDs)
	}
}

func TestAppsGitCredentialListReturnsScanErrors(t *testing.T) {
	t.Run("storage root error", func(t *testing.T) {
		factory, stdout, _ := newAppsExecuteFactory(t)
		root := filepath.Join(core.GetConfigDir(), "spark")
		if err := os.WriteFile(root, []byte("not a dir"), 0600); err != nil {
			t.Fatalf("write storage root blocker: %v", err)
		}
		err := runAppsShortcut(t, AppsGitCredentialList, []string{"+git-credential-list", "--as", "user"}, factory, stdout)
		if err == nil || !strings.Contains(err.Error(), "apps storage: read root") {
			t.Fatalf("execute list root error = %v", err)
		}
	})

	t.Run("record error", func(t *testing.T) {
		factory, _, _ := newAppsExecuteFactory(t)
		if err := Write("app_xxx", gitcred.MetadataFilename, []byte("{bad json")); err != nil {
			t.Fatalf("write invalid metadata: %v", err)
		}
		_, err := listGitCredentialRecords(factory.Keychain, time.Now)
		if err == nil || !strings.Contains(err.Error(), "invalid git.json") {
			t.Fatalf("listGitCredentialRecords record error = %v", err)
		}
	})
}

func TestListGitCredentialRecordsSortsDuplicateDecodedAppIDs(t *testing.T) {
	factory, _, _ := newAppsExecuteFactory(t)
	kc := newAppsTestKeychain()
	factory.Keychain = kc
	now := time.Unix(1780000000, 0)
	manager := newGitCredentialManager("app_x", kc, nil)
	manager.Now = func() time.Time { return now }
	record := gitcred.CredentialRecord{
		AppID:      "app_x",
		GitHTTPURL: "https://example.com/git/u/app.git",
		Username:   "x-access-token",
		PATRef:     "ref",
		Status:     gitcred.StatusConfirmed,
		ExpiresAt:  now.Add(time.Hour).Unix(),
	}
	kc.values[record.PATRef] = "pat"
	if err := manager.Store.Upsert(record); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}
	if err := os.Mkdir(filepath.Join(core.GetConfigDir(), "spark", "app%5Fx"), 0700); err != nil {
		t.Fatalf("mkdir duplicate encoded app dir: %v", err)
	}

	records, err := listGitCredentialRecords(kc, func() time.Time { return now })
	if err != nil {
		t.Fatalf("listGitCredentialRecords returned error: %v", err)
	}
	if len(records) != 2 || records[0].AppID != "app_x" || records[1].AppID != "app_x" {
		t.Fatalf("records = %#v, want duplicate decoded app_x records", records)
	}
}

func TestGitCredentialListPayloadDoesNotExposeExpiry(t *testing.T) {
	payload := gitCredentialListPayload([]gitcred.ListRecord{{
		AppID:      "app_xxx",
		GitHTTPURL: "https://example.com/git/u/app.git",
		Status:     gitcred.ListStatusExpired,
		ExpiresAt:  1780000000,
		Expired:    true,
	}})
	for _, key := range []string{"expires_at", "expires_at_iso", "expired"} {
		if _, ok := payload[0][key]; ok {
			t.Fatalf("payload exposes %s: %#v", key, payload[0])
		}
	}
	if got := payload[0]["status"]; got != "refresh_required" {
		t.Fatalf("payload status = %q, want refresh_required", got)
	}
	for _, value := range payload[0] {
		if strings.Contains(fmt.Sprint(value), "expired") {
			t.Fatalf("payload exposes expired concept: %#v", payload[0])
		}
	}
}

func TestAppsGitCredentialRemoveReportsGitConfigWarning(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	factory.Keychain = newAppsTestKeychain()
	installAppsFakeGit(t, 7) // unsetting useHttpPath exits non-zero -> ConfigWarning
	expiresAt := time.Now().Add(24 * time.Hour).Unix()
	for _, appID := range []string{"app_one", "app_two"} {
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/spark/v1/apps/" + appID + "/git_info",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"gitURL":      "https://example.com/git/u/" + appID + ".git",
					"token":       "pat-token",
					"expiredTime": float64(expiresAt),
				},
			},
		})
		if err := runAppsShortcut(t, AppsGitCredentialInit, []string{"+git-credential-init", "--app-id", appID, "--as", "user"}, factory, stdout); err != nil {
			t.Fatalf("init %s err=%v", appID, err)
		}
	}
	// Pretty output surfaces the cleanup-warning block.
	if err := runAppsShortcut(t, AppsGitCredentialRemove, []string{"+git-credential-remove", "--app-id", "app_one", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("remove pretty err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "Git config cleanup warning") || !strings.Contains(got, "Reason:") {
		t.Fatalf("pretty remove missing git config warning: %s", got)
	}
	// JSON output exposes git_config_warning.
	if err := runAppsShortcut(t, AppsGitCredentialRemove, []string{"+git-credential-remove", "--app-id", "app_two", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("remove json err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "git_config_warning") {
		t.Fatalf("json remove missing git_config_warning: %s", got)
	}
}

func TestAppsGitCredentialRemoveRequiresAppID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsGitCredentialRemove, []string{"+git-credential-remove", "--app-id", " ", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "--app-id is required") {
		t.Fatalf("expected --app-id validation error, got %v", err)
	}
}

func TestAppsGitCredentialRemoveReturnsStoreError(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := Write("app_xxx", gitcred.MetadataFilename, []byte("{bad json")); err != nil {
		t.Fatalf("write invalid metadata: %v", err)
	}
	err := runAppsShortcut(t, AppsGitCredentialRemove, []string{"+git-credential-remove", "--app-id", "app_xxx", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "invalid git.json") {
		t.Fatalf("expected remove store error, got %v", err)
	}
}

func assertStringSliceEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("slice len = %d, want %d; got %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slice[%d] = %q, want %q; got %#v", i, got[i], want[i], got)
		}
	}
}

func TestGitCredentialLocalErrorWrapsOnlyPlainErrors(t *testing.T) {
	plain := errors.New("git config failed")
	wrapped := gitCredentialLocalError("List local Miaoda Git credentials", plain)
	var configErr *errs.ConfigError
	if !errors.As(wrapped, &configErr) {
		t.Fatalf("plain local error wrapped as %T, want *errs.ConfigError", wrapped)
	}
	if !errors.Is(wrapped, plain) {
		t.Fatalf("wrapped error does not preserve cause")
	}

	typed := &errs.ConfigError{Problem: errs.Problem{
		Category: errs.CategoryConfig,
		Subtype:  errs.SubtypeInvalidConfig,
		Message:  "already typed",
	}}
	if got := gitCredentialLocalError("action", typed); got != typed {
		t.Fatalf("typed error was rewrapped: %#v", got)
	}

	validationErr := errs.NewValidationError(errs.SubtypeInvalidArgument, "bad app")
	if got := gitCredentialLocalError("action", validationErr); got != error(validationErr) {
		t.Fatalf("typed validation error was rewrapped: %#v", got)
	}

	if got := gitCredentialLocalError("action", nil); got != nil {
		t.Fatalf("nil error must stay nil, got %#v", got)
	}
}

func TestRunGitCredentialHelperActions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	factory, stdout, _ := newAppsExecuteFactory(t)
	kc := newAppsTestKeychain()
	factory.Keychain = kc
	storage := gitCredentialAppStorage{}
	manager := gitcred.NewManager(gitcred.NewAppStore("app_xxx", storage), gitcred.NewSecretStore(kc), nil, testAppsIssuer{next: &gitcred.IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		Username:   "x-access-token",
		PAT:        "pat-token",
		ExpiresAt:  time.Now().Add(24 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return time.Unix(1780000000, 0) }
	cfg, err := factory.Config()
	if err != nil {
		t.Fatalf("factory Config returned error: %v", err)
	}
	if _, err := manager.Init(context.Background(), profileFromConfig(cfg), "app_xxx"); err != nil {
		t.Fatalf("seed Init returned error: %v", err)
	}

	factory.IOStreams.In = bytes.NewBufferString("protocol=https\nhost=example.com\npath=/git/u/app.git\n\n")
	if err := runGitCredentialHelper(context.Background(), factory, "app_xxx", "get"); err != nil {
		t.Fatalf("helper get returned error: %v", err)
	}
	if got := stdout.String(); got != "username=x-access-token\npassword=pat-token\n\n" {
		t.Fatalf("helper get stdout = %q", got)
	}
	stdout.Reset()
	factory.IOStreams.In = bytes.NewBufferString("protocol=https\nhost=example.com\n\n")
	if err := runGitCredentialHelper(context.Background(), factory, "app_xxx", "store"); err != nil {
		t.Fatalf("helper store returned error: %v", err)
	}
	factory.IOStreams.In = bytes.NewBufferString("protocol=https\nhost=example.com\npath=/git/u/app.git\n\n")
	if err := runGitCredentialHelper(context.Background(), factory, "app_xxx", "erase"); err != nil {
		t.Fatalf("helper erase returned error: %v", err)
	}
	var stderr bytes.Buffer
	factory.IOStreams.ErrOut = &stderr
	factory.IOStreams.In = bytes.NewBufferString("bad-input-without-equals\n")
	if err := runGitCredentialHelper(context.Background(), factory, "app_xxx", "get"); err != nil {
		t.Fatalf("helper bad get returned error: %v", err)
	}
	if !strings.Contains(stderr.String(), "protocol and host") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	stderr.Reset()
	factory.IOStreams.In = errorReader{}
	if err := runGitCredentialHelper(context.Background(), factory, "app_xxx", "get"); err != nil {
		t.Fatalf("helper reader error returned error: %v", err)
	}
	if !strings.Contains(stderr.String(), "read failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	stderr.Reset()
	factory.Config = func() (*core.CliConfig, error) { return nil, errors.New("config failed") }
	factory.IOStreams.In = bytes.NewBufferString("protocol=https\nhost=example.com\npath=/git/u/app.git\n\n")
	if err := runGitCredentialHelper(context.Background(), factory, "app_xxx", "get"); err != nil {
		t.Fatalf("helper config error returned error: %v", err)
	}
	if !strings.Contains(stderr.String(), "config failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	cfg = &core.CliConfig{AppID: "cli", AppSecret: "secret", Brand: core.BrandFeishu, UserOpenId: "ou_test"}
	factory.Config = func() (*core.CliConfig, error) { return cfg, nil }
	stderr.Reset()
	if err := runGitCredentialHelper(context.Background(), factory, "app_xxx", "unknown"); err != nil {
		t.Fatalf("helper unknown returned error: %v", err)
	}
	if !strings.Contains(stderr.String(), `unsupported git credential action "unknown"`) {
		t.Fatalf("stderr = %q", stderr.String())
	}
	stderr.Reset()
	if err := runGitCredentialHelper(context.Background(), factory, "", "get"); err != nil {
		t.Fatalf("helper missing appID returned error: %v", err)
	}
	if !strings.Contains(stderr.String(), "missing app_id") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if err := runGitCredentialHelper(context.Background(), nil, "app_xxx", "get"); err != nil {
		t.Fatalf("helper nil factory returned error: %v", err)
	}
	if err := runGitCredentialHelper(context.Background(), &cmdutil.Factory{}, "app_xxx", "get"); err != nil {
		t.Fatalf("helper nil streams returned error: %v", err)
	}
	factory.IOStreams.In = bytes.NewBufferString("protocol=https\nhost=example.com\n\n")
	cmd := newGitCredentialHelperCommand(factory)
	if err := cmd.Flags().Set("app-id", "app_xxx"); err != nil {
		t.Fatalf("set app-id returned error: %v", err)
	}
	if err := cmd.RunE(cmd, []string{"store"}); err != nil {
		t.Fatalf("helper command returned error: %v", err)
	}
}

func TestFactoryIssuerBranches(t *testing.T) {
	factory, _, reg := newAppsExecuteFactory(t)
	expiresAt := time.Now().Add(24 * time.Hour).Unix()
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_xxx/git_info",
		Body: map[string]interface{}{
			"gitURL":      "https://example.com/git/u/app.git",
			"token":       "pat-token",
			"expiredTime": float64(expiresAt),
			"BaseResp": map[string]interface{}{
				"StatusCode": 0,
			},
		},
	})
	issued, err := (factoryIssuer{f: factory}).Issue(context.Background(), "app_xxx", gitcred.ProfileContext{})
	if err != nil {
		t.Fatalf("factory issuer returned error: %v", err)
	}
	if issued.PAT != "pat-token" {
		t.Fatalf("PAT = %q", issued.PAT)
	}

	factory.Config = func() (*core.CliConfig, error) { return nil, errors.New("config failed") }
	if _, err := (factoryIssuer{f: factory}).Issue(context.Background(), "app_xxx", gitcred.ProfileContext{}); err == nil {
		t.Fatal("factory issuer config error returned nil")
	}

	factory.Config = func() (*core.CliConfig, error) {
		return &core.CliConfig{AppID: "cli", AppSecret: "secret", Brand: core.BrandFeishu}, nil
	}
	if _, err := (factoryIssuer{f: factory}).Issue(context.Background(), "app_xxx", gitcred.ProfileContext{}); err == nil {
		t.Fatal("factory issuer without login returned nil")
	}
	factory.Config = func() (*core.CliConfig, error) {
		return &core.CliConfig{AppID: "cli", AppSecret: "secret", Brand: core.BrandFeishu, UserOpenId: "ou_test"}, nil
	}
	factory.LarkClient = func() (*lark.Client, error) { return nil, errors.New("sdk failed") }
	if _, err := (factoryIssuer{f: factory}).Issue(context.Background(), "app_xxx", gitcred.ProfileContext{}); err == nil {
		t.Fatal("factory issuer SDK error returned nil")
	}

	factory, _, reg = newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/spark/v1/apps/app_xxx/git_info",
		RawBody: []byte("{bad json"),
	})
	if _, err := (factoryIssuer{f: factory}).Issue(context.Background(), "app_xxx", gitcred.ProfileContext{}); err == nil {
		t.Fatal("factory issuer parse error returned nil")
	}

	factory, _, _ = newAppsExecuteFactory(t)
	if _, err := (factoryIssuer{f: factory}).Issue(context.Background(), "app_xxx", gitcred.ProfileContext{}); err == nil {
		t.Fatal("factory issuer request error returned nil")
	}
}

func TestGitCredentialHelpersAndParsers(t *testing.T) {
	if issuePath(" app/with space ") != "/open-apis/spark/v1/apps/app%2Fwith%20space/git_info" {
		t.Fatalf("issuePath escaped incorrectly: %s", issuePath(" app/with space "))
	}
	if got := gitCredentialIssueParams(" app_xxx ")["app_id"]; got != "app_xxx" {
		t.Fatalf("param app_id = %q", got)
	}
	if initStatus(nil) != "initialized" || initStatus(&gitcred.InitResult{Refreshed: true}) != "refreshed" {
		t.Fatalf("initStatus mismatch")
	}
	if got := profileFromConfig(nil); got != (gitcred.ProfileContext{}) {
		t.Fatalf("profileFromConfig(nil) = %#v", got)
	}

	for _, data := range []map[string]interface{}{
		{"credential": map[string]interface{}{"gitURL": "https://example.com/repo.git", "token": "pat"}},
		{"git_credential": map[string]interface{}{"git_url": "https://example.com/repo.git", "password": "pat"}},
		{"gitInfo": map[string]interface{}{"repository_url": "https://example.com/repo.git", "pat": "pat", "expired_time": "1780050600"}},
		{"git_info": map[string]interface{}{"GitUrl": "https://example.com/repo.git", "Token": "pat", "ExpiredTime": "1780050600"}},
	} {
		if _, err := issuedFromData("app_xxx", data); err != nil {
			t.Fatalf("issuedFromData nested returned error: %v", err)
		}
	}
	if _, err := issuedFromData("app_xxx", map[string]interface{}{"token": "pat"}); err == nil {
		t.Fatal("issuedFromData missing gitURL returned nil error")
	}
	if _, err := issuedFromData("app_xxx", map[string]interface{}{"gitURL": "https://example.com/repo.git"}); err == nil {
		t.Fatal("issuedFromData missing token returned nil error")
	}
	if got := firstInt64(map[string]interface{}{"n": int(7)}, "n"); got != 7 {
		t.Fatalf("firstInt64 int = %d", got)
	}
	if got := firstInt64(map[string]interface{}{"n": int64(9)}, "n"); got != 9 {
		t.Fatalf("firstInt64 int64 = %d", got)
	}
	if got := firstInt64(map[string]interface{}{"n": "bad"}, "n"); got != 0 {
		t.Fatalf("firstInt64 bad string = %d", got)
	}
	if logIDString(nil) != "" {
		t.Fatal("logIDString(nil) should be empty")
	}
}

func TestParseIssueCredentialDataErrors(t *testing.T) {
	if _, err := parseIssueCredentialData(nil, errors.New("transport failed"), errclass.ClassifyContext{}); err == nil {
		t.Fatal("parseIssueCredentialData transport error returned nil")
	}
	if _, err := parseIssueCredentialData(nil, nil, errclass.ClassifyContext{}); err == nil {
		t.Fatal("parseIssueCredentialData nil response returned nil")
	}
	if _, err := parseIssueCredentialData(&larkcore.ApiResp{StatusCode: http.StatusOK, RawBody: []byte("{bad json")}, nil, errclass.ClassifyContext{}); err == nil {
		t.Fatal("parseIssueCredentialData bad json returned nil")
	}
	header := http.Header{"X-Tt-Logid": []string{"log_x"}}
	if _, err := parseIssueCredentialData(&larkcore.ApiResp{StatusCode: http.StatusBadRequest, RawBody: []byte(`{"msg":"bad request"}`), Header: header}, nil, errclass.ClassifyContext{}); err == nil || !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("HTTP error = %v", err)
	}
	if _, err := parseIssueCredentialData(&larkcore.ApiResp{StatusCode: http.StatusInternalServerError, RawBody: []byte(`{}`), Header: header}, nil, errclass.ClassifyContext{}); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("HTTP fallback error = %v", err)
	}
	if _, err := parseIssueCredentialData(&larkcore.ApiResp{StatusCode: http.StatusOK, RawBody: []byte(`{"code":999,"msg":"failed"}`), Header: header}, nil, errclass.ClassifyContext{}); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("code error = %v", err)
	}
	data, err := parseIssueCredentialData(&larkcore.ApiResp{StatusCode: http.StatusOK, RawBody: []byte(`{"code":0}`), Header: header}, nil, errclass.ClassifyContext{})
	if err != nil {
		t.Fatalf("code zero without data returned error: %v", err)
	}
	if data["log_id"] != "log_x" {
		t.Fatalf("log_id = %v", data["log_id"])
	}
	data, err = parseIssueCredentialData(&larkcore.ApiResp{StatusCode: http.StatusOK, RawBody: []byte(`null`), Header: header}, nil, errclass.ClassifyContext{})
	if err != nil {
		t.Fatalf("null response with log id returned error: %v", err)
	}
	if data["log_id"] != "log_x" {
		t.Fatalf("null response log_id = %v", data["log_id"])
	}
	if _, err := parseIssueCredentialData(&larkcore.ApiResp{StatusCode: http.StatusOK, RawBody: []byte(`{"BaseResp":{"StatusCode":7,"StatusMessage":"denied"}}`), Header: header}, nil, errclass.ClassifyContext{}); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("BaseResp error = %v", err)
	}
	if _, err := parseIssueCredentialData(&larkcore.ApiResp{StatusCode: http.StatusOK, RawBody: []byte(`{"baseResp":{"statusCode":7}}`)}, nil, errclass.ClassifyContext{}); err == nil || !strings.Contains(err.Error(), "non-zero BaseResp") {
		t.Fatalf("BaseResp fallback error = %v", err)
	}
}

// TestParseIssueCredentialData503IsRetryableWithHint verifies that a 5xx Git
// credential issuance failure is flagged retryable and carries the developer-access hint.
func TestParseIssueCredentialData503IsRetryableWithHint(t *testing.T) {
	header := http.Header{"X-Tt-Logid": []string{"log_x"}}
	_, err := parseIssueCredentialData(&larkcore.ApiResp{StatusCode: http.StatusServiceUnavailable, RawBody: []byte(`{"msg":"upstream busy"}`), Header: header}, nil, errclass.ClassifyContext{})
	if err == nil {
		t.Fatal("expected 503 error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed errs.Problem, got %T: %v", err, err)
	}
	if !p.Retryable {
		t.Fatalf("503 should be retryable, got Retryable=false")
	}
	if !strings.Contains(p.Hint, "developer access") {
		t.Fatalf("hint missing 'developer access': %q", p.Hint)
	}
}

// TestParseIssueCredentialDataBusinessCodeHasHintNotRetryable verifies that a
// non-zero business code (no HTTP status) carries the hint but is not retryable.
func TestParseIssueCredentialDataBusinessCodeHasHintNotRetryable(t *testing.T) {
	header := http.Header{"X-Tt-Logid": []string{"log_x"}}
	_, err := parseIssueCredentialData(&larkcore.ApiResp{StatusCode: http.StatusOK, RawBody: []byte(`{"code":999,"msg":"no developer access"}`), Header: header}, nil, errclass.ClassifyContext{})
	if err == nil {
		t.Fatal("expected business-code error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed errs.Problem, got %T: %v", err, err)
	}
	if p.Retryable {
		t.Fatalf("business code != 0 must not be retryable, got Retryable=true")
	}
	if !strings.Contains(p.Hint, "developer access") {
		t.Fatalf("hint missing 'developer access': %q", p.Hint)
	}
}

// TestParseIssueCredentialDataMessageAddsNoExtraSecret verifies the security
// condition that apps does not ADDITIONALLY inject any token/secret into the
// Git-credential error it builds. The server `msg` is passed through verbatim
// into Problem.Message, and the only thing apps adds is the static
// gitCredentialIssueHint — which itself contains no secret. We feed a benign
// server msg and assert (a) Message equals that msg exactly, and (b) neither
// Message nor Hint contains any token/secret-shaped string.
//
// Note: server msg passthrough is the shared classifier's responsibility;
// apps adds only a static hint. There is no msg redaction in this path, so
// this test does not assert a redaction that does not exist — it asserts
// that apps injects nothing sensitive of its own.
func TestParseIssueCredentialDataMessageAddsNoExtraSecret(t *testing.T) {
	const serverMsg = "permission denied"
	header := http.Header{"X-Tt-Logid": []string{"log_x"}}

	for _, tc := range []struct {
		name string
		resp *larkcore.ApiResp
	}{
		{
			name: "http error path",
			resp: &larkcore.ApiResp{
				StatusCode: http.StatusForbidden,
				RawBody:    []byte(`{"msg":"` + serverMsg + `"}`),
				Header:     header,
			},
		},
		{
			name: "business code path",
			resp: &larkcore.ApiResp{
				StatusCode: http.StatusOK,
				RawBody:    []byte(`{"code":999,"msg":"` + serverMsg + `"}`),
				Header:     header,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseIssueCredentialData(tc.resp, nil, errclass.ClassifyContext{})
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed errs.Problem, got %T: %v", err, err)
			}
			// (a) The server msg survives into the message. The business-code
			// path passes it through verbatim; the HTTP-status path reports
			// "HTTP <status>: <body>" via the shared classifier, so assert
			// containment rather than equality.
			if !strings.Contains(p.Message, serverMsg) {
				t.Fatalf("Message = %q, want it to contain server msg %q", p.Message, serverMsg)
			}
			// apps adds only the static hint — assert that exact static text,
			// proving apps injects no per-request secret into the hint either.
			if p.Hint != gitCredentialIssueHint {
				t.Fatalf("Hint = %q, want the static gitCredentialIssueHint", p.Hint)
			}
			// (b) Neither field may contain a token/secret-shaped string that
			// apps could have added on top of the framework passthrough.
			secret := regexp.MustCompile(`(?i)(pat-[a-z0-9]+|secret\s*[=:]\s*\S|token\s*[=:]\s*\S|password\s*[=:]\s*\S)`)
			for field, val := range map[string]string{"Message": p.Message, "Hint": p.Hint} {
				if secret.MatchString(val) {
					t.Fatalf("%s leaks a token/secret-shaped string: %q", field, val)
				}
			}
		})
	}
}

type errorReader struct{}

func (errorReader) Read(p []byte) (int, error) {
	return 0, errors.New("read failed")
}

type appsTestKeychain struct {
	values map[string]string
}

func newAppsTestKeychain() *appsTestKeychain {
	return &appsTestKeychain{values: map[string]string{}}
}

func (k *appsTestKeychain) Get(service, account string) (string, error) {
	return k.values[account], nil
}

func (k *appsTestKeychain) Set(service, account, value string) error {
	k.values[account] = value
	return nil
}

func (k *appsTestKeychain) Remove(service, account string) error {
	delete(k.values, account)
	return nil
}

type testAppsIssuer struct {
	next *gitcred.IssuedCredential
}

func (i testAppsIssuer) Issue(ctx context.Context, appID string, profile gitcred.ProfileContext) (*gitcred.IssuedCredential, error) {
	out := *i.next
	out.AppID = appID
	return &out, nil
}

func installAppsFakeGit(t *testing.T, failUseHTTPPathExit int) {
	t.Helper()
	dir := t.TempDir()
	gitPath := filepath.Join(dir, "git")
	script := `#!/bin/sh
case "$*" in
  *"--get"*) exit 1 ;;
esac
exit 0
`
	if failUseHTTPPathExit != 0 {
		script = `#!/bin/sh
case "$*" in
  *"--get"*) exit 1 ;;
esac
case "$*" in
  *useHttpPath*) exit 7 ;;
esac
exit 0
`
	}
	if err := os.WriteFile(gitPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestParseIssueCredentialData_SharedClassifierCoverage pins the canonical
// classifications the shared classifier provides on the credential-issue
// path: a generic missing-scope code becomes a typed permission error with
// the missing scopes extracted, and an HTTP 503 becomes a retryable
// network/server_error — neither collapses to api/unknown.
func TestParseIssueCredentialData_SharedClassifierCoverage(t *testing.T) {
	header := http.Header{"X-Tt-Logid": []string{"log_x"}}

	t.Run("missing scope classifies as authorization with scopes", func(t *testing.T) {
		body := `{"code":99991676,"msg":"token scope insufficient","error":{"permission_violations":[{"subject":"spark:app:read"}]}}`
		_, err := parseIssueCredentialData(&larkcore.ApiResp{
			StatusCode: http.StatusOK, RawBody: []byte(body), Header: header,
		}, nil, errclass.ClassifyContext{})
		var permErr *errs.PermissionError
		if !errors.As(err, &permErr) {
			t.Fatalf("want *errs.PermissionError, got %T: %v", err, err)
		}
		if permErr.Subtype != errs.SubtypeTokenScopeInsufficient {
			t.Fatalf("subtype = %q, want %q", permErr.Subtype, errs.SubtypeTokenScopeInsufficient)
		}
		if len(permErr.MissingScopes) != 1 || permErr.MissingScopes[0] != "spark:app:read" {
			t.Fatalf("MissingScopes = %v, want [spark:app:read]", permErr.MissingScopes)
		}
	})

	t.Run("http 503 classifies as retryable network server_error", func(t *testing.T) {
		_, err := parseIssueCredentialData(&larkcore.ApiResp{
			StatusCode: http.StatusServiceUnavailable, RawBody: []byte(`{"msg":"upstream busy"}`), Header: header,
		}, nil, errclass.ClassifyContext{})
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("want typed problem, got %T: %v", err, err)
		}
		if p.Category != errs.CategoryNetwork || p.Subtype != errs.SubtypeNetworkServer {
			t.Fatalf("classification = %s/%s, want network/server_error", p.Category, p.Subtype)
		}
		if !p.Retryable {
			t.Fatalf("retryable = false, want true for 5xx")
		}
	})
}
