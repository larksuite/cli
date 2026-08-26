// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package commandtest

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/extension/download"
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

func TestRecorderScriptsFileDownloadAndMatchesDryRunIntent(t *testing.T) {
	request := command.GET("/open-apis/drive/v1/files/file_1/download").Set("version", "7")
	target := command.FileTarget{Name: "reports/file.bin"}
	recorder := New(t).ReplyFile("GET", "/open-apis/drive/v1/files/file_1/download", "application/octet-stream", []byte("payload"))

	options := command.DownloadOptions{Representation: download.Immutable, Transfer: download.Options{PartSize: 4}}
	artifact, err := command.Download(context.Background(), recorder.CommandContext(command.IdentityUser), request, target, options)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Name != target.Name || artifact.Location != target.Name || artifact.Size != 7 || artifact.ContentType != "application/octet-stream" {
		t.Fatalf("artifact = %#v", artifact)
	}
	files := recorder.Files()
	if len(files) != 1 || string(files[0].Content) != "payload" || files[0].Target.Name != target.Name || !reflect.DeepEqual(files[0].Options, options) {
		t.Fatalf("recorded files = %#v", files)
	}
	recorder.AssertDryRunMatches(command.NewDryRun(request).File(target.Intent("OpenAPI response body")))
	recorder.AssertScriptConsumed()
}

func TestBusinessShortcutComposesOAPIAndURLDownload(t *testing.T) {
	type args struct {
		ID     string `flag:"file-token" schema:"required;minLength=1" doc:"file token"`
		Output string `flag:"output" schema:"required;minLength=1" doc:"output path"`
	}
	type descriptor struct {
		DownloadURL string `json:"download_url"`
	}
	type data struct {
		ID       string           `json:"id" schema:"required" doc:"file token"`
		Artifact command.Artifact `json:"artifact" schema:"required" doc:"saved artifact"`
	}
	const sourceURL = "https://cdn.example.com/files/report.bin?signature=test"
	request := func(args *args) command.Request {
		return command.GET("/open-apis/drive/v1/files/" + command.PathSegment(args.ID) + "/download_url")
	}
	target := func(args *args) command.FileTarget { return command.FileTarget{Name: args.Output} }
	definition := command.Definition[args, data]{
		Metadata: command.CommandMetadata{
			Service: command.DomainDrive, Command: "+test-backup", Description: "Back up one file", Risk: command.RiskWrite,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{command.IdentityUser: {}}},
		},
		Hooks: command.Hooks[args, data]{
			DryRun: func(_ context.Context, _ command.CommandContext, args *args) *command.DryRun {
				return command.NewDryRun(request(args)).File(target(args).Intent("resolved URL response body"))
			},
			Execute: func(ctx context.Context, commandContext command.CommandContext, args *args) (command.Result[data], error) {
				resolved, err := command.CallJSON[descriptor](ctx, commandContext, request(args))
				if err != nil {
					return command.Result[data]{}, err
				}
				artifact, err := command.DownloadURL(ctx, commandContext, resolved.DownloadURL, target(args),
					command.DownloadOptions{Representation: download.Immutable})
				if err != nil {
					return command.Result[data]{}, err
				}
				return command.Success(data{ID: args.ID, Artifact: artifact}), nil
			},
		},
	}
	input := &args{ID: "file_1", Output: "report.bin"}
	recorder := New(t, Respond(map[string]any{"download_url": sourceURL}), RespondFile("application/octet-stream", []byte("payload")))
	execution, err := Execute(context.Background(), recorder, command.IdentityUser, definition, input)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Data.ID != "file_1" || execution.Data.Artifact.Size != 7 || !reflect.DeepEqual(recorder.URLs(), []string{sourceURL}) {
		t.Fatalf("execution = %#v, URLs = %#v", execution, recorder.URLs())
	}
	files := recorder.Files()
	if len(files) != 1 || files[0].SourceURL != sourceURL || string(files[0].Content) != "payload" {
		t.Fatalf("recorded files = %#v", files)
	}
	preview, err := Preview(context.Background(), recorder, command.IdentityUser, definition, input)
	if err != nil {
		t.Fatal(err)
	}
	recorder.AssertDryRunMatches(preview)
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

