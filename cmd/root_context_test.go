// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"testing"
	"time"
)

func TestExecutionContextFollowsParentAndStop(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, stop := newExecutionContext(parent)
	cancelParent()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("execution context did not follow parent cancellation")
	}
	stop()

	ctx, stop = newExecutionContext(context.Background())
	stop()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("execution context stop did not release signal subscription")
	}
}
