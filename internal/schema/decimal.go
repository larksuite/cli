// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"math/big"
	"strings"
)

// decimal represents a valid JSON number as sign * digits * 10^exponent.
// Keeping the exponent symbolic avoids both float64 rounding and allocating
// gigabytes of zeros for inputs such as 1e1000000000.
type decimal struct {
	sign     int
	digits   string
	exponent *big.Int
}

func decimalOf(raw string) decimal {
	number := decimal{sign: 1, exponent: new(big.Int)}
	if strings.HasPrefix(raw, "-") {
		number.sign = -1
		raw = raw[1:]
	}
	if index := strings.IndexAny(raw, "eE"); index >= 0 {
		number.exponent.SetString(raw[index+1:], 10)
		raw = raw[:index]
	}
	if index := strings.IndexByte(raw, '.'); index >= 0 {
		number.exponent.Sub(number.exponent, big.NewInt(int64(len(raw)-index-1)))
		raw = raw[:index] + raw[index+1:]
	}
	raw = strings.TrimLeft(raw, "0")
	if raw == "" {
		number.sign = 0
		number.digits = "0"
		number.exponent.SetInt64(0)
		return number
	}
	number.digits = strings.TrimRight(raw, "0")
	number.exponent.Add(number.exponent, big.NewInt(int64(len(raw)-len(number.digits))))
	return number
}

func (number decimal) integer() bool { return number.exponent.Sign() >= 0 }

// compareDecimal accepts valid JSON numeric literals, as supplied by the JSON
// decoder and the schema assembler's numeric bounds.
func compareDecimal(a, b string) int {
	left, right := decimalOf(a), decimalOf(b)
	if left.sign < right.sign {
		return -1
	}
	if left.sign > right.sign {
		return 1
	}
	if left.sign == 0 {
		return 0
	}
	leftMagnitude := new(big.Int).Add(left.exponent, big.NewInt(int64(len(left.digits))))
	rightMagnitude := new(big.Int).Add(right.exponent, big.NewInt(int64(len(right.digits))))
	if compared := leftMagnitude.Cmp(rightMagnitude); compared != 0 {
		return compared * left.sign
	}
	for index := 0; index < max(len(left.digits), len(right.digits)); index++ {
		l, r := byte('0'), byte('0')
		if index < len(left.digits) {
			l = left.digits[index]
		}
		if index < len(right.digits) {
			r = right.digits[index]
		}
		if l < r {
			return -left.sign
		}
		if l > r {
			return left.sign
		}
	}
	return 0
}
