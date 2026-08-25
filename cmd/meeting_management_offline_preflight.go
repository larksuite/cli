// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	extcredential "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/shortcuts"
	shortcutcommon "github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

type offlineMeetingManagementEligibilityProbes struct {
	registeredPluginCount      func() int
	credentialProviderSnapshot func() ([]offlineMeetingManagementCredentialProvider, bool)
	stat                       func(string) (fs.FileInfo, error)
	getenv                     func(string) string
	baseConfigDir              func() string
}

type offlineMeetingManagementCredentialProvider struct {
	name        string
	packagePath string
	typeName    string
}

var standardOfflineMeetingManagementCredentialProvider = offlineMeetingManagementCredentialProvider{
	name:        "env",
	packagePath: "github.com/larksuite/cli/extension/credential/env",
	typeName:    "Provider",
}

func defaultOfflineMeetingManagementEligibilityProbes() offlineMeetingManagementEligibilityProbes {
	return offlineMeetingManagementEligibilityProbes{
		registeredPluginCount:      func() int { return len(platform.RegisteredPlugins()) },
		credentialProviderSnapshot: snapshotOfflineMeetingManagementCredentialProviders,
		stat:                       os.Stat,
		getenv:                     os.Getenv,
		baseConfigDir:              core.GetBaseConfigDir,
	}
}

func snapshotOfflineMeetingManagementCredentialProviders() (
	snapshot []offlineMeetingManagementCredentialProvider,
	determinate bool,
) {
	determinate = false
	defer func() {
		if recover() != nil {
			snapshot = nil
			determinate = false
		}
	}()
	return describeOfflineMeetingManagementCredentialProviders(extcredential.Providers())
}

func describeOfflineMeetingManagementCredentialProviders(
	providers []extcredential.Provider,
) ([]offlineMeetingManagementCredentialProvider, bool) {
	snapshot := make([]offlineMeetingManagementCredentialProvider, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			return nil, false
		}
		value := reflect.ValueOf(provider)
		switch value.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
			if value.IsNil() {
				return nil, false
			}
		}

		providerType := reflect.TypeOf(provider)
		if providerType.Kind() == reflect.Ptr {
			providerType = providerType.Elem()
		}
		snapshot = append(snapshot, offlineMeetingManagementCredentialProvider{
			name:        provider.Name(),
			packagePath: providerType.PkgPath(),
			typeName:    providerType.Name(),
		})
	}
	return snapshot, true
}

func hasOnlyStandardOfflineMeetingManagementCredentialProvider(
	snapshot func() ([]offlineMeetingManagementCredentialProvider, bool),
) (eligible bool) {
	if snapshot == nil {
		return false
	}
	defer func() {
		if recover() != nil {
			eligible = false
		}
	}()

	providers, determinate := snapshot()
	return determinate && len(providers) == 1 &&
		providers[0] == standardOfflineMeetingManagementCredentialProvider
}

var offlineMeetingManagementEligibilityProbesForInvocation = defaultOfflineMeetingManagementEligibilityProbes

var offlineMeetingManagementCredentialEnvKeys = []string{
	envvars.CliAppID,
	envvars.CliAppSecret,
	envvars.CliBrand,
	envvars.CliUserAccessToken,
	envvars.CliTenantAccessToken,
	envvars.CliDefaultAs,
	envvars.CliStrictMode,
	envvars.CliAuthProxy,
	envvars.CliProxyKey,
}

