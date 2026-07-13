// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseDocumentRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        string
		wantKind     string
		wantToken    string
		wantFragment string
		wantErr      string
	}{
		{
			name:      "docx url",
			input:     "https://example.larksuite.com/docx/xxxxxx?from=wiki",
			wantKind:  "docx",
			wantToken: "xxxxxx",
		},
		{
			name:      "wiki url",
			input:     "https://example.larksuite.com/wiki/xxxxxx?from=wiki",
			wantKind:  "wiki",
			wantToken: "xxxxxx",
		},
		{
			name:         "wiki url with selection anchor",
			input:        "https://example.larksuite.com/wiki/xxxxxx#share-CUE3d6Ykno2fkexEvt8cGF8Wnse",
			wantKind:     "wiki",
			wantToken:    "xxxxxx",
			wantFragment: "share-CUE3d6Ykno2fkexEvt8cGF8Wnse",
		},
		{
			name:      "doc url",
			input:     "https://example.larksuite.com/doc/xxxxxx",
			wantKind:  "doc",
			wantToken: "xxxxxx",
		},
		{
			name:      "raw token",
			input:     "xxxxxx",
			wantKind:  "docx",
			wantToken: "xxxxxx",
		},
		{
			name:    "unsupported url",
			input:   "https://example.com/not-a-doc",
			wantErr: "unsupported --doc input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseDocumentRef(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Kind != tt.wantKind {
				t.Fatalf("parseDocumentRef(%q) kind = %q, want %q", tt.input, got.Kind, tt.wantKind)
			}
			if got.Token != tt.wantToken {
				t.Fatalf("parseDocumentRef(%q) token = %q, want %q", tt.input, got.Token, tt.wantToken)
			}
			if got.Fragment != tt.wantFragment {
				t.Fatalf("parseDocumentRef(%q) fragment = %q, want %q", tt.input, got.Fragment, tt.wantFragment)
			}
		})
	}
}

func TestBuildDriveRouteExtraEscapesJSON(t *testing.T) {
	t.Parallel()

	got, err := buildDriveRouteExtra(`doc-"quoted"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `{"drive_route_token":"doc-\"quoted\""}`
	if got != want {
		t.Fatalf("buildDriveRouteExtra() = %q, want %q", got, want)
	}
}

func TestAppendDocWarning(t *testing.T) {
	t.Parallel()

	appendDocWarning(nil, "ignored")

	empty := map[string]interface{}{}
	appendDocWarning(empty, "   ")
	if _, ok := empty["warnings"]; ok {
		t.Fatalf("blank warning should be ignored: %#v", empty)
	}

	tests := []struct {
		name string
		data map[string]interface{}
		want interface{}
	}{
		{
			name: "missing warnings",
			data: map[string]interface{}{},
			want: []string{"new warning"},
		},
		{
			name: "string slice warnings",
			data: map[string]interface{}{"warnings": []string{"old warning"}},
			want: []string{"old warning", "new warning"},
		},
		{
			name: "interface slice warnings",
			data: map[string]interface{}{"warnings": []interface{}{"old warning"}},
			want: []interface{}{"old warning", "new warning"},
		},
		{
			name: "scalar warning",
			data: map[string]interface{}{"warnings": "old warning"},
			want: []interface{}{"old warning", "new warning"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appendDocWarning(tt.data, "new warning")
			if got := tt.data["warnings"]; !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("warnings = %#v, want %#v", got, tt.want)
			}
		})
	}
}
