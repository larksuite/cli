// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// Named A1 boundaries have defined units: row numbers or column letters.
// Generic numeric indexes remain unsupported because their base is unknown.
func resizeBoundaryRange(flags interface {
	Str(string) string
	Changed(string) bool
}, dimension string) (string, bool, error) {
	axis := "row"
	if dimension == "column" {
		axis = "col"
	}
	start, end := "start-"+axis, "end-"+axis
	if !flags.Changed(start) && !flags.Changed(end) {
		return "", false, nil
	}
	if !flags.Changed(start) {
		return "", true, sheetsValidationForFlag(start, "--%s is required with --%s", start, end)
	}
	first := strings.TrimSpace(flags.Str(start))
	last := first
	if flags.Changed(end) {
		last = strings.TrimSpace(flags.Str(end))
	}
	for _, field := range []string{start, end} {
		if !flags.Changed(field) {
			continue
		}
		value := strings.TrimSpace(flags.Str(field))
		dim, _, err := parseA1Position(value)
		if err != nil {
			return "", true, sheetsValidationForFlag(field, "--%s must name one A1 %s", field, axis).WithCause(err)
		}
		if dim != dimension {
			return "", true, sheetsValidationForFlag(field, "--%s must use %s A1 units", field, dimension)
		}
	}
	rng := first + ":" + last
	dim, lo, hi, err := parseA1Range(rng)
	if err != nil {
		return "", true, sheetsValidationForFlag(end, "invalid %s boundaries: %v", axis, err).WithCause(err)
	}
	if flags.Changed("range") {
		d, a, b, err := parseA1Range(strings.TrimSpace(flags.Str("range")))
		if err != nil || d != dim || a != lo || b != hi {
			return "", true, sheetsValidationForFlag("range", "--range conflicts with --%s / --%s", start, end).WithCause(err)
		}
		return flags.Str("range"), true, nil
	}
	return rng, true, nil
}

func normalizeResizeBoundaries(dimension string) func(context.Context, *common.FlagContext) error {
	return func(_ context.Context, flags *common.FlagContext) error {
		rng, changed, err := resizeBoundaryRange(flags, dimension)
		if err != nil || !changed {
			return err
		}
		return flags.SetCanonical("range", rng)
	}
}
