// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/runtimeplan"
	"github.com/larksuite/cli/internal/vfs"
)

type recordingProfileKeychain struct {
	gets    int
	sets    int
	removes int
}

func (k *recordingProfileKeychain) Get(_, _ string) (string, error) {
	k.gets++
	return "", nil
}

func (k *recordingProfileKeychain) Set(_, _, _ string) error {
	k.sets++
	return nil
}

func (k *recordingProfileKeychain) Remove(_, _ string) error {
	k.removes++
	return nil
}

func TestProfileMutationCommandsAreDeniedBeforeLocalStateChanges(t *testing.T) {
	denied := errs.NewValidationError(
		errs.SubtypeFailedPrecondition,
		"local credential management is unavailable in this runtime",
	).WithHint("manage credentials through the active provider")
	plan := runtimeplan.New(runtimeplan.Options{
		Capabilities: func(capability runtimeplan.Capability) error {
			if capability == runtimeplan.CapabilityLocalProfileMutation {
				return denied
			}
			return nil
		},
	})

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "add",
			args: []string{"add", "--name", "new", "--app-id", "app-new", "--app-secret-stdin"},
		},
		{
			name: "use",
			args: []string{"use", "target"},
		},
		{
			name: "rename",
			args: []string{"rename", "target", "renamed"},
		},
		{
			name: "remove",
			args: []string{"remove", "target"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir := setupProfileConfigDir(t)
			saveManagedGateFixture(t)
			configPath := filepath.Join(configDir, "config.json")
			before, err := vfs.ReadFile(configPath)
			if err != nil {
				t.Fatalf("ReadFile(before) error = %v", err)
			}

			f, _, _, _ := cmdutil.TestFactoryWithRuntimePlan(t, nil, plan)
			f.IOStreams.In = strings.NewReader("must-not-be-read\n")
			keychain := &recordingProfileKeychain{}
			f.Keychain = keychain

			cmd := NewCmdProfile(f)
			cmd.SetArgs(tt.args)
			err = cmd.Execute()
			if !errors.Is(err, denied) {
				t.Fatalf("Execute() error = %v, want denied runtime error", err)
			}

			after, readErr := vfs.ReadFile(configPath)
			if readErr != nil {
				t.Fatalf("ReadFile(after) error = %v", readErr)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("config changed despite runtime denial:\nbefore: %s\nafter:  %s", before, after)
			}
			if keychain.gets != 0 || keychain.sets != 0 || keychain.removes != 0 {
				t.Fatalf("keychain calls = get:%d set:%d remove:%d, want none",
					keychain.gets, keychain.sets, keychain.removes)
			}
		})
	}
}

func TestProfileListRemainsAvailableWhenLocalMutationIsDenied(t *testing.T) {
	setupProfileConfigDir(t)
	saveManagedGateFixture(t)
	plan := runtimeplan.New(runtimeplan.Options{
		Capabilities: func(capability runtimeplan.Capability) error {
			if capability == runtimeplan.CapabilityLocalProfileMutation {
				return errs.NewValidationError(
					errs.SubtypeFailedPrecondition,
					"local credential management is unavailable in this runtime",
				)
			}
			return nil
		},
	})
	f, stdout, _, _ := cmdutil.TestFactoryWithRuntimePlan(t, nil, plan)

	cmd := NewCmdProfile(f)
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile list was blocked by mutation capability: %v", err)
	}
	if !strings.Contains(stdout.String(), `"name": "default"`) {
		t.Fatalf("profile list output = %s, want default profile", stdout.String())
	}
}

func TestProfileMutationCommandsRemainAvailableByDefault(t *testing.T) {
	setupProfileConfigDir(t)
	saveManagedGateFixture(t)
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	// origin/main allows Profile preparation while an environment/extension
	// provider is active. The managed runtime blocks this through its explicit
	// plan policy; generic provider ownership must not change Standard.
	f.Credential = credential.NewCredentialProvider(
		[]extcred.Provider{profileEnvironmentProvider{}},
		nil,
		nil,
		nil,
	)

	cmd := NewCmdProfile(f)
	args := []string{"use", "target"}
	matched, _, err := cmd.Find(args)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("profile use with default runtime plan error = %v", err)
	}
	if f.CurrentCommand != matched {
		t.Fatalf("CurrentCommand = %v, want matched command %v", f.CurrentCommand, matched)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if saved.CurrentApp != "target" || saved.PreviousApp != "default" {
		t.Fatalf("selection = current:%q previous:%q, want target/default",
			saved.CurrentApp, saved.PreviousApp)
	}
}

type profileEnvironmentProvider struct{}

func (profileEnvironmentProvider) Name() string { return "env" }

func (profileEnvironmentProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return &extcred.Account{
		AppID:               "cli_environment",
		Brand:               extcred.BrandFeishu,
		SupportedIdentities: extcred.SupportsAll,
	}, nil
}

func (profileEnvironmentProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	return &extcred.Token{Value: "environment-token"}, nil
}

func saveManagedGateFixture(t *testing.T) {
	t.Helper()
	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{
				Name:      "default",
				AppId:     "app-default",
				AppSecret: core.PlainSecret("secret-default"),
				Brand:     core.BrandFeishu,
			},
			{
				Name:  "target",
				AppId: "app-target",
				AppSecret: core.SecretInput{Ref: &core.SecretRef{
					Source: "keychain",
					ID:     "appsecret:app-target",
				}},
				Brand: core.BrandLark,
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}
}
