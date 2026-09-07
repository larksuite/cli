// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // Compiler diagnostics are build-time declaration errors wrapped by the command-set startup guard.
package common

import (
	"fmt"
	"sort"
)

func validateOutputHooks(definition typedOutputDefinition, renderers map[string]rendererMarker) error {
	rendererNames := make([]string, 0, len(renderers))
	for name := range renderers {
		rendererNames = append(rendererNames, name)
	}
	sort.Strings(rendererNames)
	for _, name := range rendererNames {
		renderer := renderers[name]
		if renderer.isNil {
			return fmt.Errorf("Hooks.Renderers[%q] is nil", name)
		}
		if name != "pretty" {
			return fmt.Errorf("Hooks.Renderers[%q] is invalid: custom renderers are only supported for pretty; table, csv, and ndjson use framework formatters", name)
		}
		if definition.Mode == typedOutputFixedJSON {
			return fmt.Errorf("Hooks.Renderers[%q] conflicts with Output.Mode %q: fixed JSON output does not execute custom renderers", name, definition.Mode)
		}
	}
	return nil
}

// rendererMarker lets the bridge compiler inspect nil renderer values without
// exposing the private compiled hook type.
type rendererMarker struct{ isNil bool }
