// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/runtimebootstrap"
	"github.com/larksuite/cli/internal/runtimeplan"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/spf13/cobra"
)

const (
	runtimeCatalogHelperEnv = "GO_TEST_RUNTIME_CATALOG_HELPER"
	runtimeCatalogOrderEnv  = "GO_TEST_RUNTIME_CATALOG_ORDER"
	remoteOnlyService       = "remote_only_service"
)

var _ = flag.String("runtime-catalog-helper", "", "internal runtime catalog isolation test helper nonce")

func isRuntimeCatalogIsolationHelper() bool {
	envNonce := os.Getenv(runtimeCatalogHelperEnv)
	if envNonce == "" {
		return false
	}
	const prefix = "-runtime-catalog-helper="
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix) == envNonce
		}
	}
	return false
}

func TestRuntimeCatalogIsBuildLocalAcrossOrder(t *testing.T) {
	if isRuntimeCatalogIsolationHelper() {
		runRuntimeCatalogIsolationHelper(t, os.Getenv(runtimeCatalogOrderEnv))
		return
	}

	for _, order := range []string{"ordinary-managed", "managed-ordinary"} {
		t.Run(order, func(t *testing.T) {
			configDir := t.TempDir()
			seedRemoteOnlyCatalog(t, configDir)

			nonce := uuid.NewString()
			cmd := exec.Command(
				os.Args[0],
				"-test.run", "^TestRuntimeCatalogIsBuildLocalAcrossOrder$",
				"-runtime-catalog-helper="+nonce,
			)
			cmd.Env = envWithOverrides(os.Environ(),
				runtimeCatalogHelperEnv+"="+nonce,
				runtimeCatalogOrderEnv+"="+order,
				"LARKSUITE_CLI_CONFIG_DIR="+configDir,
				"LARKSUITE_CLI_REMOTE_META=on",
				"LARKSUITE_CLI_META_TTL=3600",
				"LARKSUITE_CLI_APP_ID=",
				"LARKSUITE_CLI_APP_SECRET=",
				"LARKSUITE_CLI_USER_ACCESS_TOKEN=",
				"LARKSUITE_CLI_TENANT_ACCESS_TOKEN=",
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("helper failed: %v\n%s", err, out)
			}
		})
	}
}

func envWithOverrides(base []string, overrides ...string) []string {
	keys := make(map[string]struct{}, len(overrides))
	for _, value := range overrides {
		key, _, _ := strings.Cut(value, "=")
		keys[key] = struct{}{}
	}
	out := make([]string, 0, len(base)+len(overrides))
	for _, value := range base {
		key, _, _ := strings.Cut(value, "=")
		if _, replaced := keys[key]; !replaced {
			out = append(out, value)
		}
	}
	return append(out, overrides...)
}

func runRuntimeCatalogIsolationHelper(t *testing.T, order string) {
	t.Helper()
	cmdutil.SetFlagCompletionsEnabled(true)
	t.Cleanup(func() { cmdutil.SetFlagCompletionsEnabled(false) })
	ordinaryPlan := runtimeplan.Default()
	managedPlan := runtimeplan.New(runtimeplan.Options{
		Metadata: runtimeplan.MetadataEmbeddedOnly,
	})

	switch order {
	case "ordinary-managed":
		ordinaryRuntime, ordinaryTree := buildCatalogIsolationTree(t, ordinaryPlan)
		assertCatalogSelection(t, ordinaryRuntime, ordinaryTree, apicatalog.SourceRuntime, true)

		managedRuntime, managedTree := buildCatalogIsolationTree(t, managedPlan)
		assertCatalogSelection(t, managedRuntime, managedTree, apicatalog.SourceEmbedded, false)
		assertCatalogSelection(t, ordinaryRuntime, ordinaryTree, apicatalog.SourceRuntime, true)
	case "managed-ordinary":
		managedRuntime, managedTree := buildCatalogIsolationTree(t, managedPlan)
		assertCatalogSelection(t, managedRuntime, managedTree, apicatalog.SourceEmbedded, false)
		if brand := registry.ConfiguredBrand(); brand != "" {
			t.Fatalf("managed Build initialized process-wide runtime registry with brand %q", brand)
		}

		ordinaryRuntime, ordinaryTree := buildCatalogIsolationTree(t, ordinaryPlan)
		assertCatalogSelection(t, ordinaryRuntime, ordinaryTree, apicatalog.SourceRuntime, true)
		assertCatalogSelection(t, managedRuntime, managedTree, apicatalog.SourceEmbedded, false)
	default:
		t.Fatalf("unknown helper order %q", order)
	}
}

