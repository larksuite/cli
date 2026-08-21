// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import (
	"fmt"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

const (
	hintReplayForbidden = "The write result is unknown. Do not replay the original request."
	hintReplaySafe      = "The write result is unknown. Retrying the original request is safe."
	hintSameKey         = "The write result is unknown. Retry only with the same idempotency key."
	hintUnsafeEvidence  = "The server response could not be safely mapped to the original request. Do not retry the write based on this response."
)

func invalidRequiredResult(field string) error {
	return errs.NewInternalError(errs.SubtypeInvalidResponse,
		"successful response is missing required field %q", field)
}

type invalidEvidenceError struct {
	cause error
}

func (e *invalidEvidenceError) Error() string {
	return e.cause.Error()
}

func (e *invalidEvidenceError) Unwrap() error {
	return e.cause
}

func invalidEvidence(field string) error {
	return &invalidEvidenceError{
		cause: errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"response evidence in %q cannot be mapped to the original request",
			field,
		).WithHint(hintUnsafeEvidence),
	}
}

func requiredResultPresent(data any, spec requiredSpec) bool {
	root, ok := data.(map[string]any)
	if !ok {
		return false
	}
	switch spec.Shape {
	case requiredTopString:
		return nonEmptyString(root[spec.Field]) != ""
	case requiredTopObject:
		object, ok := root[spec.Field].(map[string]any)
		return ok && len(object) > 0
	case requiredNestedString:
		object, ok := root[spec.Field].(map[string]any)
		return ok && nonEmptyString(object[spec.Child]) != ""
	default:
		return false
	}
}

func checkedResponse(data any) (map[string]any, error) {
	root, ok := data.(map[string]any)
	if !ok {
		return nil, invalidEvidence("response")
	}
	return root, nil
}

func validateEvidence(result extraction, requested []ledgerItem, field string, requireRequested bool) error {
	if !result.present {
		return nil
	}
	if result.rejectedCount != 0 ||
		result.rawCount != result.selectedCount+result.rejectedCount {
		return invalidEvidence(field)
	}
	if !requireRequested {
		return nil
	}
	requestedSet := make(map[string]struct{}, len(requested))
	for _, item := range requested {
		requestedSet[item.key] = struct{}{}
	}
	for _, item := range result.items {
		if _, ok := requestedSet[item.key]; !ok {
			return invalidEvidence(field)
		}
	}
	return nil
}

func finalizeBatch(s *Session, data any) (Result, error) {
	root, err := checkedResponse(data)
	if err != nil {
		return Result{}, err
	}
	requested := append([]ledgerItem{}, s.requested...)
	failed := make([]ledgerItem, 0)
	for _, spec := range s.contract.Strategy.Failures {
		evidence := extract(root, spec)
		if err := validateEvidence(evidence, requested, spec.Field, true); err != nil {
			return Result{}, err
		}
		failed = append(failed, evidence.items...)
	}

	responsePending := make([]ledgerItem, 0)
	for _, spec := range s.contract.Strategy.Pending {
		evidence := extract(root, spec)
		if err := validateEvidence(evidence, requested, spec.Field, true); err != nil {
			return Result{}, err
		}
		responsePending = append(responsePending, evidence.items...)
	}

	syntheticPending := make([]ledgerItem, 0)
	if s.hasFact(FactFlagFeedLayerPending) {
		syntheticPending = append(syntheticPending, ledgerItem{key: "feed", value: "feed"})
	}

	if spec := s.contract.Strategy.ResultLedger; spec != nil {
		evidence := extract(root, *spec)
		if err := validateEvidence(evidence, nil, spec.Field, false); err != nil {
			return Result{}, err
		}
		requested = append(requested, evidence.items...)
		failed = append(failed, statusFailures(root, *spec)...)
	}

	// Response pending can only classify an original request. Synthetic pending
	// represents a logical sub-request performed by a shortcut.
	requested = append(requested, syntheticPending...)
	pending := append(responsePending, syntheticPending...)
	ledger := completion(requested, failed, pending, s.contract.PartialRecovery)
	root["completion"] = ledger
	result := Result{OK: ledger.Status == "complete", Data: root}
	if !result.OK {
		result.ExitCode = output.ExitAPI
	}
	return result, nil
}

func statusFailures(root map[string]any, spec evidenceSpec) []ledgerItem {
	values, _ := root[spec.Field].([]any)
	failed := make([]ledgerItem, 0)
	for _, value := range values {
		object, _ := value.(map[string]any)
		if fmt.Sprint(object["status"]) != "failed" {
			continue
		}
		item, ok := stringItem(object[spec.IDField])
		if ok {
			failed = append(failed, item)
		}
	}
	return failed
}

func finalizeAssertion(s *Session, data any) (Result, error) {
	root, err := checkedResponse(data)
	if err != nil {
		return Result{}, err
	}
	actual := make(map[string]struct{})
	responseSetPresent := false
	for _, spec := range s.contract.Strategy.ResponseSets {
		evidence := extract(root, spec)
		if err := validateEvidence(evidence, nil, spec.Field, false); err != nil {
			return Result{}, err
		}
		responseSetPresent = responseSetPresent || evidence.present
		for _, item := range evidence.items {
			actual[item.key] = struct{}{}
		}
	}
	if !responseSetPresent {
		return Result{}, invalidEvidence("response_sets")
	}
	failed := make([]ledgerItem, 0)
	for _, item := range s.requested {
		_, exists := actual[item.key]
		if (s.contract.Strategy.Assertion == AssertRequestedPresent && !exists) ||
			(s.contract.Strategy.Assertion == AssertRequestedAbsent && exists) {
			failed = append(failed, item)
		}
	}
	ledger := completion(s.requested, failed, nil, PartialRecoveryFailedItemsOnly)
	root["completion"] = ledger
	result := Result{OK: ledger.Status == "complete", Data: root}
	if !result.OK {
		result.ExitCode = output.ExitAPI
	}
	return result, nil
}
