// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"encoding/json"
	"testing"
)

func TestFormatScalar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   any
		want string
	}{
		{"float64 large integer avoids scientific", float64(1185356), "1185356"},
		{"float64 below scientific threshold", float64(358934), "358934"},
		{"float64 zero", float64(0), "0"},
		{"float64 huge", float64(20 * 1024 * 1024), "20971520"},
		{"float64 negative", float64(-42), "-42"},
		{"float64 fractional preserved", float64(3.14), "3.14"},
		{"json number scientific integer", json.Number("1.185356e6"), "1185356"},
		{"json number MaxInt64 scientific", json.Number("9.223372036854775807e18"), "9223372036854775807"},
		{"json number MaxInt64 exact", json.Number("9223372036854775807"), "9223372036854775807"},
		{"json number negative scientific", json.Number("-4.2e2"), "-420"},
		{"json number fractional scientific", json.Number("1.25e-3"), "0.00125"},
		{"json number plain exponent", json.Number("1e3"), "1000"},
		{"json number zero avoids exponent expansion", json.Number("0e-1048576"), "0"},
		{"json number decimal unchanged", json.Number("3.14"), "3.14"},
		{"json number insignificant fraction dropped", json.Number("1.0"), "1"},
		{"json number trailing fraction zeros dropped", json.Number("1.50"), "1.5"},
		{"json number leading fraction", json.Number("0.5"), "0.5"},
		{"string pass-through", "hello", "hello"},
		{"bool true", true, "true"},
		{"int via %v", 42, "42"},
		{"int64 via %v", int64(9007199254740992), "9007199254740992"},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := FormatScalar(tt.in); got != tt.want {
				t.Fatalf("FormatScalar(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
