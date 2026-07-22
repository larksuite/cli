// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/beevik/etree"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// SlidesXMLGet fetches the full XML presentation content. When --output is
// provided it writes reindented XML to a local file, and --raw prints
// reindented XML to stdout; otherwise it returns the server's original
// content unmodified in the standard JSON envelope. Use --slide-id or
// --slide-number to fetch one page.
var SlidesXMLGet = common.Shortcut{
	Service:     "slides",
	Command:     "+xml-get",
	Description: "Fetch presentation XML or one slide XML",
	Risk:        "read",
	Scopes:      []string{"slides:presentation:read"},
	// wiki:node:read is required only when --presentation is a wiki URL.
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "presentation", Desc: "xml_presentation_id, slides URL, or wiki URL that resolves to slides", Required: true},
		{Name: "output", Desc: "local XML output path; the saved file is formatted for readability; must be a relative path within the current directory; existing file is overwritten; omit to return the server's original XML in the JSON envelope"},
		{Name: "raw", Type: "bool", Desc: "print formatted XML to stdout without the JSON envelope; incompatible with --output and --jq"},
		{Name: "slide-id", Desc: "slide page identifier; omit both slide selectors to fetch full presentation XML"},
		{Name: "slide-number", Type: "int", Desc: "1-based slide page number; omit both slide selectors to fetch full presentation XML"},
		{Name: "revision-id", Type: "int", Default: "-1", Desc: "presentation revision_id; -1 means latest"},
		{Name: "remove-attr-id", Type: "bool", Desc: "remove XML id attributes in the returned content; useful for read-only inspection, not precise block editing"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return err
		}
		if revisionID := runtime.Int("revision-id"); revisionID < -1 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--revision-id must be -1 or a non-negative integer").WithParam("--revision-id")
		}
		if ref.Kind == "wiki" {
			if err := runtime.EnsureScopes([]string{"wiki:node:read"}); err != nil {
				return err
			}
		}
		if err := validateSlidesXMLGetSelector(runtime); err != nil {
			return err
		}
		outputPath := strings.TrimSpace(runtime.Str("output"))
		if outputPath != "" {
			if _, err := runtime.ResolveSavePath(outputPath); err != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output invalid: %v", err).WithParam("--output").WithCause(err)
			}
		}
		if runtime.Bool("raw") {
			if outputPath != "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--raw cannot be used with --output").WithParam("--raw")
			}
			if runtime.JqExpr != "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--raw cannot be used with --jq").WithParam("--raw")
			}
			if runtime.Changed("format") && runtime.Format != "json" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--raw cannot be used with --format %s", runtime.Format).WithParam("--raw")
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		presentationID := ref.Token
		dry := common.NewDryRunAPI()
		if ref.Kind == "wiki" {
			presentationID = "<resolved_slides_token>"
			dry.Desc("2-step orchestration: resolve wiki → fetch presentation XML").
				GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Resolve wiki node to slides presentation").
				Params(map[string]interface{}{"token": ref.Token})
		} else {
			dry.Desc("Fetch presentation XML")
		}
		params := map[string]interface{}{
			"revision_id": runtime.Int("revision-id"),
		}
		slideID := strings.TrimSpace(runtime.Str("slide-id"))
		slideNumber := runtime.Int("slide-number")
		if slideID != "" {
			params["slide_id"] = slideID
		}
		if slideNumber > 0 {
			params["slide_number"] = slideNumber
		}
		if slideID == "" && slideNumber == 0 && runtime.Bool("remove-attr-id") {
			params["remove_attr_id"] = true
		}
		path := fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s", validate.EncodePathSegment(presentationID))
		if slideID != "" || slideNumber > 0 {
			path += "/slide"
		}
		dry.GET(path).Params(params)
		if outputPath := strings.TrimSpace(runtime.Str("output")); outputPath != "" {
			return dry.Set("output", outputPath).Set("stdout_content", "suppressed; formatted XML content is saved to --output during execution")
		}
		if runtime.Bool("raw") {
			return dry.Set("output", "<stdout>").Set("stdout_content", "formatted XML content is printed to stdout during execution")
		}
		return dry.Set("output", "<stdout>").Set("stdout_content", "JSON envelope with XML content is printed to stdout during execution")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return err
		}
		presentationID, err := resolvePresentationID(runtime, ref)
		if err != nil {
			return err
		}

		if err := validateSlidesXMLGetSelector(runtime); err != nil {
			return err
		}
		params := map[string]interface{}{
			"revision_id": runtime.Int("revision-id"),
		}

		slideID := strings.TrimSpace(runtime.Str("slide-id"))
		slideNumber := runtime.Int("slide-number")
		content, out, err := fetchSlidesXMLGetContent(runtime, presentationID, params, slideID, slideNumber)
		if err != nil {
			return err
		}
		outputPath := strings.TrimSpace(runtime.Str("output"))
		return outputSlidesXMLGetContent(runtime, content, outputPath, out)
	},
}

