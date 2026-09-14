// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"math"
	"testing"
)

// TestRoundUsagePercent covers the rounding contract shared by +db-quota-get and
// +file-quota-get. The first two cases are values observed on the wire, which is
// what motivated the rounding: a 2 GB quota yields a tail long enough to bury the
// digits that matter.
func TestRoundUsagePercent(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want float64
		ok   bool
	}{
		{"observed db value", 5.40924072265625, 5.4, true},
		{"observed file value", 0.4398345947265625, 0.4, true},
		{"rounds up at .05", 1.96, 2.0, true},
		{"rounds down below .05", 1.94, 1.9, true},
		{"whole number stays whole", float64(50), 50, true},
		{"zero", float64(0), 0, true},
		// Over-quota must survive: the server does not clamp and neither may we.
		{"over 100 is not clamped", 180.4567, 180.5, true},
		// Below half a tenth there is no non-zero first decimal to report;
		// storage_used_bytes still carries the exact figure.
		{"tiny usage collapses to zero", 0.04, 0, true},
		{"json.Number from the wire", json.Number("5.40924072265625"), 5.4, true},
		{"missing", nil, 0, false},
		{"non-numeric", "12%", 0, false},
		{"NaN is not a percentage", math.NaN(), 0, false},
		{"Inf is not a percentage", math.Inf(1), 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := roundUsagePercent(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("roundUsagePercent(%v) = (%v, %v), want (%v, %v)", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// TestRoundUsagePercent_EncodesWithoutTail pins the thing the change is actually
// for: the rounded value must serialise short. Asserting on the float alone would
// pass even if encoding/json re-introduced a tail from the binary representation.
func TestRoundUsagePercent_EncodesWithoutTail(t *testing.T) {
	cases := map[string]string{
		"5.40924072265625":   "5.4",
		"0.4398345947265625": "0.4",
		"1.9195556640625":    "1.9",
		"2.0061492919921875": "2",
		"14.600000000000001": "14.6",
	}

	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			v, ok := roundUsagePercent(json.Number(in))
			if !ok {
				t.Fatalf("roundUsagePercent(%s) reported non-numeric", in)
			}
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(b) != want {
				t.Fatalf("json.Marshal(roundUsagePercent(%s)) = %s, want %s", in, b, want)
			}
		})
	}
}

// TestPutUsagePercent_OmitsRatherThanFabricates guards the absent case: writing a
// 0 for a field the server never sent would read as "nothing used" instead of
// "not reported".
func TestPutUsagePercent_OmitsRatherThanFabricates(t *testing.T) {
	for _, src := range []map[string]interface{}{
		{},
		{"usage_percent": nil},
		{"usage_percent": "n/a"},
	} {
		dst := map[string]interface{}{}
		putUsagePercent(dst, src)
		if _, ok := dst["usage_percent"]; ok {
			t.Errorf("src %v: usage_percent must be omitted, got %v", src, dst)
		}
	}
}

// TestQuotaProjections_RoundUsagePercentAlike is the alignment guard: both
// commands must report the same precision for the same input, so a future change
// to one cannot silently drift from the other.
func TestQuotaProjections_RoundUsagePercentAlike(t *testing.T) {
	const raw = 5.40924072265625
	const want = 5.4

	db := projectDbQuota(map[string]interface{}{
		"storage_used_bytes": 116162560, "storage_quota_bytes": 2147483648,
		"usage_percent": raw, "tables": 1, "views": 0,
	})
	file := projectFileQuota(map[string]interface{}{
		"storage_used_bytes": 116162560, "storage_quota_bytes": 2147483648,
		"usage_percent": raw, "files": 6,
	})

	if db["usage_percent"] != want {
		t.Errorf("projectDbQuota usage_percent = %v, want %v", db["usage_percent"], want)
	}
	if file["usage_percent"] != want {
		t.Errorf("projectFileQuota usage_percent = %v, want %v", file["usage_percent"], want)
	}
	if db["usage_percent"] != file["usage_percent"] {
		t.Errorf("db and file must report the same precision: %v vs %v", db["usage_percent"], file["usage_percent"])
	}
}
