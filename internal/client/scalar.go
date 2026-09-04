// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"fmt"
	"strconv"
)

// FormatScalar renders a decoded JSON scalar as it travels on the wire in a
// query string or multipart form field. A float64 is written in plain decimal
// notation rather than with Go's default %v, which switches to scientific
// notation at 1e6 and above and below 1e-4: a second-precision timestamp would
// otherwise go out as "1.7e+09", which backends reject when parsing a numeric
// field. Every other type falls through to %v.
func FormatScalar(v any) string {
	if n, ok := v.(float64); ok {
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return fmt.Sprintf("%v", v)
}
