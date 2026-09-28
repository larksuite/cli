// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

import "github.com/larksuite/cli/extension/citation"

// CitationDefinition declares the sources a read command may cite. Build runs
// only for a successful envelope, after content scanning. It must not make API
// calls, and all text must come from the final data that was scanned.
type CitationDefinition[Data any] struct {
	SourceTypes []citation.SourceType
	Build       func(Data) []citation.Citation
}

func bindCitation[Data any](citationDefinition *CitationDefinition[Data]) *CitationDefinition[any] {
	if citationDefinition == nil {
		return nil
	}
	hostCitation := &CitationDefinition[any]{SourceTypes: append([]citation.SourceType(nil), citationDefinition.SourceTypes...)}
	if buildCitations := citationDefinition.Build; buildCitations != nil {
		hostCitation.Build = func(data any) []citation.Citation {
			// The Execute binder preserves Data through the erased host result.
			// A nil interface is its zero value; typed nil pointers retain their type.
			var typedData Data
			if data != nil {
				typed, ok := data.(Data)
				if !ok {
					// A host result that is not the declared Data type is a
					// binder contract breach; fail closed to no citations.
					return nil
				}
				typedData = typed
			}
			return buildCitations(typedData)
		}
	}
	return hostCitation
}

func cloneHostCitation(citationDefinition *CitationDefinition[any]) *CitationDefinition[any] {
	if citationDefinition == nil {
		return nil
	}
	return &CitationDefinition[any]{
		SourceTypes: append([]citation.SourceType(nil), citationDefinition.SourceTypes...),
		Build:       citationDefinition.Build,
	}
}
