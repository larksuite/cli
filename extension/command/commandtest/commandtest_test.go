// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package commandtest

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/larksuite/cli/extension/command"
)

func TestRecorderScriptsRequestsScopesAndDryRun(t *testing.T) {
	request := command.GET("/open-apis/im/v1/chats/chat_1").Set("user_id_type", "open_id")
	recorder := New(t, Respond(map[string]any{"chat_id": "chat_1"}))
	ctx := recorder.ExecutionContext(context.Background())
	commandContext := recorder.CommandContext(command.IdentityUser)

	if err := command.PreflightScopes(commandContext, "im:chat:read"); err != nil {
		t.Fatal(err)
	}
	var data struct {
		ChatID string `json:"chat_id"`
	}
	data, err := command.CallJSON[struct {
		ChatID string `json:"chat_id"`
	}](ctx, commandContext, request)
	if err != nil {
		t.Fatal(err)
	}
	if data.ChatID != "chat_1" {
		t.Fatalf("chat ID = %q", data.ChatID)
	}
	if got := recorder.ScopeChecks(); !reflect.DeepEqual(got, [][]string{{"im:chat:read"}}) {
		t.Fatalf("scope checks = %#v", got)
	}
	recorder.AssertDryRunMatches(command.Preview(request))
	recorder.AssertScriptConsumed()
}

func TestRecorderReturnsScriptedFailuresInOrder(t *testing.T) {
	want := errors.New("scripted failure")
	recorder := New(t, Fail(want), Respond(map[string]any{"id": "second"}))
	commandContext := recorder.CommandContext(command.IdentityBot)
	request := command.POST("/open-apis/base/v1/apps/app_1").Body(map[string]any{"name": "fixture"})

	if _, err := command.CallJSON[map[string]any](context.Background(), commandContext, request); !errors.Is(err, want) {
		t.Fatalf("first error = %v", err)
	}
	data, err := command.CallJSON[map[string]any](context.Background(), commandContext, request)
	if err != nil {
		t.Fatal(err)
	}
	if data["id"] != "second" {
		t.Fatalf("second response = %#v", data)
	}
	recorder.AssertScriptConsumed()
}

func TestRecorderInjectsCancellationIntoPaginationWait(t *testing.T) {
	recorder := New(t, Respond(map[string]any{
		"items":      []map[string]any{{"id": "first"}},
		"has_more":   true,
		"page_token": "next",
	}))
	recorder.SetPagination(command.PaginationOptions{All: true, MaxPages: 2, Delay: time.Hour})
	recorder.CancelAfterRequest(1)
	ctx := recorder.ExecutionContext(context.Background())

	_, err := command.CollectPages[struct {
		ID string `json:"id"`
	}](ctx, recorder.CommandContext(command.IdentityUser), command.GET("/open-apis/contact/v3/users"))
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("pagination error = %v", err)
	}
	recorder.AssertScriptConsumed()
}

func TestExecuteRunsPreparationAndReturnsTypedOutcome(t *testing.T) {
	type args struct {
		ID string `flag:"id" schema:"required" doc:"identifier"`
	}
	type data struct {
		ID string `json:"id" schema:"required" doc:"identifier"`
	}
	definition := command.Definition[args, data]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+test-execute", Description: "Test execute", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{command.IdentityUser: {}}},
		},
		Hooks: command.Hooks[args, data]{
			Normalize: func(_ context.Context, _ command.CommandContext, args *args) error {
				args.ID = "normalized-" + args.ID
				return nil
			},
			Execute: func(_ context.Context, _ command.CommandContext, args *args) (command.Result[data], error) {
				return command.Partial(data{ID: args.ID}), nil
			},
		},
	}
	recorder := New(t)
	execution, err := Execute(context.Background(), recorder, command.IdentityUser, definition, &args{ID: "one"})
	if err != nil {
		t.Fatal(err)
	}
	if execution.Data.ID != "normalized-one" || !execution.Partial {
		t.Fatalf("execution = %#v", execution)
	}
}
