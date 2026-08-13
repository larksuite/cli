// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"context"
	"errors"
	"testing"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/runtimeplan"
)

type countingRuntimeCredentialProvider struct {
	accountCalls int
	tokenCalls   int
}

func (p *countingRuntimeCredentialProvider) Name() string { return "counting-runtime-provider" }

func (p *countingRuntimeCredentialProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	p.accountCalls++
	return &extcred.Account{
		AppID:               "test",
		Brand:               extcred.BrandFeishu,
		SupportedIdentities: extcred.SupportsBot,
	}, nil
}

func (p *countingRuntimeCredentialProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	p.tokenCalls++
	return &extcred.Token{Value: "must-not-be-resolved"}, nil
}

func TestRunShortcut_DeniedRuntimeCapabilityBlocksExecute(t *testing.T) {
	deniedErr := errors.New("runtime capability denied")
	f := newTestFactory()
	cmdutil.TestSetRuntimePlan(t, f, runtimeplan.New(runtimeplan.Options{
		Capabilities: func(capability runtimeplan.Capability) error {
			if capability == runtimeplan.CapabilityRealtimeEvents {
				return deniedErr
			}
			return nil
		},
	}))

	executed := false
	shortcut := &Shortcut{
		Service:   "test",
		Command:   "runtime-capability",
		AuthTypes: []string{"bot"},
		Execute: func(context.Context, *RuntimeContext) error {
			executed = true
			return nil
		},
	}
	cmd := newTestShortcutCmd(shortcut, f)
	cmdutil.SetRuntimeCapabilities(cmd, runtimeplan.CapabilityRealtimeEvents)
	cmd.Flags().Set("as", "bot")

	err := runShortcut(cmd, f, shortcut, false)
	if !errors.Is(err, deniedErr) {
		t.Fatalf("runShortcut() error = %v, want denied capability error", err)
	}
	if executed {
		t.Fatal("Execute ran despite a denied runtime capability")
	}
}

func TestRunShortcut_DryRunBypassesDeniedRuntimeCapability(t *testing.T) {
	deniedErr := errors.New("runtime capability denied")
	f := newTestFactory()
	cmdutil.TestSetRuntimePlan(t, f, runtimeplan.New(runtimeplan.Options{
		Capabilities: func(capability runtimeplan.Capability) error {
			if capability == runtimeplan.CapabilityRealtimeEvents {
				return deniedErr
			}
			return nil
		},
	}))

	dryRunCalled := false
	shortcut := &Shortcut{
		Service:   "test",
		Command:   "runtime-capability",
		AuthTypes: []string{"bot"},
		DryRun: func(context.Context, *RuntimeContext) *DryRunAPI {
			dryRunCalled = true
			return NewDryRunAPI().GET("/open-apis/test")
		},
		Execute: func(context.Context, *RuntimeContext) error {
			t.Fatal("Execute should not run in dry-run")
			return nil
		},
	}
	cmd := newTestShortcutCmd(shortcut, f)
	cmdutil.SetRuntimeCapabilities(cmd, runtimeplan.CapabilityRealtimeEvents)
	cmd.Flags().Set("as", "bot")
	cmd.Flags().Set("dry-run", "true")

	if err := runShortcut(cmd, f, shortcut, false); err != nil {
		t.Fatalf("runShortcut() error = %v, want dry-run to bypass denied capability", err)
	}
	if !dryRunCalled {
		t.Fatal("DryRun was not called after bypassing the denied capability")
	}
}

