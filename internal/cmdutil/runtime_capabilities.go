// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/runtimeplan"
)

const (
	runtimeCapabilitiesAnnotation = "lark:runtimeCapabilities"
	noRuntimeCapabilities         = "-"
)

// SetRuntimeCapabilities declares the runtime surfaces required by a command.
// A zero-capability declaration explicitly overrides a parent default.
func SetRuntimeCapabilities(cmd *cobra.Command, capabilities ...runtimeplan.Capability) {
	if cmd == nil {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	if len(capabilities) == 0 {
		cmd.Annotations[runtimeCapabilitiesAnnotation] = noRuntimeCapabilities
		return
	}
	values := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		if capability != "" {
			values = append(values, string(capability))
		}
	}
	if len(values) == 0 {
		cmd.Annotations[runtimeCapabilitiesAnnotation] = noRuntimeCapabilities
		return
	}
	cmd.Annotations[runtimeCapabilitiesAnnotation] = strings.Join(values, ",")
}

// GetRuntimeCapabilities resolves the nearest explicit declaration from the
// leaf command toward its parents.
func GetRuntimeCapabilities(cmd *cobra.Command) []runtimeplan.Capability {
	for current := cmd; current != nil; current = current.Parent() {
		raw, ok := current.Annotations[runtimeCapabilitiesAnnotation]
		if !ok {
			continue
		}
		if raw == "" || raw == noRuntimeCapabilities {
			return nil
		}
		parts := strings.Split(raw, ",")
		out := make([]runtimeplan.Capability, 0, len(parts))
		for _, part := range parts {
			if capability := strings.TrimSpace(part); capability != "" {
				out = append(out, runtimeplan.Capability(capability))
			}
		}
		return out
	}
	return nil
}

// RequireRuntimeCapabilities applies the invocation plan and the selected
// credential source to a command's source-neutral capability declaration.
func (f *Factory) RequireRuntimeCapabilities(
	ctx context.Context,
	command string,
	capabilities ...runtimeplan.Capability,
) error {
	if f == nil {
		return nil
	}
	plan := runtimeplan.Ensure(f.runtimePlan)
	for _, capability := range capabilities {
		if err := plan.Require(capability); err != nil {
			return err
		}
		if capability != runtimeplan.CapabilityLocalCredentialManagement || f.Credential == nil {
			continue
		}
		providerName, err := f.Credential.ActiveExtensionProviderName(ctx)
		if err != nil {
			return err
		}
		if providerName == "" {
			continue
		}
		if command == "" {
			command = "credential management"
		}
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"%q cannot manage credentials owned by provider %q", command, providerName).
			WithHint("manage authorization through the active credential provider, or use a local Profile credential source")
	}
	return nil
}

// RequireCommandRuntimeCapabilities checks the declaration resolved for cmd.
func (f *Factory) RequireCommandRuntimeCapabilities(ctx context.Context, cmd *cobra.Command) error {
	command := "credential management"
	if cmd != nil {
		command = cmd.CommandPath()
	}
	return f.RequireRuntimeCapabilities(ctx, command, GetRuntimeCapabilities(cmd)...)
}
