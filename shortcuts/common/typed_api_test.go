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
