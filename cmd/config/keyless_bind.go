// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/keylessprovider"
	"github.com/larksuite/cli/internal/keysigner"
)

const keylessBindProbeTimeout = 12 * time.Second

var fetchTATForBind = fetchTATForFreshBind

func fetchTATForFreshBind(ctx context.Context, httpClient *http.Client, brand core.LarkBrand, clientID string, signer keysigner.Signer, provider, keyRef string) (string, func() error, error) {
	helper, commitProviderManifest, err := keylessprovider.PrepareRefresh(ctx, provider)
	if err != nil {
		return "", nil, err
	}
	token, err := credential.FetchTATWithAssertionWithHelper(ctx, httpClient, brand, clientID, signer, helper, keyRef)
	if err != nil {
		return "", nil, err
	}
	return token, commitProviderManifest, nil
}

// validateBindResult proves that an OpenClaw keyless account can be used by
// the exact helper/keyRef/appID tuple that will be persisted. Minting a TAT is
// intentional: pubkey alone only proves that the helper runs (and some signer
// implementations create a missing key during pubkey); a successful token mint
// proves that this public key is already registered to the selected app, so no
// attach flow or second user authorization is needed.
func validateBindResult(parent context.Context, opts *BindOptions, result *BindResult) error {
	if result == nil || result.AppConfig == nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "config bind produced no app configuration")
	}
	app := result.AppConfig
	if app.AuthMethod != core.AuthMethodPrivateKeyJWT {
		return nil
	}
	if app.KeyRef == nil || app.KeyRef.ID == "" {
		return errs.NewConfigError(errs.SubtypeInvalidConfig,
			"private_key_jwt bind for app %s is missing keyRef", app.AppId)
	}
	if strings.TrimSpace(app.KeyRef.Provider) != core.KeylessProviderLarkSuite {
		return errs.NewConfigError(errs.SubtypeInvalidClient,
			"OpenClaw private_key_jwt bind for app %s did not select provider %s", app.AppId, core.KeylessProviderLarkSuite)
	}
	if opts == nil || opts.Factory == nil || opts.Factory.HttpClient == nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "cannot validate keyless bind without an HTTP client")
	}
	httpClient, err := opts.Factory.HttpClient()
	if err != nil {
		return errs.NewNetworkError(errs.SubtypeNetworkTransport,
			"cannot create HTTP client for keyless bind validation: %v", err).WithCause(err)
	}

	ctx, cancel := context.WithTimeout(parent, keylessBindProbeTimeout)
	defer cancel()
	_, commitProviderManifest, err := fetchTATForBind(
		ctx, httpClient, app.Brand, app.AppId, keysigner.Active(), app.KeyRef.Provider, app.KeyRef.ID,
	)
	if err != nil {
		if errs.IsTyped(err) {
			return err
		}
		return errs.NewConfigError(errs.SubtypeInvalidClient,
			"OpenClaw signer could not authenticate app %s: %v", app.AppId, err).
			WithHint("repair or reinstall the OpenClaw Feishu plugin and its platform signer dependency, verify the keyless account, then retry config bind").
			WithCause(err)
	}
	if commitProviderManifest == nil {
		return errs.NewInternalError(errs.SubtypeStorage,
			"OpenClaw signer validation did not produce a provider manifest commit")
	}
	result.commitProviderManifest = commitProviderManifest
	return nil
}
