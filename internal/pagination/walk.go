// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package pagination owns the bounded cursor walk shared by built-in and
// externally assembled commands.
package pagination

import (
	"context"
	"fmt"
	"time"
)

// CollectAllHardPageBound caps complete-set collections. It is deliberately
// tighter than the user-facing --page-limit maximum (1000): such a collection
// holds every page in memory before the caller's writes run, so its upper bound
// is a host resource decision, not a display preference. Value from the
// extension design's Phase 0 (owner plan §8.3).
//
// It lives here because the production host adapter and the commandtest
// recorder must enforce the same bound: a business command whose tests pass at
// 300 pages but fails in production at 101 is worse than one that fails in both.
const CollectAllHardPageBound = 100

// CursorErrorKind identifies an invalid cursor transition returned by an API.
type CursorErrorKind uint8

const (
	// CursorMissing means the API reported another page without a cursor.
	CursorMissing CursorErrorKind = iota + 1
	// CursorRepeated means the API returned a cursor already observed by the walk.
	CursorRepeated
)

// CursorError reports an invalid cursor transition.
type CursorError struct {
	Kind  CursorErrorKind
	Page  int
	Token string
}

func (e *CursorError) Error() string {
	switch e.Kind {
	case CursorMissing:
		return fmt.Sprintf("pagination page %d reports more pages without a page token", e.Page)
	case CursorRepeated:
		return fmt.Sprintf("pagination page %d repeated page token %q", e.Page, e.Token)
	default:
		return fmt.Sprintf("pagination page %d returned an invalid cursor", e.Page)
	}
}

// WaitError reports cancellation or failure while delaying between pages.
type WaitError struct{ Err error }

func (e *WaitError) Error() string { return e.Err.Error() }
func (e *WaitError) Unwrap() error { return e.Err }

// State describes the completed portion of a cursor walk.
type State struct {
	Complete  bool
	Pages     int
	NextToken string
}

// Fetch obtains and consumes one page, then returns its cursor state.
type Fetch func(ctx context.Context, pageNumber int, pageToken string) (hasMore bool, nextToken string, err error)

// Options configures one bounded cursor walk.
type Options struct {
	InitialToken string
	MaxPages     int
	Delay        time.Duration
	Fetch        Fetch
	Wait         func(context.Context, time.Duration) error
}

// Walk follows page tokens until exhaustion or MaxPages is reached.
func Walk(ctx context.Context, options Options) (State, error) {
	state := State{NextToken: options.InitialToken}
	seen := make(map[string]struct{}, options.MaxPages)
	if options.InitialToken != "" {
		seen[options.InitialToken] = struct{}{}
	}
	token := options.InitialToken
	wait := options.Wait
	if wait == nil {
		wait = WaitContext
	}

	for pageNumber := 1; pageNumber <= options.MaxPages; pageNumber++ {
		hasMore, nextToken, err := options.Fetch(ctx, pageNumber, token)
		if err != nil {
			state.NextToken = token
			return state, err
		}
		state.Pages++
		if !hasMore {
			state.Complete = true
			state.NextToken = ""
			return state, nil
		}
		if nextToken == "" {
			return state, &CursorError{Kind: CursorMissing, Page: pageNumber}
		}
		if _, duplicate := seen[nextToken]; duplicate {
			return state, &CursorError{Kind: CursorRepeated, Page: pageNumber, Token: nextToken}
		}
		state.NextToken = nextToken
		if pageNumber == options.MaxPages {
			return state, nil
		}
		seen[nextToken] = struct{}{}
		token = nextToken
		if options.Delay > 0 {
			if err := wait(ctx, options.Delay); err != nil {
				return state, &WaitError{Err: err}
			}
		}
	}

	return state, fmt.Errorf("pagination exhausted its page budget without producing a terminal result")
}

// WaitContext waits for one inter-page delay and observes cancellation.
func WaitContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
