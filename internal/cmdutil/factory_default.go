// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/riskcontrol"
	_ "github.com/larksuite/cli/internal/security/contentsafety" // register content safety provider
	"github.com/larksuite/cli/internal/transport"
	_ "github.com/larksuite/cli/internal/vfs/localfileio" // register default FileIO provider
)

func init() {
	// Stable package wiring: assign once during initialization rather than on
	// every Build/NewDefault call.
	keychain.RuntimeDirFunc = core.GetRuntimeDir
}

// NewDefault creates a production Factory with cached closures.
// Initialization follows a credential-first order:
//
//	Phase 1: HttpClient (no credential dependency)
//	Phase 2: Credential (sole data source for account info)
//	Phase 3: Config derived from Credential
//	Phase 4: LarkClient derived from Credential and workspace policy
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
	workspaceConfig := core.NewConfigSnapshot()
	bootstrapHostSignalSource := sync.OnceValue(func() riskcontrol.Source {
		return resolveSDKHostSignalSource(workspaceConfig)
	})
	// Install after workspace selection so the dependency bootstrap bridge uses
	// the correct shared proxy configuration. NewDefault is also used by cmd.Build
	// consumers, so this keeps their request routing identical to cmd.Execute.
	transport.InstallSDKTransportBridge(func(base http.RoundTripper) http.RoundTripper {
		return buildSDKPlatformTransportWithBase(
			base,
			bootstrapHostSignalSource(),
		)
	})

	// Phase 0: FileIO provider (no dependency)
	f.FileIOProvider = fileio.GetProvider()

	// Phase 1: HttpClient (no credential dependency)
	f.HttpClient = cachedHttpClientFunc(f, workspaceConfig)

	// Phase 2: Credential (sole data source)
	// Keychain is read via closure so callers can replace f.Keychain after construction.
	f.Credential = buildCredentialProvider(credentialDeps{
		Keychain:      func() keychain.KeychainAccess { return f.Keychain },
		Profile:       inv.Profile,
		ProfileSource: inv.ProfileSource,
		HttpClient:    f.HttpClient,
		ErrOut:        f.IOStreams.ErrOut,
	})

	// Phase 3: Runtime config contains resolved account data only.
	f.Config = sync.OnceValues(func() (*core.CliConfig, error) {
		acct, err := f.Credential.ResolveAccount(context.Background())
		if err != nil {
			return nil, err
		}
		cfg := acct.ToCliConfig()
		registry.InitWithBrand(cfg.Brand)
		return cfg, nil
	})

	// Phase 4: LarkClient composes account data and workspace policy at the SDK
	// transport boundary.
	f.LarkClient = cachedLarkClientFunc(f, workspaceConfig)

	return f
}

// safeRedirectPolicy permits cross-origin redirects only for bodyless GET and
// HEAD requests. This allows API download redirects while preventing OAuth or
// other credential-bearing request bodies from being replayed to another
// origin. HTTPS requests can never be downgraded to HTTP.
func safeRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errs.NewNetworkError(errs.SubtypeNetworkTransport, "too many redirects")
	}
	if len(via) == 0 {
		return nil
	}
	original := via[0]
	previous := via[len(via)-1]
	if previous.URL != nil && req.URL != nil && strings.EqualFold(previous.URL.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https") {
		return errs.NewSecurityPolicyError(
			errs.SubtypeAccessDenied,
			"redirect from HTTPS to %s is not allowed",
			req.URL.Scheme,
		)
	}
	if !sameRedirectOrigin(previous.URL, req.URL) {
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			return errs.NewSecurityPolicyError(
				errs.SubtypeAccessDenied,
				"cross-origin redirect for HTTP method %s is not allowed",
				req.Method,
			)
		}
		if req.Body != nil || req.GetBody != nil {
			return errs.NewSecurityPolicyError(
				errs.SubtypeAccessDenied,
				"cross-origin redirect with a request body is not allowed",
			)
		}
	}
	// net/http copies initial headers onto every redirect request. Continue
	// stripping credentials for every hop outside the initial origin, even when
	// two consecutive redirect targets share an origin.
	if !sameRedirectOrigin(original.URL, req.URL) {
		req.Header.Del("Authorization")
		req.Header.Del("X-Lark-MCP-UAT")
		req.Header.Del("X-Lark-MCP-TAT")
	}
	return nil
}

