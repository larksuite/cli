// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package publiccontent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/larksuite/cli/internal/core"
)

var (
	catalogEmailRE                  = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	catalogInternalHostRE           = regexp.MustCompile(`(?i)\b(?:[a-z0-9-]+\.)+(?:internal|corp|boe|byted\.org)\b`)
	catalogInstructionOverrideRE    = regexp.MustCompile(`(?i)\b(?:ignore|disregard)\s+(?:(?:all|any|the)\s+)?(?:prior|previous|earlier)\s+instructions\b`)
	catalogSystemPromptDisclosureRE = regexp.MustCompile(`(?i)\b(?:reveal|show|display|print|expose|return)\s+(?:(?:the|your)\s+)?system\s+prompt\b`)
)

func isCatalogFile(file string) bool {
	normalized := strings.TrimPrefix(strings.ReplaceAll(file, "\\", "/"), "./")
	if normalized == "internal/registry/catalog/manifest.json" {
		return true
	}
	return strings.HasPrefix(normalized, "internal/registry/catalog/services/") &&
		strings.HasSuffix(normalized, ".json")
}

func scanCatalogSafety(file, text string) []Finding {
	root, err := parseCatalogJSON([]byte(text))
	if err == nil {
		var out []Finding
		scanCatalogJSONNode(file, root, "", "", "", &out)
		return out
	}
	return scanCatalogRawSafety(file, text)
}

func scanCatalogRawSafety(file, text string) []Finding {
	var out []Finding
	unsafeEmails := catalogUnsafeEmails(text)
	for i, line := range strings.Split(text, "\n") {
		lineNo := i + 1
		for _, email := range catalogEmailRE.FindAllString(line, -1) {
			if unsafeEmails[strings.ToLower(email)] {
				out = append(out, newFinding(
					"public_content_catalog_pii", file, lineNo, "file", "email address",
				))
			}
		}
		if catalogInternalHostRE.MatchString(line) {
			out = append(out, newFinding(
				"public_content_catalog_internal_host", file, lineNo, "file", "internal host name",
			))
		}
		if catalogPromptInjectionMarker(line) {
			out = append(out, newFinding(
				"public_content_catalog_prompt_injection", file, lineNo, "file", "prompt-injection marker",
			))
		}
	}
	return out
}

type catalogJSONNode struct {
	key      string
	keyLine  int
	path     string
	line     int
	text     string
	children []*catalogJSONNode
	object   bool
	array    bool
}

func parseCatalogJSON(data []byte) (*catalogJSONNode, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	root, err := parseCatalogJSONValue(decoder, data, "$")
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("catalog JSON contains more than one value")
		}
		return nil, err
	}
	return root, nil
}

func parseCatalogJSONValue(decoder *json.Decoder, data []byte, path string) (*catalogJSONNode, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	node := &catalogJSONNode{path: path}
	switch typed := token.(type) {
	case json.Delim:
		switch typed {
		case '{':
			node.object = true
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("catalog JSON object key is not a string")
				}
				keyLine := catalogJSONLine(data, decoder.InputOffset())
				child, err := parseCatalogJSONValue(decoder, data, catalogJSONPropertyPath(path, key))
				if err != nil {
					return nil, err
				}
				child.key = key
				child.keyLine = keyLine
				node.children = append(node.children, child)
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
		case '[':
			node.array = true
			for index := 0; decoder.More(); index++ {
				child, err := parseCatalogJSONValue(decoder, data, fmt.Sprintf("%s[%d]", path, index))
				if err != nil {
					return nil, err
				}
				node.children = append(node.children, child)
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unexpected catalog JSON delimiter %q", typed)
		}
	case string:
		node.text = typed
		node.line = catalogJSONLine(data, decoder.InputOffset())
	}
	return node, nil
}

func catalogJSONLine(data []byte, offset int64) int {
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	return bytes.Count(data[:offset], []byte{'\n'}) + 1
}

func catalogJSONPropertyPath(parent, key string) string {
	if catalogJSONIdentifier(key) {
		return parent + "." + key
	}
	return parent + "[" + strconv.Quote(key) + "]"
}

func catalogJSONIdentifier(value string) bool {
	for index, char := range value {
		if index == 0 {
			if char != '_' && !unicode.IsLetter(char) {
				return false
			}
			continue
		}
		if char != '_' && !unicode.IsLetter(char) && !unicode.IsDigit(char) {
			return false
		}
	}
	return value != ""
}

func scanCatalogJSONNode(file string, node *catalogJSONNode, key, parentKey, description string, out *[]Finding) {
	switch {
	case node.object:
		objectDescription := ""
		for _, child := range node.children {
			if child.key == "description" && !child.object && !child.array {
				objectDescription = child.text
				break
			}
		}
		for _, child := range node.children {
			scanCatalogString(file, child.key, child.keyLine, child.path, true, "", "", "", out)
			scanCatalogJSONNode(file, child, child.key, key, objectDescription, out)
		}
	case node.array:
		for _, child := range node.children {
			scanCatalogJSONNode(file, child, key, parentKey, description, out)
		}
	default:
		scanCatalogString(file, node.text, node.line, node.path, false, key, parentKey, description, out)
	}
}

