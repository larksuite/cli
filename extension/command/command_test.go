// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

import (
	"context"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type contractArgs struct {
	ID string `flag:"id" schema:"required;minLength=1" doc:"resource ID"`
}

type contractData struct {
	ID string `json:"id" schema:"required" doc:"resource ID"`
}

func TestDefineCopiesMutableMetadata(t *testing.T) {
	scopes := []string{"im:chat:read"}
	tips := []string{"Example"}
	definition := Definition[contractArgs, contractData]{
		Metadata: CommandMetadata{
			Service: "im", Command: "+contract-copy", Description: "Copy test", Risk: RiskRead, Tips: tips,
			Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{
				IdentityUser: {RequiredScopes: scopes},
			}},
		},
		Hooks: Hooks[contractArgs, contractData]{
			Execute: func(_ context.Context, _ CommandContext, args *contractArgs) (Result[contractData], error) {
				return Success(contractData{ID: args.ID}), nil
			},
		},
	}
	declared := Define(definition)
	scopes[0] = "changed"
	tips[0] = "changed"
	definition.Metadata.Authorization.Identities[IdentityUser] = IdentityAuthorization{}

	host := InspectCommand(declared)
	if got := host.Metadata.Authorization.Identities[IdentityUser].RequiredScopes; !reflect.DeepEqual(got, []string{"im:chat:read"}) {
		t.Fatalf("required scopes = %#v", got)
	}
	if !reflect.DeepEqual(host.Metadata.Tips, []string{"Example"}) {
		t.Fatalf("tips = %#v", host.Metadata.Tips)
	}
}

func TestDefineCopiesNestedJSONValues(t *testing.T) {
	defaultValue := map[string]any{"nested": []any{"original"}}
	shapeValue := map[string]any{"state": "original"}
	declaration := Define(Definition[contractArgs, contractData]{
		Metadata: CommandMetadata{
			Service: "im", Command: "+contract", Description: "Contract", Risk: RiskRead,
			Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{
				IdentityUser: {RequiredScopes: []string{"im:chat:read"}},
			}},
		},
		Input: InputDefinition{Fields: []InputField{{
			Name: "id", Default: InputDefault{Set: true, Value: defaultValue}, Shape: ConstShape{Value: shapeValue},
		}}},
		Hooks: Hooks[contractArgs, contractData]{Execute: func(context.Context, CommandContext, *contractArgs) (Result[contractData], error) {
			return Success(contractData{}), nil
		}},
	})
	defaultValue["nested"].([]any)[0] = "mutated"
	shapeValue["state"] = "mutated"

	definition := InspectCommand(declaration)
	gotDefault := definition.Input.Fields[0].Default.Value.(map[string]any)["nested"].([]any)[0]
	if gotDefault != "original" {
		t.Fatalf("captured default = %v", gotDefault)
	}
	gotShape := definition.Input.Fields[0].Shape.(ConstShape).Value.(map[string]any)["state"]
	if gotShape != "original" {
		t.Fatalf("captured shape = %v", gotShape)
	}
}

func TestRequestMethodsAndSameOriginValidation(t *testing.T) {
	requests := []Request{
		GET("/open-apis/im/v1/chats"), POST("/open-apis/im/v1/chats"),
		PUT("/open-apis/im/v1/chats/id"), PATCH("/open-apis/im/v1/chats/id"), DELETE("/open-apis/im/v1/chats/id"),
	}
	wantMethods := []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	for index, request := range requests {
		view := InspectRequest(request.Set("page_size", 20).Body(map[string]any{"name": "x"}))
		if view.Method != wantMethods[index] {
			t.Errorf("request %d method = %q", index, view.Method)
		}
		if err := ValidateRequestView(view); err != nil {
			t.Errorf("request %d validation: %v", index, err)
		}
	}
	for _, invalid := range []Request{
		GET("https://open.feishu.cn/open-apis/im/v1/chats"),
		GET("/open-apis/../auth/v3/tenant_access_token/internal"),
		GET("/internal/service"),
		GET("/open-apis/im/v1/chats?token=x"),
	} {
		if err := ValidateRequestView(InspectRequest(invalid)); err == nil {
			t.Errorf("request %#v passed same-origin validation", InspectRequest(invalid))
		}
	}
}

func TestCollectPagesUsesHostPolicyAndMetadata(t *testing.T) {
	responses := []map[string]any{
		{"items": []any{map[string]any{"id": "1"}}, "has_more": true, "page_token": "next"},
		{"items": []any{map[string]any{"id": "2"}}, "has_more": false},
	}
	var calls []RequestView
	ctx := NewCommandContext(ContextOptions{
		Identity: IdentityUser,
		CollectPages: func(_ context.Context, request Request, all bool) ([]map[string]any, HostPagination, error) {
			if all {
				t.Fatal("CollectPages forced full pagination")
			}
			calls = append(calls, InspectRequest(request), InspectRequest(request.Set("page_token", "next")))
			return responses, HostPagination{Complete: true, Pages: 2}, nil
		},
	})
	page, err := CollectPages[contractData](context.Background(), ctx, GET("/open-apis/im/v1/chats"))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Pages() != 2 || !page.Complete() || page.NextToken() != "" {
		t.Fatalf("page = %#v, pages=%d complete=%v next=%q", page.Items, page.Pages(), page.Complete(), page.NextToken())
	}
	if got := calls[1].Query["page_token"]; got != "next" {
		t.Fatalf("second request page_token = %#v", got)
	}
	result := Success(page)
	host := hostResult(result)
	if host.Pagination == nil || host.Pagination.Pages != 2 || host.Pagination.Items != 2 || !host.Pagination.Complete {
		t.Fatalf("host pagination = %#v", host.Pagination)
	}
}

func TestPageResultCountsFilteredItems(t *testing.T) {
	page := Page[contractData]{
		Items: []contractData{{ID: "one"}, {ID: "two"}},
		meta:  &paginationMeta{Complete: true, Pages: 1, Items: 2},
	}
	page.Items = page.Items[:1]
	result := hostResult(Success(page))
	if result.Pagination == nil || result.Pagination.Items != 1 {
		t.Fatalf("filtered pagination = %#v", result.Pagination)
	}
}

func TestDryRunPreventsRequestsAndScopeChecks(t *testing.T) {
	calls := 0
	ctx := NewCommandContext(ContextOptions{
		Identity: IdentityUser,
		DryRun:   true,
		CallJSON: func(context.Context, Request) (map[string]any, error) {
			calls++
			return nil, nil
		},
		PreflightScopes: func(...string) error {
			calls++
			return nil
		},
	})
	if _, err := CallJSON[contractData](context.Background(), ctx, GET("/open-apis/im/v1/chats/id")); err == nil {
		t.Fatal("CallJSON during dry-run succeeded")
	}
	if err := PreflightScopes(ctx, "im:chat:read"); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("host callbacks during dry-run = %d", calls)
	}
}

func TestPublicPackageHasNoForbiddenImports(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if importPath == "github.com/larksuite/cli/cmd" || importPath == "github.com/larksuite/cli/shortcuts/common" || strings.Contains(importPath, "/internal/") {
				t.Errorf("%s imports forbidden package %q", file, importPath)
			}
		}
	}
}
