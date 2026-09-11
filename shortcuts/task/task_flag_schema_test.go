// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"encoding/json"
	"testing"

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

	nested, err := taskUpdateDataFlagSchema(catalog, "data.due.timestamp")
	if err != nil {
		t.Fatalf("taskUpdateDataFlagSchema(data.due.timestamp) error = %v", err)
	}
	var timestampSchema map[string]interface{}
	if err := json.Unmarshal(nested, &timestampSchema); err != nil {
		t.Fatalf("decode nested schema: %v\n%s", err, nested)
	}
	if timestampSchema["type"] != "string" {
		t.Fatalf("nested schema = %#v, want string", timestampSchema)
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
	if _, err := taskUpdateDataFlagSchema(catalog, "unknown"); err == nil {
		t.Fatal("taskUpdateDataFlagSchema(unknown) error = nil, want validation error")
	}
}
