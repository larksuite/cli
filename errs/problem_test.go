// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

import (
	"reflect"
	"testing"
)

func TestProblemError(t *testing.T) {
	tests := []struct {
		name string
		p    Problem
		want string
	}{
		{"empty message", Problem{}, ""},
		{"plain message", Problem{Message: "boom"}, "boom"},
		{"message ignores hint", Problem{Message: "msg", Hint: "do x"}, "msg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (&tt.p).Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestProblemError_NilReceiverDoesNotPanic pins the nil-receiver guard on
// (*Problem).Error(). Without it, a nil *Problem stored in an error interface
// would panic when the root dispatcher calls err.Error() for logging.
func TestProblemError_NilReceiverDoesNotPanic(t *testing.T) {
	var p *Problem // nil
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("(*Problem)(nil).Error() panicked: %v", r)
		}
	}()
	if got := p.Error(); got != "" {
		t.Errorf("(*Problem)(nil).Error() = %q, want \"\"", got)
	}
}

func TestProblemDetailReturnsReceiver(t *testing.T) {
	p := &Problem{Message: "x"}
	if got := p.ProblemDetail(); got != p {
		t.Errorf("ProblemDetail() = %p, want receiver %p", got, p)
	}
}

func TestProblemHasNoComponentField(t *testing.T) {
	if f, ok := reflect.TypeOf(Problem{}).FieldByName("Component"); ok {
		t.Errorf("Problem.Component must not exist; got field %#v", f)
	}
}

func TestProblemHasNoDocURLField(t *testing.T) {
	if f, ok := reflect.TypeOf(Problem{}).FieldByName("DocURL"); ok {
		t.Errorf("Problem.DocURL must not exist on the base Problem (PermissionError carries ConsoleURL instead); got field %#v", f)
	}
}

func TestProblemCategoryTagIsType(t *testing.T) {
	f, ok := reflect.TypeOf(Problem{}).FieldByName("Category")
	if !ok {
		t.Fatalf("Problem.Category must exist")
	}
	if got := f.Tag.Get("json"); got != "type" {
		t.Errorf("Problem.Category json tag = %q, want %q", got, "type")
	}
}
