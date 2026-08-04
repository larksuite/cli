// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar

package credential_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	sidecarprovider "github.com/larksuite/cli/extension/credential/sidecar"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/sidecar"
)

func newRealSidecarCredentialProvider(t *testing.T) *credential.CredentialProvider {
	t.Helper()
	t.Setenv(envvars.CliAuthProxy, "http://127.0.0.1:16384")
	t.Setenv(envvars.CliProxyKey, "test-key")
	t.Setenv(envvars.CliAppID, "cli_sidecar")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv(envvars.CliDefaultAs, "")
	t.Setenv(envvars.CliStrictMode, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	return credential.NewCredentialProvider(
		[]extcred.Provider{&sidecarprovider.Provider{}},
		nil,
		nil,
		nil,
	)
}

func TestAuthSidecarInvalidPolicyUsesValidationContract(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  string
	}{
		{name: "default as", key: envvars.CliDefaultAs},
		{name: "strict mode", key: envvars.CliStrictMode},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cp := newRealSidecarCredentialProvider(t)
			t.Setenv(tt.key, "banana")

			_, err := cp.ResolveAccount(context.Background())
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error = %T %v, want typed validation error", err, err)
			}
			if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("problem = %s/%s, want %s/%s", problem.Category, problem.Subtype, errs.CategoryValidation, errs.SubtypeInvalidArgument)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T %v, want ValidationError", err, err)
			}
			if validationErr.Param != tt.key {
				t.Fatalf("param = %q, want %q", validationErr.Param, tt.key)
			}
			if got := output.ExitCodeOf(err); got != output.ExitValidation {
				t.Fatalf("exit code = %d, want %d", got, output.ExitValidation)
			}
			if !strings.Contains(problem.Hint, tt.key) {
				t.Fatalf("hint = %q, want variable name %s", problem.Hint, tt.key)
			}
			var blockErr *extcred.BlockError
			if !errors.As(err, &blockErr) ||
				blockErr.Code != extcred.BlockReasonInvalidPolicy ||
				blockErr.Param != tt.key {
				t.Fatalf("cause = %T %v, want classified BlockError for %s", err, err, tt.key)
			}
		})
	}
}

func TestAuthSidecarGateProbeUsesValidationContract(t *testing.T) {
	cp := newRealSidecarCredentialProvider(t)
	t.Setenv(envvars.CliStrictMode, "banana")

	name, err := cp.ActiveExtensionProviderName(context.Background())
	if name != "" {
		t.Fatalf("provider name = %q, want empty on invalid policy", name)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T %v, want typed validation error", err, err)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want ValidationError", err, err)
	}
	if problem.Category != errs.CategoryValidation ||
		problem.Subtype != errs.SubtypeInvalidArgument ||
		validationErr.Param != envvars.CliStrictMode {
		t.Fatalf("problem = %+v param = %q, want validation/invalid_argument param %s", problem, validationErr.Param, envvars.CliStrictMode)
	}
}

func TestAuthSidecarTokenHonorsSelectedAppID(t *testing.T) {
	t.Run("matching app returns sentinel", func(t *testing.T) {
		cp := newRealSidecarCredentialProvider(t)

		result, err := cp.ResolveToken(context.Background(), credential.TokenSpec{
			Type:  credential.TokenTypeUAT,
			AppID: "cli_sidecar",
		})
		if err != nil {
			t.Fatalf("ResolveToken: %v", err)
		}
		if result == nil || result.Token != sidecar.SentinelUAT {
			t.Fatalf("result = %+v, want sidecar UAT sentinel", result)
		}
	})

	for _, tt := range []struct {
		name  string
		appID string
	}{
		{name: "empty app id", appID: ""},
		{name: "conflicting app id", appID: "cli_other"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cp := newRealSidecarCredentialProvider(t)

			result, err := cp.ResolveToken(context.Background(), credential.TokenSpec{
				Type:  credential.TokenTypeUAT,
				AppID: tt.appID,
			})
			if result != nil {
				t.Fatalf("result = %+v, want no sidecar sentinel", result)
			}
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error = %T %v, want typed internal error", err, err)
			}
			if problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeUnknown {
				t.Fatalf("problem = %s/%s, want %s/%s", problem.Category, problem.Subtype, errs.CategoryInternal, errs.SubtypeUnknown)
			}
			if strings.Contains(err.Error(), sidecar.SentinelUAT) {
				t.Fatalf("error leaked sidecar sentinel: %v", err)
			}
		})
	}
}
