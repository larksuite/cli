// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import "math"

// usagePercentDecimals is the precision usage_percent is reported at, shared by
// +db-quota-get and +file-quota-get so the two stay aligned.
//
// One decimal is what the number can actually support: it is derived server-side
// as used/quota*100 in float64, so the tail is binary-representation noise rather
// than measurement — a 2 GB quota reported as 5.40924072265625% is precise to
// about a megabyte in a field whose companion storage_used_bytes already carries
// the exact figure. Printing the full tail costs an agent context on digits that
// carry no information and reads as false precision.
const usagePercentDecimals = 1

// roundUsagePercent rounds a raw usage_percent to usagePercentDecimals places,
// reporting whether the input was numeric at all.
//
// Deliberately NOT clamped to 100: the server does not clamp either, because
// exceeding a quota is the one reading that must stay visible, and folding 180%
// down to 100% would hide it.
//
// The result stays a float64 rather than a formatted string so callers keep a
// comparable number — `-q '.data.usage_percent > 80'` has to work. Values that
// land on a whole number encode as "2" rather than "2.0", which is what "at most
// one decimal" means.
func roundUsagePercent(raw interface{}) (float64, bool) {
	v, ok := numericAsFloat(raw)
	if !ok {
		return 0, false
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	shift := math.Pow(10, usagePercentDecimals)
	return math.Round(v*shift) / shift, true
}

// putUsagePercent copies usage_percent from src into dst, rounded. A missing or
// non-numeric value is left out entirely rather than written as 0 — the caller's
// contract is that an absent field means "not reported", and a fabricated zero
// would read as "nothing used".
func putUsagePercent(dst, src map[string]interface{}) {
	if p, ok := roundUsagePercent(src["usage_percent"]); ok {
		dst["usage_percent"] = p
	}
}
