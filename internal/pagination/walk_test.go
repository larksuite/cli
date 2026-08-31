// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package pagination

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestWalkFollowsCursorsToCompletion(t *testing.T) {
	var tokens []string
	state, err := Walk(context.Background(), Options{
		MaxPages: 3,
		Fetch: func(_ context.Context, page int, token string) (bool, string, error) {
			tokens = append(tokens, token)
			if page == 1 {
				return true, "next", nil
			}
			return false, "", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !state.Complete || state.Pages != 2 || state.NextToken != "" {
		t.Fatalf("state = %#v", state)
	}
	if !reflect.DeepEqual(tokens, []string{"", "next"}) {
		t.Fatalf("tokens = %#v", tokens)
	}
}

func TestWalkRejectsInvalidCursorTransitions(t *testing.T) {
	for _, test := range []struct {
		name string
		walk func(context.Context, int, string) (bool, string, error)
		kind CursorErrorKind
	}{
		{name: "missing", kind: CursorMissing, walk: func(context.Context, int, string) (bool, string, error) {
			return true, "", nil
		}},
		{name: "repeated", kind: CursorRepeated, walk: func(context.Context, int, string) (bool, string, error) {
			return true, "resume", nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Walk(context.Background(), Options{InitialToken: "resume", MaxPages: 2, Fetch: test.walk})
			var cursorErr *CursorError
			if !errors.As(err, &cursorErr) || cursorErr.Kind != test.kind {
				t.Fatalf("Walk() error = %T %v", err, err)
			}
		})
	}
}

func TestWalkPreservesResumeTokenWhenWaitIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	state, err := Walk(ctx, Options{
		MaxPages: 2,
		Delay:    time.Second,
		Fetch: func(context.Context, int, string) (bool, string, error) {
			return true, "next", nil
		},
	})
	var waitErr *WaitError
	if !errors.As(err, &waitErr) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Walk() error = %T %v", err, err)
	}
	if state.Pages != 1 || state.NextToken != "next" {
		t.Fatalf("state = %#v", state)
	}
}
