// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
)

func TestDocsWritePreservesServiceFailureDetails(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		args   []string
	}{
		{
			name:   "create",
			method: "POST",
			path:   "/open-apis/docs_ai/v1/documents",
			args:   []string{"+create", "--content", "<p>content</p>", "--as", "user"},
		},
		{
			name:   "update",
			method: "PUT",
			path:   "/open-apis/docs_ai/v1/documents/doxcn_failed",
			args:   []string{"+update", "--doc", "doxcn_failed", "--command", "append", "--content", "<p>content</p>", "--as", "user"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-result-"+tt.name))
			registerDocsAIStub(reg, tt.method, tt.path, map[string]interface{}{
				"result":   "failed",
				"warnings": []interface{}{"the target block is not replaceable"},
			})

			shortcut := DocsCreate
			if tt.name == "update" {
				shortcut = DocsUpdate
			}
			err := mountAndRunDocs(t, shortcut, tt.args, f, stdout)
			var partial *output.PartialFailureError
			if !errors.As(err, &partial) {
				t.Fatalf("error = %T %v, want *output.PartialFailureError", err, err)
			}

			var envelope struct {
				OK   bool `json:"ok"`
				Data struct {
					Result   string   `json:"result"`
					Warnings []string `json:"warnings"`
				} `json:"data"`
			}
			if decodeErr := json.Unmarshal(stdout.Bytes(), &envelope); decodeErr != nil {
				t.Fatalf("decode stdout: %v\n%s", decodeErr, stdout.String())
			}
			if envelope.OK || envelope.Data.Result != "failed" || len(envelope.Data.Warnings) != 1 ||
				envelope.Data.Warnings[0] != "the target block is not replaceable" {
				t.Fatalf("envelope = %+v", envelope)
			}
		})
	}
}
