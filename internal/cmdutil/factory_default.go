// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"golang.org/x/term"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/util"
)

// NewDefault creates a production Factory with cached closures.
func NewDefault() *Factory {
	f := &Factory{
		Keychain: keychain.Default(),
	}
	f.IOStreams = &IOStreams{
		In:         os.Stdin,
		Out:        os.Stdout,
		ErrOut:     os.Stderr,
		IsTerminal: term.IsTerminal(int(os.Stdin.Fd())),
	}
	f.Config = cachedConfigFunc(f)
	f.AuthConfig = cachedAuthConfigFunc(f)
	f.HttpClient = cachedHttpClientFunc()
	f.LarkClient = cachedLarkClientFunc(f)
	return f
}

func cachedConfigFunc(f *Factory) func() (*core.CliConfig, error) {
	return sync.OnceValues(func() (*core.CliConfig, error) {
		cfg, err := core.RequireConfig(f.Keychain)
		if err != nil {
			return cfg, err
		}
		registry.InitWithBrand(cfg.Brand)
		return cfg, nil
	})
}

func cachedAuthConfigFunc(f *Factory) func() (*core.CliConfig, error) {
	return sync.OnceValues(func() (*core.CliConfig, error) {
		return core.RequireAuth(f.Keychain)
	})
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

func cachedHttpClientFunc() func() (*http.Client, error) {
	return sync.OnceValues(func() (*http.Client, error) {
		util.WarnIfProxied(os.Stderr)

		var transport http.RoundTripper = util.NewBaseTransport()
		transport = &RetryTransport{Base: transport}
		transport = &SecurityHeaderTransport{Base: transport}

		transport = &auth.SecurityPolicyTransport{Base: transport} // Add our global response interceptor
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
		cfg, err := f.Config()
		if err != nil {
			return nil, err
		}
		opts := []lark.ClientOptionFunc{
			lark.WithLogLevel(larkcore.LogLevelError),
			lark.WithHeaders(BaseSecurityHeaders()),
		}
		// Build SDK transport chain
		util.WarnIfProxied(os.Stderr)
		var sdkTransport http.RoundTripper = util.NewBaseTransport()
		sdkTransport = &UserAgentTransport{Base: sdkTransport}
		sdkTransport = &auth.SecurityPolicyTransport{Base: sdkTransport}
		opts = append(opts, lark.WithHttpClient(&http.Client{
			Transport:     sdkTransport,
			CheckRedirect: safeRedirectPolicy,
		}))
		ep := core.ResolveEndpoints(cfg.Brand)
		opts = append(opts, lark.WithOpenBaseUrl(ep.Open))
		client := lark.NewClient(cfg.AppID, cfg.AppSecret, opts...)
		return client, nil
	})
}
