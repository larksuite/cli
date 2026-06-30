// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consumecli

import (
	"context"
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/appmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	eventlib "github.com/larksuite/cli/internal/event"
)

func TestSanitizeOutputDirPreservesPathValidationCause(t *testing.T) {
	_, err := SanitizeOutputDir("../escape")
	if err == nil {
		t.Fatal("expected unsafe output-dir error")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *errs.ValidationError", err)
	}
	if validationErr.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", validationErr.Subtype, errs.SubtypeInvalidArgument)
	}
	if validationErr.Param != "--output-dir" {
		t.Fatalf("param = %q, want --output-dir", validationErr.Param)
	}
	if errors.Is(err, ErrOutputDirUnsafe) {
		t.Fatalf("cause should be concrete path validation error, not sentinel: %v", err)
	}
	if errors.Unwrap(err) == nil {
		t.Fatalf("expected concrete path validation cause: %v", err)
	}
}

func TestPreflightScopesUsesParamAwareScopes(t *testing.T) {
	f := &cmdutil.Factory{
		Credential: credential.NewCredentialProvider(nil, nil, &preflightTokenResolver{
			result: &credential.TokenResult{Token: "t", Scopes: "mail:event"},
		}, nil),
	}
	pf := &PreflightCtx{
		Factory:  f,
		AppID:    "app-1",
		Identity: core.AsUser,
		KeyDef: &eventlib.KeyDefinition{
			Key:    "test.mail",
			Scopes: []string{"mail:event", "mail:user_mailbox.message:readonly"},
			ScopesForParams: func(params map[string]string) []string {
				if params["msg_format"] == "event" {
					return []string{"mail:event"}
				}
				return []string{"mail:event", "mail:user_mailbox.message:readonly"}
			},
		},
		Params: map[string]string{"msg_format": "event"},
		AppVer: &appmeta.AppVersion{},
	}

	if err := PreflightScopes(context.Background(), pf); err != nil {
		t.Fatalf("event format should only require mail:event: %v", err)
	}

	pf.Params = map[string]string{"msg_format": "metadata"}
	err := PreflightScopes(context.Background(), pf)
	if err == nil {
		t.Fatal("metadata format should require message scopes")
	}
	var permErr *errs.PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("error type = %T, want *errs.PermissionError", err)
	}
	if len(permErr.MissingScopes) != 1 || permErr.MissingScopes[0] != "mail:user_mailbox.message:readonly" {
		t.Fatalf("missing scopes = %#v", permErr.MissingScopes)
	}
}

type preflightTokenResolver struct {
	result *credential.TokenResult
	err    error
}

func (r *preflightTokenResolver) ResolveToken(context.Context, credential.TokenSpec) (*credential.TokenResult, error) {
	return r.result, r.err
}
