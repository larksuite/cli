// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/envvars"
)

func TestWrapCitationBuilderDisabledReturnsNil(t *testing.T) {
	t.Setenv(envvars.CliCitation, "0")
	called := false
	wrapped := wrapCitationBuilder(&bytes.Buffer{}, "lark x +y", []citation.SourceType{citation.SourceWiki},
		func() []citation.Citation { called = true; return nil })
	if wrapped != nil {
		t.Fatal("wrapped builder must be nil when gate is off")
	}
	if called {
		t.Fatal("builder must not run when gate is off")
	}
}

func TestWrapCitationBuilderNilBuildReturnsNil(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	if wrapCitationBuilder(&bytes.Buffer{}, "lark x +y", []citation.SourceType{citation.SourceWiki}, nil) != nil {
		t.Fatal("nil build must wrap to nil")
	}
}

func TestWrapCitationBuilderValidation(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	var errOut bytes.Buffer
	wrapped := wrapCitationBuilder(&errOut, "lark x +y", []citation.SourceType{citation.SourceWiki},
		func() []citation.Citation {
			return []citation.Citation{
				{SourceType: citation.SourceWiki, URL: "https://ok.example/1", Title: "keep"},
				{SourceType: citation.SourceUnset, URL: "https://ok.example/2", Title: "unset"},
				{SourceType: citation.SourceMessage, URL: "https://ok.example/3", Title: "undeclared"},
				{SourceType: citation.SourceWiki, URL: "http://plain.example/4", Title: "http"},
				{SourceType: citation.SourceWiki, URL: "file:///etc/passwd", Title: "file"},
				{SourceType: citation.SourceWiki, URL: "/relative/path", Title: "relative"},
				{SourceType: citation.SourceWiki, URL: "", Title: "no-url-silent"},
			}
		})
	got := wrapped()
	if len(got) != 1 || got[0].Title != "keep" {
		t.Fatalf("wrapped() = %#v, want only the valid entry", got)
	}
	warnings := errOut.String()
	for _, want := range []string{
		"warning: lark x +y: dropped citation: source_type is unset",
		fmt.Sprintf("warning: lark x +y: dropped citation: source_type %d is not declared by this command", citation.SourceMessage),
		"warning: lark x +y: dropped citation: url is not an absolute https URL",
	} {
		if !strings.Contains(warnings, want) {
			t.Errorf("stderr missing %q; got:\n%s", want, warnings)
		}
	}
	// http/file/relative 共 3 条 url 告警
	if n := strings.Count(warnings, "url is not an absolute https URL"); n != 3 {
		t.Errorf("url warnings = %d, want 3", n)
	}
	// 无 url 条目静默丢弃，不产生告警
	if strings.Contains(warnings, "no-url") || strings.Count(warnings, "warning:") != 5 {
		t.Errorf("unexpected warnings:\n%s", warnings)
	}
}

func TestValidateCitationDeclaration(t *testing.T) {
	valid := &CitationDefinition{SourceTypes: []citation.SourceType{citation.SourceWiki}}
	if err := validateCitationDeclaration(valid, "read"); err != nil {
		t.Fatalf("valid declaration rejected: %v", err)
	}
	// Use a calculated unallocated source type to verify validation rejects undefined types
	unallocated := citation.SourceType(int(citation.SourceMeetingNote) + 100)
	cases := []struct {
		name string
		def  *CitationDefinition
		risk string
	}{
		{"risk not explicit read", valid, ""},
		{"risk write", valid, "write"},
		{"empty source types", &CitationDefinition{}, "read"},
		{"unset source type", &CitationDefinition{SourceTypes: []citation.SourceType{citation.SourceUnset}}, "read"},
		{"unallocated source type", &CitationDefinition{SourceTypes: []citation.SourceType{unallocated}}, "read"},
	}
	for _, tc := range cases {
		if err := validateCitationDeclaration(tc.def, tc.risk); err == nil {
			t.Errorf("%s: want error, got nil", tc.name)
		}
	}
}