func scanCatalogString(
	file, text string,
	line int,
	path string,
	objectKey bool,
	key, parentKey, description string,
	out *[]Finding,
) {
	location := "JSON path " + path
	if objectKey {
		location = "JSON object key at path " + path
	}
	for _, email := range catalogEmailRE.FindAllString(text, -1) {
		if objectKey || !safeCatalogEmail(email, key, parentKey, description, text) {
			*out = append(*out, newFinding(
				"public_content_catalog_pii", file, line, "file", "email address at "+location,
			))
		}
	}
	if catalogInternalHostRE.MatchString(text) {
		*out = append(*out, newFinding(
			"public_content_catalog_internal_host", file, line, "file", "internal host name at "+location,
		))
	}
	if catalogPromptInjectionMarker(text) {
		*out = append(*out, newFinding(
			"public_content_catalog_prompt_injection", file, line, "file", "prompt-injection marker at "+location,
		))
	}
}

func catalogPromptInjectionMarker(text string) bool {
	if catalogInstructionOverrideRE.MatchString(text) ||
		catalogSystemPromptDisclosureRE.MatchString(text) {
		return true
	}
	lower := strings.ToLower(text)
	return strings.Contains(lower, "developer message") ||
		strings.Contains(lower, "<|system|>") ||
		strings.Contains(lower, "<|assistant|>")
}

func catalogUnsafeEmails(text string) map[string]bool {
	unsafe := map[string]bool{}
	var document interface{}
	if json.Unmarshal([]byte(text), &document) != nil {
		for _, email := range catalogEmailRE.FindAllString(text, -1) {
			unsafe[strings.ToLower(email)] = true
		}
		return unsafe
	}
	walkCatalogJSON(document, "", "", "", unsafe)
	return unsafe
}

func walkCatalogJSON(value interface{}, key, parentKey, description string, unsafe map[string]bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		objectDescription, _ := typed["description"].(string)
		for childKey, child := range typed {
			walkCatalogJSON(child, childKey, key, objectDescription, unsafe)
		}
	case []interface{}:
		for _, child := range typed {
			walkCatalogJSON(child, key, parentKey, description, unsafe)
		}
	case string:
		for _, email := range catalogEmailRE.FindAllString(typed, -1) {
			if !safeCatalogEmail(email, key, parentKey, description, typed) {
				unsafe[strings.ToLower(email)] = true
			}
		}
	}
}

func safeCatalogEmail(email, key, parentKey, description, value string) bool {
	localRaw, domainRaw, ok := strings.Cut(email, "@")
	if !ok {
		return false
	}
	local := strings.ToLower(localRaw)
	domain := strings.ToLower(domainRaw)
	if key == "example" && (domain == "example.com" || domain == "xxx.xx" ||
		placeholderEmailParts(local, domain)) {
		return true
	}
	if key == "example" && parentKey == "third_party_email" &&
		conventionalDocPerson(local) && genericDocEmailDomain(domain) {
		return true
	}
	lowerDescription := strings.ToLower(description)
	if key == "example" && safeCalendarResourceID(local, domain, parentKey, lowerDescription, value) {
		return true
	}
	if key == "example" && safeTechnicalMessageID(localRaw, domain, parentKey, lowerDescription) {
		return true
	}
	if key == "example" && parentKey == "references" &&
		strings.Contains(lowerDescription, "references") &&
		technicalReferenceLocalPart(local) {
		return true
	}
	if key == "example" && (parentKey == "in_reply_to" || parentKey == "reply_to") &&
		strings.Contains(lowerDescription, strings.ReplaceAll(parentKey, "_", "-")) &&
		strongTechnicalHeaderLocalPart(local) {
		return true
	}
	// Some descriptions contain a JSON example rather than using the schema's
	// dedicated example field. Reserved example domains remain non-identities.
	return strings.Contains(value, "**示例值：**") && domain == "example.com"
}

func conventionalDocPerson(local string) bool {
	switch strings.ToLower(local) {
	case "zhangsan", "lisi", "wangwu", "zhaoliu":
		return true
	default:
		return false
	}
}

func genericDocEmailDomain(domain string) bool {
	switch strings.ToLower(domain) {
	case "email.com", "example.com", "test.com":
		return true
	default:
		return false
	}
}

