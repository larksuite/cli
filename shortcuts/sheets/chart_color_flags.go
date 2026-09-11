// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

// chartColorPaletteAlias maps an AI-friendly palette name to the wire value the
// server understands. The wire vocabulary (brandColorSeries@v2,
// singleColorSeries-W-@v2, ...) leaks palette-engine internals that mislead
// callers: "W" is wathet (cyan) not white, "primary" is actually the muted
// light tint, and the @v2 suffix plus the single-color dash shape are pure
// noise. The flag enum advertises only the friendly names; legacy wire values
// still work because Normalize folds them back to their friendly spelling.
type chartColorPaletteAlias struct {
	friendly string
	wire     string
}

// chartColorPaletteAliases is the single source for both directions of
// translation and for the set of legacy wire values accepted for compatibility.
var chartColorPaletteAliases = []chartColorPaletteAlias{
	{friendly: "brand", wire: "brandColorSeries@v2"},
	{friendly: "rainbow", wire: "rainbowColorSeries@v2"},
	{friendly: "contrast", wire: "complementaryColorSeries@v2"},
	{friendly: "diverging", wire: "converseColorSeries@v2"},
	{friendly: "muted", wire: "primaryColorSeries@v2"},
	{friendly: "mono-blue", wire: "singleColorSeries-B-@v2"},
	{friendly: "mono-cyan", wire: "singleColorSeries-W-@v2"},
	{friendly: "mono-green", wire: "singleColorSeries-G-@v2"},
	{friendly: "mono-yellow", wire: "singleColorSeries-Y-@v2"},
	{friendly: "mono-orange", wire: "singleColorSeries-O-@v2"},
	{friendly: "mono-red", wire: "singleColorSeries-R-@v2"},
	{friendly: "mono-gray", wire: "singleColorSeries-D-@v2"},
}

// friendlyToWireChartPalette returns the wire value for a friendly palette name.
func friendlyToWireChartPalette(friendly string) (string, bool) {
	for _, alias := range chartColorPaletteAliases {
		if friendly == alias.friendly {
			return alias.wire, true
		}
	}
	return "", false
}

// wireToFriendlyChartPalette returns the friendly name for a legacy wire value.
func wireToFriendlyChartPalette(wire string) (string, bool) {
	for _, alias := range chartColorPaletteAliases {
		if wire == alias.wire {
			return alias.friendly, true
		}
	}
	return "", false
}

// normalizeChartColorPalette is the sheets adapter for the framework Normalize
// phase, which runs before enum validation. The enum advertises only friendly
// names, so a legacy wire value would fail validation; this folds it back to its
// friendly spelling first. Friendly and unknown values are left untouched — the
// former passes validation, the latter is rejected there with the standard
// message. Body assembly later translates the friendly value back to wire.
func normalizeChartColorPalette(_ context.Context, flags *common.FlagContext) error {
	if !flags.Changed("color-palette") {
		return nil
	}
	value := flags.Str("color-palette")
	if friendly, ok := wireToFriendlyChartPalette(value); ok {
		return flags.SetCanonical("color-palette", friendly)
	}
	return nil
}
