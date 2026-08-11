// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

type protocolArtifact struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	MediaType string `json:"media_type"`
}
type protocolData struct {
	Artifacts []protocolArtifact `json:"artifacts"`
	Failures  []compilerItem     `json:"failures"`
}

func TestValidateTypedResultProtocolArtifactAndPartial(t *testing.T) {
	command := &compiledCommand{output: OutputDefinition{
		Artifacts: []ArtifactDefinition{{Name: "files", ItemsPath: "/artifacts", PathField: "/path", SizeField: "/size", MediaTypeField: "/media_type"}},
		Outcomes:  OutcomeDefinition{PartialFailure: &PartialFailureDefinition{ExitCode: 7, FailedItems: &FailedItemDefinition{ItemsPath: "/failures", IdentityPaths: []string{"/id"}, StatePath: "/state", FailedValues: []JSONValue{"failed"}}}},
	}}
	result := compiledResult{outcome: OutcomePartial, data: protocolData{
		Artifacts: []protocolArtifact{{Path: "artifacts/artifact.bin", Size: 3, MediaType: "application/octet-stream"}},
		Failures:  []compilerItem{{ID: "item-1", State: "failed"}},
	}}
	if err := validateTypedResultProtocol(command, result); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTypedResultMetaContract(t *testing.T) {
	count := 2
	validComplete := &ResultPaginationMeta{Complete: true, Pages: 1, Items: 0}
	validIncomplete := &ResultPaginationMeta{Complete: false, Pages: 2, Items: 3, NextToken: "next"}
	for _, meta := range []*ResultMeta{
		{Count: &count},
		{Pagination: validComplete},
		{Count: &count, Pagination: validIncomplete},
	} {
		if err := validateTypedResultMeta(ResultMetaDefinition{Count: true, Pagination: true}, meta); err != nil {
			t.Fatalf("valid meta %#v: %v", meta, err)
		}
	}

	negative := -1
	tests := []struct {
		name       string
		definition ResultMetaDefinition
		meta       *ResultMeta
		want       string
	}{
		{name: "empty", definition: ResultMetaDefinition{Count: true}, meta: &ResultMeta{}, want: "Meta is empty"},
		{name: "undeclared count", meta: &ResultMeta{Count: &count}, want: "undeclared meta.count"},
		{name: "negative count", definition: ResultMetaDefinition{Count: true}, meta: &ResultMeta{Count: &negative}, want: "count must be non-negative"},
		{name: "undeclared pagination", meta: &ResultMeta{Pagination: validComplete}, want: "undeclared meta.pagination"},
		{name: "zero pages", definition: ResultMetaDefinition{Pagination: true}, meta: &ResultMeta{Pagination: &ResultPaginationMeta{Complete: true}}, want: "pages must be at least 1"},
		{name: "negative items", definition: ResultMetaDefinition{Pagination: true}, meta: &ResultMeta{Pagination: &ResultPaginationMeta{Complete: true, Pages: 1, Items: -1}}, want: "items must be non-negative"},
		{name: "complete with token", definition: ResultMetaDefinition{Pagination: true}, meta: &ResultMeta{Pagination: &ResultPaginationMeta{Complete: true, Pages: 1, NextToken: "next"}}, want: "must not include next_token"},
		{name: "incomplete without token", definition: ResultMetaDefinition{Pagination: true}, meta: &ResultMeta{Pagination: &ResultPaginationMeta{Pages: 1}}, want: "must include next_token"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTypedResultMeta(test.definition, test.meta)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryInternal || !strings.Contains(problem.Message, test.want) {
				t.Fatalf("error = %#v, problem = %#v, want containing %q", err, problem, test.want)
			}
		})
	}
}

func TestOutputMetaFromTypedClonesPagination(t *testing.T) {
	count := 4
	pagination := &ResultPaginationMeta{Complete: false, Pages: 1, Items: 4, NextToken: "next"}
	meta := &ResultMeta{Count: &count, Pagination: pagination}
	converted := outputMetaFromTyped(meta)
	count = 9
	pagination.NextToken = "mutated"
	if converted.Count != 4 || converted.Pagination == nil || converted.Pagination.NextToken != "next" {
		t.Fatalf("converted meta was mutated through caller pointers: %#v", converted)
	}
}

