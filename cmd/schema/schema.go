// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"context"
	"errors"
	identitypkg "github.com/larksuite/cli/internal/identity"
	"github.com/larksuite/cli/internal/meta"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/schema"
	"github.com/spf13/cobra"
)

// CommandVisibility reports whether one canonical generated-command path is
// referenceable in the current build. Paths use the same segments as
// apicatalog.MethodRef.CommandPath (for example
// ["mail", "user_mailbox.messages", "list"]). A nil visibility keeps the
// complete schema catalog.
//
// The callback is deliberately command-facing rather than policy-facing:
// cmd/schema only consumes the final build-local presentation surface and does
// not know why a command is or is not referenceable.
type CommandVisibility func(path []string) bool

// SchemaOptions holds all inputs for the schema command.
type SchemaOptions struct {
	Factory *cmdutil.Factory
	Ctx     context.Context

	// Args are the positional path segments, in either the dotted single-arg
	// form ("im.messages.reply") or the space-separated form ("im messages
	// reply"); apicatalog.ParsePath normalizes both.
	Args []string
}

// NewCmdSchema creates the schema command. If runF is non-nil it is called instead of the default runner (test hook).
func NewCmdSchema(f *cmdutil.Factory, runF func(*SchemaOptions) error) *cobra.Command {
	return NewCmdSchemaWithVisibility(f, nil, runF)
}

// NewCmdSchemaWithVisibility creates the schema command projected through one
// build-local command surface. Existing callers should use NewCmdSchema; the
// root builder uses this form so schema execution and completion share the
// exact presentation plan captured by that Cobra tree.
func NewCmdSchemaWithVisibility(
	f *cmdutil.Factory,
	visibility CommandVisibility,
	runF func(*SchemaOptions) error,
) *cobra.Command {
	opts := &SchemaOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "schema [path | service resource method]",
		Short: "View API method parameters, types, and scopes",
		Args:  cobra.MaximumNArgs(8),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Args = append([]string(nil), args...)
			opts.Ctx = cmd.Context()
			if runF != nil {
				return runF(opts)
			}
			return schemaRunWithVisibility(opts, visibility)
		},
	}
	cmdutil.DisableAuthCheck(cmd)

	// Tolerated for agent compatibility; ignored — schema only emits the JSON
	// envelope, and its output is identity-independent (strict-mode filtering
	// comes from ResolveStrictMode, never from --as).
	cmd.Flags().String("format", "json", "")
	cmd.Flags().Bool("json", true, "")
	cmd.Flags().String("as", "", "")
	_ = cmd.Flags().MarkHidden("format")
	_ = cmd.Flags().MarkHidden("json")
	_ = cmd.Flags().MarkHidden("as")

	cmd.ValidArgsFunction = completeSchemaPath(f, visibility)
	cmdutil.SetRisk(cmd, cmdutil.RiskRead)

	return cmd
}

// completeSchemaPath is a thin adapter over the schema catalog's Complete.
// It uses the same source as schema execution so completion candidates match
// what `schema` can resolve.
func completeSchemaPath(
	f *cmdutil.Factory,
	visibility CommandVisibility,
) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		mode := f.ResolveStrictMode(cmd.Context())
		catalog := projectSchemaCatalog(registry.SchemaCatalog(), visibility)
		completions, noSpace := catalog.Complete(args, toComplete, registry.FilterForStrictMode(mode))
		directive := cobra.ShellCompDirectiveNoFileComp
		if noSpace {
			directive |= cobra.ShellCompDirectiveNoSpace
		}
		return completions, directive
	}
}

func schemaRunWithVisibility(opts *SchemaOptions, visibility CommandVisibility) error {
	out := opts.Factory.IOStreams.Out
	mode := opts.Factory.ResolveStrictMode(opts.Ctx)
	return runSchemaWithVisibility(out, apicatalog.ParsePath(opts.Args), mode, visibility)
}

// runSchemaWithVisibility resolves the path through the schema catalog and renders the
// matching envelope(s). The catalog owns navigation (Resolve + MethodRefs) and
// schema owns rendering (Envelope/Envelopes); this adapter only chooses the
// output shape — a single resolved method renders as one envelope object,
// anything broader as an array — and maps resolve failures to hints.
func runSchemaWithVisibility(
	out io.Writer,
	parts []string,
	mode identitypkg.StrictMode,
	visibility CommandVisibility,
) error {
	return runSchemaCatalog(out, parts, mode, registry.SchemaCatalog(), visibility)
}