func TestRunShortcut_LocalIntrospectionSkipsRuntimeAndCredentialInitialization(t *testing.T) {
	deniedErr := errors.New("runtime capability denied")
	f := newTestFactory()
	configCalls := 0
	clientCalls := 0
	f.Config = func() (*core.CliConfig, error) {
		configCalls++
		return nil, errors.New("config must not be loaded")
	}
	f.LarkClient = func() (*lark.Client, error) {
		clientCalls++
		return nil, errors.New("client must not be initialized")
	}
	cmdutil.TestSetRuntimePlan(t, f, runtimeplan.New(runtimeplan.Options{
		Capabilities: func(capability runtimeplan.Capability) error {
			if capability == runtimeplan.CapabilityRealtimeEvents {
				return deniedErr
			}
			return nil
		},
	}))

	shortcut := &Shortcut{
		Service:   "test",
		Command:   "runtime-capability",
		AuthTypes: []string{"bot"},
		Flags:     []Flag{{Name: "local-only", Type: "bool"}},
		Execute: func(context.Context, *RuntimeContext) error {
			t.Fatal("Execute should not run after local introspection handled the invocation")
			return errors.New("unreachable")
		},
	}
	cmd := newTestShortcutCmd(shortcut, f)
	cmdutil.SetRuntimeCapabilities(cmd, runtimeplan.CapabilityRealtimeEvents)
	cmd.SetContext(context.WithValue(cmd.Context(), runtimeBehaviorContextKey{}, LocalIntrospection(
		func(cmd *cobra.Command) ([]byte, bool, error) {
			want, err := cmd.Flags().GetBool("local-only")
			if err != nil || !want {
				return nil, false, err
			}
			return []byte(`{"local":true}`), true, nil
		},
	)))
	// Install a fresh provider after mounting. Its sync.Once state is empty, so
	// moving identity resolution ahead of the local hook would make this test
	// observe the regression.
	provider := &countingRuntimeCredentialProvider{}
	f.Credential = credential.NewCredentialProvider(
		[]extcred.Provider{provider},
		nil,
		nil,
		nil,
	)
	cmd.Flags().Set("as", "bot")
	cmd.Flags().Set("local-only", "true")

	if err := runShortcut(cmd, f, shortcut, false); err != nil {
		t.Fatalf("runShortcut() error = %v, want local introspection to finish before runtime initialization", err)
	}
	if configCalls != 0 {
		t.Fatalf("Config called %d times, want 0", configCalls)
	}
	if clientCalls != 0 {
		t.Fatalf("LarkClient called %d times, want 0", clientCalls)
	}
	if provider.accountCalls != 0 || provider.tokenCalls != 0 {
		t.Fatalf("credential provider calls = account:%d token:%d, want 0",
			provider.accountCalls, provider.tokenCalls)
	}
	if got := f.IOStreams.Out.(*bytes.Buffer).String(); got != "{\"local\":true}\n" {
		t.Fatalf("stdout = %q, want local introspection output", got)
	}
}

func TestRunShortcut_DryRunKeepsPrecedenceOverLocalIntrospection(t *testing.T) {
	f := newTestFactory()
	localCalled := false
	dryRunCalled := false
	shortcut := &Shortcut{
		Service:   "test",
		Command:   "runtime-capability",
		AuthTypes: []string{"bot"},
		Flags:     []Flag{{Name: "local-only", Type: "bool"}},
		DryRun: func(context.Context, *RuntimeContext) *DryRunAPI {
			dryRunCalled = true
			return NewDryRunAPI().GET("/open-apis/test")
		},
		Execute: func(context.Context, *RuntimeContext) error {
			t.Fatal("Execute should not run in dry-run")
			return errors.New("unreachable")
		},
	}
	cmd := newTestShortcutCmd(shortcut, f)
	cmd.SetContext(context.WithValue(cmd.Context(), runtimeBehaviorContextKey{}, LocalIntrospection(
		func(*cobra.Command) ([]byte, bool, error) {
			localCalled = true
			return []byte(`{"local":true}`), true, nil
		},
	)))
	cmd.Flags().Set("as", "bot")
	cmd.Flags().Set("local-only", "true")
	cmd.Flags().Set("dry-run", "true")

	if err := runShortcut(cmd, f, shortcut, false); err != nil {
		t.Fatalf("runShortcut() error = %v", err)
	}
	if localCalled {
		t.Fatal("LocalIntrospection ran even though --dry-run has compatibility precedence")
	}
	if !dryRunCalled {
		t.Fatal("DryRun was not called")
	}
}

func TestRunShortcut_LocalIntrospectionPreservesLocalFlagValidation(t *testing.T) {
	f := newTestFactory()
	shortcut := &Shortcut{
		Service:   "test",
		Command:   "runtime-capability",
		AuthTypes: []string{"user"},
		Flags: []Flag{
			{Name: "local-only", Type: "bool"},
			{Name: "mode", Default: "safe", Enum: []string{"safe"}},
		},
		Execute: func(context.Context, *RuntimeContext) error {
			t.Fatal("Execute should not run in local introspection")
			return errors.New("unreachable")
		},
	}
	cmd := newTestShortcutCmd(shortcut, f)
	cmd.SetContext(context.WithValue(cmd.Context(), runtimeBehaviorContextKey{}, LocalIntrospection(
		func(*cobra.Command) ([]byte, bool, error) {
			return []byte(`{"local":true}`), true, nil
		},
	)))
	cmd.Flags().Set("local-only", "true")
	cmd.Flags().Set("mode", "unsafe")

	err := runShortcut(cmd, f, shortcut, false)
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T %v, want typed validation error", err, err)
	}
	var validationErr *errs.ValidationError
	if problem.Subtype != errs.SubtypeInvalidArgument ||
		!errors.As(err, &validationErr) ||
		validationErr.Param != "--mode" {
		t.Fatalf("error = %#v, want invalid_argument for --mode", err)
	}
}
