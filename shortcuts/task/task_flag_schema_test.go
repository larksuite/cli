// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/meta"
)

func TestTaskUpdateDataFlagSchemaProjectsTaskInput(t *testing.T) {
	catalog := apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{
		{
			Name: "task",
			Resources: map[string]meta.Resource{
				"tasks": {
					Methods: map[string]meta.Method{
						"patch": {
							RequestBody: map[string]meta.Field{
								"task": {
									Type: "object",
									Properties: map[string]meta.Field{
										"summary": {Type: "string"},
										"due": {
											Type: "object",
											Properties: map[string]meta.Field{
												"timestamp": {Type: "string"},
											},
										},
										"custom_fields": {
											Type: "array",
											Properties: map[string]meta.Field{
												"guid": {Type: "string"},
											},
										},
									},
								},
								"update_fields": {Type: "array", Required: true},
							},
						},
					},
				},
			},
		},
	})

	raw, err := taskUpdateDataFlagSchema(catalog, "data")
	if err != nil {
		t.Fatalf("taskUpdateDataFlagSchema(data) error = %v", err)
	}
	var schema struct {
		Type       string                 `json:"type"`
		Properties map[string]interface{} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode schema: %v\n%s", err, raw)
	}
	if schema.Type != "object" || schema.Properties["summary"] == nil || schema.Properties["update_fields"] != nil {
		t.Fatalf("projected schema = %#v, want task object only", schema)
	}

	for _, flagName := range []string{"data.due.timestamp", "data.custom_fields.guid"} {
		t.Run(flagName, func(t *testing.T) {
			nested, err := taskUpdateDataFlagSchema(catalog, flagName)
			if err != nil {
				t.Fatalf("taskUpdateDataFlagSchema(%s) error = %v", flagName, err)
			}
			var nestedSchema map[string]interface{}
			if err := json.Unmarshal(nested, &nestedSchema); err != nil {
				t.Fatalf("decode nested schema: %v\n%s", err, nested)
			}
			if nestedSchema["type"] != "string" {
				t.Fatalf("nested schema = %#v, want string", nestedSchema)
			}
		})
	}
}

func TestTaskUpdateDataFlagSchemaListsAndValidatesFlag(t *testing.T) {
	catalog := apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{
		{Name: "task", Resources: map[string]meta.Resource{
			"tasks": {Methods: map[string]meta.Method{
				"patch": {RequestBody: map[string]meta.Field{"task": {Type: "object"}}},
			}},
		}},
	})

	listed, err := taskUpdateDataFlagSchema(catalog, "")
	if err != nil {
		t.Fatalf("taskUpdateDataFlagSchema(list) error = %v", err)
	}
	if string(listed) == "" {
		t.Fatal("taskUpdateDataFlagSchema(list) returned empty output")
	}
	_, err = taskUpdateDataFlagSchema(catalog, "unknown")
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("error = %T %v, want typed invalid-argument error", err, err)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Param != "--flag-name" {
		t.Fatalf("error param = %#v, want --flag-name", validationErr)
	}
}

func TestPrintTaskUpdateDataFlagSchemaUsesEmbeddedCatalog(t *testing.T) {
	raw, err := printTaskUpdateDataFlagSchema("data.due.timestamp")
	if err != nil {
		t.Fatalf("printTaskUpdateDataFlagSchema(data.due.timestamp) error = %v", err)
	}
	var schema struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode schema: %v\n%s", err, raw)
	}
	if schema.Type != "string" {
		t.Fatalf("embedded task due timestamp schema type = %q, want string", schema.Type)
	}
}