func runSchemaCatalog(
	out io.Writer,
	parts []string,
	mode identitypkg.StrictMode,
	catalog apicatalog.Catalog,
	visibility CommandVisibility,
) error {
	// Test the source catalog before presentation projection. A distribution
	// that intentionally conceals every generated method still has metadata;
	// bare `schema` should render an empty list rather than claim metadata is
	// unavailable.
	if len(catalog.Services()) == 0 {
		// No embedded metadata and the runtime fallback is empty too: offline
		// with a cold cache, remote meta off, or an unwritable cache dir.
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "No API metadata available").
			WithHint("this binary has no embedded API metadata; run any command with network access to the open platform once so metadata can be fetched and cached")
	}
	catalog = projectSchemaCatalog(catalog, visibility)
	target, err := catalog.Resolve(parts)
	if err != nil {
		return resolveError(err)
	}
	refs := catalog.MethodRefs(target, registry.FilterForStrictMode(mode))
	if target.Kind == apicatalog.TargetMethod {
		if len(refs) == 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"Method %s not available in current identity mode", target.Method.SchemaPath()).
				WithHint("strict mode hides methods the active account identity cannot call; it is shown for an identity (user or bot) that has the required access token")
		}
		output.PrintJson(out, schema.EnvelopeOf(refs[0]))
		return nil
	}
	output.PrintJson(out, schema.Envelopes(refs))
	return nil
}

// projectSchemaCatalog produces the metadata view corresponding to one final
// command surface. It lives in cmd/schema so apicatalog remains a policy-free
// navigation module. Resolve, broad listings, and Complete all consume the
// same projected Catalog, which also prevents resolve-error candidate hints
// from naming concealed resources or methods.
//
// Unchanged branches retain their original maps. A parent is removed when
// projection removed its last reachable method, so a fully concealed service
// cannot survive as an empty schema namespace. Originally-empty, unaffected
// metadata remains unchanged for backward compatibility.
func projectSchemaCatalog(catalog apicatalog.Catalog, visibility CommandVisibility) apicatalog.Catalog {
	if visibility == nil {
		return catalog
	}

	services := make([]meta.Service, 0, len(catalog.Services()))
	changed := false
	for _, service := range catalog.Services() {
		servicePath := []string{service.Name}
		if !visibility(servicePath) {
			changed = true
			continue
		}

		resources, resourceChanged, hasVisibleMethod := projectSchemaResources(
			service.Resources,
			servicePath,
			visibility,
		)
		if resourceChanged && !hasVisibleMethod {
			changed = true
			continue
		}
		if resourceChanged {
			service.Resources = resources
			changed = true
		}
		services = append(services, service)
	}
	if !changed {
		return catalog
	}
	return apicatalog.New(catalog.Source(), services)
}

func projectSchemaResources(
	resources map[string]meta.Resource,
	parentPath []string,
	visibility CommandVisibility,
) (projected map[string]meta.Resource, changed, hasVisibleMethod bool) {
	projected = make(map[string]meta.Resource, len(resources))
	for name, resource := range resources {
		resourcePath := appendPath(parentPath, name)
		if !visibility(resourcePath) {
			changed = true
			continue
		}

		methods := make(map[string]meta.Method, len(resource.Methods))
		resourceChanged := false
		resourceHasVisibleMethod := false
		for methodName, method := range resource.Methods {
			if !visibility(appendPath(resourcePath, methodName)) {
				resourceChanged = true
				continue
			}
			methods[methodName] = method
			resourceHasVisibleMethod = true
		}

		subResources, subChanged, subHasVisibleMethod := projectSchemaResources(
			resource.Resources,
			resourcePath,
			visibility,
		)
		resourceChanged = resourceChanged || subChanged
		resourceHasVisibleMethod = resourceHasVisibleMethod || subHasVisibleMethod

		if resourceChanged && !resourceHasVisibleMethod {
			// Projection removed the final method below this resource. Keeping
			// the empty group would still reveal a concealed schema namespace.
			changed = true
			continue
		}
		if resourceChanged {
			resource.Methods = methods
			resource.Resources = subResources
			changed = true
		}
		projected[name] = resource
		hasVisibleMethod = hasVisibleMethod || resourceHasVisibleMethod
	}

	if !changed {
		return resources, false, hasVisibleMethod
	}
	return projected, true, hasVisibleMethod
}

func appendPath(parent []string, segment string) []string {
	path := make([]string, len(parent)+1)
	copy(path, parent)
	path[len(parent)] = segment
	return path
}

// resolveError maps a catalog *ResolveError to a typed *errs.ValidationError
// (CategoryValidation drives the exit code; Hint promotes to the envelope),
// preserving the historical message + hint text.
func resolveError(err error) error {
	var re *apicatalog.ResolveError
	if !errors.As(err, &re) {
		return err
	}
	switch re.Kind {
	case apicatalog.ErrService:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "Unknown service: %s", re.Subject).
			WithHint("Available: %s", strings.Join(re.Candidates, ", "))
	case apicatalog.ErrResource:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "Unknown resource: %s", re.Subject).
			WithHint("Available: %s", strings.Join(re.Candidates, ", "))
	case apicatalog.ErrMethod:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "Unknown method: %s", re.Subject).
			WithHint("Available: %s", strings.Join(re.Candidates, ", "))
	case apicatalog.ErrPath:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "Unknown path: %s", re.Subject).
			WithHint("Method %q exists but the trailing segments %q do not resolve", re.Method, re.Trailing)
	}
	return err
}
