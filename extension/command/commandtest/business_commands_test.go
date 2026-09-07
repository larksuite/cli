// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package commandtest_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/extension/command/commandtest"
	"github.com/larksuite/cli/internal/commandhost"
	internalpagination "github.com/larksuite/cli/internal/pagination"
)

type documentGetArgs struct {
	DocumentID string `flag:"document-id" schema:"required;minLength=1" doc:"document identifier"`
}

type documentData struct {
	Content string `json:"content" schema:"required" doc:"document text content"`
}

func documentGetDefinition() command.Definition[documentGetArgs, documentData] {
	request := func(args *documentGetArgs) command.Request {
		return command.GET("/open-apis/docx/v1/documents/" + command.PathSegment(args.DocumentID) + "/raw_content")
	}
	return command.Definition[documentGetArgs, documentData]{
		Metadata: command.CommandMetadata{
			Service: "docs", Command: "+business-document-get", Description: "Read one document", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{"docx:document:readonly"}},
			}},
		},
		Hooks: command.Hooks[documentGetArgs, documentData]{
			DryRun: func(_ context.Context, _ command.CommandContext, args *documentGetArgs) *command.DryRun {
				return command.NewDryRun(request(args))
			},
			Execute: func(ctx context.Context, commandContext command.CommandContext, args *documentGetArgs) (command.Result[documentData], error) {
				data, err := command.CallJSON[documentData](ctx, commandContext, request(args))
				if err != nil {
					return command.Result[documentData]{}, err
				}
				return command.Success(data), nil
			},
		},
	}
}

type chatListArgs struct {
	PageSize int `flag:"page-size" schema:"optional;default=20;minimum=1;maximum=100" doc:"items per page"`
}

type chatData struct {
	ChatID string `json:"chat_id" schema:"required" doc:"chat identifier"`
	Name   string `json:"name" schema:"required" doc:"chat name"`
}

func chatListDefinition() command.Definition[chatListArgs, command.Page[chatData]] {
	request := func(args *chatListArgs) command.Request {
		return command.GET("/open-apis/im/v1/chats").Set("page_size", args.PageSize)
	}
	return command.Definition[chatListArgs, command.Page[chatData]]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+business-chat-list", Description: "List chats", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{"im:chat:read"}},
			}},
		},
		Hooks: command.Hooks[chatListArgs, command.Page[chatData]]{
			DryRun: func(_ context.Context, _ command.CommandContext, args *chatListArgs) *command.DryRun {
				return command.NewDryRun(request(args))
			},
			Execute: func(ctx context.Context, commandContext command.CommandContext, args *chatListArgs) (command.Result[command.Page[chatData]], error) {
				page, err := command.CollectPages[chatData](ctx, commandContext, request(args))
				if err != nil {
					return command.Result[command.Page[chatData]]{}, err
				}
				return command.Success(page), nil
			},
		},
	}
}

type taskAuditArgs struct {
	IncludeOwners bool `flag:"include-owners" schema:"optional;default=false" doc:"include owner names"`
}

type taskRecord struct {
	TaskID  string `json:"task_id" schema:"required" doc:"task identifier"`
	OwnerID string `json:"owner_id" schema:"required" doc:"owner identifier"`
}

type taskAuditItem struct {
	TaskID    string `json:"task_id" schema:"required" doc:"task identifier"`
	OwnerName string `json:"owner_name" schema:"required" doc:"owner name"`
	State     string `json:"state" schema:"required;enum=success|failed" doc:"enrichment state"`
}

type taskAuditData struct {
	Items    []taskAuditItem   `json:"items" schema:"required;nonnullable" doc:"task audit results"`
	Failures []command.Failure `json:"failures" schema:"required;nonnullable" doc:"safe failure details"`
}

