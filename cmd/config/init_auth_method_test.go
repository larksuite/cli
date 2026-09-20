// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
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
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/vfs"
)

type registrationTestTransport func(*http.Request) (*http.Response, error)

func (f registrationTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestConfigInitPrivateKeyJWTKeepsRegistrationMethod(t *testing.T) {
	for _, tc := range []struct {
		name      string
		supported []string
		wantError bool
		software  bool
	}{
		{"old method only", []string{core.AuthMethodPrivateKeyJWT}, false, false},
		{"both methods", []string{core.AuthMethodPrivateKeyJWT, core.AuthMethodPrivateKeyJWTLocalKeyPair}, false, false},
		{"local method only", []string{core.AuthMethodPrivateKeyJWTLocalKeyPair}, true, false},
		{"software fallback keeps old method", []string{core.AuthMethodPrivateKeyJWT}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearAgentEnv(t)
			t.Setenv("LARK_CLI_NO_PROXY", "")
			t.Setenv("LARKSUITE_CLI_PROXY_ENABLE", "false")
			t.Setenv("LARKSUITE_CLI_PROXY_ADDRESS", "")
			t.Setenv("LARKSUITE_CLI_CA_PATH", "")
			signers := []keysigner.Signer{newProbeTestSigner(t)}
			rt := &fakeRT{}
			f, _ := fakeFactory(t, rt)
			wantProvider := keysigner.MacOSKeychainSignerName
			if tc.software {
				wantProvider = keysigner.SoftwareSignerName
				software := newProbeTestSigner(t)
				software.name = keysigner.SoftwareSignerName
				signers = []keysigner.Signer{authMethodTestSigner{ensureErr: keysigner.ErrUnavailable}, software}
			}
			out := new(bytes.Buffer)
			f.IOStreams.Out = out
			var actions []string
			previousTransport := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = previousTransport })
			http.DefaultTransport = registrationTestTransport(func(req *http.Request) (*http.Response, error) {
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
					body, err := json.Marshal(map[string]interface{}{"nonce": "test-nonce", "supported_auth_methods": tc.supported})
					if err != nil {
						t.Fatal(err)
					}
					return jsonResp(200, string(body)), nil
				case "begin":
					if req.Form.Get("auth_method") != core.AuthMethodPrivateKeyJWT || req.Form.Get("auth_attestation") == "" {
						t.Fatalf("begin did not preserve private_key_jwt: %v", req.Form)
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
				opts.registrationSigners = signers
				return configInitRun(opts)
			})
			cmd.SetArgs([]string{"--new", "--private-key-jwt"})
			err := cmd.ExecuteContext(ctx)
			if tc.wantError {
				problem, ok := errs.ProblemOf(err)
				if !ok || problem.Category != errs.CategoryConfig || problem.Subtype != errs.SubtypeInvalidClient {
					t.Fatalf("unsupported method error = %v", err)
				}
				if !slices.Equal(actions, []string{"init"}) || rt.oauthCalls != 0 || out.Len() != 0 {
					t.Fatalf("unsupported method continued: actions=%v, token calls=%d, output=%s", actions, rt.oauthCalls, out)
				}
				if _, err := vfs.Stat(core.GetConfigPath()); err == nil {
					t.Fatal("unsupported method saved a config")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(actions, []string{"init", "begin", "poll"}) || rt.oauthCalls != 1 {
				t.Fatalf("actions=%v, token calls=%d", actions, rt.oauthCalls)
			}
			saved, err := core.LoadMultiAppConfig()
			if err != nil {
				t.Fatal(err)
			}
			app := saved.CurrentAppConfig("")
			if app == nil || app.AuthMethod != core.AuthMethodPrivateKeyJWT || app.KeyRef == nil || app.KeyRef.Source != core.SecretSourceTEE || app.KeyRef.Provider != wantProvider {
				t.Fatalf("saved app = %+v", app)
			}
			var result struct {
				AuthMethod string `json:"authMethod"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.AuthMethod != core.AuthMethodPrivateKeyJWT {
				t.Fatalf("output = %s, error = %v", out, err)
			}
		})
	}
}

func TestConfigInitPrivateKeyFileSelectsLocalMethod(t *testing.T) {
	clearAgentEnv(t)
	key := newProbeTestSigner(t).key
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	path := filepath.Join(t.TempDir(), "private.pem")
	if err := vfs.WriteFile(path, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	rt := &fakeRT{oauthHandler: func(req *http.Request) (*http.Response, error) {
		if err := req.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if req.Method != http.MethodPost || req.Form.Get("client_id") != "cli_file" || req.Form.Get("client_assertion") == "" || req.Form.Has("client_secret") {
			t.Fatalf("file assertion request = %s %v", req.Method, req.Form)
		}
		return jsonResp(200, `{"access_token":"test-token"}`), nil
	}}
	f, _ := fakeFactory(t, rt)
	out := new(bytes.Buffer)
	f.IOStreams.Out = out
	cmd := NewCmdConfigInit(f, nil)
	cmd.SetArgs([]string{"--app-id", "cli_file", "--private-key-file", path})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if rt.oauthCalls != 1 || rt.tatCalls != 0 {
		t.Fatalf("token calls: assertion=%d, secret=%d", rt.oauthCalls, rt.tatCalls)
	}
	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	app := saved.CurrentAppConfig("")
	if app == nil || app.AuthMethod != core.AuthMethodPrivateKeyJWTLocalKeyPair || app.KeyRef == nil ||
		app.KeyRef.Source != core.SecretSourceKeyFile || app.KeyRef.ID != path || app.KeyRef.Provider != "" || !app.AppSecret.IsZero() {
		t.Fatalf("saved file app = %+v", app)
	}
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
	if data, err := vfs.ReadFile(path); err != nil || !bytes.Equal(data, keyPEM) {
		t.Fatalf("private key changed: %v", err)
	}
}

type authMethodTestSigner struct {
	ensureErr error
	signErr   error
}

func (authMethodTestSigner) Name() string { return keysigner.MacOSKeychainSignerName }

func (authMethodTestSigner) SecurityLevel() keysigner.SecurityLevel { return keysigner.SecurityLevelL2 }

func (s authMethodTestSigner) EnsureKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	if s.ensureErr != nil {
		return nil, s.ensureErr
	}
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	return private.Public(), nil
}

func (authMethodTestSigner) PublicKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	return nil, nil
}

func (s authMethodTestSigner) Sign(context.Context, keysigner.KeyRef, []byte) ([]byte, string, error) {
	if s.signErr != nil {
		return nil, "", s.signErr
	}
	return make([]byte, 64), keysigner.AlgES256, nil
}

func (authMethodTestSigner) DeleteKey(context.Context, keysigner.KeyRef) error { return nil }

// TestResolveRegisterAuthMethod covers the non-interactive gating paths. The
// darwin keychain signer is compiled into every build, so the test cannot rely
// on the binary lacking a signer — it forces a known no-signer state for the
// rejection cases, then registers a stub for the success case.
func TestResolveRegisterAuthMethod(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	ctx := context.Background()

	if m, err := resolveRegisterAuthMethod(ctx, core.AuthMethodClientSecret, nil); err != nil || m != core.AuthMethodClientSecret {
		t.Errorf("client_secret: got (%q, %v), want (client_secret, nil)", m, err)
	}

	if m, err := resolveRegisterAuthMethod(ctx, "", nil); err != nil || m != core.AuthMethodClientSecret {
		t.Errorf("default: got (%q, %v), want (client_secret, nil)", m, err)
	}

	if _, err := resolveRegisterAuthMethod(ctx, "bogus", nil); err == nil {
		t.Error("bogus auth-method: expected error")
	}

	for _, method := range []string{
		core.AuthMethodPrivateKeyJWT,
		core.AuthMethodPrivateKeyJWTLocalKeyPair,
	} {
		if _, err := resolveRegisterAuthMethod(ctx, method, nil); err == nil {
			t.Errorf("%s without a signer: expected error", method)
		}
	}

	signers := []keysigner.Signer{authMethodTestSigner{}}
	if got, err := resolveRegisterAuthMethod(ctx, core.AuthMethodPrivateKeyJWT, signers); err != nil || got != core.AuthMethodPrivateKeyJWT {
		t.Errorf("private_key_jwt with signer: got (%q, %v)", got, err)
	}
	if _, err := resolveRegisterAuthMethod(ctx, core.AuthMethodPrivateKeyJWTLocalKeyPair, signers); err == nil {
		t.Fatal("local key-pair authentication must not enter platform registration")
	} else if problem, ok := errs.ProblemOf(err); !ok || problem.Category != errs.CategoryValidation ||
		problem.Subtype != errs.SubtypeInvalidArgument || !strings.Contains(problem.Hint, "--private-key-file") {
		t.Fatalf("local key-pair registration error = %v", err)
	}

	if m, err := resolveRegisterAuthMethod(ctx, "", signers); err != nil || m != core.AuthMethodClientSecret {
		t.Errorf("default with terminal signer: got (%q, %v), want (client_secret, nil)", m, err)
	}
}

func TestConfigInitRunRejectsPrivateKeyJWTIncompatibleModes(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*ConfigInitOptions, *cmdutil.Factory)
		wantTarget string
		wantParam  string
	}{
		{
			name: "target app id",
			configure: func(opts *ConfigInitOptions, _ *cmdutil.Factory) {
				opts.AppID = "cli_test"
			},
			wantTarget: "--app-id",
			wantParam:  "--private-key-jwt",
		},
		{
			name: "new app with target app id",
			configure: func(opts *ConfigInitOptions, _ *cmdutil.Factory) {
				opts.New = true
				opts.AppID = "cli_test"
			},
			wantTarget: "--app-id",
			wantParam:  "--private-key-jwt",
		},
		{
			name: "app secret stdin import",
			configure: func(opts *ConfigInitOptions, f *cmdutil.Factory) {
				opts.AppSecretStdin = true
				f.IOStreams.In = strings.NewReader("secret\n")
			},
			wantTarget: "--app-secret-stdin",
			wantParam:  "--private-key-jwt",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, _, _, _ := cmdutil.TestFactory(t, nil)
			opts := &ConfigInitOptions{
				Factory:       f,
				Ctx:           context.Background(),
				PrivateKeyJWT: true,
			}
			tc.configure(opts, f)

			err := configInitRun(opts)
			if err == nil {
				t.Fatal("expected incompatible mode error")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error is not typed: %T %[1]v", err)
			}
			if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("problem = %s/%s, want validation/invalid_argument", problem.Category, problem.Subtype)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T, want *errs.ValidationError", err)
			}
			if validationErr.Param != tc.wantParam {
				t.Fatalf("param = %q, want %s", validationErr.Param, tc.wantParam)
			}
			if !strings.Contains(problem.Message, tc.wantTarget) {
				t.Fatalf("message = %q, want %s", problem.Message, tc.wantTarget)
			}
		})
	}
}

func TestResolveRegisterAuthMethod_PrivateKeyJWTRejectsUnavailableHardware(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	_, err := resolveRegisterAuthMethod(context.Background(), core.AuthMethodPrivateKeyJWT, []keysigner.Signer{
		authMethodTestSigner{ensureErr: keysigner.ErrUnavailable},
	})
	if err == nil {
		t.Fatal("private_key_jwt with unavailable signer hardware: expected error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error is not typed: %T %[1]v", err)
	}
	if problem.Category != errs.CategoryConfig || problem.Subtype != errs.SubtypeInvalidClient {
		t.Fatalf("problem = %s/%s, want config/invalid_client", problem.Category, problem.Subtype)
	}
	wantMessage := "this machine does not support --private-key-jwt"
	if problem.Message != wantMessage {
		t.Fatalf("message = %q, want %q", problem.Message, wantMessage)
	}
	if strings.Contains(problem.Message, "sks") || strings.Contains(problem.Message, "/dev/tpm") || strings.Contains(problem.Message, "tpm") || strings.Contains(problem.Message, "TEE") || strings.Contains(problem.Message, "Keychain") {
		t.Fatalf("message exposes backend detail: %q", problem.Message)
	}
	if !strings.Contains(problem.Hint, "omit --private-key-jwt") {
		t.Fatalf("hint = %q, want guidance to omit --private-key-jwt", problem.Hint)
	}
	if strings.Contains(problem.Hint, "fix the local signer") {
		t.Fatalf("hint exposes unnecessary signer recovery: %q", problem.Hint)
	}
}

func TestResolveRegisterAuthMethod_PrivateKeyJWTRejectsProbeError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	probeErr := errors.New("probe exploded")

	_, err := resolveRegisterAuthMethod(context.Background(), core.AuthMethodPrivateKeyJWT, []keysigner.Signer{
		authMethodTestSigner{signErr: probeErr},
	})
	if err == nil {
		t.Fatal("private_key_jwt with probe error: expected error")
	}
	if !errors.Is(err, probeErr) {
		t.Fatalf("error does not preserve probe cause: %v", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error is not typed: %T %[1]v", err)
	}
	if problem.Category != errs.CategoryConfig || problem.Subtype != errs.SubtypeInvalidClient {
		t.Fatalf("problem = %s/%s, want config/invalid_client", problem.Category, problem.Subtype)
	}
	wantMessage := "this machine does not support --private-key-jwt"
	if problem.Message != wantMessage {
		t.Fatalf("message = %q, want %q", problem.Message, wantMessage)
	}
	if strings.Contains(problem.Message, "probe") || strings.Contains(problem.Message, "keychain signer") {
		t.Fatalf("message exposes probe detail: %q", problem.Message)
	}
	if !strings.Contains(problem.Hint, "omit --private-key-jwt") {
		t.Fatalf("hint = %q, want guidance to omit --private-key-jwt", problem.Hint)
	}
	if strings.Contains(problem.Hint, "fix the local signer") {
		t.Fatalf("hint exposes unnecessary signer recovery: %q", problem.Hint)
	}
}

func TestConfigInitRun_PrivateKeyJWTRejectsBeforeInteractiveMode(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.IOStreams.IsTerminal = true
	opts := &ConfigInitOptions{
		Factory:       f,
		Ctx:           context.Background(),
		PrivateKeyJWT: true,
		Lang:          "zh_cn",
		UILang:        "zh_cn",
		registrationSigners: []keysigner.Signer{
			authMethodTestSigner{ensureErr: keysigner.ErrUnavailable},
		},
	}

	for _, terminal := range []bool{true, false} {
		f.IOStreams.IsTerminal = terminal
		err := configInitRun(opts)
		if err == nil {
			t.Fatal("config init --private-key-jwt on unsupported machine: expected error before interactive mode")
		}
		problem, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("error is not typed: %T %[1]v", err)
		}
		if problem.Category != errs.CategoryConfig || problem.Subtype != errs.SubtypeInvalidClient {
			t.Fatalf("problem = %s/%s, want config/invalid_client", problem.Category, problem.Subtype)
		}
		if problem.Message != "this machine does not support --private-key-jwt" {
			t.Fatalf("message = %q", problem.Message)
		}
	}
}

func TestExistingAppRequiresSecret(t *testing.T) {
	if !existingAppRequiresSecret(core.AuthMethodClientSecret) {
		t.Error("client_secret existing app should require App Secret")
	}
	if existingAppRequiresSecret("") != true {
		t.Error("default existing app should require App Secret")
	}
	for _, method := range []string{
		core.AuthMethodPrivateKeyJWT,
		core.AuthMethodPrivateKeyJWTLocalKeyPair,
	} {
		if existingAppRequiresSecret(method) {
			t.Errorf("%s existing app should not require App Secret", method)
		}
	}
}

// TestValidatePKJWTKeyBinding covers the guard that rejects a registration
// resolving to either private-key JWT method with no signing key bound.
func TestValidatePKJWTKeyBinding(t *testing.T) {
	for _, method := range []string{
		core.AuthMethodPrivateKeyJWT,
		core.AuthMethodPrivateKeyJWTLocalKeyPair,
	} {
		if err := validatePKJWTKeyBinding(method, ""); err == nil {
			t.Errorf("%s with empty keyLabel: expected error", method)
		}
		if err := validatePKJWTKeyBinding(method, "agent-key"); err != nil {
			t.Errorf("%s with keyLabel: expected nil, got %v", method, err)
		}
	}
	if err := validatePKJWTKeyBinding(core.AuthMethodClientSecret, ""); err != nil {
		t.Errorf("client_secret: expected nil, got %v", err)
	}
}
