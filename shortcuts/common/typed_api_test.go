// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"net/http"
	"testing"

	"github.com/larksuite/cli/internal/commandbridge"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/spf13/cobra"
)

func TestTypedClassifyContextPreservesCommandPath(t *testing.T) {
	root := &cobra.Command{Use: "lark"}
	service := &cobra.Command{Use: "fixture"}
	command := &cobra.Command{Use: "+typed"}
	root.AddCommand(service)
	service.AddCommand(command)

	ctx := typedCommandContext{runtime: &RuntimeContext{
		Cmd:        command,
		Config:     &core.CliConfig{AppID: "cli_test", Brand: core.BrandFeishu},
		resolvedAs: core.AsBot,
	}}
	classify := typedClassifyContext(ctx)
	if classify.AppID != "cli_test" || classify.Identity != string(core.AsBot) || classify.LarkCmd != "fixture +typed" {
		t.Fatalf("classify context = %#v", classify)
	}
}

func TestDoHostedAPIJSONPreservesSuccessData(t *testing.T) {
	runtime, registry := newCallAPITypedRuntime(t)
	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/x/y",
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Tt-Logid":   []string{"header-log-id"},
		},
		Body: map[string]any{
			"code": float64(0),
			"data": map[string]any{"log_id": "business-log-id", "value": "original"},
		},
	})

	data, err := DoHostedAPIJSON(context.Background(), typedCommandContext{runtime: runtime}, "GET", "/open-apis/x/y", nil, nil, commandbridge.Access{})
	if err != nil {
		t.Fatal(err)
	}
	if data["log_id"] != "business-log-id" || data["value"] != "original" {
		t.Fatalf("success data = %#v", data)
	}
}

// An Agent Employee that hits an authorization error through a typed shortcut
// must be told to ask the Agent platform, not to run the human device login.
// The classifier decides that from CredentialKind, so the typed path has to
// carry it like the untyped path does.
func TestTypedClassifyContextCarriesCredentialKind(t *testing.T) {
	ctx := typedCommandContext{runtime: &RuntimeContext{
		Cmd: &cobra.Command{Use: "+typed"},
		Config: &core.CliConfig{
			AppID:           "cli_test",
			Brand:           core.BrandFeishu,
			CredentialKind:  core.CredentialAAT,
			PrincipalOpenID: "ou_agent",
		},
		resolvedAs: core.AsUser,
	}}
	classify := typedClassifyContext(ctx)
	if classify.CredentialKind != string(core.CredentialAAT) {
		t.Fatalf("CredentialKind = %q, want %q", classify.CredentialKind, core.CredentialAAT)
	}
}