// canRunOfflineMeetingManagementPreflight is deliberately fail closed. The
// minimal tree cannot reproduce policy contributed by Plugin.Install, a YAML
// policy, custom credential providers, or strict-mode stored in config/credential
// state. If any such state may exist, the caller must use the complete command
// tree so its authoritative enforcement and presentation paths run before an
// observable result.
func canRunOfflineMeetingManagementPreflight(
	inv cmdutil.InvocationContext,
	presentation restrictionPresentationConfig,
	probes offlineMeetingManagementEligibilityProbes,
) bool {
	if presentation.enabled ||
		inv.ProfileSource != core.ProfileFromConfig || inv.Profile != "" ||
		probes.registeredPluginCount == nil || probes.registeredPluginCount() != 0 ||
		!hasOnlyStandardOfflineMeetingManagementCredentialProvider(probes.credentialProviderSnapshot) ||
		probes.stat == nil || probes.getenv == nil || probes.baseConfigDir == nil {
		return false
	}
	for _, key := range offlineMeetingManagementCredentialEnvKeys {
		if probes.getenv(key) != "" {
			return false
		}
	}

	baseDir := probes.baseConfigDir()
	if baseDir == "" {
		return false
	}
	runtimeDir := baseDir
	if workspace := core.DetectWorkspaceFromEnv(probes.getenv); !workspace.IsLocal() {
		runtimeDir = filepath.Join(baseDir, string(workspace))
	}
	for _, path := range []string{
		filepath.Join(baseDir, userPolicyFileName),
		filepath.Join(runtimeDir, "config.json"),
	} {
		if _, err := probes.stat(path); !errors.Is(err, os.ErrNotExist) {
			// Both an existing file (err == nil) and an indeterminate stat error
			// require the full tree. Absence is the only proof that permits the
			// dependency-free fast path.
			return false
		}
	}
	return true
}

// isOfflineMeetingManagementInvocation recognizes only the two destructive VC
// commands whose definitions explicitly opt into confirmation-before-network.
// Help/version keep the normal command tree so presentation and plugin policy
// remain unchanged for introspection. Local command flags are intentionally
// accepted only after the command path; that is the canonical Cobra spelling.
func isOfflineMeetingManagementInvocation(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "--help=true" ||
			arg == "-v" || arg == "--version" || arg == "--version=true" {
			return false
		}
	}

	commandIndex := 0
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--profile":
			index++
			if index >= len(args) {
				return false
			}
			continue
		case strings.HasPrefix(arg, "--profile="):
			continue
		case len(arg) > 0 && arg[0] == '-':
			// Keep scanning so malformed or unknown flags on one of these
			// invocations are rejected by the dependency-free Cobra tree.
			continue
		}

		switch commandIndex {
		case 0:
			if arg != "vc" {
				return false
			}
			commandIndex++
		case 1:
			return arg == "+meeting-end" || arg == "+meeting-participant-kickout"
		}
	}
	return false
}

// runOfflineMeetingManagementPreflight executes a minimal in-memory Cobra
// tree. terminal is true for every local outcome (validation, dry-run, or
// confirmation-required); false means explicit --yes passed and the caller
// must continue through the unchanged full startup/execution path.
func runOfflineMeetingManagementPreflight(
	ctx context.Context,
	inv cmdutil.InvocationContext,
	presentation restrictionPresentationConfig,
	streams *cmdutil.IOStreams,
	args []string,
) (terminal bool, exitCode int) {
	if !isOfflineMeetingManagementInvocation(args) ||
		!canRunOfflineMeetingManagementPreflight(inv, presentation, offlineMeetingManagementEligibilityProbesForInvocation()) {
		return false, 0
	}

	// The literal factory intentionally has no notice provider: this local
	// preflight must neither evaluate nor leak notices from another invocation.
	f := &cmdutil.Factory{IOStreams: streams, Invocation: inv}
	root := &cobra.Command{Use: "lark-cli", SilenceErrors: true, SilenceUsage: true}
	root.SetContext(ctx)
	root.SetIn(streams.In)
	root.SetOut(streams.Out)
	root.SetErr(streams.ErrOut)
	root.SetArgs(args)
	RegisterGlobalFlags(root.PersistentFlags(), &GlobalOptions{})
	root.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		f.CurrentCommand = cmd
	}

	if err := shortcuts.RegisterOfflineMeetingManagementPreflight(ctx, root, f); err != nil {
		return true, handleRootError(f, err, nil)
	}
	err := root.Execute()
	if errors.Is(err, shortcutcommon.ErrOfflinePreflightPassed) {
		return false, 0
	}
	if err != nil {
		return true, handleRootError(f, err, nil)
	}
	return true, 0
}
