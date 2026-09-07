// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package commandhost adapts the public command extension contract to Typed Shortcuts.
package commandhost

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/commandbridge"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts"
	"github.com/larksuite/cli/shortcuts/common"
)

// CompileSets validates and compiles a complete external contribution without registration.
func CompileSets(sets []command.Set) ([]common.Shortcut, error) {
	sets = command.CloneSets(sets)
	if len(sets) == 0 {
		return nil, nil
	}

	builtins := shortcuts.AllShortcuts()
	paths := make(map[string]string, len(builtins))
	for _, shortcut := range builtins {
		paths[shortcut.Service+" "+shortcut.Command] = "built-in command"
	}
	existingDomains := businessDomains()

	compiled := make([]common.Shortcut, 0)
	for setIndex, set := range sets {
		domain := command.InspectDomain(set.Domain)
		if err := validateDomain(domain, existingDomains); err != nil {
			return nil, fmt.Errorf("command set %d: %w", setIndex+1, err)
		}
		if len(set.Commands) == 0 {
			return nil, fmt.Errorf("command set %d for domain %q has no commands", setIndex+1, domain.Name)
		}
		for commandIndex, declaration := range set.Commands {
			definition := command.InspectCommand(declaration)
			if string(definition.Metadata.Service) != domain.Name {
				return nil, fmt.Errorf("command set %d command %d: Metadata.Service %q does not match domain %q",
					setIndex+1, commandIndex+1, definition.Metadata.Service, domain.Name)
			}
			shortcutPath := string(definition.Metadata.Service) + " " + definition.Metadata.Command
			if owner, duplicate := paths[shortcutPath]; duplicate {
				return nil, fmt.Errorf("command set %d command %d: command path %q conflicts with %s",
					setIndex+1, commandIndex+1, shortcutPath, owner)
			}
			shortcut, err := compileCommand(definition)
			if err != nil {
				return nil, fmt.Errorf("command set %d command %d (%s): %w", setIndex+1, commandIndex+1, shortcutPath, err)
			}
			paths[shortcutPath] = fmt.Sprintf("command set %d command %d", setIndex+1, commandIndex+1)
			compiled = append(compiled, shortcut)
		}
	}
	return compiled, nil
}

// ValidateDeclaration compiles one declaration through the production compiler
// and discards the result. A business test harness calls it so a wrong tag,
// Shape or relation fails in the unit test rather than at CLI startup: executing
// hooks directly exercises none of the compiler's contract checks, which is the
// gap that let a green test ship a command that cannot mount.
//
// Domain existence and path collisions are deliberately not checked here. Those
// are properties of a whole contribution and belong to CompileSets, which runs
// during build.
func ValidateDeclaration(declaration command.Command) error {
	_, err := CompileDeclaration(declaration)
	return err
}

// CompileDeclaration compiles one declaration into a mountable shortcut without
// the whole-contribution checks CompileSets performs. Host-side callers that
// need a compiled command outside a command set use it so they exercise the
// production compiler rather than a parallel construction path.
func CompileDeclaration(declaration command.Command) (common.Shortcut, error) {
	return compileCommand(command.InspectCommand(declaration))
}

// businessDomains reports the domains a command set may extend. It reads the
// service registry rather than deriving domains from shortcuts.AllShortcuts:
// approval, attendance and mindnotes ship only typed and raw API commands, and
// a shortcut-derived set would reject them as non-existent.
func businessDomains() map[string]struct{} {
	names := registry.AllServiceNames()
	domains := make(map[string]struct{}, len(names))
	for _, name := range names {
		domains[name] = struct{}{}
	}
	return domains
}

func validateDomain(domain command.HostDomain, existing map[string]struct{}) error {
	name := strings.TrimSpace(domain.Name)
	if name == "" || name != domain.Name {
		return fmt.Errorf("domain name must be non-empty and trimmed")
	}
	if _, ok := existing[name]; !ok {
		return fmt.Errorf("ExtendDomain target %q does not exist", name)
	}
	return nil
}

func compileCommand(definition command.HostDefinition) (common.Shortcut, error) {
	hooks := convertHooks(definition)
	hooks.NewArgs = definition.NewArgs
	return common.CompileCommandDefinition(commandbridge.Definition{
		Metadata:   definition.Metadata,
		Input:      definition.Input,
		Output:     definition.Output,
		ArgsType:   definition.ArgsType,
		DataType:   definition.DataType,
		Hooks:      hooks,
		PageOutput: definition.PageOutput,
	}, commandbridge.Access{})
}

