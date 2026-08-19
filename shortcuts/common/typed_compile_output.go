// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // Registration-time compiler diagnostics are programmer errors surfaced through Define's panic boundary.
package common

import (
	"fmt"
	"sort"
)

func validateOutputHooks(definition OutputDefinition, renderers map[string]RendererMarker) error {
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
		if definition.Mode == OutputFixedJSON {
			return fmt.Errorf("Hooks.Renderers[%q] conflicts with Output.Mode %q: fixed JSON output does not execute custom renderers", name, definition.Mode)
		}
	}
	return nil
}

// RendererMarker lets the generic compiler inspect nil renderer values without
// adapting Args/Data hooks or exposing the private compiled hook type.
type RendererMarker struct{ isNil bool }

func rendererMarkers[Data any](renderers map[string]typedRenderer[Data]) map[string]RendererMarker {
	markers := make(map[string]RendererMarker, len(renderers))
	for name, renderer := range renderers {
		markers[name] = RendererMarker{isNil: renderer == nil}
	}
	return markers
}
