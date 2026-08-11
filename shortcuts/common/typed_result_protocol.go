// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
)

// validateTypedResultProtocol checks only the local Outcome/Artifact receipts
// declared by OutputDefinition. It deliberately does not validate Data against
// the complete output schema on every invocation.
func validateTypedResultProtocol(command *compiledCommand, result compiledResult) error {
	if result.outcome != OutcomeSuccess && result.outcome != OutcomePartial {
		return nil
	}
	if err := validateTypedResultMeta(command.output.Meta, result.meta); err != nil {
		return err
	}
	if result.outcome != OutcomePartial && len(command.output.Artifacts) == 0 {
		return nil
	}
	encoded, err := json.Marshal(result.data)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "typed result cannot be inspected for its declared output protocol").WithCause(err)
	}
	var data any
	if err := json.Unmarshal(encoded, &data); err != nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "typed result cannot be decoded for its declared output protocol").WithCause(err)
	}
	if result.outcome == OutcomePartial {
		if err := validatePartialReceipt(command.output.Outcomes.PartialFailure, data); err != nil {
			return err
		}
	}
	for _, artifact := range command.output.Artifacts {
		if err := validateArtifactReceipt(artifact, data); err != nil {
			return err
		}
	}
	return nil
}

func validateTypedResultMeta(definition ResultMetaDefinition, meta *ResultMeta) error {
	if meta == nil {
		return nil
	}
	if meta.Count == nil && meta.Pagination == nil {
		return resultProtocolError("typed Result Meta is empty")
	}
	if meta.Count != nil {
		if !definition.Count {
			return resultProtocolError("typed Result returned undeclared meta.count")
		}
		if *meta.Count < 0 {
			return resultProtocolError("typed Result meta.count must be non-negative")
		}
	}
	if meta.Pagination != nil {
		if !definition.Pagination {
			return resultProtocolError("typed Result returned undeclared meta.pagination")
		}
		pagination := meta.Pagination
		if pagination.Pages < 1 {
			return resultProtocolError("typed Result meta.pagination.pages must be at least 1")
		}
		if pagination.Items < 0 {
			return resultProtocolError("typed Result meta.pagination.items must be non-negative")
		}
		if pagination.Complete && pagination.NextToken != "" {
			return resultProtocolError("typed Result complete pagination must not include next_token")
		}
		if !pagination.Complete && pagination.NextToken == "" {
			return resultProtocolError("typed Result incomplete pagination must include next_token")
		}
	}
	return nil
}

func validatePartialReceipt(definition *PartialFailureDefinition, data any) error {
	if definition == nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "typed Partial result has no compiled partial-failure contract")
	}
	if definition.FailedItems == nil {
		return nil
	}
	failed := definition.FailedItems
	value, ok := jsonPointerValue(data, failed.ItemsPath)
	if !ok {
		return resultProtocolError("partial failed-items path %q is missing from Data", failed.ItemsPath)
	}
	items, ok := value.([]any)
	if !ok {
		return resultProtocolError("partial failed-items path %q is not an array", failed.ItemsPath)
	}
	if len(items) == 0 {
		return resultProtocolError("Partial result contains no failed items at %q", failed.ItemsPath)
	}
	matched := 0
	for index, item := range items {
		for _, identityPath := range failed.IdentityPaths {
			if _, ok := jsonPointerValue(item, identityPath); !ok {
				return resultProtocolError("partial failed item %d is missing identity path %q", index, identityPath)
			}
		}
		if failed.AllItems {
			matched++
			continue
		}
		state, ok := jsonPointerValue(item, failed.StatePath)
		if !ok {
			return resultProtocolError("partial failed item %d is missing state path %q", index, failed.StatePath)
		}
		for _, expected := range failed.FailedValues {
			if reflect.DeepEqual(state, normalizedJSONValue(expected)) {
				matched++
				break
			}
		}
	}
	if matched == 0 {
		return resultProtocolError("Partial result has no item matching the declared failed values")
	}
	return nil
}

func validateArtifactReceipt(definition ArtifactDefinition, data any) error {
	value, ok := jsonPointerValue(data, definition.ItemsPath)
	if !ok || value == nil {
		if definition.Optional {
			return nil
		}
		return resultProtocolError("artifact %q items path %q is missing from Data", definition.Name, definition.ItemsPath)
	}
	items := []any{value}
	if array, ok := value.([]any); ok {
		items = array
	}
	for index, item := range items {
		pathValue, ok := jsonPointerValue(item, definition.PathField)
		if !ok {
			return resultProtocolError("artifact %q item %d is missing path field %q", definition.Name, index, definition.PathField)
		}
		path, ok := pathValue.(string)
		if !ok || path == "" {
			return resultProtocolError("artifact %q item %d has an invalid path receipt", definition.Name, index)
		}
		if definition.SizeField != "" {
			sizeValue, ok := jsonPointerValue(item, definition.SizeField)
			if !ok {
				return resultProtocolError("artifact %q item %d is missing size field %q", definition.Name, index, definition.SizeField)
			}
			size, ok := jsonInteger(sizeValue)
			if !ok || size < 0 {
				return resultProtocolError("artifact %q item %d has an invalid size receipt", definition.Name, index)
			}
		}
		if definition.MediaTypeField != "" {
			mediaType, ok := jsonPointerValue(item, definition.MediaTypeField)
			if !ok {
				return resultProtocolError("artifact %q item %d is missing media type field %q", definition.Name, index, definition.MediaTypeField)
			}
			if _, ok := mediaType.(string); !ok {
				return resultProtocolError("artifact %q item %d has a non-string media type receipt", definition.Name, index)
			}
		}
	}
	return nil
}

func jsonPointerValue(value any, pointer string) (any, bool) {
	if pointer == "" {
		return value, true
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, false
	}
	current := value
	for _, encoded := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		segment, ok := decodeJSONPointerSegment(encoded)
		if !ok {
			return nil, false
		}
		switch container := current.(type) {
		case map[string]any:
			current, ok = container[segment]
			if !ok {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(container) {
				return nil, false
			}
			current = container[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func decodeJSONPointerSegment(segment string) (string, bool) {
	var builder strings.Builder
	for index := 0; index < len(segment); index++ {
		if segment[index] != '~' {
			builder.WriteByte(segment[index])
			continue
		}
		if index+1 >= len(segment) {
			return "", false
		}
		index++
		switch segment[index] {
		case '0':
			builder.WriteByte('~')
		case '1':
			builder.WriteByte('/')
		default:
			return "", false
		}
	}
	return builder.String(), true
}

func normalizedJSONValue(value any) any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return value
	}
	return normalized
}

func jsonInteger(value any) (int64, bool) {
	number, ok := value.(float64)
	if !ok || number != float64(int64(number)) {
		return 0, false
	}
	return int64(number), true
}

func resultProtocolError(format string, args ...any) error {
	return errs.NewInternalError(errs.SubtypeUnknown, format, args...)
}
