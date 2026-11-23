// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	_ "github.com/larksuite/cli/extension/credential/env"
	"github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/output"
)

type envCredentialTestTransport struct{ target *url.URL }

func (p envCredentialTestTransport) Name() string { return "env-credential-test" }
func (p envCredentialTestTransport) ResolveInterceptor(context.Context) transport.Interceptor {
	return p
}
func (p envCredentialTestTransport) PreRoundTrip(req *http.Request) func(*http.Response, error) {
	req.URL.Scheme, req.URL.Host, req.Host = p.target.Scheme, p.target.Host, p.target.Host
	return nil
}

type envCredentialTestKeychain struct {
	noopKeychain
	account, value string
	reads          int
}

func (k *envCredentialTestKeychain) Set(_ string, account, value string) error {
	k.account, k.value = account, value
	return nil
}

func (k *envCredentialTestKeychain) Get(service, account string) (string, error) {
	k.reads++
	if service == keychain.LarkCliService && account == k.account {
		return k.value, nil
	}
	return "", nil
}

func TestExecuteEnvCredentialSources(t *testing.T) {
	dir := tmpHome(t)
	t.Setenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER", "1")
	t.Setenv("LARKSUITE_CLI_NO_SKILLS_NOTIFIER", "1")
	for _, key := range []string{
		envvars.CliAppID, envvars.CliAppSecret, envvars.CliUserAccessToken,
		envvars.CliTenantAccessToken, envvars.CliTenantAccessTokenSource,
		envvars.CliStrictMode, envvars.CliDefaultAs, envvars.CliProfile,
	} {
		t.Setenv(key, "")
	}
	const appID, uat, fileTAT, storedTAT = "fixture-app", "fixture-user", "fixture-file-bot", "fixture-store-bot"
	for key, value := range map[string]string{
		envvars.CliAppIDFile: appID, envvars.CliUserAccessTokenFile: uat, envvars.CliTenantAccessTokenFile: fileTAT,
	} {
		path := filepath.Join(dir, key)
		if err := os.WriteFile(path, []byte(value+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(key, path)
	}
	kc := &envCredentialTestKeychain{}
	store := credential.NewTenantTokenStore(func() keychain.KeychainAccess { return kc })
	if err := store.Set(appID, storedTAT); err != nil {
		t.Fatal(err)
	}
	forwardedTokens := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/authen/v1/user_info":
			if r.Header.Get("Authorization") != "Bearer "+uat {
				t.Error("user_info was not authorized with the file UAT")
			}
			io.WriteString(w, `{"code":0,"data":{"open_id":"ou_fixture","name":"Fixture"}}`)
		case "/open-apis/im/v1/messages/om_fixture/read_users":
			if r.Method != http.MethodGet || r.URL.Query().Get("user_id_type") != "open_id" {
				t.Error("unexpected read_users request")
			}
			forwardedTokens <- r.Header.Get("Authorization")
			io.WriteString(w, `{"code":0,"data":{"items":[]}}`)
		default:
			t.Errorf("unexpected mock request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	previousTransport := transport.GetProvider()
	transport.Register(envCredentialTestTransport{target})
	t.Cleanup(func() { transport.Register(previousTransport) })

	args := []string{"im", "messages", "read_users", "--params", `{"message_id":"om_fixture","user_id_type":"open_id"}`}
	for _, tc := range []struct {
		name, source, identity, token string
		reads                         int
	}{
		{"file TAT", "", "bot", fileTAT, 0},
		{"file UAT with store selected", "credential-store", "user", uat, 0},
		{"stored TAT overrides file TAT", "credential-store", "bot", storedTAT, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envvars.CliTenantAccessTokenSource, tc.source)
			code, stdout, stderr := executeWithCapturedOS(t, []BuildOption{WithKeychain(kc)}, append(args, "--as", tc.identity)...)
			var result struct {
				OK       bool   `json:"ok"`
				Identity string `json:"identity"`
			}
			if code != 0 || json.Unmarshal([]byte(stdout), &result) != nil || !result.OK || result.Identity != tc.identity {
				t.Fatalf("execution exit=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			select {
			case got := <-forwardedTokens:
				if got != "Bearer "+tc.token {
					t.Fatal("request used the wrong credential source")
				}
			default:
				t.Fatal("no API request reached the local mock")
			}
			if kc.reads != tc.reads {
				t.Fatalf("keychain reads = %d, want %d", kc.reads, tc.reads)
			}
		})
	}

	t.Setenv(envvars.CliTenantAccessTokenFile, "misplaced-fixture-secret")
	readsBefore := kc.reads
	code, stdout, stderr := executeWithCapturedOS(t, []BuildOption{WithKeychain(kc)}, append(args, "--as", "bot")...)
	var failure struct {
		OK       bool   `json:"ok"`
		Identity string `json:"identity"`
		Error    struct {
			Type    errs.Category `json:"type"`
			Message string        `json:"message"`
		} `json:"error"`
	}
	if code != output.ExitInternal || stdout != "" || json.Unmarshal([]byte(stderr), &failure) != nil ||
		failure.OK || failure.Identity != "bot" || failure.Error.Type != errs.CategoryInternal ||
		!strings.Contains(failure.Error.Message, envvars.CliTenantAccessTokenFile) ||
		strings.Contains(stderr, "misplaced-fixture-secret") || len(forwardedTokens) != 0 || kc.reads != readsBefore {
		t.Fatalf("invalid path exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}
