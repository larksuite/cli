// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"strings"
	"testing"
)

func TestValidateTypedResultMetaContract(t *testing.T) {
	validIncomplete := &typedResultPaginationMeta{Pages: 2, Items: 3, NextToken: "next"}
	validComplete := &typedResultPaginationMeta{Complete: true, Pages: 1, Items: 0}
	for _, meta := range []*typedResultMeta{
		{Pagination: validIncomplete},
		{Pagination: validComplete},
	} {
		if err := validateTypedResultMeta(typedResultMetaDefinition{Pagination: true}, meta); err != nil {
			t.Fatalf("valid meta %#v rejected: %v", meta, err)
		}
	}

	tests := []struct {
		name       string
		definition typedResultMetaDefinition
		meta       *typedResultMeta
		want       string
	}{
		{name: "empty", definition: typedResultMetaDefinition{Pagination: true}, meta: &typedResultMeta{}, want: "Meta is empty"},
		{name: "undeclared", meta: &typedResultMeta{Pagination: validIncomplete}, want: "undeclared meta.pagination"},
		{name: "zero pages", definition: typedResultMetaDefinition{Pagination: true}, meta: &typedResultMeta{Pagination: &typedResultPaginationMeta{}}, want: "pages must be at least 1"},
		{name: "negative items", definition: typedResultMetaDefinition{Pagination: true}, meta: &typedResultMeta{Pagination: &typedResultPaginationMeta{Pages: 1, Items: -1, NextToken: "next"}}, want: "items must be non-negative"},
		{name: "complete token", definition: typedResultMetaDefinition{Pagination: true}, meta: &typedResultMeta{Pagination: &typedResultPaginationMeta{Complete: true, Pages: 1, NextToken: "next"}}, want: "must not include next_token"},
		{name: "incomplete missing token", definition: typedResultMetaDefinition{Pagination: true}, meta: &typedResultMeta{Pagination: &typedResultPaginationMeta{Pages: 1}}, want: "must include next_token"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTypedResultMeta(test.definition, test.meta)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestOutputMetaFromTypedClonesPagination(t *testing.T) {
	pagination := &typedResultPaginationMeta{Complete: false, Pages: 2, Items: 7, NextToken: "next"}
	meta := &typedResultMeta{Pagination: pagination}
	converted := outputMetaFromTyped(meta)
	pagination.NextToken = "mutated"
	if converted.Pagination == nil || converted.Pagination.NextToken != "next" {
		t.Fatalf("converted meta = %#v", converted)
	}
}
