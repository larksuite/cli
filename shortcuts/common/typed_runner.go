// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

func runTypedShortcut(cmdFactory *cmdutil.Factory, runtime *RuntimeContext, shortcut *Shortcut) error {
	command := shortcut.typed
	if command == nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "typed runner received a legacy shortcut")
	}
	if err := validateTypedStdinInputs(runtime, command); err != nil {
		return err
	}
	if err := resolveInputFlags(runtime, shortcut.Flags); err != nil {
		return attributeAliasValidationError(runtime, err)
	}
	bound, err := bindTypedArgs(runtime, command)
	if err != nil {
		return attributeAliasValidationError(runtime, err)
	}
	commandContext := typedCommandContext{runtime: runtime, command: command}
	if command.hooks.normalize != nil {
		if err := command.hooks.normalize(runtime.ctx, commandContext, bound.value); err != nil {
			return attributeAliasValidationError(runtime, err)
		}
	}
	if err := validateCompiledRelations(command, bound.value, bound.provided, typedStageAfterPrepare); err != nil {
		return err
	}
	if command.hooks.validate != nil {
		if err := command.hooks.validate(runtime.ctx, commandContext, bound.value); err != nil {
			return attributeAliasValidationError(runtime, err)
		}
	}
	if runtime.Bool("dry-run") {
		if command.hooks.dryRun == nil {
			return ValidationErrorf("--dry-run is not supported for %s %s", shortcut.Service, shortcut.Command).WithParam("--dry-run")
		}
		preview, err := command.hooks.dryRun(runtime.ctx, commandContext, bound.value)
		if err != nil {
			return attributeAliasValidationError(runtime, err)
		}
		if preview != nil {
			preview.Context(runtime.Config.AppID, runtime.UserOpenId())
		}
		return cmdutil.WriteDryRun(preview, cmdutil.DryRunOutputOptions{Format: runtime.Format, JqExpr: runtime.JqExpr, CommandPath: runtime.Cmd.CommandPath(), Identity: runtime.As(), Out: cmdFactory.IOStreams.Out, ErrOut: cmdFactory.IOStreams.ErrOut})
	}
	if shortcut.Risk == string(typedRiskHighRiskWrite) && !runtime.Bool("yes") {
		return cmdutil.RequireConfirmation(shortcut.Service + " " + shortcut.Command)
	}
	result, err := command.hooks.execute(runtime.ctx, commandContext, bound.value)
	if err != nil {
		if result.outcome != "" || result.meta != nil {
			return errs.NewInternalError(errs.SubtypeUnknown, "typed Execute returned both Result and error").WithCause(err)
		}
		return attributeAliasValidationError(runtime, err)
	}
	if err := validateTypedResultProtocol(command, result); err != nil {
		return err
	}
	return emitTypedResult(runtime, command, result)
}

func validateTypedStdinInputs(runtime *RuntimeContext, command *compiledCommand) error {
	var selected []string
	for _, field := range command.fields {
		supportsStdin := false
		for _, source := range field.cli.ValueSources {
			if source == typedSourceStdin {
				supportsStdin = true
				break
			}
		}
		if !supportsStdin {
			continue
		}
		names := []string{field.name}
		for _, alias := range field.cli.Aliases {
			names = append(names, alias.Name)
		}
		for _, name := range names {
			flag := runtime.Cmd.Flags().Lookup(name)
			if flag == nil || !flag.Changed {
				continue
			}
			value, err := runtime.Cmd.Flags().GetString(name)
			if err != nil {
				return errs.NewInternalError(errs.SubtypeUnknown, "failed to inspect stdin source --%s", name).WithCause(err)
			}
			if value == "-" {
				selected = append(selected, field.name)
				break
			}
		}
	}
	if len(selected) > 1 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "at most one parameter may read stdin in one invocation; provide the other values through their remaining declared sources").WithParam("--" + selected[1])
	}
	return nil
}