func sameRedirectOrigin(left, right *url.URL) bool {
	if left == nil || right == nil {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		effectivePort(left) == effectivePort(right)
}

func effectivePort(candidate *url.URL) string {
	if port := candidate.Port(); port != "" {
		return port
	}
	switch strings.ToLower(candidate.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

// warnIfProxied is a test seam for the proxy-warning gate. Production wires it
// to transport.WarnIfProxied; tests swap in a spy to count invocations. It is
// needed because the real function is guarded by an internal sync.Once, so
// calling it directly would only fire on the first test (see
// factory_proxy_warn_test.go). The terminal check is the IOStreams
// .StderrIsTerminal field, which tests set directly.
var warnIfProxied = transport.WarnIfProxied

func cachedHttpClientFunc(f *Factory, workspaceConfig workspaceConfigSource) func() (*http.Client, error) {
	return sync.OnceValues(func() (*http.Client, error) {
		if f.IOStreams.StderrIsTerminal {
			warnIfProxied(f.IOStreams.ErrOut)
		}

		hostSignalSource := resolveSDKHostSignalSource(workspaceConfig)
		shared := transport.Shared()
		outbound := riskcontrol.NewTransport(shared, hostSignalSource)
		platform := buildDirectHTTPTransport(outbound, true)
		external := buildDirectHTTPTransport(outbound, false)
		client := &http.Client{
			Transport:     transport.NewHTTPPolicyRouter(platform, external),
			Timeout:       30 * time.Second,
			CheckRedirect: safeRedirectPolicy,
		}
		return client, nil
	})
}

func buildDirectHTTPTransport(base http.RoundTripper, platform bool) http.RoundTripper {
	var builtIn http.RoundTripper = &RetryTransport{Base: base}
	builtIn = &SecurityHeaderTransport{Base: builtIn}
	if platform {
		builtIn = &auth.SecurityPolicyTransport{Base: builtIn}
	}
	return builtIn
}

func cachedLarkClientFunc(f *Factory, workspaceConfig workspaceConfigSource) func() (*lark.Client, error) {
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
		if f.IOStreams.StderrIsTerminal {
			warnIfProxied(f.IOStreams.ErrOut)
		}
		hostSignalSource := resolveSDKHostSignalSource(workspaceConfig)
		opts = append(opts, lark.WithHttpClient(&http.Client{
			Transport:     buildSDKTransport(hostSignalSource),
			CheckRedirect: safeRedirectPolicy,
		}))
		ep := core.ResolveEndpoints(acct.Brand)
		opts = append(opts, lark.WithOpenBaseUrl(ep.Open))
		return lark.NewClient(acct.AppID, credential.RuntimeAppSecret(acct.AppSecret), opts...), nil
	})
}

func buildSDKTransport(hostSignalSource riskcontrol.Source) http.RoundTripper {
	return buildSDKTransportWithBase(transport.Shared(), hostSignalSource)
}

func buildSDKPlatformTransportWithBase(
	base http.RoundTripper,
	hostSignalSource riskcontrol.Source,
) http.RoundTripper {
	outbound := riskcontrol.NewTransport(base, hostSignalSource)
	return buildSDKHTTPTransport(outbound, true)
}

func buildSDKTransportWithBase(
	base http.RoundTripper,
	hostSignalSource riskcontrol.Source,
) http.RoundTripper {
	// Risk control is the innermost trusted boundary for both request classes.
	// It therefore observes the final URL and strips extension-supplied reserved
	// headers immediately before the network transport.
	outbound := riskcontrol.NewTransport(base, hostSignalSource)
	return transport.NewHTTPPolicyRouter(
		buildSDKHTTPTransport(outbound, true),
		buildSDKHTTPTransport(outbound, false),
	)
}

func buildSDKHTTPTransport(base http.RoundTripper, platform bool) http.RoundTripper {
	var builtIn http.RoundTripper = &RetryTransport{Base: base}
	builtIn = &UserAgentTransport{Base: builtIn}
	builtIn = &BuildHeaderTransport{Base: builtIn}
	builtIn = &SecurityHeaderTransport{Base: builtIn}
	if platform {
		builtIn = &auth.SecurityPolicyTransport{Base: builtIn}
	}
	return builtIn
}

type credentialDeps struct {
	Keychain      func() keychain.KeychainAccess
	Profile       string
	ProfileSource core.ProfileSource
	HttpClient    func() (*http.Client, error)
	ErrOut        io.Writer
}

func buildCredentialProvider(deps credentialDeps) *credential.CredentialProvider {
	store := credential.NewTenantTokenStore(deps.Keychain)
	providers := withTenantAccessTokenLookup(extcred.Providers(), func(_ context.Context, appID string) (*extcred.Token, error) {
		value, found, err := store.Get(appID)
		if err != nil || !found {
			return nil, err
		}
		return &extcred.Token{Value: value, Source: "keychain:tenant-access-token"}, nil
	})
	defaultAcct := credential.NewDefaultAccountProvider(deps.Keychain, deps.Profile, deps.ProfileSource)
	defaultToken := credential.NewDefaultTokenProvider(defaultAcct, deps.HttpClient, deps.ErrOut)
	// NOTE: Do not pass deps.ErrOut as warnOut. Credential resolution
	// happens before the command runs, so any plain-text warning written
	// to stderr would break the JSON envelope contract that AI agents
	// depend on. enrichUserInfo failures are already non-fatal (the
	// provider clears unverified identity fields), so silencing the
	// warning is safe.
	return credential.NewCredentialProvider(providers, defaultAcct, defaultToken, deps.HttpClient)
}

func withTenantAccessTokenLookup(
	providers []extcred.Provider,
	lookup func(context.Context, string) (*extcred.Token, error),
) []extcred.Provider {
	for i, provider := range providers {
		configurer, ok := provider.(interface {
			WithTenantAccessTokenLookup(func(context.Context, string) (*extcred.Token, error)) extcred.Provider
		})
		if ok {
			providers[i] = configurer.WithTenantAccessTokenLookup(lookup)
		}
	}
	return providers
}
