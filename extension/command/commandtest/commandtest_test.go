// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package commandtest

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
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
	recorder.AssertDryRunMatches(command.NewDryRun(request))
	recorder.AssertScriptConsumed()
}

func TestRecorderRequestsAreDeepCopiedWithJSONNumbers(t *testing.T) {
	body := map[string]any{"name": "original"}
	request := command.POST("/open-apis/im/v1/chats").Set("page_size", 20).Body(body)
	recorder := New(t, Respond(map[string]any{}))
	if _, err := command.CallJSON[map[string]any](context.Background(), recorder.CommandContext(command.IdentityUser), request); err != nil {
		t.Fatal(err)
	}
	body["name"] = "mutated"

	requests := recorder.Requests()
	if len(requests) != 1 || requests[0].Query["page_size"] != json.Number("20") {
		t.Fatalf("recorded query = %#v", requests)
	}
	recordedBody, ok := requests[0].Body.(map[string]any)
	if !ok || recordedBody["name"] != "original" {
		t.Fatalf("recorded body = %#v", requests[0].Body)
	}
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

func TestRecorderReplyJSONChecksRequestInOrder(t *testing.T) {
	recorder := New(t).
		ReplyJSON("GET", "/open-apis/im/v1/chats/first", map[string]any{"id": "first"}).
		ReplyJSON("GET", "/open-apis/im/v1/chats/second", map[string]any{"id": "second"})
	commandContext := recorder.CommandContext(command.IdentityUser)
	for _, id := range []string{"first", "second"} {
		data, err := command.CallJSON[map[string]any](context.Background(), commandContext, command.GET("/open-apis/im/v1/chats/"+id))
		if err != nil {
			t.Fatal(err)
		}
		if data["id"] != id {
			t.Fatalf("response = %#v", data)
		}
	}
	recorder.AssertScriptConsumed()
}

func TestRecorderReplyJSONRejectsUnexpectedRequest(t *testing.T) {
	recorder := New(t).ReplyJSON("POST", "/open-apis/im/v1/chats", map[string]any{})
	_, err := command.CallJSON[map[string]any](context.Background(), recorder.CommandContext(command.IdentityUser), command.GET("/open-apis/im/v1/chats"))
	if err == nil || !strings.Contains(err.Error(), "expected") {
		t.Fatalf("CallJSON() error = %v", err)
	}
	recorder.AssertScriptConsumed()
}

func TestRecorderInjectsCancellationIntoPaginationWait(t *testing.T) {
	recorder := New(t, Respond(map[string]any{
		"items":      []map[string]any{{"id": "first"}},
		"has_more":   true,
		"page_token": "next",
	}))
	recorder.SetPagination(command.PaginationOptions{All: true, MaxPages: 2, Delay: time.Minute})
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

func TestExecuteRejectsResultAndErrorTogether(t *testing.T) {
	type args struct{}
	type data struct{}
	sentinel := errors.New("execute failed")
	definition := command.Definition[args, data]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+test-result-error", Description: "Test result protocol", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{command.IdentityUser: {}}},
		},
		Hooks: command.Hooks[args, data]{
			Execute: func(context.Context, command.CommandContext, *args) (command.Result[data], error) {
				return command.Success(data{}), sentinel
			},
		},
	}
	_, err := Execute(context.Background(), New(t), command.IdentityUser, definition, &args{})
	if err == nil || !errors.Is(err, sentinel) || err == sentinel {
		t.Fatalf("Execute() error = %v", err)
	}
}

// DryRun cannot fail, so Validate is the only hook that can stop a preview.
func TestPreviewPropagatesValidateError(t *testing.T) {
	type args struct{}
	type data struct{}
	sentinel := command.ValidationErrorf("dry-run input is invalid")
	definition := command.Definition[args, data]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+test-dry-run-error", Description: "Test dry-run error", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{command.IdentityUser: {}}},
		},
		Hooks: command.Hooks[args, data]{
			Validate: func(context.Context, command.CommandContext, *args) error {
				return sentinel
			},
			DryRun: func(context.Context, command.CommandContext, *args) *command.DryRun {
				return command.NewDryRun(command.GET("/open-apis/im/v1/chats"))
			},
			Execute: func(context.Context, command.CommandContext, *args) (command.Result[data], error) {
				return command.Success(data{}), nil
			},
		},
	}
	_, err := Preview(context.Background(), New(t), command.IdentityUser, definition, &args{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Preview() error = %v", err)
	}
}

func TestRunWithFlagsRejectsNonPageOutput(t *testing.T) {
	type args struct{}
	type data struct{}
	definition := command.Definition[args, data]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+test-non-page", Description: "Test non-page flags", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{command.IdentityUser: {}}},
		},
		Hooks: command.Hooks[args, data]{
			Execute: func(context.Context, command.CommandContext, *args) (command.Result[data], error) {
				return command.Success(data{}), nil
			},
		},
	}
	_, err := RunWithFlags(context.Background(), New(t), command.IdentityUser, definition, &args{}, "--page-all")
	if err == nil {
		t.Fatal("RunWithFlags() error is nil")
	}
}
