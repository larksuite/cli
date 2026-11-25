// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/auth/jwt"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keylesshelper"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/vfs"
)

type registrationTestTransport func(*http.Request) (*http.Response, error)

func (f registrationTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func isolateRegistrationStorage(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", root)
	t.Setenv("LARKSUITE_CLI_DATA_DIR", root)
}

func mockRegistrationTransport(t *testing.T, handler registrationTestTransport) {
	t.Helper()
	t.Setenv("LARK_CLI_NO_PROXY", "")
	t.Setenv("LARKSUITE_CLI_PROXY_ENABLE", "false")
	t.Setenv("LARKSUITE_CLI_PROXY_ADDRESS", "")
	t.Setenv("LARKSUITE_CLI_CA_PATH", "")
	previousTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	http.DefaultTransport = handler
}

func TestConfigInitNewDefaultsToClientSecret(t *testing.T) {
	clearAgentEnv(t)
	mockRegistrationTransport(t, func(req *http.Request) (*http.Response, error) {
		switch req.FormValue("action") {
		case "begin":
			if req.Form.Get("auth_method") != core.AuthMethodClientSecret || req.Form.Has("auth_attestation") {
				t.Fatal("expected client_secret registration without an attestation")
			}
			return jsonResp(200, `{"device_code":"test-device","verification_uri_complete":"https://example.com/verify","expire_in":5,"interval":1}`), nil
		case "poll":
			return jsonResp(200, `{"client_id":"cli_test","client_secret":"test-secret"}`), nil
		default:
			return nil, errors.New("unexpected registration action")
		}
	})
	rt := &fakeRT{}
	f, _ := fakeFactory(t, rt)
	cmd := NewCmdConfigInit(f, nil)
	cmd.SetArgs([]string{"--new"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if rt.tatCalls != 1 || rt.probeCalls != 1 {
		t.Fatalf("client-secret registration made %d TAT and %d probe calls", rt.tatCalls, rt.probeCalls)
	}
	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	app := saved.CurrentAppConfig("")
	if app == nil || app.AppId != "cli_test" || app.AppSecret.IsZero() || app.AuthMethod != "" || app.KeyRef != nil {
		t.Fatal("expected app-secret configuration without signing fields")
	}
}

func registrationTokenTransport(t *testing.T, appID string) *fakeRT {
	t.Helper()
	return &fakeRT{tatHandler: func(req *http.Request) (*http.Response, error) {
		if err := req.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := req.Form.Get("grant_type"); got != jwt.GrantTypeJWTBearer {
			t.Fatalf("grant_type = %q", got)
		}
		if req.Header.Get("DPoP") != "" {
			t.Fatal("registration probe requested a DPoP-bound token")
		}
		if req.Method != http.MethodPost || req.Form.Get("client_id") != appID || req.Form.Get("client_assertion") == "" ||
			req.Form.Has("client_secret") || req.Header.Get("Authorization") != "" {
			t.Fatalf("registration probe did not use a client assertion: %v", req.Form)
		}
		return jsonResp(200, `{"access_token":"test-token","expires_in":7200,"token_type":"Bearer"}`), nil
	}}
}

func TestConfigInitPrivateKeyJWTLocalKeyPairRegistration(t *testing.T) {
	for _, tc := range []struct {
		name          string
		supported     []string
		failedActions []string
		provider      string
		profile       string
		targetAppID   string
	}{
		{"client secret only", []string{core.AuthMethodClientSecret}, []string{"init"}, keysigner.MacOSKeychainSignerName, "", ""},
		{"client_secret and private_key_jwt_local_keypair", []string{core.AuthMethodClientSecret, core.AuthMethodPrivateKeyJWTLocalKeyPair}, nil, keysigner.MacOSKeychainSignerName, "", ""},
		{"private_key_jwt_local_keypair only", []string{core.AuthMethodPrivateKeyJWTLocalKeyPair}, nil, keysigner.MacOSSecureEnclaveSignerName, "prod", ""},
		{"capabilities omitted", nil, nil, keysigner.MacOSKeychainSignerName, "", ""},
		{"software fallback uses private_key_jwt_local_keypair", []string{core.AuthMethodPrivateKeyJWTLocalKeyPair}, nil, keysigner.SoftwareSignerName, "", ""},
		{"existing app", nil, nil, keysigner.MacOSKeychainSignerName, "prod", "cli_test"},
		{"existing app mismatch", nil, []string{"init", "begin", "poll"}, keysigner.MacOSKeychainSignerName, "", "cli_other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearAgentEnv(t)
			registeredSigner := newProbeTestSigner(t)
			registeredSigner.name = tc.provider
			signers := []keysigner.Signer{registeredSigner}
			rt := registrationTokenTransport(t, "cli_test")
			f, stderr := fakeFactory(t, rt)
			if tc.provider == keysigner.SoftwareSignerName {
				signers = []keysigner.Signer{&probeTestSigner{ensureErr: keysigner.ErrUnavailable}, registeredSigner}
			}
			out := new(bytes.Buffer)
			f.IOStreams.Out = out
			var actions []string
			mockRegistrationTransport(t, func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost || req.URL.String() != core.ResolveEndpoints(core.BrandFeishu).Accounts+larkauth.PathAppRegistration {
					t.Fatalf("unexpected registration request: %s %s", req.Method, req.URL)
				}
				if err := req.ParseForm(); err != nil {
					t.Fatal(err)
				}
				action := req.Form.Get("action")
				actions = append(actions, action)
				switch action {
				case "init":
					if registeredSigner.deleteCalls != 1 {
						t.Fatalf("temporary keys deleted before init = %d, want 1", registeredSigner.deleteCalls)
					}
					body, err := json.Marshal(map[string]interface{}{"nonce": "test-nonce", "supported_auth_methods": tc.supported})
					if err != nil {
						t.Fatal(err)
					}
					return jsonResp(200, string(body)), nil
				case "begin":
					if req.Form.Get("client_id") != tc.targetAppID || req.Form.Has("client_id") != (tc.targetAppID != "") {
						t.Fatalf("begin client_id = %v, want %q", req.Form["client_id"], tc.targetAppID)
					}
					if req.Form.Get("auth_method") != core.AuthMethodPrivateKeyJWTLocalKeyPair {
						t.Fatal("incorrect registration authentication method")
					}
					parts := strings.Split(req.Form.Get("auth_attestation"), ".")
					if len(parts) != 3 {
						t.Fatal("begin sent an invalid attestation")
					}
					payload, err := base64.RawURLEncoding.DecodeString(parts[1])
					var claims struct {
						Nonce string `json:"nonce"`
					}
					if err != nil || json.Unmarshal(payload, &claims) != nil || claims.Nonce != "test-nonce" {
						t.Fatal("begin attestation did not use the nonce from init")
					}
					return jsonResp(200, `{"device_code":"test-device","verification_uri_complete":"https://example.com/verify","expire_in":60,"interval":1}`), nil
				case "poll":
					return jsonResp(200, `{"client_id":"cli_test"}`), nil
				default:
					t.Fatalf("unexpected registration action: %q", action)
					return nil, errors.New("unexpected registration action")
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := NewCmdConfigInit(f, func(opts *ConfigInitOptions) error {
				opts.registrationStore = keylesshelper.NewKeyStore(signers...)
				return configInitRun(opts)
			})
			args := []string{"--new", "--private-key-jwt"}
			if tc.targetAppID != "" {
				args = []string{"--app-id", tc.targetAppID, "--private-key-jwt"}
			}
			if tc.profile != "" {
				args = append(args, "--name", tc.profile)
			}
			cmd.SetArgs(args)
			err := cmd.ExecuteContext(ctx)
			if tc.targetAppID != "" && !strings.Contains(stderr.String(), "clientID="+tc.targetAppID) {
				t.Fatalf("verification URL missing target app %q (command error: %v)", tc.targetAppID, err)
			}
			if tc.failedActions != nil {
				problem, ok := errs.ProblemOf(err)
				if !ok || problem.Category != errs.CategoryConfig || problem.Subtype != errs.SubtypeInvalidClient {
					t.Fatalf("registration error = %v", err)
				}
				if !slices.Equal(actions, tc.failedActions) || rt.tatCalls != 0 || out.Len() != 0 {
					t.Fatalf("failed registration: actions=%v, token calls=%d, output=%s", actions, rt.tatCalls, out)
				}
				if _, err := vfs.Stat(core.GetConfigPath()); err == nil {
					t.Fatal("failed registration saved a config")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(actions, []string{"init", "begin", "poll"}) || rt.tatCalls != 1 || rt.probeCalls != 0 {
				t.Fatalf("actions=%v, TAT calls=%d, probe calls=%d", actions, rt.tatCalls, rt.probeCalls)
			}
			saved, err := core.LoadMultiAppConfig()
			if err != nil {
				t.Fatal(err)
			}
			app := saved.CurrentAppConfig(tc.profile)
			if app == nil || app.AppId != "cli_test" || (tc.profile != "" && app.Name != tc.profile) ||
				app.AuthMethod != core.AuthMethodPrivateKeyJWTLocalKeyPair || !app.AppSecret.IsZero() {
				t.Fatalf("saved app = %+v", app)
			}
			if ref := app.KeyRef; ref == nil || ref.ID == "" || ref.Source != core.KeySourceTEE || ref.Provider != tc.provider {
				t.Fatalf("saved keyRef = %+v", ref)
			}
			assertRegistrationOutput(t, out, registeredSigner.key)
		})
	}
}

func assertRegistrationOutput(t *testing.T, out *bytes.Buffer, key *ecdsa.PrivateKey) {
	t.Helper()
	var result struct {
		AuthMethod string `json:"authMethod"`
		KeyID      string `json:"kid"`
	}
	wantKid, err := keysigner.PublicKeyThumbprint(key.Public())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.AuthMethod != core.AuthMethodPrivateKeyJWTLocalKeyPair || result.KeyID != wantKid {
		t.Fatalf("output = %s, error = %v", out, err)
	}
}

func writeTestPrivateKey(t *testing.T, key *ecdsa.PrivateKey) (string, []byte) {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	path := filepath.Join(t.TempDir(), "private.pem")
	if err := vfs.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, data
}

func TestConfigInitPrivateKeyFileSelectsPrivateKeyJWTLocalKeyPair(t *testing.T) {
	for name, explicit := range map[string]bool{"implicit method": false, "explicit method": true} {
		t.Run(name, func(t *testing.T) {
			clearAgentEnv(t)
			key := newProbeTestSigner(t).key
			path, keyPEM := writeTestPrivateKey(t, key)
			rt := registrationTokenTransport(t, "cli_file")
			f, _ := fakeFactory(t, rt)
			out := new(bytes.Buffer)
			f.IOStreams.Out = out
			cmd := NewCmdConfigInit(f, func(opts *ConfigInitOptions) error {
				opts.registrationStore = keylesshelper.NewKeyStore()
				return configInitRun(opts)
			})
			inputPath := path
			if !explicit {
				cmdutil.TestChdir(t, filepath.Dir(path))
				inputPath = filepath.Base(path)
			}
			args := []string{"--app-id", "cli_file", "--private-key-file", inputPath}
			if explicit {
				args = append(args, "--private-key-jwt")
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if rt.tatCalls != 1 {
				t.Fatalf("token calls = %d, want 1", rt.tatCalls)
			}
			saved, err := core.LoadMultiAppConfig()
			if err != nil {
				t.Fatal(err)
			}
			app := saved.CurrentAppConfig("")
			if app == nil || app.AuthMethod != core.AuthMethodPrivateKeyJWTLocalKeyPair || app.KeyRef == nil ||
				app.KeyRef.Source != core.KeySourceFile || app.KeyRef.ID != path || app.KeyRef.Provider != "" || !app.AppSecret.IsZero() {
				t.Fatalf("saved file app = %+v", app)
			}
			assertRegistrationOutput(t, out, key)
			cfg, err := core.ResolveConfigFromMulti(saved, f.Keychain, "", core.ProfileFromConfig)
			if err != nil {
				t.Fatal(err)
			}
			signer, err := larkauth.ResolveConfigSigner(cfg, f.Keychain)
			if err != nil {
				t.Fatal(err)
			}
			if err := runProbePrivateKeyJWTLocalKeyPair(context.Background(), f, cfg.Brand, cfg.AppID, signer, cfg.KeyLabel); err != nil {
				t.Fatal(err)
			}
			if rt.tatCalls != 2 {
				t.Fatalf("reloaded token calls = %d, want 2", rt.tatCalls)
			}
			if data, err := vfs.ReadFile(path); err != nil || !bytes.Equal(data, keyPEM) {
				t.Fatalf("private key changed: %v", err)
			}
		})
	}
}

func TestConfigInitRejectsIncompatiblePrivateKeyJWTFlags(t *testing.T) {
	for _, tc := range []struct {
		target string
		args   []string
	}{
		{"--app-id", []string{"--new", "--app-id", "cli_test"}},
		{"--app-secret-stdin", []string{"--app-id", "cli_test", "--app-secret-stdin"}},
		{"--app-secret-stdin", []string{"--app-secret-stdin"}},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			clearAgentEnv(t)
			f, _ := fakeFactory(t, &fakeRT{})
			cmd := NewCmdConfigInit(f, nil)
			cmd.SetArgs(append([]string{"--private-key-jwt"}, tc.args...))
			err := cmd.Execute()
			var invalid *errs.ValidationError
			if !errors.As(err, &invalid) || invalid.Subtype != errs.SubtypeInvalidArgument || invalid.Param != "--private-key-jwt" || !strings.Contains(invalid.Message, tc.target) {
				t.Fatalf("expected conflicting flags in validation error, got %v", err)
			}
		})
	}
}

func TestConfigInitRejectsUnavailableSignerBeforeRegistration(t *testing.T) {
	signFailure := newProbeTestSigner(t)
	signFailure.signErr = errors.New("probe signing failed")
	for _, tc := range []struct {
		terminal bool
		signer   *probeTestSigner
		cause    error
	}{
		{true, signFailure, signFailure.signErr},
		{false, &probeTestSigner{ensureErr: keysigner.ErrUnavailable}, keysigner.ErrUnavailable},
	} {
		f, _ := fakeFactory(t, &fakeRT{})
		f.IOStreams.IsTerminal = tc.terminal
		err := configInitRun(&ConfigInitOptions{
			Factory: f, Ctx: context.Background(), PrivateKeyJWTLocalKeyPair: true,
			registrationStore: keylesshelper.NewKeyStore(tc.signer),
		})
		problem := assertRegistrationProblem(t, err, tc.cause, errs.CategoryConfig, errs.SubtypeInvalidClient)
		if problem.Message != "this machine does not support --private-key-jwt" || !strings.Contains(problem.Hint, "omit --private-key-jwt") {
			t.Fatalf("unexpected preflight error: %+v", problem)
		}
	}
}
