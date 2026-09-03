// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FormatScalar renders a decoded JSON scalar as it travels on the wire in a
// query string or multipart form field. Every number is written in plain
// decimal notation: exponents are expanded and insignificant fraction zeros
// dropped, because some backends reject "1e6" or "1.0" when parsing integer
// fields, and a json.Number never passes through float64 so large integers
// keep every digit. Non-numeric values fall through to %v.
func FormatScalar(v any) string {
	switch n := v.(type) {
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64)
	case json.Number:
		return canonicalJSONNumber(n)
	}
	return fmt.Sprintf("%v", v)
}

// canonicalJSONNumber rewrites a valid JSON number literal in plain decimal
// notation by moving the decimal point through its original digits, so no
// precision is lost. Literals it cannot interpret are returned unchanged.
func canonicalJSONNumber(n json.Number) string {
	raw := n.String()
	mantissa := raw
	var exponent int64
	if expAt := strings.IndexAny(raw, "eE"); expAt >= 0 {
		exp, err := strconv.ParseInt(raw[expAt+1:], 10, 32)
		if err != nil {
			return raw
		}
		mantissa, exponent = raw[:expAt], exp
	}
	sign := ""
	if strings.HasPrefix(mantissa, "-") {
		sign = "-"
		mantissa = mantissa[1:]
	}

	dot := strings.IndexByte(mantissa, '.')
	integerDigits := len(mantissa)
	digits := mantissa
	if dot >= 0 {
		integerDigits = dot
		digits = mantissa[:dot] + mantissa[dot+1:]
	}
	if strings.Trim(digits, "0") == "" {
		return sign + "0"
	}
	decimalPos := int64(integerDigits) + exponent

	// Avoid turning a compact but pathological exponent into an unbounded
	// allocation. Such a value is not a practical wire field; leave it
	// unchanged so the server can reject it.
	const maxExpandedDigits = int64(1 << 20)
	if decimalPos > maxExpandedDigits || decimalPos < -maxExpandedDigits {
		return raw
	}

	var out string
	switch {
	case decimalPos <= 0:
		out = "0." + strings.Repeat("0", int(-decimalPos)) + digits
	case decimalPos >= int64(len(digits)):
		out = digits + strings.Repeat("0", int(decimalPos)-len(digits))
	default:
		pos := int(decimalPos)
		out = digits[:pos] + "." + digits[pos:]
	}
	if strings.Contains(out, ".") {
		out = strings.TrimRight(out, "0")
		out = strings.TrimRight(out, ".")
	}
	if !strings.Contains(out, ".") {
		out = strings.TrimLeft(out, "0")
		if out == "" {
			out = "0"
		}
	}
	return sign + out
}
