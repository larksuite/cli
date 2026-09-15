// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apicatalog_test

import (
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
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
		// A resource's own dots are split here and rejoined by findResource's
		// longest-prefix descent, so this reaches the same resource as the
		// one-argument form above. Leaving them joined is what used to make the
		// mixed form below fail while its all-dotted twin resolved.
		{"nested resource space form", []string{"im", "chat.members", "bots"}, []string{"im", "chat", "members", "bots"}},
		{"mixed separate and dotted args", []string{"sheets", "spreadsheet.sheet.filters.create"},
			[]string{"sheets", "spreadsheet", "sheet", "filters", "create"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apicatalog.ParsePath(tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParsePath(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
