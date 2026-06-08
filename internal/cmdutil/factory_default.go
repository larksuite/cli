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
	"github.com/larksuite/cli/internal/transport"
	_ "github.com/larksuite/cli/internal/vfs/localfileio" // register default FileIO provider
)

// NewDefault creates a production Factory with cached closures.
// Credential is the sole data source; Config and LarkClient derive from it.
func NewDefault(streams *IOStreams, inv InvocationContext) *Factory {
	streams = normalizeStreams(streams)
	f := &Factory{
		Keychain:   keychain.Default(),
		Invocation: inv,
		IOStreams:  streams,
	}

	// Must run before any config or credential load — those paths are workspace-scoped.
	ws := core.DetectWorkspaceFromEnv(os.Getenv)
	core.SetCurrentWorkspace(ws)

	// Function variable breaks the core↔keychain import cycle.
	keychain.RuntimeDirFunc = core.GetRuntimeDir

	f.FileIOProvider = fileio.GetProvider()

	f.HttpClient = cachedHttpClientFunc(f)

	// Keychain read via closure so callers can replace f.Keychain after construction.
	f.Credential = buildCredentialProvider(credentialDeps{
		Keychain:   func() keychain.KeychainAccess { return f.Keychain },
		Profile:    inv.Profile,
		UserOpenId: inv.UserOpenId,
		UserSource: inv.UserSource,
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

	f.LarkClient = cachedLarkClientFunc(f)

	return f
}

// safeRedirectPolicy strips Authorization, X-Lark-MCP-UAT, and X-Lark-MCP-TAT
// on cross-host redirects (e.g. Lark API 302 → CDN). Other headers pass through.
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
		transport.WarnIfProxied(f.IOStreams.ErrOut)

		var rt http.RoundTripper = transport.Shared()
		rt = &RetryTransport{Base: rt}
		rt = &SecurityHeaderTransport{Base: rt}
		rt = &auth.SecurityPolicyTransport{Base: rt}
		rt = wrapWithExtension(rt)
		client := &http.Client{
			Transport:     rt,
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
		transport.WarnIfProxied(f.IOStreams.ErrOut)
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
	var sdkTransport http.RoundTripper = transport.Shared()
	sdkTransport = &RetryTransport{Base: sdkTransport}
	sdkTransport = &UserAgentTransport{Base: sdkTransport}
	sdkTransport = &BuildHeaderTransport{Base: sdkTransport}
	sdkTransport = &auth.SecurityPolicyTransport{Base: sdkTransport}
	return wrapWithExtension(sdkTransport)
}

type credentialDeps struct {
	Keychain   func() keychain.KeychainAccess
	Profile    string
	UserOpenId string
	UserSource string
	HttpClient func() (*http.Client, error)
	ErrOut     io.Writer
}

func buildCredentialProvider(deps credentialDeps) *credential.CredentialProvider {
	providers := extcred.Providers()
	defaultAcct := credential.NewDefaultAccountProvider(deps.Keychain, deps.Profile, deps.UserOpenId, deps.UserSource)
	defaultToken := credential.NewDefaultTokenProvider(defaultAcct, deps.HttpClient, deps.ErrOut)
	// Do not pass deps.ErrOut as warnOut: credential resolution runs before the
	// command, so plain-text warnings on stderr would break the JSON envelope
	// contract that AI agents depend on. enrichUserInfo failures are already
	// non-fatal (provider clears unverified identity fields).
	return credential.NewCredentialProvider(providers, defaultAcct, defaultToken, deps.HttpClient)
}
