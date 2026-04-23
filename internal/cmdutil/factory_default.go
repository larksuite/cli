// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/registry"
	_ "github.com/larksuite/cli/internal/security/contentsafety" // register content safety provider
	"github.com/larksuite/cli/internal/util"
	_ "github.com/larksuite/cli/internal/vfs/localfileio" // register default FileIO provider
)

// NewDefault creates a production Factory with cached closures.
// Initialization follows a credential-first order:
//
//	Phase 1: HttpClient (no credential dependency)
//	Phase 2: Credential (sole data source for account info)
//	Phase 3: Config derived from Credential
//	Phase 4: LarkClient derived from Credential
func NewDefault(streams *IOStreams, inv InvocationContext) *Factory {
	streams = normalizeStreams(streams)
	f := &Factory{
		Keychain:   keychain.Default(),
		Invocation: inv,
		IOStreams:  streams,
	}

	// Workspace detection: determines which config subtree to use.
	// Must run before any config or credential load, since those paths are
	// workspace-scoped. Default is WorkspaceLocal — existing behavior unchanged.
	ws := core.DetectWorkspaceFromEnv(os.Getenv)
	core.SetCurrentWorkspace(ws)

	// Inject workspace-aware dir into keychain's log system.
	// This breaks the core↔keychain import cycle by using a function variable.
	keychain.RuntimeDirFunc = core.GetRuntimeDir

	// Phase 0: FileIO provider (no dependency)
	f.FileIOProvider = fileio.GetProvider()

	// Phase 1: HttpClient (no credential dependency)
	f.HttpClient = cachedHttpClientFunc(f)

	// Phase 2: Credential (sole data source)
	// Keychain is read via closure so callers can replace f.Keychain after construction.
	f.Credential = buildCredentialProvider(credentialDeps{
		Keychain:   func() keychain.KeychainAccess { return f.Keychain },
		Profile:    inv.Profile,
		HttpClient: f.HttpClient,
		ErrOut:     f.IOStreams.ErrOut,
	})

	// Phase 3: Config derived from Credential via an explicit conversion boundary.
	f.Config = sync.OnceValues(func() (*core.CliConfig, error) {
		acct, err := f.Credential.ResolveAccount(context.Background())
		if err != nil {
			return nil, err
		}
		cfg := acct.ToCliConfig()
		registry.InitWithBrand(cfg.Brand)
		return cfg, nil
	})

	// Phase 4: LarkClient from Credential (placeholder AppSecret)
	f.LarkClient = cachedLarkClientFunc(f)

	return f
}

// safeRedirectPolicy prevents credential headers from being forwarded
// when a response redirects to a different host (e.g. Lark API 302 → CDN).
// Strips Authorization, X-Lark-MCP-UAT, and X-Lark-MCP-TAT on cross-host
// redirects; other headers like X-Cli-* pass through.
func safeRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("too many redirects")
	}
	if len(via) > 0 && req.URL.Host != via[0].URL.Host {
		req.Header.Del("Authorization")
		req.Header.Del("X-Lark-MCP-UAT")
		req.Header.Del("X-Lark-MCP-TAT")
	}
	return nil
}

func cachedHttpClientFunc(f *Factory) func() (*http.Client, error) {
	return sync.OnceValues(func() (*http.Client, error) {
		util.WarnIfProxied(f.IOStreams.ErrOut)

		var transport http.RoundTripper = util.SharedTransport()
		transport = &RetryTransport{Base: transport}
		transport = &SecurityHeaderTransport{Base: transport}
		transport = &auth.SecurityPolicyTransport{Base: transport} // Add our global response interceptor
		transport = wrapWithExtension(transport)
		client := &http.Client{
			Transport:     transport,
			Timeout:       30 * time.Second,
			CheckRedirect: safeRedirectPolicy,
		}
		return client, nil
	})
}

func cachedLarkClientFunc(f *Factory) func() (*lark.Client, error) {
	return sync.OnceValues(func() (*lark.Client, error) {
		acct, err := f.Credential.ResolveAccount(context.Background())
		if err != nil {
			return nil, err
		}
		opts := []lark.ClientOptionFunc{
			lark.WithEnableTokenCache(false),
			lark.WithLogLevel(larkcore.LogLevelError),
			lark.WithHeaders(BaseSecurityHeaders()),
		}
		util.WarnIfProxied(f.IOStreams.ErrOut)
		opts = append(opts, lark.WithHttpClient(&http.Client{
			Transport:     buildSDKTransport(),
			CheckRedirect: safeRedirectPolicy,
		}))
		ep := core.ResolveEndpoints(acct.Brand)
		opts = append(opts, lark.WithOpenBaseUrl(ep.Open))
		return lark.NewClient(acct.AppID, credential.RuntimeAppSecret(acct.AppSecret), opts...), nil
	})
}

func buildSDKTransport() http.RoundTripper {
	var sdkTransport http.RoundTripper = util.SharedTransport()
	sdkTransport = &RetryTransport{Base: sdkTransport}
	sdkTransport = &UserAgentTransport{Base: sdkTransport}
	sdkTransport = &BuildHeaderTransport{Base: sdkTransport}
	sdkTransport = &auth.SecurityPolicyTransport{Base: sdkTransport}
	return wrapWithExtension(sdkTransport)
}

type credentialDeps struct {
	Keychain   func() keychain.KeychainAccess
	Profile    string
	HttpClient func() (*http.Client, error)
	ErrOut     io.Writer
}

func buildCredentialProvider(deps credentialDeps) *credential.CredentialProvider {
	providers := extcred.Providers()
	defaultAcct := credential.NewDefaultAccountProvider(deps.Keychain, deps.Profile)
	defaultToken := credential.NewDefaultTokenProvider(defaultAcct, deps.HttpClient, deps.ErrOut)
	// NOTE: Do not pass deps.ErrOut as warnOut. Credential resolution
	// happens before the command runs, so any plain-text warning written
	// to stderr would break the JSON envelope contract that AI agents
	// depend on. enrichUserInfo failures are already non-fatal (the
	// provider clears unverified identity fields), so silencing the
	// warning is safe.
	return credential.NewCredentialProvider(providers, defaultAcct, defaultToken, deps.HttpClient)
}
