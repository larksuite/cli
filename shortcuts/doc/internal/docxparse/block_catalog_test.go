// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

import "testing"

func TestIsPresentationBlockType(t *testing.T) {
	tests := []struct {
		tag  string
		want bool
	}{
		{tag: "img", want: true},
		{tag: "whiteboard", want: true},
		{tag: "html5-block", want: true},
		{tag: "table", want: true},
		{tag: "pre", want: true},
		{tag: "p", want: false},
		{tag: "h1", want: false},
		{tag: "li", want: false},
		{tag: "code", want: false},
		{tag: "future-widget", want: false},
	}

	for _, test := range tests {
		t.Run(test.tag, func(t *testing.T) {
			if got := IsPresentationBlockType(test.tag); got != test.want {
				t.Fatalf("IsPresentationBlockType(%q) = %v, want %v", test.tag, got, test.want)
			}
		})
	}
}