func taskAuditDefinition() command.Definition[taskAuditArgs, taskAuditData] {
	listRequest := command.GET("/open-apis/task/v2/tasks").Set("page_size", 50)
	return command.Definition[taskAuditArgs, taskAuditData]{
		Metadata: command.CommandMetadata{
			Service: "task", Command: "+business-task-audit", Description: "Audit tasks with optional owners", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {
					RequiredScopes: []string{"task:task:read"},
					ConditionalScopes: []command.ConditionalScope{{
						Scopes: []string{"contact:user.base:readonly"}, When: "--include-owners is true",
						Params: []string{"include-owners"}, Requirement: command.ScopeBestEffort,
					}},
				},
			}},
		},
		Hooks: command.Hooks[taskAuditArgs, taskAuditData]{
			DryRun: func(_ context.Context, _ command.CommandContext, _ *taskAuditArgs) *command.DryRun {
				return command.NewDryRun(listRequest).Desc("Owner requests depend on task owner identifiers returned by the list call.")
			},
			Execute: func(ctx context.Context, commandContext command.CommandContext, args *taskAuditArgs) (command.Result[taskAuditData], error) {
				tasks, err := command.CollectAllPages[taskRecord](ctx, commandContext, listRequest)
				if err != nil {
					return command.Result[taskAuditData]{}, err
				}
				data := taskAuditData{Items: make([]taskAuditItem, 0, len(tasks))}
				if !args.IncludeOwners {
					for _, task := range tasks {
						data.Items = append(data.Items, taskAuditItem{TaskID: task.TaskID, State: "success"})
					}
					return command.Success(data), nil
				}
				if err := command.PreflightScopes(commandContext, "contact:user.base:readonly"); err != nil {
					for _, task := range tasks {
						data.Items = append(data.Items, taskAuditItem{TaskID: task.TaskID, State: "failed"})
					}
					data.Failures = append(data.Failures, command.SnapshotFailure(err))
					return command.Success(data), nil
				}
				for _, task := range tasks {
					owner, ownerErr := command.CallJSON[struct {
						Name string `json:"name"`
					}](ctx, commandContext, command.GET("/open-apis/contact/v3/users/"+command.PathSegment(task.OwnerID)))
					if ownerErr != nil {
						data.Items = append(data.Items, taskAuditItem{TaskID: task.TaskID, State: "failed"})
						data.Failures = append(data.Failures, command.SnapshotFailure(ownerErr))
						continue
					}
					data.Items = append(data.Items, taskAuditItem{TaskID: task.TaskID, OwnerName: owner.Name, State: "success"})
				}
				return command.Success(data), nil
			},
		},
	}
}

type memberListArgs struct {
	ChatID         string `flag:"chat-id" schema:"required;minLength=1" doc:"chat identifier"`
	IncludeMembers bool   `flag:"include-members" schema:"optional;default=false" doc:"include member identifiers"`
}

type memberListData struct {
	ChatID  string   `json:"chat_id" schema:"required" doc:"chat identifier"`
	Members []string `json:"members" schema:"required;nonnullable" doc:"member identifiers"`
}

func memberListDefinition() command.Definition[memberListArgs, memberListData] {
	return command.Definition[memberListArgs, memberListData]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+business-chat-inspect", Description: "Inspect a chat", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {
					RequiredScopes: []string{"im:chat:read"},
					ConditionalScopes: []command.ConditionalScope{{
						Scopes: []string{"im:chat.members:read"}, When: "--include-members is true",
						Params: []string{"include-members"}, Requirement: command.ScopeRequired,
					}},
				},
			}},
		},
		Hooks: command.Hooks[memberListArgs, memberListData]{
			DryRun: func(_ context.Context, _ command.CommandContext, args *memberListArgs) *command.DryRun {
				preview := command.NewDryRun(command.GET("/open-apis/im/v1/chats/" + command.PathSegment(args.ChatID)))
				if args.IncludeMembers {
					preview.Add(command.GET("/open-apis/im/v1/chats/" + command.PathSegment(args.ChatID) + "/members"))
				}
				return preview
			},
			Execute: func(ctx context.Context, commandContext command.CommandContext, args *memberListArgs) (command.Result[memberListData], error) {
				data, err := command.CallJSON[memberListData](ctx, commandContext, command.GET("/open-apis/im/v1/chats/"+command.PathSegment(args.ChatID)))
				if err != nil {
					return command.Result[memberListData]{}, err
				}
				if !args.IncludeMembers {
					return command.Success(data), nil
				}
				if err := command.PreflightScopes(commandContext, "im:chat.members:read"); err != nil {
					return command.Result[memberListData]{}, err
				}
				members, err := command.CallJSON[struct {
					Items []string `json:"items"`
				}](ctx, commandContext, command.GET("/open-apis/im/v1/chats/"+command.PathSegment(args.ChatID)+"/members"))
				if err != nil {
					return command.Result[memberListData]{}, err
				}
				data.Members = members.Items
				return command.Success(data), nil
			},
		},
	}
}

