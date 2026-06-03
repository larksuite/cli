// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"testing"
)

// dummyOneOf demonstrates how a sub-struct opts into OneOfMarker by adding
// the empty OneOf() method.
type dummyOneOf struct{ A, B *string }

func (dummyOneOf) OneOf() {}

func TestOneOfMarker_Detection(t *testing.T) {
	var v interface{} = dummyOneOf{}
	if _, ok := v.(OneOfMarker); !ok {
		t.Fatal("dummyOneOf should satisfy OneOfMarker")
	}
}

// dummyValidatable implements Validatable.
type dummyValidatable struct{}

func (dummyValidatable) ValidateValue(rt *RuntimeContext, flag string) error { return nil }

func TestValidatable_InterfaceShape(t *testing.T) {
	var _ Validatable = dummyValidatable{}
}

// dummyNormalizable implements Normalizable[string].
type dummyNormalizable struct{}

func (dummyNormalizable) Normalize(ctx context.Context, raw string) (string, []string, error) {
	return raw, nil, nil
}

func TestNormalizable_InterfaceShape(t *testing.T) {
	var n Normalizable[string] = dummyNormalizable{}
	got, hints, err := n.Normalize(context.Background(), "x")
	if err != nil || got != "x" || hints != nil {
		t.Errorf("dummy Normalize round-trip: got=%q hints=%v err=%v", got, hints, err)
	}
}

func TestHelpExample_Fields(t *testing.T) {
	ex := HelpExample{Title: "send text", Cmd: "--chat-id oc_x --text hi"}
	if ex.Title != "send text" || ex.Cmd != "--chat-id oc_x --text hi" {
		t.Errorf("HelpExample: got %+v", ex)
	}
}
