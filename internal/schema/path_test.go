// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"reflect"
	"testing"
)

func TestParsePath(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"empty args -> nil", nil, nil},
		{"empty slice -> nil", []string{}, nil},
		{"single dotted", []string{"im.messages.reply"}, []string{"im", "messages", "reply"}},
		{"single no-dot", []string{"im"}, []string{"im"}},
		{"multi args", []string{"im", "messages", "reply"}, []string{"im", "messages", "reply"}},
		{"two args", []string{"im", "messages"}, []string{"im", "messages"}},
		{"nested resource dotted", []string{"im.chat.members.bots"}, []string{"im", "chat", "members", "bots"}},
		{"nested resource space form", []string{"im", "chat.members", "bots"}, []string{"im", "chat.members", "bots"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParsePath(tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParsePath(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