func validateSlidesXMLGetSelector(runtime *common.RuntimeContext) error {
	slideID := strings.TrimSpace(runtime.Str("slide-id"))
	slideNumber := runtime.Int("slide-number")
	if runtime.Changed("slide-id") && slideID == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide-id cannot be empty").WithParam("--slide-id")
	}
	if slideID != "" && slideNumber > 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide-id cannot be used with --slide-number").WithParam("--slide-id")
	}
	if runtime.Changed("slide-number") && slideNumber < 1 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide-number must be a positive integer").WithParam("--slide-number")
	}
	if (slideID != "" || slideNumber > 0) && runtime.Bool("remove-attr-id") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--remove-attr-id is only supported when fetching full presentation XML").WithParam("--remove-attr-id")
	}
	return nil
}

func fetchSlidesXMLGetContent(runtime *common.RuntimeContext, presentationID string, params map[string]interface{}, slideID string, slideNumber int) (string, map[string]interface{}, error) {
	if slideID != "" || slideNumber > 0 {
		if slideID != "" {
			params["slide_id"] = slideID
		}
		if slideNumber > 0 {
			params["slide_number"] = slideNumber
		}
		data, err := runtime.CallAPITyped(
			"GET",
			fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s/slide", validate.EncodePathSegment(presentationID)),
			params,
			nil,
		)
		if err != nil {
			return "", nil, err
		}
		slide := common.GetMap(data, "slide")
		content := common.GetString(slide, "content")
		if content == "" {
			return "", nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "slides xml get returned empty slide.content")
		}
		slideOut := map[string]interface{}{
			"content": content,
		}
		actualSlideID := common.GetString(slide, "slide_id")
		if actualSlideID == "" {
			actualSlideID = slideID
		}
		if actualSlideID != "" {
			slideOut["slide_id"] = actualSlideID
		}
		if slideNumber > 0 {
			slideOut["slide_number"] = slideNumber
		}
		out := map[string]interface{}{
			"xml_presentation_id": presentationID,
			"scope":               "slide",
			"slide":               slideOut,
		}
		if actualSlideID != "" {
			out["slide_id"] = actualSlideID
		}
		if slideNumber > 0 {
			out["slide_number"] = slideNumber
		}
		if revisionID := common.GetFloat(data, "revision_id"); revisionID > 0 {
			out["revision_id"] = int(revisionID)
			slideOut["revision_id"] = int(revisionID)
		}
		return content, out, nil
	}

	if runtime.Bool("remove-attr-id") {
		params["remove_attr_id"] = true
	}
	data, err := runtime.CallAPITyped(
		"GET",
		fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s", validate.EncodePathSegment(presentationID)),
		params,
		nil,
	)
	if err != nil {
		return "", nil, err
	}

	presentation := common.GetMap(data, "xml_presentation")
	content := common.GetString(presentation, "content")
	if content == "" {
		return "", nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "slides xml get returned empty xml_presentation.content")
	}
	presentationOut := map[string]interface{}{
		"content": content,
	}
	out := map[string]interface{}{
		"xml_presentation_id": presentationID,
		"scope":               "presentation",
		"xml_presentation":    presentationOut,
	}
	if revisionID := common.GetFloat(presentation, "revision_id"); revisionID > 0 {
		out["revision_id"] = int(revisionID)
		presentationOut["revision_id"] = int(revisionID)
	}
	if runtime.Bool("remove-attr-id") {
		out["remove_attr_id"] = true
	}
	return content, out, nil
}

