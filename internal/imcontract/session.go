// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import (
	"errors"
	"strings"

	"github.com/larksuite/cli/errs"
)

type Session struct {
	contract          Contract
	requested         []ledgerItem
	hasIdempotencyKey bool
	facts             []Fact
}

func NewSession(contract Contract) *Session {
	return &Session{contract: contract}
}

func (s *Session) Contract() Contract {
	return s.contract
}

func (s *Session) ObserveRequest(body map[string]any) error {
	if spec := s.contract.Strategy.Request; spec.Field != "" {
		evidence := extract(body, spec)
		if !evidence.present || evidence.selectedCount == 0 ||
			evidence.rejectedCount != 0 ||
			evidence.rawCount != evidence.selectedCount+evidence.rejectedCount {
			return errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"IM write request field %q has an unsupported shape",
				spec.Field,
			)
		}
		s.requested = uniqueItems(append(s.requested, evidence.items...))
	}
	if strings.TrimSpace(stableID(body["uuid"])) != "" {
		s.hasIdempotencyKey = true
	}
	return nil
}

func (s *Session) ObserveResponse(_ map[string]any) {}

func (s *Session) RecordFact(f Fact) {
	switch f.Kind {
	case FactMediaPreuploadPerformed, FactWriteAttempted:
		if s.hasFact(f.Kind) {
			return
		}
		s.facts = append(s.facts, Fact{Kind: f.Kind})
	case FactFlagFeedLayerPending:
		s.facts = append(s.facts, Fact{Kind: f.Kind, Item: "feed"})
	}
}

func (s *Session) hasFact(kind FactKind) bool {
	for _, fact := range s.facts {
		if fact.Kind == kind {
			return true
		}
	}
	return false
}

func (s *Session) FinalizeSuccess(data any) (Result, error) {
	s.RecordFact(Fact{Kind: FactWriteAttempted})
	switch s.contract.Strategy.Kind {
	case AuthoritativeAckKind:
		return Result{OK: true, Data: data}, nil
	case RequiredResultKind:
		if !requiredResultPresent(data, s.contract.Strategy.Required) {
			return Result{}, s.FinalizeError(invalidRequiredResult(requiredLabel(s.contract.Strategy.Required)))
		}
		if supportsMessageMentionResult(s.contract.Key) {
			result, err := finalizeMessageMentions(data)
			if err != nil {
				return Result{}, s.FinalizeError(err)
			}
			return result, nil
		}
		return Result{OK: true, Data: data}, nil
	case BatchPartialKind:
		return finalizeBatch(s, data)
	case RequiredResultBatchPartialKind:
		result, err := finalizeBatch(s, data)
		if err != nil {
			return Result{}, err
		}
		if !result.OK {
			return result, nil
		}
		if !requiredResultPresent(data, s.contract.Strategy.Required) {
			return Result{}, s.FinalizeError(invalidRequiredResult(requiredLabel(s.contract.Strategy.Required)))
		}
		return result, nil
	case ResponseSetAssertionKind:
		return finalizeAssertion(s, data)
	case AcceptanceOnlyKind:
		m, err := checkedResponse(data)
		if err != nil {
			return Result{}, err
		}
		m["completion"] = map[string]any{
			"status":               "accepted_unverified",
			"final_state_verified": false,
			"retry_scope":          "none",
		}
		return Result{OK: true, Data: m, Hint: s.contract.HelpPolicy.Text()}, nil
	default:
		return Result{}, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"unsupported IM write contract strategy %q",
			s.contract.Strategy.Kind,
		)
	}
}

func supportsMessageMentionResult(key ContractKey) bool {
	return key == "im +messages-send" || key == "im +messages-reply"
}

func requiredLabel(spec requiredSpec) string {
	if spec.Child == "" {
		return spec.Field
	}
	return spec.Field + "/" + spec.Child
}

func (s *Session) FinalizeError(err error) error {
	problem, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	if problem.Subtype == errs.SubtypeRateLimit {
		problem.Retryable = false
		problem.Hint = ""
		return err
	}
	transient := problem.Category == errs.CategoryNetwork ||
		(problem.Category == errs.CategoryAPI && problem.Retryable)
	if !transient && problem.Subtype != errs.SubtypeInvalidResponse {
		return err
	}
	if !s.hasFact(FactWriteAttempted) {
		return err
	}
	var evidenceErr *invalidEvidenceError
	if errors.As(err, &evidenceErr) {
		problem.Retryable = false
		problem.Hint = hintUnsafeEvidence
		return err
	}
	mode := s.contract.ReplayMode
	if s.hasFact(FactMediaPreuploadPerformed) {
		mode = ReplayForbidden
	}
	switch mode {
	case ReplaySafe:
		problem.Retryable = true
		problem.Hint = hintReplaySafe
	case ReplaySameIdempotencyKey:
		if s.hasIdempotencyKey {
			problem.Retryable = true
			problem.Hint = hintSameKey
			return err
		}
		fallthrough
	default:
		problem.Retryable = false
		problem.Hint = hintReplayForbidden
	}
	return err
}
