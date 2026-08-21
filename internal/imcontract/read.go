// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import (
	"maps"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/output"
)

const (
	hintSinglePage     = "Result is incomplete. Re-run with --page-all --page-limit 0 when exhaustive output is required."
	hintPageLimit      = "Result is incomplete because --page-limit was reached. Use --page-limit 0 only when exhaustive output is required."
	hintReadFailed     = "The read is incomplete. Retry the read; do not infer that missing items do not exist."
	hintTokenUnusable  = "The server did not provide a usable next page token. Report the result as incomplete."
	hintStartPage      = "This read started from a supplied page token and does not prove the collection was exhausted from the beginning."
	hintServerTruncate = "The server truncated the result. Narrow the query range before retrying."
	hintSearchEmpty    = "The search was exhausted, but an empty search result does not prove that the resource does not exist."
)

type ReadOptions struct {
	FullRead bool
}

// ReadResult is the IM-only interpretation of neutral pagination facts.
// Error is deliberately a copied Problem rather than the original error so
// causes and typed-error extension fields cannot leak into stdout.
type ReadResult struct {
	OK       bool
	Data     any
	Meta     *output.Meta
	Notice   map[string]interface{}
	Error    *errs.Problem
	Hint     string
	ExitCode int
	Cause    error `json:"-"`
}

// ReadSession is independent from the write Session. It records typed
// pagination and, for explicitly opted-in searches, materialization evidence;
// it never observes raw request or response bodies.
type ReadSession struct {
	contract                Contract
	options                 ReadOptions
	status                  client.PaginationStatus
	observed                bool
	materialization         MaterializationStatus
	materializationObserved bool
}

func NewReadSession(contract Contract, options ReadOptions) (*ReadSession, error) {
	if !contract.Strategy.Kind.IsRead() {
		return nil, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"unsupported IM read contract strategy %q",
			contract.Strategy.Kind,
		)
	}
	return &ReadSession{contract: contract, options: options}, nil
}

func (s *ReadSession) ObservePagination(status client.PaginationStatus) {
	s.status = status
	s.observed = true
}

// ObserveOutputPagination adapts the shared output pagination contract into
// the neutral facts consumed by the IM read contract. Existing IM commands
// that already record a richer PaginationStatus keep that observation.
func (s *ReadSession) ObserveOutputPagination(meta *output.PaginationMeta, startedFromToken bool) error {
	if s.observed {
		return nil
	}
	// Entity and materialize reads answer by ID or fetch a single resource, so
	// they have no pagination fact to observe. Treating their absent metadata
	// as an invalid response would reject a correct result, so the session
	// declines the observation instead of demanding one.
	if !s.RequiresPagination() {
		return nil
	}
	if meta == nil || meta.Pages < 1 {
		// Name the offending contract and keep the diagnosis local: the server
		// answered fine, the command just never handed its pagination facts to
		// the contract. Blaming the response here used to send agents off to a
		// different command instead of surfacing the wiring bug. Only
		// compile-time metadata may be interpolated — never a server-supplied
		// cursor or response body.
		return errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"%s is registered as a paginated IM read but produced no pagination facts",
			s.contract.Key,
		).WithHint("the command's contract registration or its pagination metadata wiring is wrong; report this command")
	}
	status := client.PaginationStatus{
		PagesFetched:  meta.Pages,
		HasMore:       !meta.Complete,
		NextPageToken: meta.NextToken,
	}
	switch {
	case startedFromToken:
		status.StopReason = client.StopReasonStartPageToken
	case meta.Complete:
		status.StopReason = client.StopReasonExhausted
	case s.options.FullRead:
		status.StopReason = client.StopReasonPageLimit
	default:
		status.StopReason = client.StopReasonSinglePage
	}
	s.ObservePagination(status)
	return nil
}

func (s *ReadSession) ObserveMaterialization(status MaterializationStatus) {
	s.materialization = status
	s.materializationObserved = true
}

func (s *ReadSession) RequiresPagination() bool {
	return s.contract.Strategy.Kind == CollectionReadKind || s.contract.Strategy.Kind == SearchReadKind
}

// FinalizeError applies the IM read retry contract to a typed error. Reads may
// be retried after transport failures and server errors. Rate limits and all
// other API or validation failures do not authorize an Agent retry.
func (s *ReadSession) FinalizeError(err error) error {
	problem, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	normalizeReadProblem(problem)
	return err
}

func (s *ReadSession) Finalize(data any) (ReadResult, error) {
	switch s.contract.Strategy.Kind {
	case EntityReadKind, MaterializeReadKind:
		return ReadResult{
			OK:   true,
			Data: data,
			Hint: s.contract.Strategy.ReadHint,
		}, nil
	case CollectionReadKind, SearchReadKind:
		if !s.observed {
			return ReadResult{}, errs.NewInternalError(
				errs.SubtypeInvalidResponse,
				"IM collection read completed without pagination status",
			)
		}
	default:
		return ReadResult{}, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"unsupported IM read contract strategy %q",
			s.contract.Strategy.Kind,
		)
	}

	data = canonicalReadData(data, s.status)
	result, err := finalizePagedRead(data, s.status, s.options.FullRead)
	if err != nil {
		return ReadResult{}, err
	}
	if result.Meta != nil && result.Meta.Pagination != nil {
		result.Meta.Pagination.Items = readItemCount(data, s.contract.Strategy.CollectionField)
	}
	if s.contract.Strategy.RequiresMaterialization {
		result, err = s.finalizeMaterialization(result)
		if err != nil {
			return ReadResult{}, err
		}
	}
	if s.contract.Strategy.Kind == SearchReadKind &&
		s.status.StopReason == client.StopReasonExhausted &&
		searchCollectionEmpty(data, s.contract.Strategy.CollectionField) {
		result.Hint = joinHints(result.Hint, hintSearchEmpty)
	}
	return result, nil
}

