// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

// CheckAdHocSubtype detects `Subtype: "ad_hoc_*"` literals (and the
// errs.Subtype("ad_hoc_*") cast form) and emits a LABEL diagnostic so a CI
// workflow can tag the PR with `needs-taxonomy-decision`. This is a
// governance signal, NOT a hard rejection — ad_hoc_* is the reserved
// temporary namespace and is allowed for ≤1 week while taxonomy is finalized.
func CheckAdHocSubtype(path, src string) []Violation {
	v, _ := scanSubtype(path, src, nil, nil, nil, "")
	out := v[:0]
	for _, vv := range v {
		if vv.Action == ActionLabel {
			out = append(out, vv)
		}
	}
	return out
}
