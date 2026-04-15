// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signature

import (
	"encoding/json"
	"regexp"
	"strings"
)

// variableMetaProps represents the JSON structure in data-variable-meta-props attributes.
type variableMetaProps struct {
	ID          string `json:"id"`
	Type        string `json:"type"`        // "text" or "image"
	DisplayName string `json:"displayName"` // human-readable label
	Width       string `json:"width"`       // image width (for type=image)
	Style       string `json:"style"`       // CSS style
	Circle      bool   `json:"circle"`      // circular image
}

// variableSpanRe matches <span data-variable-meta-props='...'> and captures the JSON and inner content.
// Group 1: JSON attribute value (double-quoted), Group 2: (single-quoted), Group 3: inner content.
//
// Limitation: uses regex instead of DOM parsing (Go has no built-in DOMParser like JS).
// If a variable <span> contains nested <span> tags, [\s\S]*? will match to the
// innermost </span>, potentially truncating content. In practice, Lark's signature
// templates do not nest <span> inside variable spans (verified against mail-editor
// source and test data). If this becomes an issue, consider using golang.org/x/net/html.
var variableSpanRe = regexp.MustCompile(
	`<span\s+data-variable-meta-props=(?:"([^"]*?)"|'([^']*?)')>([\s\S]*?)</span>`)

// InterpolateTemplate replaces template variables in a TENANT signature's content HTML.
// For USER signatures (no template variables), it returns sig.Content unchanged.
//
// Parameters:
//   - sig: the signature object
//   - lang: language code for i18n ("zh_cn", "en_us", "ja_jp")
//   - senderName: sender display name (overrides B-NAME)
//   - senderEmail: sender email address (overrides B-ENTERPRISE-EMAIL)
func InterpolateTemplate(sig *Signature, lang, senderName, senderEmail string) string {
	if !sig.HasTemplateVars() {
		return sig.Content
	}

	// Build value map from user_fields with i18n resolution.
	valueMap := make(map[string]string, len(sig.UserFields)+2)
	for key, field := range sig.UserFields {
		valueMap[key] = field.Resolve(lang)
	}

	// Fixed injections override API values.
	if senderName != "" {
		valueMap["B-NAME"] = senderName
	}
	if senderEmail != "" {
		valueMap["B-ENTERPRISE-EMAIL"] = senderEmail
	}

	// Replace each <span data-variable-meta-props='...'> with interpolated content.
	result := variableSpanRe.ReplaceAllStringFunc(sig.Content, func(match string) string {
		submatches := variableSpanRe.FindStringSubmatch(match)
		if submatches == nil {
			return match
		}

		// JSON is in group 1 (double-quoted) or group 2 (single-quoted).
		attrJSON := submatches[1]
		if attrJSON == "" {
			attrJSON = submatches[2]
		}

		// Unescape HTML entities in the JSON attribute value.
		attrJSON = unescapeHTMLEntities(attrJSON)

		var meta variableMetaProps
		if err := json.Unmarshal([]byte(attrJSON), &meta); err != nil {
			return match // preserve original on parse failure
		}

		val, ok := valueMap[meta.ID]
		if !ok {
			val = "" // variable not in map, replace with empty
		}

		switch meta.Type {
		case "text":
			return interpolateText(val, meta.Style)
		case "image":
			return interpolateImage(val, meta)
		default:
			return val
		}
	})

	return result
}

// interpolateText returns the replacement for a text variable.
func interpolateText(val, style string) string {
	if val == "" {
		return ""
	}
	// If value looks like a URL, wrap in <a>.
	if isURL(val) {
		escaped := escapeHTML(val)
		return `<a href="` + escaped + `" target="_blank" rel="noopener noreferrer">` + escaped + `</a>`
	}
	if style != "" {
		return `<span style="` + escapeHTML(style) + `">` + escapeHTML(val) + `</span>`
	}
	return escapeHTML(val)
}

// interpolateImage returns the replacement for an image variable.
func interpolateImage(val string, meta variableMetaProps) string {
	if val == "" {
		return ""
	}
	var attrs []string
	attrs = append(attrs, `src="`+escapeHTML(val)+`"`)
	if meta.Width != "" {
		attrs = append(attrs, `width="`+escapeHTML(meta.Width)+`"`)
	}
	var styles []string
	if meta.Style != "" {
		styles = append(styles, meta.Style)
	}
	if meta.Circle {
		styles = append(styles, "border-radius: 100%")
	}
	if len(styles) > 0 {
		attrs = append(attrs, `style="`+escapeHTML(strings.Join(styles, ";"))+`"`)
	}
	return `<img ` + strings.Join(attrs, " ") + `>`
}

func isURL(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

func unescapeHTMLEntities(s string) string {
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	return s
}