func safeCalendarResourceID(local, domain, field, description, value string) bool {
	if domain != "group.calendar.feishu.cn" ||
		!strings.HasPrefix(local, "feishu.cn_") ||
		len(strings.TrimPrefix(local, "feishu.cn_")) < 8 {
		return false
	}
	for _, r := range strings.TrimPrefix(local, "feishu.cn_") {
		if !unicode.IsLower(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	switch field {
	case "calendar_id", "organizer_calendar_id":
		return true
	case "app_link":
		expectedPrefix := core.ResolveEndpoints(core.BrandFeishu).AppLink +
			"/client/calendar/event/detail?"
		return strings.Contains(description, "app_link") &&
			strings.HasPrefix(value, expectedPrefix)
	default:
		return false
	}
}

type catalogCredentialLineState struct {
	trusted bool
	unsafe  bool
}

func catalogTrustedGenericCredentialLines(data []byte) map[int]bool {
	root, err := parseCatalogJSON(data)
	if err != nil {
		return nil
	}
	states := map[int]catalogCredentialLineState{}
	walkCatalogCredentialContext(root, "", "", states)
	trusted := map[int]bool{}
	for line, state := range states {
		if state.trusted && !state.unsafe {
			trusted[line] = true
		}
	}
	return trusted
}

func walkCatalogCredentialContext(node *catalogJSONNode, key, parentKey string, states map[int]catalogCredentialLineState) {
	switch {
	case node.object:
		for _, child := range node.children {
			walkCatalogCredentialContext(child, child.key, key, states)
		}
	case node.array:
		for _, child := range node.children {
			walkCatalogCredentialContext(child, key, parentKey, states)
		}
	default:
		for _, finding := range scanText("catalog-value", "file", node.text, false) {
			if finding.Rule != "public_content_generic_credential" {
				continue
			}
			state := states[node.line]
			if (key == "example" || key == "default") && safeCatalogResourceLink(parentKey, node.text) {
				state.trusted = true
			} else {
				state.unsafe = true
			}
			states[node.line] = state
		}
	}
}

func safeCatalogResourceLink(field, value string) bool {
	if field != "share_link" {
		return false
	}
	parsed, err := url.Parse(value)
	expectedAppLink, expectedErr := url.Parse(core.ResolveEndpoints(core.BrandFeishu).AppLink)
	if err != nil || expectedErr != nil || parsed.Scheme != expectedAppLink.Scheme || parsed.Host != expectedAppLink.Host ||
		parsed.Path != "/client/chat/chatter/add_by_link" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	query := parsed.Query()
	if len(query) != 1 || len(query["link_token"]) != 1 {
		return false
	}
	return catalogPublicResourceIdentifier(query.Get("link_token"))
}

func catalogPublicResourceIdentifier(value string) bool {
	parts := strings.Split(value, "-")
	canonicalUUID := len(parts) == 5 && len(parts[0]) == 8 && len(parts[1]) == 4 &&
		len(parts[2]) == 4 && len(parts[3]) == 4 && len(parts[4]) == 12
	publicLinkID := len(parts) == 5 && len(parts[0]) == 7 && len(parts[1]) == 4 &&
		len(parts[2]) == 4 && len(parts[3]) == 4 && len(parts[4]) == 14
	if !canonicalUUID && !publicLinkID {
		return false
	}
	for _, part := range parts {
		for _, char := range part {
			if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'z')) {
				return false
			}
		}
	}
	return true
}

func placeholderEmailParts(local, domain string) bool {
	host := strings.TrimSuffix(domain, ".com")
	placeholderWords := map[string]bool{
		"abc": true, "aac": true, "aba": true, "adc": true,
		"hello": true, "world": true, "user": true, "test": true,
	}
	return placeholderWords[local] && placeholderWords[host]
}

func safeTechnicalMessageID(local, domain, field, description string) bool {
	if field != "smtp_message_id" ||
		!strings.Contains(description, "rfc协议id") ||
		domain != "outlook.com" ||
		len(local) != 16 {
		return false
	}

	var digits, uppercaseRuns, uppercaseRun, lowercaseRun int
	previousDigit := false
	for _, char := range []byte(local) {
		switch {
		case char >= '0' && char <= '9':
			if previousDigit {
				return false
			}
			digits++
			previousDigit = true
			uppercaseRun = 0
			lowercaseRun = 0
		case char >= 'A' && char <= 'Z':
			if uppercaseRun == 0 {
				uppercaseRuns++
			}
			uppercaseRun++
			if uppercaseRun > 3 {
				return false
			}
			previousDigit = false
			lowercaseRun = 0
		case char >= 'a' && char <= 'z':
			lowercaseRun++
			if lowercaseRun > 3 {
				return false
			}
			previousDigit = false
			uppercaseRun = 0
		default:
			return false
		}
	}
	return digits >= 2 && uppercaseRuns >= 2
}

func technicalReferenceLocalPart(local string) bool {
	parts := strings.FieldsFunc(local, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})
	if len(parts) < 2 {
		return false
	}
	var hasLetters, hasDigits bool
	for _, part := range parts {
		if len(part) < 4 || !allHex(part) {
			return false
		}
		for _, r := range part {
			hasLetters = hasLetters || unicode.IsLetter(r)
			hasDigits = hasDigits || unicode.IsDigit(r)
		}
	}
	return hasLetters && hasDigits
}

func strongTechnicalHeaderLocalPart(local string) bool {
	parts := strings.Split(local, ".")
	if len(parts) >= 3 {
		for _, part := range parts {
			if len(part) < 4 || !allHex(part) {
				return false
			}
		}
		return true
	}
	parts = strings.Split(local, "-")
	if len(parts) == 5 {
		for _, part := range parts {
			if !allHex(part) {
				return false
			}
		}
		return true
	}
	return false
}

func allHex(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !unicode.IsDigit(r) &&
			!(r >= 'a' && r <= 'f') &&
			!(r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}