func convertHooks(definition command.HostDefinition) commandbridge.Hooks {
	hooks := definition.Hooks
	return commandbridge.Hooks{
		Normalize: adaptHook(hooks.Normalize),
		Validate:  adaptHook(hooks.Validate),
		DryRun:    adaptDryRunHook(hooks.DryRun),
		Execute:   adaptExecuteHook(definition),
		Renderers: cloneRenderers(hooks.Renderers),
	}
}

func adaptHook(hook func(context.Context, command.CommandContext, any) error) func(context.Context, commandbridge.RuntimeContext, any) error {
	if hook == nil {
		return nil
	}
	return func(ctx context.Context, host commandbridge.RuntimeContext, args any) error {
		return hook(ctx, inputStageContext(host), args)
	}
}

func adaptDryRunHook(hook func(context.Context, command.CommandContext, any) *command.DryRun) func(context.Context, commandbridge.RuntimeContext, any) (any, error) {
	if hook == nil {
		return nil
	}
	return func(ctx context.Context, host commandbridge.RuntimeContext, args any) (any, error) {
		return convertDryRun(hook(ctx, publicContext(host), args))
	}
}

func adaptExecuteHook(definition command.HostDefinition) func(context.Context, commandbridge.RuntimeContext, any) (commandbridge.Result, error) {
	hook := definition.Hooks.Execute
	if hook == nil {
		return nil
	}
	return func(ctx context.Context, host commandbridge.RuntimeContext, args any) (commandbridge.Result, error) {
		result, err := hook(ctx, publicContext(host), args)
		// Check the extension-facing protocol at the extension boundary, where
		// the diagnostic can name the public constructor. commandtest runs the
		// same check, so the two surfaces cannot drift apart.
		if err == nil {
			if invalid := command.ValidateHostResult(definition, result); invalid != nil {
				return commandbridge.Result{}, invalid
			}
		}
		return commandbridge.Result{Data: result.Data, Outcome: result.Outcome, Pagination: result.Pagination}, err
	}
}

func cloneRenderers(renderers map[string]func(io.Writer, any) error) map[string]func(io.Writer, any) error {
	if len(renderers) == 0 {
		return nil
	}
	cloned := make(map[string]func(io.Writer, any) error, len(renderers))
	for name, renderer := range renderers {
		cloned[name] = renderer
	}
	return cloned
}

// inputStageContext serves Normalize and Validate. Both run before the
// high-risk confirmation gate, so they get the same context minus the network:
// otherwise a high-risk command could reach the API from Validate and leave
// remote side effects behind before the user was ever asked to confirm.
func inputStageContext(host commandbridge.RuntimeContext) command.CommandContext {
	return command.NewCommandContext(command.ContextOptions{
		Identity:        host.Identity(),
		DryRun:          host.IsDryRun(),
		InputStage:      true,
		PreflightScopes: host.RequireConditionalScopes,
	})
}

// commandPages is the accumulator the public command contract needs: it keeps
// each page undecoded, because the business command's own item type lives in
// its module and Page[T] decodes there.
type commandPages struct{ data []map[string]any }

func (c *commandPages) AddPage(page map[string]any) error {
	c.data = append(c.data, page)
	return nil
}

func publicContext(host commandbridge.RuntimeContext) command.CommandContext {
	return command.NewCommandContext(command.ContextOptions{
		Identity: host.Identity(),
		DryRun:   host.IsDryRun(),
		CallJSON: func(ctx context.Context, request command.Request) (map[string]any, error) {
			view := command.InspectRequest(request)
			return common.DoHostedAPIJSON(ctx, host, view.Method, view.Path, queryParams(view.Query), view.Body, commandbridge.Access{})
		},
		Download: func(ctx context.Context, request command.Request, target command.FileTarget, options command.DownloadOptions) (command.Artifact, error) {
			return downloadCommand(ctx, host, request, target, options)
		},
		DownloadURL: func(ctx context.Context, rawURL string, target command.FileTarget, options command.DownloadOptions) (command.Artifact, error) {
			return downloadURLCommand(ctx, host, rawURL, target, options)
		},
		PreflightScopes: host.RequireConditionalScopes,
		CollectPages: func(ctx context.Context, request command.Request, all bool) ([]map[string]any, command.HostPagination, error) {
			view := command.InspectRequest(request)
			if err := command.ValidateRequestView(view); err != nil {
				return nil, command.HostPagination{}, err
			}
			pages := &commandPages{}
			meta, err := common.CollectHostedPages(ctx, host, common.PageRequest{
				Method: view.Method, Path: view.Path, Params: projectedQuery(view.Query), Body: view.Body,
			}, all, pages, commandbridge.Access{})
			pagination := command.HostPagination{
				Complete: meta.Complete, Pages: meta.Pages,
				NextToken: meta.NextToken,
			}
			return pages.data, pagination, err
		},
	})
}