// outputSlidesXMLGetContent routes the fetched XML to its output surface.
// Only the text surfaces are reindented: --raw stdout and --output files are
// read directly by humans and line tools. The JSON envelope carries the
// server content verbatim instead -- inside a JSON string every newline is
// escaped to \n, so formatting there buys no readability and only inflates
// the payload, while passthrough keeps that read path byte-exact without
// even parsing the content.
func outputSlidesXMLGetContent(runtime *common.RuntimeContext, content string, outputPath string, out map[string]interface{}) error {
	if outputPath == "" {
		if !runtime.Bool("raw") {
			runtime.OutFormatRaw(out, nil, nil)
			return nil
		}
		formatted, _ := prettyPrintXMLOrOriginal(runtime, content)
		if _, err := fmt.Fprint(runtime.IO().Out, formatted); err != nil {
			return errs.NewInternalError(errs.SubtypeFileIO, "write XML content to stdout: %v", err).WithCause(err)
		}
		return nil
	}

	formatted, prettyPrinted := prettyPrintXMLOrOriginal(runtime, content)
	result, err := runtime.FileIO().Save(outputPath, fileio.SaveOptions{
		ContentType:   "application/xml",
		ContentLength: int64(len(formatted)),
	}, bytes.NewReader([]byte(formatted)))
	if err != nil {
		return common.WrapSaveErrorTyped(err)
	}
	resolvedPath, err := runtime.ResolveSavePath(outputPath)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO, "resolve saved XML path %s: %v", outputPath, err).WithCause(err)
	}

	fileOut := map[string]interface{}{
		"xml_presentation_id": out["xml_presentation_id"],
		"scope":               out["scope"],
		"path":                resolvedPath,
		"size":                result.Size(),
		"content_saved":       true,
		"pretty_printed":      prettyPrinted,
	}
	for _, key := range []string{"revision_id", "remove_attr_id", "slide_id", "slide_number"} {
		if value, ok := out[key]; ok {
			fileOut[key] = value
		}
	}
	runtime.Out(fileOut, nil)
	return nil
}

// textBearingTags are the SML elements whose schema content model is
// mixed (arbitrary text interleaved with inline markup): the <p> paragraph
// container and its inline formatting children, plus chart title/subtitle.
// See slides_xml_schema_definition.xml, <p> element docs: a deliberate space
// or tab is represented via &#32;/&#9; character references. Those references
// are masked before XML parsing so etree cannot resolve away their lexical
// representation, and reindentStructural never descends into these elements.
var textBearingTags = map[string]bool{
	"p":             true,
	"strong":        true,
	"em":            true,
	"u":             true,
	"span":          true,
	"del":           true,
	"a":             true,
	"shadow":        true,
	"outline":       true,
	"chartTitle":    true,
	"chartSubTitle": true,
}

