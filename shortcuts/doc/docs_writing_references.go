// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const docsWritingReferencesAPIPath = "/open-apis/docs_ai/v1/writing_references"

type docsWritingReferencesEnvelope struct {
	Result *string `json:"result"`
}

type docsWritingReferencesResult struct {
	EditedDocuments    []*docsWritingReference `json:"edited_documents"`
	TemplateConfigured *bool                   `json:"template_configured"`
	RecentDocuments    []*docsWritingReference `json:"recent_documents"`
	TemplateDocuments  []*docsWritingReference `json:"template_documents"`
}

type docsWritingReference struct {
	DocumentID string  `json:"document_id"`
	Title      string  `json:"title"`
	URL        string  `json:"url"`
	Content    *string `json:"content"`
}

func validateDocsWritingReferences(runtime *common.RuntimeContext) error {
	for _, name := range []string{"content", "doc", "presentation-decision"} {
		if runtime.Str(name) != "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--%s is not supported with --command writing-references", name).WithParam("--" + name)
		}
	}
	if runtime.As().IsBot() {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--command writing-references requires user identity").WithParam("--as").WithHint("Use --as user.")
	}
	return runtime.EnsureScopes([]string{"docx:document:readonly"})
}

func executeDocsWritingReferences(runtime *common.RuntimeContext) error {
	data, err := doDocAPI(runtime, "POST", docsWritingReferencesAPIPath, struct{}{})
	if err != nil {
		return err
	}
	raw, result, err := projectDocsWritingReferences(data)
	if err != nil {
		return err
	}
	// Keep the server's field presence and future extensions intact. In particular,
	// do not invent content for metadata-only entries or drop explicitly empty XML.
	runtime.OutFormatRaw(raw, &output.Meta{Count: len(result.RecentDocuments)}, func(w io.Writer) {
		printDocsWritingReferences(w, "Recent documents", result.RecentDocuments)
		if result.EditedDocuments != nil {
			printDocsWritingReferences(w, "Edited documents", result.EditedDocuments)
		}
		printDocsWritingReferences(w, "Template documents", result.TemplateDocuments)
	})
	return runtime.OutputError()
}

func projectDocsWritingReferences(data map[string]interface{}) (map[string]interface{}, docsWritingReferencesResult, error) {
	var result docsWritingReferencesResult
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, result, errs.NewInternalError(errs.SubtypeInvalidResponse, "encode writing references response").WithCause(err)
	}
	var envelope docsWritingReferencesEnvelope
	if err = json.Unmarshal(encoded, &envelope); err != nil {
		return nil, result, errs.NewInternalError(errs.SubtypeInvalidResponse, "decode writing references response").WithCause(err)
	}
	if envelope.Result == nil {
		return nil, result, errs.NewInternalError(errs.SubtypeInvalidResponse, "writing references response is missing result")
	}
	raw := json.RawMessage(*envelope.Result)
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, result, errs.NewInternalError(errs.SubtypeInvalidResponse, "writing references result must be a JSON object").WithCause(err)
	}
	if result.TemplateConfigured == nil || result.RecentDocuments == nil || result.TemplateDocuments == nil {
		return nil, result, errs.NewInternalError(errs.SubtypeInvalidResponse, "writing references result is missing its configuration flag or document arrays")
	}
	for _, documents := range [][]*docsWritingReference{result.RecentDocuments, result.EditedDocuments, result.TemplateDocuments} {
		for _, document := range documents {
			if document == nil {
				return nil, result, errs.NewInternalError(errs.SubtypeInvalidResponse, "writing references contains a null document")
			}
		}
	}
	// Fully decode string escapes for readable XML while retaining arbitrary
	// extension fields and the precision of server-provided integer values.
	decoder := json.NewDecoder(strings.NewReader(*envelope.Result))
	decoder.UseNumber()
	var payload map[string]interface{}
	if err := decoder.Decode(&payload); err != nil {
		return nil, result, errs.NewInternalError(errs.SubtypeInvalidResponse, "decode writing references payload").WithCause(err)
	}
	return payload, result, nil
}

func printDocsWritingReferences(w io.Writer, label string, documents []*docsWritingReference) {
	fmt.Fprintf(w, "%s (%d)\n", label, len(documents))
	for i, document := range documents {
		fmt.Fprintf(w, "%d. %s\n   %s\n", i+1, document.Title, document.URL)
		if document.Content != nil {
			fmt.Fprintf(w, "content:\n%s\n", *document.Content)
		}
	}
}
