// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"io"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

type contractArgs struct {
	ID string `flag:"id" schema:"required;minLength=1" doc:"resource ID"`
}

type contractData struct {
	ID string `json:"id" schema:"required" doc:"resource ID"`
}

func TestDefineCopiesMutableMetadata(t *testing.T) {
	scopes := []string{"im:chat:read"}
	definition := Definition[contractArgs, contractData]{
		Metadata: CommandMetadata{
			Service: "im", Command: "+contract-copy", Description: "Copy test", Risk: RiskRead,
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
	definition.Metadata.Authorization.Identities[IdentityUser] = IdentityAuthorization{}

	host := InspectCommand(declared)
	if got := host.Metadata.Authorization.Identities[IdentityUser].RequiredScopes; !reflect.DeepEqual(got, []string{"im:chat:read"}) {
		t.Fatalf("required scopes = %#v", got)
	}
}

func TestHostHooksRejectMismatchedErasedValues(t *testing.T) {
	declaration := Define(Definition[contractArgs, contractData]{
		Metadata: CommandMetadata{
			Service: "im", Command: "+host-types", Description: "Host type checks", Risk: RiskRead,
			Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{IdentityUser: {}}},
		},
		Hooks: Hooks[contractArgs, contractData]{
			Normalize: func(context.Context, CommandContext, *contractArgs) error { return nil },
			Validate:  func(context.Context, CommandContext, *contractArgs) error { return nil },
			DryRun:    func(context.Context, CommandContext, *contractArgs) *DryRun { return NewDryRun() },
			Execute: func(context.Context, CommandContext, *contractArgs) (Result[contractData], error) {
				return Success(contractData{}), nil
			},
			PrettyRenderer: func(io.Writer, contractData) error { return nil },
		},
	})
	host := InspectCommand(declaration)
	commandContext := NewCommandContext(ContextOptions{})
	wrong := &struct{}{}
	assertInternal := func(name string, err error) {
		t.Helper()
		var internal *errs.InternalError
		if !errors.As(err, &internal) || internal.Subtype != errs.SubtypeUnknown {
			t.Fatalf("%s error = %#v", name, err)
		}
	}

	assertInternal("Normalize", host.Hooks.Normalize(context.Background(), commandContext, wrong))
	assertInternal("Validate", host.Hooks.Validate(context.Background(), commandContext, wrong))
	if dryRun := host.Hooks.DryRun(context.Background(), commandContext, wrong); dryRun != nil {
		t.Fatalf("DryRun = %#v", dryRun)
	}
	_, err := host.Hooks.Execute(context.Background(), commandContext, wrong)
	assertInternal("Execute", err)
	assertInternal("renderer", host.Hooks.Renderers["pretty"](io.Discard, wrong))
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

func TestDefineCopiesShapePointersAndTypedJSONContainers(t *testing.T) {
	minLength := 1
	maxLength := 64
	minItems := 1
	maxItems := 20
	minimum := int64(0)
	maximum := 100.0
	defaultValue := map[string][]string{"ids": {"original"}}
	declaration := Define(Definition[contractArgs, contractData]{
		Metadata: CommandMetadata{
			Service: "im", Command: "+contract-deep-copy", Description: "Deep copy", Risk: RiskRead,
			Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{IdentityUser: {}}},
		},
		Input: InputDefinition{Fields: []InputField{{
			Name:    "id",
			Shape:   StringShape{MinLength: &minLength, MaxLength: &maxLength},
			Default: InputDefault{Set: true, Value: defaultValue},
		}}},
		Output: OutputDefinition{
			Data: DataDefinition{
				Shape: ArrayShape{
					Items: IntegerShape{Minimum: &minimum}, MinItems: &minItems, MaxItems: &maxItems,
				},
				Overrides: []DataField{{Path: "/score", Shape: NumberShape{Maximum: &maximum}}},
			},
		},
		Hooks: Hooks[contractArgs, contractData]{Execute: func(context.Context, CommandContext, *contractArgs) (Result[contractData], error) {
			return Success(contractData{}), nil
		}},
	})

	minLength = 2
	maxLength = 32
	minItems = 2
	maxItems = 10
	minimum = 1
	maximum = 50
	defaultValue["ids"][0] = "mutated"

	first := InspectCommand(declaration)
	assertCopiedDefinitionValues(t, first)
	*first.Input.Fields[0].Shape.(StringShape).MinLength = 9
	first.Input.Fields[0].Default.Value.(map[string][]string)["ids"][0] = "inspected"

	second := InspectCommand(declaration)
	assertCopiedDefinitionValues(t, second)
}

func assertCopiedDefinitionValues(t *testing.T, definition HostDefinition) {
	t.Helper()
	stringShape := definition.Input.Fields[0].Shape.(StringShape)
	if *stringShape.MinLength != 1 || *stringShape.MaxLength != 64 {
		t.Fatalf("string constraints = %#v", stringShape)
	}
	arrayShape := definition.Output.Data.Shape.(ArrayShape)
	integerShape := arrayShape.Items.(IntegerShape)
	if *arrayShape.MinItems != 1 || *arrayShape.MaxItems != 20 || *integerShape.Minimum != 0 {
		t.Fatalf("array constraints = %#v, item constraints = %#v", arrayShape, integerShape)
	}
	numberShape := definition.Output.Data.Overrides[0].Shape.(NumberShape)
	if *numberShape.Maximum != 100 {
		t.Fatalf("number constraints = %#v", numberShape)
	}
	defaultValue := definition.Input.Fields[0].Default.Value.(map[string][]string)
	if defaultValue["ids"][0] != "original" {
		t.Fatalf("default value = %#v", defaultValue)
	}
}

func TestRequestCopiesNestedQueryAndBodyValues(t *testing.T) {
	query := map[string][]string{"ids": {"original"}}
	shared := []string{"first", "second"}
	body := map[string]any{"items": []map[string]string{{"id": "original"}}}
	request := GET("/open-apis/im/v1/chats").
		Params(map[string]any{"filter": query, "all": shared, "first": shared[:1]}).
		Body(body)

	query["ids"][0] = "mutated"
	body["items"].([]map[string]string)[0]["id"] = "mutated"
	first := InspectRequest(request)
	if got := first.Query["filter"].(map[string][]string)["ids"][0]; got != "original" {
		t.Fatalf("query value = %q", got)
	}
	if got := len(first.Query["all"].([]string)); got != 2 {
		t.Fatalf("full shared slice length = %d", got)
	}
	if got := len(first.Query["first"].([]string)); got != 1 {
		t.Fatalf("short shared slice length = %d", got)
	}
	if got := first.Body.(map[string]any)["items"].([]map[string]string)[0]["id"]; got != "original" {
		t.Fatalf("body value = %q", got)
	}

	first.Query["filter"].(map[string][]string)["ids"][0] = "inspected"
	first.Body.(map[string]any)["items"].([]map[string]string)[0]["id"] = "inspected"
	second := InspectRequest(request)
	if got := second.Query["filter"].(map[string][]string)["ids"][0]; got != "original" {
		t.Fatalf("second query value = %q", got)
	}
	if got := second.Body.(map[string]any)["items"].([]map[string]string)[0]["id"]; got != "original" {
		t.Fatalf("second body value = %q", got)
	}
}

func TestPublicOutputDefinitionExcludesFileArtifacts(t *testing.T) {
	if _, present := reflect.TypeFor[OutputDefinition]().FieldByName("Artifacts"); present {
		t.Fatal("OutputDefinition exposes file artifacts")
	}
}

func TestValueShapeClosedSet(t *testing.T) {
	StringShape{}.valueShape()
	BooleanShape{}.valueShape()
	IntegerShape{}.valueShape()
	NumberShape{}.valueShape()
	NullShape{}.valueShape()
	ConstShape{}.valueShape()
	ArrayShape{}.valueShape()
	ObjectShape{}.valueShape()
	OneOfShape{}.valueShape()
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

func TestDryRunBuilderSupportsEveryRequestMethodAndModifier(t *testing.T) {
	dryRun := NewDryRun().
		GET("/open-apis/im/v1/chats").
		Set("page_size", 20).
		Params(map[string]any{"page_size": 50}).
		POST("/open-apis/im/v1/chats").
		Body(map[string]any{"name": "example"}).
		PUT("/open-apis/im/v1/chats/chat_1").
		PATCH("/open-apis/im/v1/chats/chat_1").
		DELETE("/open-apis/im/v1/chats/chat_1")
	view := InspectDryRun(dryRun)
	if len(view.Requests) != 5 {
		t.Fatalf("requests = %#v", view.Requests)
	}
	wantMethods := []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	for index, request := range view.Requests {
		if request.Method != wantMethods[index] {
			t.Errorf("request %d method = %q", index, request.Method)
		}
	}
	if view.Requests[0].Query["page_size"] != 50 {
		t.Fatalf("GET query = %#v", view.Requests[0].Query)
	}
	if view.Requests[1].Body == nil {
		t.Fatal("POST body is nil")
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
		CollectPages: func(context.Context, Request, bool) ([]map[string]any, HostPagination, error) {
			calls++
			return nil, HostPagination{}, nil
		},
	})
	if ctx.Identity() != IdentityUser {
		t.Fatalf("identity = %q", ctx.Identity())
	}
	if _, err := CallJSON[contractData](context.Background(), ctx, GET("/open-apis/im/v1/chats/id")); err == nil {
		t.Fatal("CallJSON during dry-run succeeded")
	}
	if err := PreflightScopes(ctx, "im:chat:read"); err != nil {
		t.Fatal(err)
	}
	if _, err := CollectPages[contractData](context.Background(), ctx, GET("/open-apis/im/v1/chats")); err == nil {
		t.Fatal("CollectPages during dry-run succeeded")
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

// PathSegment must keep one user value inside one path segment: separators,
// dot sequences, and query metacharacters cannot change the request target,
// and the escaped form must pass the same-origin validation.
func TestPathSegmentNeutralizesSeparatorsAndTraversal(t *testing.T) {
	cases := map[string]string{
		"oc_plain":      "oc_plain",
		"a/b":           "a%2Fb",
		"..":            "..", // url.PathEscape keeps dots; the ../ traversal form below is what must break
		"../../outside": "..%2F..%2Foutside",
		"a?x=1":         "a%3Fx=1",
		"a#frag":        "a%23frag",
	}
	for input, want := range cases {
		if got := PathSegment(input); got != want {
			t.Errorf("PathSegment(%q) = %q, want %q", input, got, want)
		}
	}
	// Traversal fails validation in both spellings: the validator decodes
	// percent-encoding before the canonical check, so escaping cannot smuggle
	// a dot sequence through, and raw concatenation is rejected outright.
	for _, spelling := range []string{PathSegment("../../etc"), "../../etc"} {
		request := GET("/open-apis/im/v1/chats/" + spelling)
		if err := ValidateRequestView(InspectRequest(request)); err == nil {
			t.Fatalf("traversal spelling %q must fail validation", spelling)
		}
	}
	// A regular escaped ID stays valid.
	ordinary := GET("/open-apis/im/v1/chats/" + PathSegment("oc_a b+c"))
	if err := ValidateRequestView(InspectRequest(ordinary)); err != nil {
		t.Fatalf("escaped ordinary id should validate: %v", err)
	}
}

// pageContext returns a context whose host callback replays the given pages.
func pageContext(pages []map[string]any) CommandContext {
	return NewCommandContext(ContextOptions{
		Identity: IdentityUser,
		CollectPages: func(_ context.Context, _ Request, _ bool) ([]map[string]any, HostPagination, error) {
			return pages, HostPagination{Complete: true, Pages: len(pages)}, nil
		},
	})
}

// Upstream list endpoints spell their array field differently (items,
// records, files, ...). Each page's single top-level array must normalize
// into Page.Items regardless of its name.
func TestCollectPagesNormalizesAnyTopLevelArrayField(t *testing.T) {
	for _, field := range []string{"items", "records", "files"} {
		pages := []map[string]any{
			{field: []any{map[string]any{"id": "1"}, map[string]any{"id": "2"}}, "has_more": false},
		}
		page, err := CollectPages[contractData](context.Background(), pageContext(pages), GET("/open-apis/x/v1/list"))
		if err != nil {
			t.Fatalf("field %q: %v", field, err)
		}
		if len(page.Items) != 2 {
			t.Fatalf("field %q: items = %d, want 2", field, len(page.Items))
		}
	}
}

// Zero or multiple top-level arrays must fail closed. Silently decoding
// nothing would let CollectAllPages report an empty-but-complete set and
// downstream writes would run against it.
func TestCollectPagesRejectsAmbiguousPageShapes(t *testing.T) {
	cases := map[string][]map[string]any{
		"no array":       {{"has_more": false, "page_token": ""}},
		"two arrays":     {{"items": []any{}, "files": []any{}, "has_more": false}},
		"null-only page": {{"items": nil, "has_more": false}},
	}
	for name, pages := range cases {
		if _, err := CollectPages[contractData](context.Background(), pageContext(pages), GET("/open-apis/x/v1/list")); err == nil {
			t.Errorf("%s: expected typed invalid-response error, got nil", name)
		}
		if _, err := CollectAllPages[contractData](context.Background(), pageContext(pages), GET("/open-apis/x/v1/list")); err == nil {
			t.Errorf("%s: CollectAllPages must not report an empty complete set", name)
		}
	}
}