// prettyPrintXML reindents xmlContent so structural elements (presentation,
// slide, shape, style, ...) each sit on their own line. The server returns
// XML as a single unbroken line, and this is what makes the --raw and
// --output text surfaces readable; the JSON envelope path never calls it
// (see outputSlidesXMLGetContent).
//
// Reindentation never enters a textBearingTags element. XML whitespace
// character references are masked before parsing and restored after
// serialization, and CDATA sections are preserved rather than collapsed into
// escaped text. This covers the schema-documented space/tab references as well
// as CR/LF references: serializing a CR reference as a literal line ending
// would make a later XML parse normalize it to LF and break read-modify-write
// round trips.
func prettyPrintXML(xmlContent string) (string, error) {
	maskedContent, maskedReferences := maskSMLWhitespaceCharacterReferences(xmlContent)
	doc := etree.NewDocument()
	doc.ReadSettings.PreserveCData = true
	if err := doc.ReadFromString(maskedContent); err != nil {
		return "", err
	}
	if root := doc.Root(); root != nil {
		reindentStructural(root, 0)
	}
	out, err := doc.WriteToString()
	if err != nil {
		return "", err
	}
	out = restoreSMLWhitespaceCharacterReferences(out, maskedReferences)
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

// prettyPrintXMLOrOriginal keeps xml-get best-effort: if the server returns
// content that is not strictly valid XML, callers still receive the original
// content and a warning on stderr instead of losing the read path. The bool
// reports whether pretty-printing succeeded, surfaced as pretty_printed in
// --output file metadata.
func prettyPrintXMLOrOriginal(runtime *common.RuntimeContext, xmlContent string) (string, bool) {
	out, err := prettyPrintXML(xmlContent)
	if err != nil {
		fmt.Fprintf(runtime.IO().ErrOut, "warning: XML pretty-print skipped; returning original server content: %v\n", err)
		return xmlContent, false
	}
	return out, true
}

type maskedXMLCharacterReference struct {
	placeholder string
	reference   string
}

var numericXMLCharacterReferencePattern = regexp.MustCompile(`&#(?:[0-9]+|x[0-9A-Fa-f]+);`)

// maskSMLWhitespaceCharacterReferences protects XML whitespace character
// references (space, tab, CR, and LF) from etree's parse/write normalization.
// Each original spelling is restored exactly, including decimal, hexadecimal,
// and zero-padded forms. The placeholder prefix is chosen not to occur in the
// input, so restoring cannot rewrite user-authored content accidentally.
func maskSMLWhitespaceCharacterReferences(xmlContent string) (string, []maskedXMLCharacterReference) {
	placeholderPrefix := "LARKCLI_XML_WHITESPACE_REFERENCE_"
	for strings.Contains(xmlContent, placeholderPrefix) {
		placeholderPrefix += "_"
	}

	maskedReferences := make([]maskedXMLCharacterReference, 0)
	maskedContent := numericXMLCharacterReferencePattern.ReplaceAllStringFunc(xmlContent, func(reference string) string {
		digits := reference[2 : len(reference)-1]
		base := 10
		if strings.HasPrefix(digits, "x") {
			base = 16
			digits = digits[1:]
		}
		value, err := strconv.ParseUint(digits, base, 32)
		if err != nil {
			return reference
		}
		switch value {
		case ' ', '\t', '\r', '\n':
		default:
			return reference
		}

		placeholder := fmt.Sprintf("%s%d_", placeholderPrefix, len(maskedReferences))
		maskedReferences = append(maskedReferences, maskedXMLCharacterReference{
			placeholder: placeholder,
			reference:   reference,
		})
		return placeholder
	})
	return maskedContent, maskedReferences
}

func restoreSMLWhitespaceCharacterReferences(xmlContent string, maskedReferences []maskedXMLCharacterReference) string {
	for _, masked := range maskedReferences {
		xmlContent = strings.ReplaceAll(xmlContent, masked.placeholder, masked.reference)
	}
	return xmlContent
}

// reindentStructural inserts newline+indent whitespace between an element's
// direct children so each nested element sits on its own line, recursing
// only into children that are not textBearingTags. Existing whitespace-only
// CharData between children is treated as pre-existing formatting and
// dropped before reinserting it at the correct depth; non-whitespace CharData
// is never touched, and text-bearing subtrees are never entered at all.
func reindentStructural(e *etree.Element, depth int) {
	// A node without element children has no nested structure to reindent.
	// Leaving the whole leaf untouched also preserves whitespace-only xs:string
	// and simple-content values such as <title> and <chartField>, including
	// adjacent plain-text and CDATA nodes.
	if textBearingTags[e.Tag] || !hasElementChild(e) {
		return
	}

	for i := len(e.Child) - 1; i >= 0; i-- {
		if cd, ok := e.Child[i].(*etree.CharData); ok && isAllWhitespace(cd.Data) {
			e.RemoveChildAt(i)
		}
	}
	if len(e.Child) == 0 {
		return
	}

	_, lastIsCharData := e.Child[len(e.Child)-1].(*etree.CharData)
	childIndent := "\n" + strings.Repeat("  ", depth+1)
	closeIndent := "\n" + strings.Repeat("  ", depth)

	for i := len(e.Child) - 1; i >= 0; i-- {
		child := e.Child[i]
		if ce, ok := child.(*etree.Element); ok {
			reindentStructural(ce, depth+1)
		}
		if _, isCharData := child.(*etree.CharData); !isCharData {
			e.InsertChildAt(i, etree.NewCharData(childIndent))
		}
	}
	if !lastIsCharData {
		e.AddChild(etree.NewCharData(closeIndent))
	}
}

func hasElementChild(e *etree.Element) bool {
	for _, child := range e.Child {
		if _, ok := child.(*etree.Element); ok {
			return true
		}
	}
	return false
}

// isAllWhitespace reports whether s is non-empty and consists only of XML
// whitespace characters (space, tab, CR, LF).
func isAllWhitespace(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r':
		default:
			return false
		}
	}
	return true
}