// canonicalReadData removes page-relative transport fields when an incomplete
// operation carries has_more=false. That value only describes the final page
// observed (for example after starting from a supplied cursor or encountering
// server truncation); it cannot prove collection-level completeness.
// meta.pagination remains the sole operation-level completeness and resume
// surface. Compatible has_more=true resume fields remain untouched.
func canonicalReadData(data any, status client.PaginationStatus) any {
	if status.StopReason == client.StopReasonExhausted {
		return data
	}
	object, ok := data.(map[string]any)
	if !ok {
		return data
	}
	hasMore, present := object["has_more"].(bool)
	if !present || hasMore {
		return data
	}
	canonical := maps.Clone(object)
	delete(canonical, "has_more")
	delete(canonical, "page_token")
	delete(canonical, "next_page_token")
	return canonical
}

func (s *ReadSession) finalizeMaterialization(result ReadResult) (ReadResult, error) {
	if !s.materializationObserved {
		return ReadResult{}, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"IM search completed without materialization status",
		)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		return ReadResult{}, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"IM search materialization requires an object result",
		)
	}
	data["materialization"] = s.materialization.ledger()
	result.Data = data

	materializationComplete := s.materialization.complete()
	if result.Meta == nil || result.Meta.Pagination == nil {
		return ReadResult{}, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"IM search materialization requires pagination completeness",
		)
	}
	if materializationComplete {
		if result.Meta.Pagination.Complete {
			result.Hint = "Results are ready to use. Use message_id/file_key directly; do not call messages-mget."
		}
		return result, nil
	}

	result.OK = false
	if result.ExitCode == 0 {
		result.ExitCode = output.ExitAPI
	}
	materializationHint := ""
	if len(s.materialization.MissingMessageIDs) > 0 {
		materializationHint = "The search is incomplete. Query only materialization.missing_message_ids with im +messages-mget."
	} else {
		materializationHint = "The search is incomplete and cannot be safely recovered by message ID. Narrow the query before retrying."
	}
	result.Hint = joinHints(result.Hint, materializationHint)
	if result.Error == nil && s.materialization.Cause != nil {
		if problem, ok := errs.ProblemOf(s.materialization.Cause); ok {
			copied := *problem
			normalizeReadProblem(&copied)
			result.Error = &copied
			result.Cause = s.materialization.Cause
			result.ExitCode = output.ExitCodeOf(s.materialization.Cause)
		}
	}
	return result, nil
}

func finalizePagedRead(data any, status client.PaginationStatus, fullRead bool) (ReadResult, error) {
	result := ReadResult{
		OK:   true,
		Data: data,
		Meta: &output.Meta{
			Pagination: &output.PaginationMeta{
				Pages:     status.PagesFetched,
				NextToken: status.NextPageToken,
			},
		},
		Notice: map[string]interface{}{
			"im_read": map[string]interface{}{
				"stop_reason": string(status.StopReason),
			},
		},
	}

	switch status.StopReason {
	case client.StopReasonExhausted:
		result.Meta.Pagination.Complete = true
	case client.StopReasonSinglePage:
		result.Hint = hintSinglePage
	case client.StopReasonPageLimit:
		result.Hint = hintPageLimit
	case client.StopReasonStartPageToken:
		result.Hint = hintStartPage
	case client.StopReasonServerTruncation:
		result.Hint = hintServerTruncate
		if fullRead {
			result.OK = false
			result.ExitCode = output.ExitAPI
		}
	case client.StopReasonTransportError, client.StopReasonAPIError,
		client.StopReasonMissingToken, client.StopReasonRepeatedToken:
		if status.Cause == nil {
			return ReadResult{}, errs.NewInternalError(
				errs.SubtypeInvalidResponse,
				"pagination stopped with %q but no typed cause was recorded",
				status.StopReason,
			)
		}
		problem, ok := errs.ProblemOf(status.Cause)
		if !ok {
			return ReadResult{}, errs.NewInternalError(
				errs.SubtypeInvalidResponse,
				"pagination stopped with an untyped cause",
			)
		}
		copied := *problem
		normalizeReadProblem(&copied)
		result.OK = false
		result.Error = &copied
		result.ExitCode = output.ExitCodeOf(status.Cause)
		result.Cause = status.Cause
		switch status.StopReason {
		case client.StopReasonMissingToken, client.StopReasonRepeatedToken:
			result.Hint = hintTokenUnusable
		default:
			result.Hint = hintReadFailed
		}
	default:
		return ReadResult{}, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"unsupported pagination stop reason %q",
			status.StopReason,
		)
	}
	return result, nil
}

func readItemCount(data any, preferredField string) int {
	m, ok := data.(map[string]any)
	if !ok {
		return 0
	}
	field := preferredField
	if field == "" {
		field = output.FindArrayField(m)
	}
	items, _ := m[field].([]interface{})
	return len(items)
}

func normalizeReadProblem(problem *errs.Problem) {
	if problem == nil {
		return
	}
	problem.Retryable = problem.Category == errs.CategoryNetwork ||
		(problem.Category == errs.CategoryAPI && problem.Subtype == errs.SubtypeServerError)
}

func searchCollectionEmpty(data any, field string) bool {
	m, ok := data.(map[string]any)
	if !ok {
		return false
	}
	value, exists := m[field]
	if !exists {
		return false
	}
	switch items := value.(type) {
	case []any:
		return len(items) == 0
	case []map[string]any:
		return len(items) == 0
	default:
		return false
	}
}

func joinHints(first, second string) string {
	if first == "" {
		return second
	}
	if second == "" {
		return first
	}
	return first + " " + second
}
