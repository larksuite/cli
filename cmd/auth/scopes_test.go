// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// stubGetAppInfoErr swaps getAppInfoFn for the duration of t so authScopesRun
// observes a fixed error from the dependency. t.Cleanup restores the prior
// value so tests cannot leak through the package-level seam.
func stubGetAppInfoErr(t *testing.T, errToReturn error) {
	t.Helper()
	prev := getAppInfoFn
	getAppInfoFn = func(ctx context.Context, f *cmdutil.Factory, appId string) (*appInfo, error) {
		return nil, errToReturn
	}
	t.Cleanup(func() { getAppInfoFn = prev })
}

// scopesTestFactory builds a Factory + ScopesOptions pair sufficient to drive
// authScopesRun. Config has a non-empty AppID so we get past the config gate
// and reach the getAppInfoFn call.
func scopesTestFactory(t *testing.T) *ScopesOptions {
	t.Helper()
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID:     "test-app",
		AppSecret: "test-secret",
		Brand:     core.BrandFeishu,
	})
	return &ScopesOptions{
		Factory: f,
		Ctx:     context.Background(),
		Format:  "json",
	}
}

// TestAuthScopesRun_NetworkErrorPassedThrough pins that a typed NetworkError
// surfaced by the dependency is not re-classified as PermissionError —
// re-auth does not fix DNS / transport failures and blanket-wrapping them
// would mislead agents into infinite re-auth loops.
func TestAuthScopesRun_NetworkErrorPassedThrough(t *testing.T) {
	netErr := errs.NewNetworkError(errs.SubtypeNetworkDNS, "DNS lookup failed")
	stubGetAppInfoErr(t, netErr)

	err := authScopesRun(scopesTestFactory(t))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var permErr *errs.PermissionError
	if errors.As(err, &permErr) {
		t.Errorf("network failure must not be classified as PermissionError; got %v", permErr)
	}
	var gotNet *errs.NetworkError
	if !errors.As(err, &gotNet) {
		t.Fatalf("network failure not preserved through authScopesRun; got %T: %v", err, err)
	}
	if gotNet != netErr {
		t.Errorf("typed network error should pass through identity-stable; got %p, want %p", gotNet, netErr)
	}
}

// TestAuthScopesRun_PermissionErrorPassedThrough pins that typed permission
// failures from the dependency also pass through — IsTyped() must not single
// out one category.
func TestAuthScopesRun_PermissionErrorPassedThrough(t *testing.T) {
	permErr := errs.NewPermissionError(errs.SubtypeMissingScope, "scope X missing").
		WithMissingScopes("im:message")
	stubGetAppInfoErr(t, permErr)

	err := authScopesRun(scopesTestFactory(t))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var got *errs.PermissionError
	if !errors.As(err, &got) {
		t.Fatalf("expected *PermissionError pass-through, got %T: %v", err, err)
	}
	if got != permErr {
		t.Errorf("typed permission error should pass through identity-stable; got %p, want %p", got, permErr)
	}
}

// TestAuthScopesRun_BareErrorWrappedAsInternal pins the unclassified branch:
// a bare error (e.g. json.Unmarshal failure inside getAppInfo) surfaces as
// *InternalError{SubtypeSDKError} with the original error preserved on
// Cause so errors.Is still walks to it.
func TestAuthScopesRun_BareErrorWrappedAsInternal(t *testing.T) {
	bareErr := fmt.Errorf("failed to parse response: unexpected EOF")
	stubGetAppInfoErr(t, bareErr)

	err := authScopesRun(scopesTestFactory(t))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var permErr *errs.PermissionError
	if errors.As(err, &permErr) {
		t.Errorf("bare getAppInfo error must not be classified as PermissionError; got %v", permErr)
	}

	var intErr *errs.InternalError
	if !errors.As(err, &intErr) {
		t.Fatalf("expected *InternalError, got %T: %v", err, err)
	}
	if intErr.Subtype != errs.SubtypeSDKError {
		t.Errorf("InternalError.Subtype = %q, want %q", intErr.Subtype, errs.SubtypeSDKError)
	}
	if !errors.Is(err, bareErr) {
		t.Error("InternalError must carry bareErr via WithCause so errors.Is walks to it")
	}
}
