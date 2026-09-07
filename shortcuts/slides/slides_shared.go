// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// Shared helpers for the whole-page slides commands: used by +add-slide,
// +delete-slide, +replace-slide, and +replace-pages.

// validateCompleteSlideXML checks that content is exactly one complete <slide>
// document: a single <slide> root, nothing but whitespace outside it, and every
// element closed. It reports the structural problem alone — callers re-tag the
// error with the flag it came from.
func validateCompleteSlideXML(content string) error {
	dec := xml.NewDecoder(strings.NewReader(content))
	depth := 0
	seenRoot := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 0 {
				if seenRoot {
					return invalidSlideXMLStructureError("multiple root elements")
				}
				if t.Name.Local != "slide" {
					return invalidSlideXMLStructureError("root element is <%s>, want <slide>", t.Name.Local)
				}
				seenRoot = true
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.ProcInst:
			// An `<?xml ...?>` prolog copied from a generic XML sample is
			// well-formed, so nothing local used to object and it reached the
			// backend, which answers 4001000 buildSnNode once the presentation
			// already exists. Every caller of this validator posts to
			// .../slide, and that is the endpoint that rejects it. Measured, so
			// it does not get "made consistent" later: .../slide/replace takes
			// the same prolog and applies the content, which is why
			// +update-slide and +replace-slide stay permissive.
			return invalidSlideXMLStructureError("<?%s ...?> declaration is not supported; remove it so the document starts with <slide>", t.Target)
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(t)) != "" {
				return invalidSlideXMLStructureError("non-whitespace text outside root element")
			}
		}
	}
	if !seenRoot {
		return invalidSlideXMLStructureError("missing root element")
	}
	if depth != 0 {
		return invalidSlideXMLStructureError("unclosed XML element")
	}
	return nil
}

func invalidSlideXMLStructureError(format string, args ...interface{}) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...)
}

// revisionFromData extracts revision_id from a response payload when present.
func revisionFromData(data map[string]interface{}) (int, bool) {
	if _, ok := data["revision_id"]; !ok {
		return 0, false
	}
	return int(common.GetFloat(data, "revision_id")), true
}
