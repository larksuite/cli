// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package identitydiag

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/requestcontext"
)

func diagnoseEditionSource(
	ctx context.Context,
	f *cmdutil.Factory,
	cfg *core.CliConfig,
	verify bool,
) (Result, bool) {
	if f == nil || f.Credential == nil {
		return Result{}, false
	}
	source, err := f.Credential.InspectSource(ctx)
	if err != nil || source == nil || !source.Managed {
		return Result{}, false
	}
	return diagnoseExtendedSource(ctx, f, cfg, source, verify), true
}

func diagnoseExtendedSource(
	ctx context.Context,
	f *cmdutil.Factory,
	cfg *core.CliConfig,
	source *credential.SourceInspection,
	verify bool,
) Result {
	provider := source.Name
	if cfg == nil || cfg.AppID == "" {
		notConfigured := Identity{
			Status:  StatusNotConfigured,
			Message: "not configured (missing app config)",
			Hint:    externalCredentialHint(provider),
		}
		return Result{Bot: notConfigured, User: notConfigured}
	}
	ids := extcred.IdentitySupport(cfg.SupportedIdentities)
	supportsBot := cfg.SupportedIdentities == 0 || ids.Has(extcred.SupportsBot)
	supportsUser := cfg.SupportedIdentities == 0 || ids.Has(extcred.SupportsUser)
	return Result{
		Bot:  diagnoseExternalBot(ctx, f, cfg, provider, supportsBot, verify),
		User: diagnoseExtendedUser(ctx, f, cfg, source, supportsUser, verify),
	}
}

func diagnoseExtendedUser(
	ctx context.Context,
	f *cmdutil.Factory,
	cfg *core.CliConfig,
	source *credential.SourceInspection,
	supported bool,
	verify bool,
) Identity {
	if !source.ProvidesOnDemandAuth {
		return diagnoseExternalUser(ctx, f, cfg, source.Name, supported, verify)
	}
	provider := source.Name
	if !supported {
		return notProvidedExternally("User", provider)
	}
	id := Identity{
		Status:    StatusReady,
		Available: true,
		Message:   "User identity: available on demand (provided by " + provider + ")",
	}
	if !verify {
		return id
	}
	token, err := f.Credential.ResolveToken(ctx, credential.NewTokenSpec(core.AsUser, cfg.AppID))
	if err != nil {
		return externalVerifyFailed(id, "User", provider, err)
	}
	info, err := fetchEditionUserInfo(ctx, f, cfg, token.Token)
	if err != nil {
		return externalVerifyFailed(id, "User", provider, err)
	}
	id.Verified = boolPtr(true)
	id.OpenID = info.OpenID
	id.UserName = info.Name
	return id
}

type editionUserInfo struct {
	OpenID string
	Name   string
}

func fetchEditionUserInfo(
	ctx context.Context,
	f *cmdutil.Factory,
	cfg *core.CliConfig,
	token string,
) (*editionUserInfo, error) {
	httpClient, err := f.HttpClient()
	if err != nil {
		return nil, fmt.Errorf("create HTTP client: %w", err)
	}
	ctx = withEditionIdentity(ctx, core.AsUser)
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()
	endpoint := strings.TrimRight(core.ResolveEndpoints(cfg.Brand).Open, "/") + "/open-apis/authen/v1/user_info"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read user identity response: %w", err)
	}
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OpenID string `json:"open_id"`
			Name   string `json:"name"`
		} `json:"data"`
	}
	parseErr := json.Unmarshal(body, &envelope)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if parseErr != nil {
			return nil, fmt.Errorf(
				"user identity verification failed: HTTP %d (invalid response body): %w",
				resp.StatusCode,
				parseErr,
			)
		}
		return nil, fmt.Errorf(
			"user identity verification failed: HTTP %d, code %d: %s",
			resp.StatusCode,
			envelope.Code,
			envelope.Msg,
		)
	}
	if parseErr != nil {
		return nil, fmt.Errorf("decode user identity response: %w", parseErr)
	}
	if envelope.Code != 0 {
		return nil, fmt.Errorf("user identity verification failed: code %d: %s", envelope.Code, envelope.Msg)
	}
	if envelope.Data.OpenID == "" {
		return nil, fmt.Errorf("user identity verification returned no open_id")
	}
	return &editionUserInfo{OpenID: envelope.Data.OpenID, Name: envelope.Data.Name}, nil
}

func withEditionIdentity(ctx context.Context, identity core.Identity) context.Context {
	return requestcontext.WithIdentity(ctx, identity)
}