func TestRecorderReplyURLChecksTheRequestedURLInOrder(t *testing.T) {
	const first = "https://cdn.example.com/files/first.bin?signature=a"
	const second = "https://cdn.example.com/files/second.bin?signature=b"
	recorder := New(t).
		ReplyURL(first, "application/octet-stream", []byte("one")).
		ReplyURL(second, "image/png", []byte("two"))
	commandContext := recorder.CommandContext(command.IdentityUser)

	for index, source := range []string{first, second} {
		target := command.FileTarget{Name: "download-" + strconv.Itoa(index) + ".bin"}
		artifact, err := command.DownloadURL(context.Background(), commandContext, source, target)
		if err != nil {
			t.Fatal(err)
		}
		if artifact.Name != target.Name || artifact.Size != 3 {
			t.Fatalf("artifact %d = %#v", index+1, artifact)
		}
	}
	if !reflect.DeepEqual(recorder.URLs(), []string{first, second}) {
		t.Fatalf("URLs = %#v", recorder.URLs())
	}
	files := recorder.Files()
	if len(files) != 2 || files[0].SourceURL != first || files[1].SourceURL != second {
		t.Fatalf("recorded files = %#v", files)
	}
	if files[0].Artifact.ContentType != "application/octet-stream" || files[1].Artifact.ContentType != "image/png" {
		t.Fatalf("content types = %q / %q", files[0].Artifact.ContentType, files[1].Artifact.ContentType)
	}
	recorder.AssertScriptConsumed()
}

func TestRecorderReplyURLRejectsUnexpectedURL(t *testing.T) {
	recorder := New(t).ReplyURL("https://cdn.example.com/files/expected.bin", "application/octet-stream", []byte("payload"))
	_, err := command.DownloadURL(context.Background(), recorder.CommandContext(command.IdentityUser),
		"https://cdn.example.com/files/other.bin", command.FileTarget{Name: "download.bin"})
	if err == nil || !strings.Contains(err.Error(), "expected") {
		t.Fatalf("DownloadURL() error = %v", err)
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

func TestExecuteRunsPreparationAndReturnsTypedData(t *testing.T) {
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
				return command.Success(data{ID: args.ID}), nil
			},
		},
	}
	recorder := New(t)
	execution, err := Execute(context.Background(), recorder, command.IdentityUser, definition, &args{ID: "one"})
	if err != nil {
		t.Fatal(err)
	}
	if execution.Data.ID != "normalized-one" {
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

// Every harness entry point must run the production compiler. A declaration the
// real CLI would refuse to mount has to fail in the unit test too -- otherwise a
// green test ships a command that cannot mount. Preview was the entry that
// skipped this, so the case covers all three.
func TestEveryEntryPointRunsTheProductionCompiler(t *testing.T) {
	type args struct {
		Value string // no flag or arg tag: the compiler refuses this
	}
	type data struct {
		OK bool `json:"ok" schema:"required" doc:"success state"`
	}
	definition := command.Definition[args, data]{
		Metadata: command.CommandMetadata{
			Service: command.DomainIm, Command: "+test-uncompilable", Description: "Missing flag tag", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{command.IdentityUser: {}}},
		},
		Hooks: command.Hooks[args, data]{
			DryRun: func(context.Context, command.CommandContext, *args) *command.DryRun {
				return command.NewDryRun(command.GET("/open-apis/im/v1/chats"))
			},
			Execute: func(context.Context, command.CommandContext, *args) (command.Result[data], error) {
				return command.Success(data{OK: true}), nil
			},
		},
	}
	const want = "must declare exactly one of flag or arg"

	t.Run("Execute", func(t *testing.T) {
		recorder := New(t, Respond(map[string]any{}))
		if _, err := Execute(context.Background(), recorder, command.IdentityUser, definition, &args{}); err == nil ||
			!strings.Contains(err.Error(), want) {
			t.Fatalf("Execute error = %v, want the compiler refusal", err)
		}
	})
	t.Run("Preview", func(t *testing.T) {
		recorder := New(t)
		if _, err := Preview(context.Background(), recorder, command.IdentityUser, definition, &args{}); err == nil ||
			!strings.Contains(err.Error(), want) {
			t.Fatalf("Preview error = %v, want the compiler refusal", err)
		}
	})
	t.Run("RunWithFlags", func(t *testing.T) {
		recorder := New(t)
		if _, err := RunWithFlags(context.Background(), recorder, command.IdentityUser, definition, &args{}); err == nil ||
			!strings.Contains(err.Error(), want) {
			t.Fatalf("RunWithFlags error = %v, want the compiler refusal", err)
		}
	})
}