func TestValidateTypedResultProtocolRejectsInvalidArtifactReceipt(t *testing.T) {
	command := &compiledCommand{output: OutputDefinition{Artifacts: []ArtifactDefinition{{Name: "files", ItemsPath: "/artifacts", PathField: "/path", SizeField: "/size"}}}}
	result := compiledResult{outcome: OutcomeSuccess, data: protocolData{Artifacts: []protocolArtifact{{Path: "", Size: -1}}}}
	err := validateTypedResultProtocol(command, result)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || !strings.Contains(problem.Message, "invalid path receipt") {
		t.Fatalf("error = %#v, problem = %#v", err, problem)
	}
}

func TestValidateTypedResultProtocolOptionalArtifact(t *testing.T) {
	command := &compiledCommand{output: OutputDefinition{Artifacts: []ArtifactDefinition{{Name: "file", ItemsPath: "/artifact", Optional: true, PathField: "/path", SizeField: "/size", MediaTypeField: "/media_type"}}}}
	for _, data := range []any{map[string]any{}, map[string]any{"artifact": nil}} {
		if err := validateTypedResultProtocol(command, compiledResult{outcome: OutcomeSuccess, data: data}); err != nil {
			t.Fatalf("optional artifact data %#v: %v", data, err)
		}
	}
	valid := map[string]any{"artifact": map[string]any{"path": "file.bin", "size": 3, "media_type": "application/octet-stream"}}
	if err := validateTypedResultProtocol(command, compiledResult{outcome: OutcomeSuccess, data: valid}); err != nil {
		t.Fatalf("present valid optional artifact: %v", err)
	}
	for _, test := range []struct {
		name string
		data map[string]any
		want string
	}{
		{name: "path", data: map[string]any{"artifact": map[string]any{"path": "", "size": 3, "media_type": "application/octet-stream"}}, want: "invalid path receipt"},
		{name: "size", data: map[string]any{"artifact": map[string]any{"path": "file.bin", "size": -1, "media_type": "application/octet-stream"}}, want: "invalid size receipt"},
		{name: "media type", data: map[string]any{"artifact": map[string]any{"path": "file.bin", "size": 3, "media_type": 7}}, want: "non-string media type receipt"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateTypedResultProtocol(command, compiledResult{outcome: OutcomeSuccess, data: test.data})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestValidateTypedResultProtocolRequiredArtifactRejectsMissingReceipt(t *testing.T) {
	command := &compiledCommand{output: OutputDefinition{Artifacts: []ArtifactDefinition{{Name: "file", ItemsPath: "/artifact", PathField: "/path"}}}}
	for _, data := range []any{map[string]any{}, map[string]any{"artifact": nil}} {
		err := validateTypedResultProtocol(command, compiledResult{outcome: OutcomeSuccess, data: data})
		problem, ok := errs.ProblemOf(err)
		if !ok || problem.Category != errs.CategoryInternal || !strings.Contains(problem.Message, "is missing from Data") {
			t.Fatalf("required artifact data %#v: error = %#v, problem = %#v", data, err, problem)
		}
	}
}

func TestValidateTypedResultProtocolAcceptsResultLevelPartial(t *testing.T) {
	command := &compiledCommand{output: OutputDefinition{Outcomes: OutcomeDefinition{PartialFailure: &PartialFailureDefinition{ExitCode: 7}}}}
	data := map[string]any{"resource_id": "resource-1", "reason": "follow-up write failed"}
	if err := validateTypedResultProtocol(command, compiledResult{outcome: OutcomePartial, data: data}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTypedResultProtocolRejectsEmptyPartial(t *testing.T) {
	command := &compiledCommand{output: OutputDefinition{Outcomes: OutcomeDefinition{PartialFailure: &PartialFailureDefinition{ExitCode: 7, FailedItems: &FailedItemDefinition{ItemsPath: "/failures", AllItems: true}}}}}
	err := validateTypedResultProtocol(command, compiledResult{outcome: OutcomePartial, data: protocolData{Failures: []compilerItem{}}})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || !strings.Contains(problem.Message, "no failed items") {
		t.Fatalf("error = %#v, problem = %#v", err, problem)
	}
}
