// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import "strings"

type inlineAttachmentElement struct {
	name string
	end  int
}

// prepareInlineDocAttachments isolates each paragraph attachment from following
// text. DocsAI otherwise accepts the XML but drops siblings after <source>.
// Insert spans in the original bytes so attributes, entities and namespaces are
// not reserialized. Other resource formats keep their existing preparation path.
func prepareInlineDocAttachments(format, content string) string {
	if strings.TrimSpace(format) == "markdown" || !strings.Contains(content, "<source") {
		return content
	}
	var out strings.Builder
	var stack []inlineAttachmentElement
	written := 0
	for offset := 0; offset < len(content); {
		relative := strings.IndexByte(content[offset:], '<')
		if relative < 0 {
			break
		}
		start := offset + relative
		if end, ok := inlineAttachmentInertEnd(content, start); ok {
			offset = end
			continue
		}
		end := findXMLStartTagEnd(content, start)
		if end < 0 {
			return content
		}
		raw := content[start+1 : end-1]
		closing := strings.HasPrefix(raw, "/")
		name := strings.TrimPrefix(raw, "/")
		if boundary := strings.IndexAny(name, " \t\r\n/"); boundary >= 0 {
			name = name[:boundary]
		}
		if closing {
			if name == "br" || name == "hr" || name == "img" || name == "col" {
				offset = end
				continue
			}
			if len(stack) == 0 || stack[len(stack)-1].name != name {
				return content
			}
			stack = stack[:len(stack)-1]
			offset = end
			continue
		}
		selfClosing := strings.HasSuffix(strings.TrimSpace(raw), "/")
		if name == "source" {
			if !selfClosing {
				closeAt := end
				for closeAt < len(content) {
					if isXMLSpace(content[closeAt]) {
						closeAt++
						continue
					}
					if strings.HasPrefix(content[closeAt:], "<!--") || strings.HasPrefix(content[closeAt:], "<![CDATA[") {
						if inertEnd, ok := inlineAttachmentInertEnd(content, closeAt); ok {
							closeAt = inertEnd
							continue
						}
					}
					break
				}
				closeEnd := findXMLStartTagEnd(content, closeAt)
				if closeEnd < 0 || !strings.HasPrefix(content[closeAt:], "</") ||
					strings.TrimSpace(content[closeAt+2:closeEnd-1]) != "source" {
					return content
				}
				end = closeEnd
			}
			if inlineAttachmentInParagraph(stack) && !inlineAttachmentHasOwnSpan(content, start, end, stack) {
				out.WriteString(content[written:start])
				out.WriteString("<span>")
				out.WriteString(content[start:end])
				out.WriteString("</span>")
				written = end
			}
		} else if !selfClosing && name != "br" && name != "hr" && name != "img" && name != "col" {
			stack = append(stack, inlineAttachmentElement{name: name, end: end})
		}
		offset = end
	}
	if len(stack) != 0 || written == 0 {
		return content
	}
	out.WriteString(content[written:])
	return out.String()
}

// inlineAttachmentInParagraph excludes card/preview figures even when nested
// under a paragraph; their sources already have a block resource boundary.
func inlineAttachmentInParagraph(stack []inlineAttachmentElement) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].name == "figure" {
			return false
		}
		if stack[i].name == "p" {
			return true
		}
	}
	return false
}

// inlineAttachmentHasOwnSpan keeps preparation idempotent without treating a
// span containing additional siblings as an attachment boundary.
func inlineAttachmentHasOwnSpan(content string, start, end int, stack []inlineAttachmentElement) bool {
	return len(stack) > 0 && stack[len(stack)-1].name == "span" &&
		stack[len(stack)-1].end == start && strings.HasPrefix(content[end:], "</span>")
}

// inlineAttachmentInertEnd skips literal markup and embedded resource bodies.
// Those bodies may contain non-XML DSL, including examples of <p>/<source>.
func inlineAttachmentInertEnd(content string, start int) (int, bool) {
	for _, delimiters := range [][2]string{{"<!--", "-->"}, {"<![CDATA[", "]]>"}, {"<?", "?>"}} {
		if strings.HasPrefix(content[start:], delimiters[0]) {
			if end := strings.Index(content[start+len(delimiters[0]):], delimiters[1]); end >= 0 {
				return start + len(delimiters[0]) + end + len(delimiters[1]), true
			}
			return len(content), true
		}
	}
	if end, ok := findMarkdownRawHTMLInertEnd(content, start); ok {
		return end, true
	}
	for _, name := range []string{"code", "whiteboard", "html5-block"} {
		nameEnd := start + 1 + len(name)
		if nameEnd >= len(content) || content[start+1:nameEnd] != name || !isLocalDocResourceHTMLTagBoundary(content[nameEnd]) {
			continue
		}
		end := findXMLStartTagEnd(content, start)
		if end < 0 {
			return len(content), true
		}
		if strings.HasSuffix(strings.TrimSpace(content[start:end]), "/>") {
			return end, true
		}
		closing := "</" + name
		for end < len(content) {
			relative := strings.Index(content[end:], closing)
			if relative < 0 {
				break
			}
			closeStart := end + relative
			boundary := closeStart + len(closing)
			if boundary < len(content) && isLocalDocResourceHTMLTagBoundary(content[boundary]) {
				if closeEnd := findXMLStartTagEnd(content, closeStart); closeEnd >= 0 {
					return closeEnd, true
				}
				break
			}
			end = boundary
		}
		return len(content), true
	}
	return 0, false
}
