// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

import (
	"strings"
	"testing"
)

func pageDefinition() HostDefinition {
	return HostDefinition{PageOutput: true}
}

func TestValidateHostResultRejectsInvalidOutcomes(t *testing.T) {
	tests := []struct {
		name    string
		outcome string
		want    string
	}{
		{"empty outcome", "", "without an outcome"},
		{"unknown outcome", "partial", `unsupported outcome "partial"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostResult(HostDefinition{}, HostResult{Outcome: tt.outcome})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateHostResultRejectsInvalidPagination(t *testing.T) {
	tests := []struct {
		name       string
		definition HostDefinition
		pagination *HostPagination
		want       string
	}{
		{
			"undeclared pagination",
			HostDefinition{},
			&HostPagination{Complete: true, Pages: 1},
			"declares no Page output",
		},
		{
			"zero pages",
			pageDefinition(),
			&HostPagination{Complete: true, Pages: 0},
			"pagination pages 0",
		},
		{
			"negative items",
			pageDefinition(),
			&HostPagination{Complete: true, Pages: 1, Items: -1},
			"negative pagination items",
		},
		{
			"complete with next token",
			pageDefinition(),
			&HostPagination{Complete: true, Pages: 1, NextToken: "tok"},
			"complete page carrying a next token",
		},
		{
			"incomplete without next token",
			pageDefinition(),
			&HostPagination{Complete: false, Pages: 1},
			"incomplete page without a next token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HostResult{Outcome: string(outcomeSuccess), Pagination: tt.pagination}
			err := ValidateHostResult(tt.definition, result)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateHostResultAcceptsDeclaredResults(t *testing.T) {
	tests := []struct {
		name       string
		definition HostDefinition
		result     HostResult
	}{
		{
			"plain success",
			HostDefinition{},
			HostResult{Outcome: string(outcomeSuccess)},
		},
		{
			"complete page",
			pageDefinition(),
			HostResult{Outcome: string(outcomeSuccess), Pagination: &HostPagination{Complete: true, Pages: 2, Items: 7}},
		},
		{
			"incomplete page with cursor",
			pageDefinition(),
			HostResult{Outcome: string(outcomeSuccess), Pagination: &HostPagination{Pages: 1, Items: 3, NextToken: "tok"}},
		},
		{
			"pagination declared through Output.Meta",
			HostDefinition{Output: OutputDefinition{Meta: ResultMetaDefinition{Pagination: true}}},
			HostResult{Outcome: string(outcomeSuccess), Pagination: &HostPagination{Complete: true, Pages: 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateHostResult(tt.definition, tt.result); err != nil {
				t.Fatalf("ValidateHostResult() error = %v, want nil", err)
			}
		})
	}
}
