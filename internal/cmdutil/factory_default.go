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
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/riskcontrol"
	"github.com/larksuite/cli/internal/runtimeplan"
	_ "github.com/larksuite/cli/internal/security/contentsafety" // register content safety provider
	"github.com/larksuite/cli/internal/transport"
	_ "github.com/larksuite/cli/internal/vfs/localfileio" // register default FileIO provider
)

func init() {
	// Stable package wiring: assign once during initialization rather than on
	// every Build/NewDefault call.
	keychain.RuntimeDirFunc = core.GetRuntimeDir
}

var initRegistryWithBrand = registry.InitWithBrand

// NewDefault creates a production Factory with cached closures.
// Initialization follows a credential-first order:
//
//	Phase 1: HttpClient (no credential dependency)
//	Phase 2: Credential (sole data source for account info)
//	Phase 3: Config derived from Credential
//	Phase 4: LarkClient derived from Credential and workspace policy
func NewDefault(streams *IOStreams, inv InvocationContext) *Factory {
	// Preserve the established standalone Factory behavior. Product-specific
	// runtime selection belongs to the CLI composition root, which calls
	// NewDefaultWithRuntimePlan with one immutable startup snapshot.
	core.SetCurrentWorkspace(core.DetectWorkspaceFromEnv(os.Getenv))
	return newDefaultWithRuntimePlan(streams, inv, nil, runtimeplan.Default(), nil, false)
}

// NewDefaultWithRuntimePlan creates a production Factory from the same
// immutable Profile snapshot and source-neutral plan used by startup routing.
func NewDefaultWithRuntimePlan(
	streams *IOStreams,
	inv InvocationContext,
	profileConfig *core.MultiAppConfig,
	plan *runtimeplan.Plan,
	apiCatalog *apicatalog.Catalog,
) *Factory {
	return newDefaultWithRuntimePlan(streams, inv, profileConfig, plan, apiCatalog, true)
}

