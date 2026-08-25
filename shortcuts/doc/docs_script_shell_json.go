// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

var docsScriptPresentationDecisionType = reflect.TypeOf(docsScriptPresentationDecision{})

// docsScriptShellJSONError is an intermediate recovery-parser error. The
// command boundary keeps the original typed strict-JSON error when recovery
// cannot be completed without guessing.
type docsScriptShellJSONError struct {
	message string
}

func (parseError *docsScriptShellJSONError) Error() string { return parseError.message }

func newDocsScriptShellJSONError(format string, args ...any) error {
	return &docsScriptShellJSONError{message: fmt.Sprintf(format, args...)}
}

// recoverDocsScriptPresentationDecisionJSON rebuilds only the JSON syntax that
// legacy native-command argument passing can remove. The Go type remains the
// source of truth for object fields and scalar types; ambiguous bare strings
// containing JSON delimiters are rejected instead of guessed.
func recoverDocsScriptPresentationDecisionJSON(raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", newDocsScriptShellJSONError("presentation decision is not valid UTF-8")
	}
	parser := docsScriptShellJSONParser{raw: raw}
	normalized, err := parser.parseValue(docsScriptPresentationDecisionType)
	if err != nil {
		return "", err
	}
	parser.skipSpace()
	if parser.offset != len(parser.raw) {
		return "", newDocsScriptShellJSONError("unexpected trailing input at byte %d", parser.offset)
	}
	return normalized, nil
}

type docsScriptShellJSONParser struct {
	raw    string
	offset int
}

func (parser *docsScriptShellJSONParser) parseValue(valueType reflect.Type) (string, error) {
	parser.skipSpace()
	if valueType.Kind() == reflect.Pointer {
		if parser.consumeLiteral("null") {
			return "null", nil
		}
		return parser.parseValue(valueType.Elem())
	}

	switch valueType.Kind() {
	case reflect.Struct:
		if parser.consumeLiteral("null") {
			return "null", nil
		}
		return parser.parseStruct(valueType)
	case reflect.Slice:
		if parser.consumeLiteral("null") {
			return "null", nil
		}
		return parser.parseSlice(valueType.Elem())
	case reflect.String:
		return parser.parseString()
	case reflect.Int:
		return parser.parseInt(valueType.Bits())
	default:
		return "", newDocsScriptShellJSONError("unsupported presentation decision field type %s", valueType)
	}
}

func (parser *docsScriptShellJSONParser) parseStruct(structType reflect.Type) (string, error) {
	if err := parser.expectByte('{'); err != nil {
		return "", err
	}
	fields := docsScriptShellJSONStructFields(structType)
	seenFields := make(map[string]struct{}, len(fields))
	var normalized strings.Builder
	normalized.WriteByte('{')

	for fieldIndex := 0; ; fieldIndex++ {
		parser.skipSpace()
		if parser.consumeByte('}') {
			normalized.WriteByte('}')
			return normalized.String(), nil
		}
		if fieldIndex > 0 {
			if err := parser.expectByte(','); err != nil {
				return "", err
			}
			parser.skipSpace()
		}

		fieldName, err := parser.parseFieldName()
		if err != nil {
			return "", err
		}
		fieldType, ok := fields[fieldName]
		if !ok {
			return "", newDocsScriptShellJSONError("field %q is not defined by %s", fieldName, structType.Name())
		}
		if _, duplicated := seenFields[fieldName]; duplicated {
			return "", newDocsScriptShellJSONError("field %q is duplicated", fieldName)
		}
		seenFields[fieldName] = struct{}{}
		if err := parser.expectByte(':'); err != nil {
			return "", err
		}
		fieldValue, err := parser.parseValue(fieldType)
		if err != nil {
			return "", newDocsScriptShellJSONError("field %s: %v", fieldName, err)
		}

		if fieldIndex > 0 {
			normalized.WriteByte(',')
		}
		encodedFieldName, _ := json.Marshal(fieldName)
		normalized.Write(encodedFieldName)
		normalized.WriteByte(':')
		normalized.WriteString(fieldValue)
	}
}

func (parser *docsScriptShellJSONParser) parseSlice(elementType reflect.Type) (string, error) {
	if err := parser.expectByte('['); err != nil {
		return "", err
	}
	var normalized strings.Builder
	normalized.WriteByte('[')

	for elementIndex := 0; ; elementIndex++ {
		parser.skipSpace()
		if parser.consumeByte(']') {
			normalized.WriteByte(']')
			return normalized.String(), nil
		}
		if elementIndex > 0 {
			if err := parser.expectByte(','); err != nil {
				return "", err
			}
		}
		element, err := parser.parseValue(elementType)
		if err != nil {
			return "", newDocsScriptShellJSONError("element %d: %v", elementIndex, err)
		}
		if elementIndex > 0 {
			normalized.WriteByte(',')
		}
		normalized.WriteString(element)
	}
}

