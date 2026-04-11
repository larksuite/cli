// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"io"
	"testing"
)

func TestCachedHttpClientFunc_ReturnsSameInstance(t *testing.T) {
	fn := cachedHttpClientFunc(&Factory{IOStreams: &IOStreams{ErrOut: io.Discard}})

	c1, err := fn()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if c1 == nil {
		t.Fatal("first call returned nil")
	}

	c2, err := fn()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if c1 != c2 {
		t.Error("expected same *http.Client instance on second call (cache hit)")
	}
}

func TestCachedHttpClientFunc_HasTimeout(t *testing.T) {
	fn := cachedHttpClientFunc(&Factory{IOStreams: &IOStreams{ErrOut: io.Discard}})
	c, _ := fn()
	if c.Timeout == 0 {
		t.Error("expected non-zero timeout")
	}
}

func TestCachedHttpClientFunc_HasRedirectPolicy(t *testing.T) {
	fn := cachedHttpClientFunc(&Factory{IOStreams: &IOStreams{ErrOut: io.Discard}})
	c, _ := fn()
	if c.CheckRedirect == nil {
		t.Error("expected CheckRedirect to be set (safeRedirectPolicy)")
	}
}
