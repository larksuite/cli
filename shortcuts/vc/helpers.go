// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/appmeta"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	meetingQueryUserScope = "vc:meeting.meetingevent:read"
	meetingQueryBotScope  = "vc:meeting.bot.join:write"
)

// meetingQueryAnyScopes are the scopes accepted by the VC meeting query
// commands (+meeting-list-active, +meeting-events). UAT recommends
// vc:meeting.meetingevent:read and TAT recommends vc:meeting.bot.join:write,
// but both identities accept either scope for compatibility.
//
// The shortcut framework's Scopes/UserScopes/BotScopes preflight is AND, so
// it cannot express "any of these". Those commands therefore leave the
// unconditional scope fields empty and call checkMeetingQueryAnyScope from
// Validate instead.
var meetingQueryAnyScopes = []string{
	meetingQueryUserScope,
	meetingQueryBotScope,
}

type meetingQueryTenantScopesFetcher func(context.Context, *common.RuntimeContext) ([]string, bool, error)

type meetingQueryAppMetaClient struct {
	apiClient *client.APIClient
	runtime   *common.RuntimeContext
}

func (c *meetingQueryAppMetaClient) CallAPI(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	resp, err := c.apiClient.DoAPI(ctx, client.RawApiRequest{
		Method: method,
		URL:    path,
		Data:   body,
		As:     core.AsBot,
	})
	if err != nil {
		return nil, err
	}
	if _, err := c.runtime.ClassifyAPIResponse(resp); err != nil {
		return json.RawMessage(resp.RawBody), err
	}
	return json.RawMessage(resp.RawBody), nil
}

func fetchMeetingQueryTenantScopes(ctx context.Context, runtime *common.RuntimeContext) ([]string, bool, error) {
	if runtime == nil || runtime.Factory == nil || runtime.Config == nil ||
		runtime.Factory.LarkClient == nil || runtime.Factory.HttpClient == nil {
		return nil, false, nil
	}
	apiClient, err := runtime.Factory.NewAPIClientWithConfig(runtime.Config)
	if err != nil {
		return nil, false, err
	}
	appVersion, err := appmeta.FetchCurrentPublished(ctx, &meetingQueryAppMetaClient{
		apiClient: apiClient,
		runtime:   runtime,
	}, runtime.Config.AppID)
	if err != nil {
		return nil, false, err
	}
	if appVersion == nil {
		return nil, false, nil
	}
	return appVersion.TenantScopes, true, nil
}

func checkMeetingQueryAnyScope(ctx context.Context, runtime *common.RuntimeContext) error {
	return checkMeetingQueryScopeWithTenantScopes(ctx, runtime, fetchMeetingQueryTenantScopes)
}

func checkMeetingQueryScopeWithTenantScopes(ctx context.Context, runtime *common.RuntimeContext, fetchTenantScopes meetingQueryTenantScopesFetcher) error {
	if runtime == nil || runtime.Config == nil {
		return nil
	}
	if runtime.As().IsBot() {
		if fetchTenantScopes == nil {
			return nil
		}
		// App metadata is a best-effort local preflight. When it is unavailable,
		// let the meeting API remain the source of truth.
		scopes, known, _ := fetchTenantScopes(ctx, runtime)
		if !known {
			return nil
		}
		if hasAnyGrantedScope(strings.Join(scopes, " "), meetingQueryAnyScopes) {
			return nil
		}
		return newMeetingQueryPermissionError(runtime, meetingQueryBotScope)
	}
	if runtime.Factory == nil || runtime.Factory.Credential == nil {
		return nil
	}
	// Resolve the identity's granted scopes. If anything about the token cannot
	// be resolved locally, skip the preflight and let the remote API be the
	// source of truth, instead of blocking a call the server might still allow.
	result, err := runtime.Factory.Credential.ResolveToken(ctx, credential.NewTokenSpec(runtime.As(), runtime.Config.AppID))
	if err != nil {
		return nil //nolint:nilerr // intentional: fall back to remote authorization
	}
	if result == nil || result.Scopes == "" {
		return nil
	}
	if hasAnyGrantedScope(result.Scopes, meetingQueryAnyScopes) {
		return nil
	}
	return newMeetingQueryPermissionError(runtime, meetingQueryUserScope)
}

func newMeetingQueryPermissionError(runtime *common.RuntimeContext, required string) error {
	permissionErr := errs.NewPermissionError(
		errs.SubtypeMissingScope,
		"missing required scope(s): %s",
		required,
	).
		WithMissingScopes(required).
		WithIdentity(string(runtime.As()))
	if runtime.As().IsBot() {
		consoleURL := registry.BuildConsoleScopeURL(runtime.Config.Brand, runtime.Config.AppID, required)
		return permissionErr.
			WithConsoleURL(consoleURL).
			WithHint("the app developer must apply for scope %s at the developer console: %s", required, consoleURL)
	}
	return permissionErr.WithHint("run `lark-cli auth login --scope %q` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", required)
}

func hasAnyGrantedScope(granted string, candidates []string) bool {
	for _, scope := range candidates {
		if len(auth.MissingScopes(granted, []string{scope})) == 0 {
			return true
		}
	}
	return false
}
