// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sourcecompat_test

import (
	"reflect"
	"testing"

	"github.com/larksuite/cli/cmd/auth"
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// Existing external consumers may legally use positional literals for these
// exported structs. Pinning their field names and order prevents an otherwise
// source-breaking field addition or reorder from passing repository-only
// keyed-literal tests.
func TestStableExportedStructLayouts(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		fields []string
	}{
		{
			name:  "errs.Problem",
			value: errs.Problem{},
			fields: []string{
				"Category", "Subtype", "Code", "Message", "Hint", "LogID",
				"Troubleshooter", "Retryable",
			},
		},
		{
			name:   "auth.CheckOptions",
			value:  auth.CheckOptions{},
			fields: []string{"Factory", "Scope", "JSON"},
		},
		{
			name:  "common.Shortcut",
			value: common.Shortcut{},
			fields: []string{
				"Service", "Command", "Description", "Risk", "Scopes",
				"UserScopes", "BotScopes", "ConditionalScopes",
				"ConditionalUserScopes", "ConditionalBotScopes", "AuthTypes",
				"Flags", "HasFormat", "Tips", "Hidden", "Normalize", "DryRun",
				"Validate", "Execute", "OnInvoke", "PrintFlagSchema", "PostMount",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			typ := reflect.TypeOf(test.value)
			if typ.NumField() != len(test.fields) {
				t.Fatalf("%s has %d fields, want %d; adding or removing fields breaks positional literals",
					test.name, typ.NumField(), len(test.fields))
			}
			for i, want := range test.fields {
				if got := typ.Field(i).Name; got != want {
					t.Fatalf("%s field %d = %q, want %q; reordering fields breaks positional literals",
						test.name, i, got, want)
				}
			}
		})
	}
}