func emitTypedResult(runtime *RuntimeContext, command *compiledCommand, result compiledResult) error {
	if result.outcome == "" {
		return errs.NewInternalError(errs.SubtypeUnknown, "typed Execute returned a Result without Outcome")
	}
	var pretty output.PrettyRenderer
	if runtime.Format == "pretty" {
		if renderer := command.hooks.renderers["pretty"]; renderer != nil {
			pretty = func(w io.Writer, _ bool) error { return renderer(w, result.data) }
		}
	}
	format := runtime.Format
	if command.output.Mode == typedOutputFixedJSON {
		// Compatibility for Legacy hooks that used RuntimeContext.Out: the
		// injected --format flag existed but output was always JSON.
		format = ""
	}
	options := output.EmitOptions{Format: format, Raw: command.output.DisableHTMLEscaping, JQ: runtime.JqExpr, Pretty: pretty, Meta: outputMetaFromTyped(result.meta)}
	switch result.outcome {
	case typedOutcomeSuccess:
		runtime.handleEmitterError(runtime.newEmitter().Success(result.data, options))
		return runtime.outputErr
	default:
		return errs.NewInternalError(errs.SubtypeUnknown, "typed Execute returned invalid Outcome %q", result.outcome)
	}
}

func outputMetaFromTyped(meta *typedResultMeta) *output.Meta {
	if meta == nil {
		return nil
	}
	converted := &output.Meta{}
	if meta.Pagination != nil {
		pagination := *meta.Pagination
		converted.Pagination = &pagination
	}
	return converted
}

type typedCommandContext struct {
	runtime *RuntimeContext
	command *compiledCommand
}

func (c typedCommandContext) Identity() typedIdentity               { return typedIdentity(c.runtime.As()) }
func (c typedCommandContext) Config() core.CliConfig                { return *c.runtime.Config }
func (c typedCommandContext) APIClient() (*client.APIClient, error) { return c.runtime.getAPIClient() }
func (c typedCommandContext) FileIO() fileio.FileIO                 { return c.runtime.FileIO() }
func (c typedCommandContext) InputResolvedFromSource(param string) bool {
	if c.runtime.InputResolvedFromSource(param) {
		return true
	}
	fieldIndex, ok := c.command.fieldByName[param]
	if !ok {
		return false
	}
	for _, alias := range c.command.fields[fieldIndex].cli.Aliases {
		if alias.Mode == typedAliasIndependent && c.runtime.InputResolvedFromSource(alias.Name) {
			return true
		}
	}
	return false
}
func (c typedCommandContext) ValidatePath(path string) error { return c.runtime.ValidatePath(path) }
func (c typedCommandContext) ResolveSavePath(path string) (string, error) {
	return c.runtime.ResolveSavePath(path)
}
func (c typedCommandContext) Stderr() io.Writer { return c.runtime.IO().ErrOut }
func (c typedCommandContext) StartSpinner(label string) func() {
	return c.runtime.StartSpinner(label)
}
func (c typedCommandContext) PresentError(err error) error { return c.runtime.PresentError(err) }
func (c typedCommandContext) IsDryRun() bool               { return c.runtime != nil && c.runtime.Bool("dry-run") }
func (c typedCommandContext) PaginationOptions() (typedPaginationOptions, error) {
	values, err := pageAllValues(c.runtime)
	if err != nil {
		return typedPaginationOptions{}, err
	}
	return typedPaginationOptions{All: values.enabled, MaxPages: values.maxPages, Delay: values.delay}, nil
}
func (c typedCommandContext) typedCommandPath() string {
	if c.runtime == nil || c.runtime.Cmd == nil {
		return ""
	}
	return strings.TrimPrefix(c.runtime.Cmd.CommandPath(), "lark ")
}
func (c typedCommandContext) RequireConditionalScopes(scopes ...string) error {
	identity := c.Identity()
	authorization, ok := c.command.metadata.Authorization.Identities[identity]
	if !ok {
		return errs.NewInternalError(errs.SubtypeUnknown, "typed shortcut %s %s has no authorization contract for identity %q", c.command.metadata.Service, c.command.metadata.Command, identity)
	}
	declared := make(map[string]struct{})
	for _, conditional := range authorization.ConditionalScopes {
		for _, scope := range conditional.Scopes {
			declared[scope] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(scopes))
	requested := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if _, duplicate := seen[scope]; duplicate {
			continue
		}
		seen[scope] = struct{}{}
		if _, ok := declared[scope]; !ok {
			return errs.NewInternalError(errs.SubtypeUnknown, "typed shortcut %s %s requested undeclared conditional scope %q for identity %q", c.command.metadata.Service, c.command.metadata.Command, scope, identity)
		}
		requested = append(requested, scope)
	}
	return c.runtime.EnsureScopes(requested)
}

var _ typedRuntimeContext = typedCommandContext{}