func TestBusinessDefinitionsCompileTogether(t *testing.T) {
	sets := []command.Set{
		{Domain: command.ExtendDomain(command.DomainDocs), Commands: []command.Command{command.Define(documentGetDefinition())}},
		{Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{command.Define(chatListDefinition()), command.Define(memberListDefinition())}},
		{Domain: command.ExtendDomain(command.DomainTask), Commands: []command.Command{command.Define(taskAuditDefinition())}},
	}
	compiled, err := commandhost.CompileSets(sets)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled) != 4 {
		t.Fatalf("compiled commands = %d", len(compiled))
	}
}

func TestSingleReadAndDryRunUseSameRequest(t *testing.T) {
	definition := documentGetDefinition()
	recorder := commandtest.New(t, commandtest.Respond(map[string]any{"content": "example"}))
	args := &documentGetArgs{DocumentID: "doc_1"}
	execution, err := commandtest.Execute(context.Background(), recorder, command.IdentityUser, definition, args)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Data.Content != "example" {
		t.Fatalf("document data = %#v", execution.Data)
	}
	preview, err := commandtest.Preview(context.Background(), recorder, command.IdentityUser, definition, args)
	if err != nil {
		t.Fatal(err)
	}
	recorder.AssertDryRunMatches(preview)
	recorder.AssertScriptConsumed()
}

func TestSingleReadPreservesTypedAPIError(t *testing.T) {
	want := command.InvalidResponseErrorf("upstream response is malformed")
	recorder := commandtest.New(t, commandtest.Fail(want))
	_, err := commandtest.Execute(context.Background(), recorder, command.IdentityUser, documentGetDefinition(), &documentGetArgs{DocumentID: "doc_1"})
	if !errors.Is(err, want) {
		t.Fatalf("single read error = %v", err)
	}
	recorder.AssertScriptConsumed()
}

func TestListCommandUsesHostPagination(t *testing.T) {
	recorder := commandtest.New(t,
		commandtest.Respond(map[string]any{
			"items": []map[string]any{{"chat_id": "chat_1", "name": "one"}}, "has_more": true, "page_token": "next",
		}),
		commandtest.Respond(map[string]any{
			"items": []map[string]any{{"chat_id": "chat_2", "name": "two"}}, "has_more": false,
		}),
	)
	execution, err := commandtest.RunWithFlags(context.Background(), recorder, command.IdentityUser,
		chatListDefinition(), &chatListArgs{PageSize: 20}, "--page-all", "--page-limit=3", "--page-delay=0")
	if err != nil {
		t.Fatal(err)
	}
	if len(execution.Data.Items) != 2 || !execution.Data.Complete() || execution.Data.Pages() != 2 {
		t.Fatalf("page = %#v, complete=%v, pages=%d", execution.Data.Items, execution.Data.Complete(), execution.Data.Pages())
	}
	requests := recorder.Requests()
	if len(requests) != 2 || requests[1].Query["page_token"] != "next" {
		t.Fatalf("requests = %#v", requests)
	}
	recorder.AssertScriptConsumed()
}