func buildCatalogIsolationTree(t *testing.T, plan *runtimeplan.Plan) (*buildRuntime, *cobra.Command) {
	t.Helper()
	profile := &core.MultiAppConfig{Apps: []core.AppConfig{{
		Name:      "default",
		AppId:     "cli_catalog_isolation",
		AppSecret: core.PlainSecret("test-secret"),
		Brand:     core.BrandFeishu,
	}}}
	runtime, root, _ := buildInternal(
		context.Background(),
		cmdutil.InvocationContext{},
		WithIO(strings.NewReader(""), io.Discard, io.Discard),
		WithStartupBrand(core.BrandFeishu),
		withRuntimeBootstrap(&runtimebootstrap.Result{ProfileConfig: profile, Plan: plan}),
		WithoutPlugins(),
		WithoutStrictMode(),
	)
	return runtime, root
}

func assertCatalogSelection(
	t *testing.T,
	runtime *buildRuntime,
	root *cobra.Command,
	wantSource apicatalog.Source,
	wantRemoteService bool,
) {
	t.Helper()
	catalog, ok := runtime.APICatalog()
	if !ok {
		t.Fatal("Build did not retain its selected API catalog")
	}
	if catalog.Source() != wantSource {
		t.Fatalf("catalog source = %q, want %q", catalog.Source(), wantSource)
	}
	_, catalogHasRemote := catalog.Service(remoteOnlyService)
	if catalogHasRemote != wantRemoteService {
		t.Fatalf("catalog remote service = %t, want %t", catalogHasRemote, wantRemoteService)
	}
	if treeHasRemote := hasImmediateCommand(root, remoteOnlyService); treeHasRemote != wantRemoteService {
		t.Fatalf("command tree remote service = %t, want %t", treeHasRemote, wantRemoteService)
	}
	schemaCmd := immediateCommand(root, "schema")
	if schemaCmd == nil || schemaCmd.ValidArgsFunction == nil {
		t.Fatal("schema completion is not installed")
	}
	completions, _ := schemaCmd.ValidArgsFunction(schemaCmd, nil, remoteOnlyService)
	schemaHasRemote := false
	for _, completion := range completions {
		if strings.HasPrefix(completion, remoteOnlyService) {
			schemaHasRemote = true
			break
		}
	}
	if schemaHasRemote != wantRemoteService {
		t.Fatalf("schema completion remote service = %t, want %t", schemaHasRemote, wantRemoteService)
	}

	var schemaOutput strings.Builder
	runtime.IOStreams.Out = &schemaOutput
	schemaErr := schemaCmd.RunE(schemaCmd, []string{remoteOnlyService})
	if wantRemoteService && schemaErr != nil {
		t.Fatalf("schema execution rejected build-local service: %v", schemaErr)
	}
	if !wantRemoteService && schemaErr == nil {
		t.Fatal("schema execution resolved service absent from build-local catalog")
	}

	authCmd := immediateCommand(root, "auth")
	loginCmd := immediateCommand(authCmd, "login")
	if loginCmd == nil {
		t.Fatal("auth login command is not installed")
	}
	completeDomain, ok := loginCmd.GetFlagCompletionFunc("domain")
	if !ok {
		t.Fatal("auth login --domain completion is not installed")
	}
	domainCompletions, _ := completeDomain(loginCmd, nil, remoteOnlyService)
	domainHasRemote := false
	for _, completion := range domainCompletions {
		if completion == remoteOnlyService {
			domainHasRemote = true
			break
		}
	}
	if domainHasRemote != wantRemoteService {
		t.Fatalf("auth domain completion remote service = %t, want %t", domainHasRemote, wantRemoteService)
	}
}

func hasImmediateCommand(root *cobra.Command, name string) bool {
	return immediateCommand(root, name) != nil
}

func immediateCommand(root *cobra.Command, name string) *cobra.Command {
	if root == nil {
		return nil
	}
	for _, child := range root.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}

func seedRemoteOnlyCatalog(t *testing.T, configDir string) {
	t.Helper()
	cacheDir := filepath.Join(configDir, "cache")
	if err := vfs.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	const version = "9999.0.0"
	data, err := json.Marshal(registry.MergedRegistry{
		Version: version,
		Services: []meta.Service{{
			Name:        remoteOnlyService,
			Version:     "v1",
			Title:       "Remote-only API",
			ServicePath: "/open-apis/remote-only/v1",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := vfs.WriteFile(filepath.Join(cacheDir, "remote_meta.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	cacheMeta, err := json.Marshal(registry.CacheMeta{
		LastCheckAt: time.Now().Unix(),
		Version:     version,
		Brand:       string(core.BrandFeishu),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := vfs.WriteFile(filepath.Join(cacheDir, "remote_meta.meta.json"), cacheMeta, 0o600); err != nil {
		t.Fatal(err)
	}
}
