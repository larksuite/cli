// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
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
		// %v renders these in scientific notation ("1e+06", "1.7e+09"), which
		// backends reject when parsing an integer field.
		{"float64 at the scientific threshold", float64(1000000), "1000000"},
		{"float64 second-precision timestamp", float64(1700000000), "1700000000"},
		{"float64 millisecond-precision timestamp", float64(1700000000000), "1700000000000"},
		{"float64 from an exponent literal", float64(1e3), "1000"},
		// A fraction whose exponent is smaller than its decimal count must not
		// acquire a leading zero: "01.2" is not a legal JSON number.
		{"float64 from a leading-zero exponent literal", float64(0.12e1), "1.2"},
		{"float64 from a wider leading-zero exponent literal", float64(0.72264e4), "7226.4"},
		{"float64 negative leading-zero exponent literal", float64(-0.375e2), "-37.5"},
		{"float64 insignificant fraction dropped", float64(1.0), "1"},
		{"float64 small fraction avoids scientific", float64(1.25e-3), "0.00125"},
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