func (parser *docsScriptShellJSONParser) parseFieldName() (string, error) {
	parser.skipSpace()
	if parser.peekByte() == '"' {
		return parser.parseJSONString()
	}
	start := parser.offset
	for parser.offset < len(parser.raw) && docsScriptShellJSONFieldByte(parser.raw[parser.offset]) {
		parser.offset++
	}
	fieldName := parser.raw[start:parser.offset]
	if fieldName == "" {
		return "", newDocsScriptShellJSONError("expected an object field at byte %d", start)
	}
	parser.skipSpace()
	return fieldName, nil
}

func (parser *docsScriptShellJSONParser) parseString() (string, error) {
	parser.skipSpace()
	if parser.peekByte() == '"' {
		value, err := parser.parseJSONString()
		if err != nil {
			return "", err
		}
		encoded, _ := json.Marshal(value)
		return string(encoded), nil
	}

	start := parser.offset
	for parser.offset < len(parser.raw) {
		switch parser.raw[parser.offset] {
		case ',', '}', ']':
			value := strings.TrimSpace(parser.raw[start:parser.offset])
			if value == "" {
				return "", newDocsScriptShellJSONError("expected a string at byte %d", start)
			}
			if strings.ContainsAny(value, `"\{[`) {
				return "", newDocsScriptShellJSONError("bare string contains ambiguous JSON syntax")
			}
			encoded, _ := json.Marshal(value)
			return string(encoded), nil
		default:
			parser.offset++
		}
	}
	return "", newDocsScriptShellJSONError("unterminated bare string at byte %d", start)
}

func (parser *docsScriptShellJSONParser) parseJSONString() (string, error) {
	start := parser.offset
	if err := parser.expectByte('"'); err != nil {
		return "", err
	}
	escaped := false
	for parser.offset < len(parser.raw) {
		current := parser.raw[parser.offset]
		parser.offset++
		if escaped {
			escaped = false
			continue
		}
		if current == '\\' {
			escaped = true
			continue
		}
		if current == '"' {
			var value string
			if err := json.Unmarshal([]byte(parser.raw[start:parser.offset]), &value); err != nil {
				return "", newDocsScriptShellJSONError("invalid JSON string at byte %d: %v", start, err)
			}
			return value, nil
		}
	}
	return "", newDocsScriptShellJSONError("unterminated JSON string at byte %d", start)
}

func (parser *docsScriptShellJSONParser) parseInt(bits int) (string, error) {
	parser.skipSpace()
	start := parser.offset
	for parser.offset < len(parser.raw) {
		switch parser.raw[parser.offset] {
		case ',', '}', ']':
			value := strings.TrimSpace(parser.raw[start:parser.offset])
			parsed, err := strconv.ParseInt(value, 10, bits)
			if err != nil {
				return "", newDocsScriptShellJSONError("invalid integer %q", value)
			}
			return strconv.FormatInt(parsed, 10), nil
		default:
			parser.offset++
		}
	}
	return "", newDocsScriptShellJSONError("unterminated integer at byte %d", start)
}

func (parser *docsScriptShellJSONParser) expectByte(expected byte) error {
	parser.skipSpace()
	if !parser.consumeByte(expected) {
		return newDocsScriptShellJSONError("expected %q at byte %d", expected, parser.offset)
	}
	return nil
}

func (parser *docsScriptShellJSONParser) consumeByte(expected byte) bool {
	if parser.offset >= len(parser.raw) || parser.raw[parser.offset] != expected {
		return false
	}
	parser.offset++
	return true
}

func (parser *docsScriptShellJSONParser) consumeLiteral(literal string) bool {
	parser.skipSpace()
	if !strings.HasPrefix(parser.raw[parser.offset:], literal) {
		return false
	}
	end := parser.offset + len(literal)
	if end < len(parser.raw) && !docsScriptShellJSONDelimiterByte(parser.raw[end]) {
		return false
	}
	parser.offset = end
	return true
}

func (parser *docsScriptShellJSONParser) peekByte() byte {
	if parser.offset >= len(parser.raw) {
		return 0
	}
	return parser.raw[parser.offset]
}

func (parser *docsScriptShellJSONParser) skipSpace() {
	for parser.offset < len(parser.raw) {
		switch parser.raw[parser.offset] {
		case ' ', '\t', '\r', '\n':
			parser.offset++
		default:
			return
		}
	}
}

func docsScriptShellJSONStructFields(structType reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type, structType.NumField())
	for index := 0; index < structType.NumField(); index++ {
		field := structType.Field(index)
		fieldName := strings.Split(field.Tag.Get("json"), ",")[0]
		if fieldName != "" && fieldName != "-" {
			fields[fieldName] = field.Type
		}
	}
	return fields
}

func docsScriptShellJSONFieldByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '_' || value == '-'
}

func docsScriptShellJSONDelimiterByte(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n', ',', '}', ']':
		return true
	default:
		return false
	}
}