func newDefaultWithRuntimePlan(
	streams *IOStreams,
	inv InvocationContext,
	profileConfig *core.MultiAppConfig,
	plan *runtimeplan.Plan,
	apiCatalog *apicatalog.Catalog,
	useProfileSnapshot bool,
) *Factory {
	streams = normalizeStreams(streams)
	var catalogSnapshot *apicatalog.Catalog
	if apiCatalog != nil {
		catalog := *apiCatalog
		catalogSnapshot = &catalog
	}
	f := &Factory{
		Keychain:    keychain.Default(),
		Invocation:  inv,
		IOStreams:   streams,
		runtimePlan: runtimeplan.Ensure(plan),
		apiCatalog:  catalogSnapshot,
	}

	// Workspace detection: determines which config subtree to use.
	// Must run before any config or credential load, since those paths are
	// workspace-scoped. Default is WorkspaceLocal — existing behavior unchanged.
	ws := core.DetectWorkspaceFromEnv(os.Getenv)
	core.SetCurrentWorkspace(ws)

	// Phase 0: FileIO provider (no dependency)
	f.FileIOProvider = fileio.GetProvider()
	workspaceConfig := core.NewConfigSnapshot()
	if profileConfig != nil {
		workspaceConfig = core.NewConfigSnapshotFrom(profileConfig)
	}
	bootstrapHostSignalSource := sync.OnceValue(func() riskcontrol.Source {
		return resolveSDKHostSignalSource(workspaceConfig)
	})
	// larkws owns a process-global bootstrap client. Its installed fallback must
	// therefore contain only policy common to every Factory; putting f's runtime
	// plan here would make the last constructed Factory control every retained
	// command tree. Actual SDK starts bind this invocation-specific builder via
	// SDKBootstrapContext.
	transport.InstallSDKTransportBridge(func(base http.RoundTripper) http.RoundTripper {
		return buildSDKPlatformTransportWithBase(base, nil)
	})
	f.sdkBootstrapContext = func(ctx context.Context) context.Context {
		return transport.WithSDKBootstrapPolicy(ctx, func(base http.RoundTripper) http.RoundTripper {
			built, err := buildSDKPlatformTransportWithRuntimePlan(
				f,
				base,
				bootstrapHostSignalSource(),
			)
			if err != nil {
				return runtimePlanFailureTransport{err: err}
			}
			return built
		})
	}

	// Phase 1: HttpClient (no credential dependency)
	f.HttpClient = cachedHttpClientFunc(f, workspaceConfig)

	// Phase 2: Credential (sole data source)
	// Keychain is read via closure so callers can replace f.Keychain after construction.
	f.Credential = buildCredentialProvider(credentialDeps{
		Keychain:              func() keychain.KeychainAccess { return f.Keychain },
		Profile:               inv.Profile,
		ProfileSource:         inv.ProfileSource,
		HttpClient:            f.HttpClient,
		ErrOut:                f.IOStreams.ErrOut,
		RuntimePlan:           f.runtimePlan,
		ProfileConfigSnapshot: profileConfig,
		UseProfileSnapshot:    useProfileSnapshot,
	})

	// Phase 3: Runtime config contains resolved account data only.
	f.Config = sync.OnceValues(func() (*core.CliConfig, error) {
		acct, err := f.Credential.ResolveAccount(context.Background())
		if err != nil {
			return nil, err
		}
		cfg := acct.ToCliConfig()
		// A composition root that supplied apiCatalog owns metadata selection.
		// Standalone legacy Factories retain the established lazy runtime-registry
		// initialization, while embedded-only plans never consume its sync.Once.
		if f.apiCatalog == nil && f.runtimePlan.AllowsRemoteMetadata() {
			initRegistryWithBrand(cfg.Brand)
		}
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
		platformBase, err := applyRuntimePlan(f, shared)
		if err != nil {
			return nil, err
		}
		// Managed runtime routing belongs only to the platform branch. Explicitly
		// external requests retain the ordinary transport policy and never inherit
		// platform credentials or response interpretation.
		platform := buildDirectHTTPTransport(
			riskcontrol.NewTransport(platformBase, hostSignalSource),
			true,
		)
		external := buildDirectHTTPTransport(
			riskcontrol.NewTransport(shared, hostSignalSource),
			false,
		)
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
		sdkTransport, err := buildSDKTransportWithRuntimePlan(
			f,
			transport.Shared(),
			hostSignalSource,
		)
		if err != nil {
			return nil, err
		}
		opts = append(opts, lark.WithHttpClient(&http.Client{
			Transport:     sdkTransport,
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

func buildSDKPlatformTransportWithRuntimePlan(
	f *Factory,
	base http.RoundTripper,
	hostSignalSource riskcontrol.Source,
) (http.RoundTripper, error) {
	managed, err := applyRuntimePlan(f, base)
	if err != nil {
		return nil, err
	}
	return buildSDKPlatformTransportWithBase(managed, hostSignalSource), nil
}

func buildSDKTransportWithBase(
	base http.RoundTripper,
	hostSignalSource riskcontrol.Source,
) http.RoundTripper {
	return buildSDKTransportBranches(base, base, hostSignalSource)
}

func buildSDKTransportWithRuntimePlan(
	f *Factory,
	base http.RoundTripper,
	hostSignalSource riskcontrol.Source,
) (http.RoundTripper, error) {
	platformBase, err := applyRuntimePlan(f, base)
	if err != nil {
		return nil, err
	}
	return buildSDKTransportBranches(platformBase, base, hostSignalSource), nil
}

func buildSDKTransportBranches(
	platformBase http.RoundTripper,
	externalBase http.RoundTripper,
	hostSignalSource riskcontrol.Source,
) http.RoundTripper {
	// Risk control is the innermost trusted boundary for both request classes.
	// It therefore observes the final URL and strips extension-supplied reserved
	// headers immediately before the network transport.
	return transport.NewHTTPPolicyRouter(
		buildSDKHTTPTransport(riskcontrol.NewTransport(platformBase, hostSignalSource), true),
		buildSDKHTTPTransport(riskcontrol.NewTransport(externalBase, hostSignalSource), false),
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

func applyRuntimePlan(f *Factory, base http.RoundTripper) (http.RoundTripper, error) {
	if f == nil {
		return base, nil
	}
	return runtimeplan.Ensure(f.runtimePlan).Wrap(base)
}

type runtimePlanFailureTransport struct{ err error }

func (t runtimePlanFailureTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, t.err
}

type credentialDeps struct {
	Keychain              func() keychain.KeychainAccess
	Profile               string
	ProfileSource         core.ProfileSource
	HttpClient            func() (*http.Client, error)
	ErrOut                io.Writer
	RuntimePlan           *runtimeplan.Plan
	ProfileConfigSnapshot *core.MultiAppConfig
	UseProfileSnapshot    bool
}

func buildCredentialProvider(deps credentialDeps) *credential.CredentialProvider {
	plan := runtimeplan.Ensure(deps.RuntimePlan)
	providers := extcred.Providers()
	localAcct := credential.NewDefaultAccountProvider(deps.Keychain, deps.Profile, deps.ProfileSource)
	if deps.UseProfileSnapshot {
		localAcct = credential.NewDefaultAccountProviderFromSnapshot(deps.Keychain, deps.Profile, deps.ProfileSource, deps.ProfileConfigSnapshot)
	}
	localToken := credential.NewDefaultTokenProvider(localAcct, deps.HttpClient, deps.ErrOut)
	var defaultAcct credential.DefaultAccountResolver = localAcct
	var defaultToken credential.DefaultTokenResolver = localToken

	if startupErr := plan.StartupError(); startupErr != nil {
		providers = []extcred.Provider{&runtimePlanErrorProvider{err: startupErr}}
		defaultAcct = nil
		defaultToken = nil
	} else if provider, replace := plan.CredentialProvider(); provider != nil {
		if replace {
			providers = []extcred.Provider{provider}
			defaultAcct = nil
			defaultToken = nil
		} else {
			providers = append([]extcred.Provider{provider}, providers...)
		}
	}
	// NOTE: Do not pass deps.ErrOut as warnOut. Credential resolution
	// happens before the command runs, so any plain-text warning written
	// to stderr would break the JSON envelope contract that AI agents
	// depend on. enrichUserInfo failures are already non-fatal (the
	// provider clears unverified identity fields), so silencing the
	// warning is safe.
	return credential.NewCredentialProvider(providers, defaultAcct, defaultToken, deps.HttpClient)
}

type runtimePlanErrorProvider struct{ err error }

func (p *runtimePlanErrorProvider) Name() string { return "runtime-policy" }

func (p *runtimePlanErrorProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return nil, p.err
}

func (p *runtimePlanErrorProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	return nil, p.err
}