func TestListCommandReadsOnePageByDefault(t *testing.T) {
	recorder := commandtest.New(t, commandtest.Respond(map[string]any{
		"items": []map[string]any{{"chat_id": "chat_1", "name": "one"}}, "has_more": true, "page_token": "next",
	}))
	execution, err := commandtest.Execute(context.Background(), recorder, command.IdentityUser, chatListDefinition(), &chatListArgs{PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if execution.Data.Complete() || execution.Data.Pages() != 1 || execution.Data.NextToken() != "next" {
		t.Fatalf("default page complete=%v pages=%d next=%q", execution.Data.Complete(), execution.Data.Pages(), execution.Data.NextToken())
	}
	if len(recorder.Requests()) != 1 {
		t.Fatalf("default requests = %#v", recorder.Requests())
	}
	recorder.AssertScriptConsumed()
}

func TestListCommandResumesAndStopsAtPageLimit(t *testing.T) {
	recorder := commandtest.New(t,
		commandtest.Respond(map[string]any{"items": []map[string]any{{"chat_id": "chat_1"}}, "has_more": true, "page_token": "next-1"}),
		commandtest.Respond(map[string]any{"items": []map[string]any{{"chat_id": "chat_2"}}, "has_more": true, "page_token": "next-2"}),
	)
	recorder.SetPagination(command.PaginationOptions{All: true, MaxPages: 2})
	page, err := command.CollectPages[chatData](context.Background(), recorder.CommandContext(command.IdentityUser),
		command.GET("/open-apis/im/v1/chats").Set("page_token", "resume"))
	if err != nil {
		t.Fatal(err)
	}
	if page.Complete() || page.Pages() != 2 || page.NextToken() != "next-2" || len(page.Items) != 2 {
		t.Fatalf("limited page complete=%v pages=%d next=%q items=%d", page.Complete(), page.Pages(), page.NextToken(), len(page.Items))
	}
	requests := recorder.Requests()
	if len(requests) != 2 || requests[0].Query["page_token"] != "resume" || requests[1].Query["page_token"] != "next-1" {
		t.Fatalf("resume requests = %#v", requests)
	}
	recorder.AssertScriptConsumed()
}

func TestCollectAllPagesRejectsInvalidCursors(t *testing.T) {
	for _, test := range []struct {
		name      string
		responses []commandtest.Response
	}{
		{name: "missing", responses: []commandtest.Response{
			commandtest.Respond(map[string]any{"items": []map[string]any{}, "has_more": true}),
		}},
		{name: "repeated", responses: []commandtest.Response{
			commandtest.Respond(map[string]any{"items": []map[string]any{}, "has_more": true, "page_token": "same"}),
			commandtest.Respond(map[string]any{"items": []map[string]any{}, "has_more": true, "page_token": "same"}),
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := commandtest.New(t, test.responses...)
			_, err := command.CollectAllPages[chatData](context.Background(), recorder.CommandContext(command.IdentityUser), command.GET("/open-apis/im/v1/chats"))
			if err == nil {
				t.Fatal("CollectAllPages() error is nil")
			}
			recorder.AssertScriptConsumed()
		})
	}
}

func TestCollectAllPagesHardLimitPreventsFollowingWrite(t *testing.T) {
	// Scripted from the shared bound, not a literal: a recorder that walked
	// further than the production host adapter would let a business command
	// pass its tests and then fail its first real --page-all run.
	responses := make([]commandtest.Response, internalpagination.CollectAllHardPageBound)
	for index := range responses {
		responses[index] = commandtest.Respond(map[string]any{
			"items": []map[string]any{}, "has_more": true, "page_token": fmt.Sprintf("page-%d", index+1),
		})
	}
	type args struct{}
	type data struct{}
	definition := command.Definition[args, data]{
		Metadata: command.CommandMetadata{
			Service: "task", Command: "+business-hard-limit", Description: "Test complete read", Risk: command.RiskWrite,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{command.IdentityUser: {}}},
		},
		Hooks: command.Hooks[args, data]{
			Execute: func(ctx context.Context, commandContext command.CommandContext, _ *args) (command.Result[data], error) {
				if _, err := command.CollectAllPages[taskRecord](ctx, commandContext, command.GET("/open-apis/task/v2/tasks")); err != nil {
					return command.Result[data]{}, err
				}
				if _, err := command.CallJSON[map[string]any](ctx, commandContext, command.POST("/open-apis/task/v2/tasks")); err != nil {
					return command.Result[data]{}, err
				}
				return command.Success(data{}), nil
			},
		},
	}
	recorder := commandtest.New(t, responses...)
	_, err := commandtest.Execute(context.Background(), recorder, command.IdentityUser, definition, &args{})
	if err == nil || !strings.Contains(err.Error(), "hard limit") {
		t.Fatalf("hard-limit error = %v", err)
	}
	var internal *errs.InternalError
	if !errors.As(err, &internal) || internal.Subtype != errs.SubtypeQuotaExceeded {
		t.Fatalf("hard-limit typed error = %#v", err)
	}
	if requests := recorder.Requests(); len(requests) != internalpagination.CollectAllHardPageBound || requests[len(requests)-1].Method != "GET" {
		t.Fatalf("requests after incomplete read = %d, last=%#v", len(requests), requests[len(requests)-1])
	}
	recorder.AssertScriptConsumed()
}

func TestBestEffortScopeFailureMarksEveryItemFailed(t *testing.T) {
	want := errs.NewPermissionError(errs.SubtypeMissingScope, "owner scope is unavailable")
	recorder := commandtest.New(t, commandtest.Respond(map[string]any{
		"items": []map[string]any{
			{"task_id": "task_1", "owner_id": "user_1"},
			{"task_id": "task_2", "owner_id": "user_2"},
		},
		"has_more": false,
	}))
	recorder.SetScopeError(want)
	execution, err := commandtest.Execute(context.Background(), recorder, command.IdentityUser, taskAuditDefinition(), &taskAuditArgs{IncludeOwners: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(execution.Data.Items) != 2 || len(execution.Data.Failures) != 1 {
		t.Fatalf("execution = %#v", execution)
	}
	for _, item := range execution.Data.Items {
		if item.State != "failed" {
			t.Fatalf("items = %#v", execution.Data.Items)
		}
	}
	if execution.Data.Failures[0].Subtype != string(errs.SubtypeMissingScope) {
		t.Fatalf("failures = %#v", execution.Data.Failures)
	}
	if requests := recorder.Requests(); len(requests) != 1 || requests[0].Method != "GET" {
		t.Fatalf("requests = %#v", requests)
	}
	recorder.AssertScriptConsumed()
}

func TestMultiCallCommandRecordsTheFailedOwner(t *testing.T) {
	wantFailure := command.InvalidResponseErrorf("owner record is unavailable")
	recorder := commandtest.New(t,
		commandtest.Respond(map[string]any{
			"items": []map[string]any{{"task_id": "task_1", "owner_id": "user_1"}}, "has_more": true, "page_token": "next",
		}),
		commandtest.Respond(map[string]any{
			"items": []map[string]any{{"task_id": "task_2", "owner_id": "user_2"}}, "has_more": false,
		}),
		commandtest.Respond(map[string]any{"name": "Owner One"}),
		commandtest.Fail(wantFailure),
	)
	execution, err := commandtest.Execute(context.Background(), recorder, command.IdentityUser, taskAuditDefinition(), &taskAuditArgs{IncludeOwners: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(execution.Data.Items) != 2 || len(execution.Data.Failures) != 1 {
		t.Fatalf("execution = %#v", execution)
	}
	if execution.Data.Items[0].OwnerName != "Owner One" || execution.Data.Items[1].State != "failed" {
		t.Fatalf("items = %#v", execution.Data.Items)
	}
	if got := recorder.ScopeChecks(); !reflect.DeepEqual(got, [][]string{{"contact:user.base:readonly"}}) {
		t.Fatalf("scope checks = %#v", got)
	}
	requests := recorder.Requests()
	if len(requests) != 4 || requests[1].Query["page_token"] != "next" {
		t.Fatalf("requests = %#v", requests)
	}
	recorder.AssertScriptConsumed()
}

func TestConditionalScopeBranchMatchesDryRun(t *testing.T) {
	definition := memberListDefinition()
	recorder := commandtest.New(t,
		commandtest.Respond(map[string]any{"chat_id": "chat_1", "members": []string{}}),
		commandtest.Respond(map[string]any{"items": []string{"user_1", "user_2"}}),
	)
	args := &memberListArgs{ChatID: "chat_1", IncludeMembers: true}
	execution, err := commandtest.Execute(context.Background(), recorder, command.IdentityUser, definition, args)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(execution.Data.Members, []string{"user_1", "user_2"}) {
		t.Fatalf("members = %#v", execution.Data.Members)
	}
	if got := recorder.ScopeChecks(); !reflect.DeepEqual(got, [][]string{{"im:chat.members:read"}}) {
		t.Fatalf("scope checks = %#v", got)
	}
	preview, err := commandtest.Preview(context.Background(), recorder, command.IdentityUser, definition, args)
	if err != nil {
		t.Fatal(err)
	}
	recorder.AssertDryRunMatches(preview)
	recorder.AssertScriptConsumed()
}

func TestConditionalScopeFailureStopsBranch(t *testing.T) {
	want := errors.New("scope missing")
	recorder := commandtest.New(t, commandtest.Respond(map[string]any{"chat_id": "chat_1", "members": []string{}}))
	recorder.SetScopeError(want)
	_, err := commandtest.Execute(context.Background(), recorder, command.IdentityUser, memberListDefinition(), &memberListArgs{
		ChatID: "chat_1", IncludeMembers: true,
	})
	if !errors.Is(err, want) {
		t.Fatalf("branch error = %v", err)
	}
	if len(recorder.Requests()) != 1 {
		t.Fatalf("requests after scope failure = %#v", recorder.Requests())
	}
	recorder.AssertScriptConsumed()
}