// canonicalQuery is the single projection from a business command's declared
// query onto the wire. Every consumer derives from it -- the live single
// request, the dry-run preview, downloads and the pagination walk -- so a
// preview can never describe a request the runtime would not send. Declaring
// it once is what keeps fmt.Sprint conversion and nil dropping from differing
// between paths.
func canonicalQuery(query map[string]any) map[string][]string {
	canonical := make(map[string][]string, len(query))
	for name, value := range query {
		if values := queryValues(value); len(values) > 0 {
			canonical[name] = values
		}
	}
	return canonical
}

func queryParams(query map[string]any) larkcore.QueryParams {
	return larkcore.QueryParams(canonicalQuery(query))
}

// projectedQuery renders the canonical projection for consumers whose parameter
// type is map[string]any. A single value stays scalar and repeated values stay a
// list; both carry exactly the strings the live request sends.
func projectedQuery(query map[string]any) map[string]any {
	canonical := canonicalQuery(query)
	if len(canonical) == 0 {
		return nil
	}
	projected := make(map[string]any, len(canonical))
	for name, values := range canonical {
		if len(values) == 1 {
			projected[name] = values[0]
			continue
		}
		projected[name] = values
	}
	return projected
}

func queryValues(value any) []string {
	value = derefQueryValue(value)
	if value == nil {
		return nil
	}
	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Array && reflected.Kind() != reflect.Slice {
		return []string{fmt.Sprint(value)}
	}
	values := make([]string, 0, reflected.Len())
	for index := 0; index < reflected.Len(); index++ {
		item := derefQueryValue(reflected.Index(index).Interface())
		if item != nil {
			values = append(values, fmt.Sprint(item))
		}
	}
	return values
}

func derefQueryValue(value any) any {
	if value == nil {
		return nil
	}
	reflected := reflect.ValueOf(value)
	for reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface {
		if reflected.IsNil() {
			return nil
		}
		reflected = reflected.Elem()
	}
	return reflected.Interface()
}

func convertDryRun(preview *command.DryRun) (*common.DryRunAPI, error) {
	if preview == nil {
		return nil, nil
	}
	view := command.InspectDryRun(preview)
	converted := common.NewDryRunAPI()
	if view.Description != "" {
		converted.Desc(view.Description)
	}
	for index, request := range view.Requests {
		if err := command.ValidateRequestView(request); err != nil {
			return nil, fmt.Errorf("dry-run request %d: %w", index+1, err)
		}
		switch request.Method {
		case "GET":
			converted.GET(request.Path)
		case "POST":
			converted.POST(request.Path)
		case "PUT":
			converted.PUT(request.Path)
		case "PATCH":
			converted.PATCH(request.Path)
		case "DELETE":
			converted.DELETE(request.Path)
		}
		if params := projectedQuery(request.Query); len(params) > 0 {
			converted.Params(params)
		}
		if request.Body != nil {
			converted.Body(request.Body)
		}
		if request.Description != "" {
			converted.Desc(request.Description)
		}
	}
	for index, file := range view.Files {
		if file.Name == "" || file.Name != strings.TrimSpace(file.Name) {
			return nil, command.ValidationErrorf("dry-run file %d: target name must be non-empty and trimmed", index+1)
		}
		policy := file.IfExists
		if policy == "" {
			policy = command.IfExistsFail
		}
		if policy != command.IfExistsFail && policy != command.IfExistsOverwrite {
			return nil, command.ValidationErrorf("dry-run file %d: unsupported conflict policy %q", index+1, policy)
		}
		converted.File(cmdutil.DryRunFileIntent{Name: file.Name, IfExists: string(policy), Content: file.Content})
	}
	return converted, nil
}
