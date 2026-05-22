// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"strings"
	"testing"
)

// validEnvelope builds a baseline valid envelope used as a starting point in
// negative tests below.
func validEnvelope() Envelope {
	props := &OrderedProps{Map: map[string]Property{}}
	return Envelope{
		Name:        "x y z",
		Description: "ok",
		InputSchema: &InputSchema{
			Type:       "object",
			Properties: props,
		},
		OutputSchema: &OutputSchema{
			Type:       "object",
			Properties: &OrderedProps{Map: map[string]Property{}},
		},
		Meta: &Meta{
			EnvelopeVersion: "1.0",
			AccessTokens:    []string{"user"},
			Risk:            "read",
			Danger:          false,
		},
	}
}

func TestLintEnvelope_Valid(t *testing.T) {
	env := validEnvelope()
	errs := lintEnvelope(env)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestLintEnvelope_L1_StructuralChecks(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Envelope)
		wantSub string
	}{
		{
			name:    "empty name",
			mutate:  func(e *Envelope) { e.Name = "" },
			wantSub: "name",
		},
		{
			name:    "nil InputSchema",
			mutate:  func(e *Envelope) { e.InputSchema = nil },
			wantSub: "inputSchema",
		},
		{
			name:    "inputSchema type not object",
			mutate:  func(e *Envelope) { e.InputSchema.Type = "string" },
			wantSub: "inputSchema.type",
		},
		{
			name:    "nil OutputSchema",
			mutate:  func(e *Envelope) { e.OutputSchema = nil },
			wantSub: "outputSchema",
		},
		{
			name:    "nil Meta",
			mutate:  func(e *Envelope) { e.Meta = nil },
			wantSub: "_meta",
		},
		{
			name:    "wrong envelope version",
			mutate:  func(e *Envelope) { e.Meta.EnvelopeVersion = "0.9" },
			wantSub: "envelope_version",
		},
		{
			name: "invalid property type",
			mutate: func(e *Envelope) {
				e.InputSchema.Properties.Order = []string{"x"}
				e.InputSchema.Properties.Map["x"] = Property{Type: "unknown_type", XIn: "body"}
			},
			wantSub: "invalid type",
		},
		{
			name: "array missing items",
			mutate: func(e *Envelope) {
				e.InputSchema.Properties.Order = []string{"x"}
				e.InputSchema.Properties.Map["x"] = Property{Type: "array", XIn: "body"} // no Items
			},
			wantSub: "items",
		},
		{
			name: "top-level missing x-in",
			mutate: func(e *Envelope) {
				e.InputSchema.Properties.Order = []string{"x"}
				e.InputSchema.Properties.Map["x"] = Property{Type: "string"} // no XIn
			},
			wantSub: "x-in",
		},
		{
			name: "invalid x-in value",
			mutate: func(e *Envelope) {
				e.InputSchema.Properties.Order = []string{"x"}
				e.InputSchema.Properties.Map["x"] = Property{Type: "string", XIn: "header"}
			},
			wantSub: "x-in",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnvelope()
			tt.mutate(&env)
			errs := lintEnvelope(env)
			if len(errs) == 0 {
				t.Fatalf("expected lint error, got none")
			}
			found := false
			for _, e := range errs {
				if strings.Contains(e.Error(), tt.wantSub) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error containing %q, got: %v", tt.wantSub, errs)
			}
		})
	}
}

func TestLintEnvelope_L2_TypeChecks(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Envelope)
		wantSub string
	}{
		{
			name: "path field not in required",
			mutate: func(e *Envelope) {
				e.InputSchema.Properties.Order = []string{"id"}
				e.InputSchema.Properties.Map["id"] = Property{Type: "string", XIn: "path"}
				// Note: Required is empty — path must be in required
			},
			wantSub: "path field",
		},
		{
			name: "format binary on non-string",
			mutate: func(e *Envelope) {
				e.InputSchema.Properties.Order = []string{"f"}
				e.InputSchema.Properties.Map["f"] = Property{Type: "integer", Format: "binary", XIn: "body"}
			},
			wantSub: "format: binary",
		},
		{
			name: "required key not in properties",
			mutate: func(e *Envelope) {
				e.InputSchema.Required = []string{"nonexistent"}
			},
			wantSub: "required",
		},
		{
			name: "minimum >= maximum",
			mutate: func(e *Envelope) {
				min, max := 50.0, 10.0
				e.InputSchema.Properties.Order = []string{"n"}
				e.InputSchema.Properties.Map["n"] = Property{Type: "integer", Minimum: &min, Maximum: &max, XIn: "query"}
			},
			wantSub: "minimum",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnvelope()
			tt.mutate(&env)
			errs := lintEnvelope(env)
			if len(errs) == 0 {
				t.Fatalf("expected lint error, got none")
			}
			found := false
			for _, e := range errs {
				if strings.Contains(e.Error(), tt.wantSub) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error containing %q, got: %v", tt.wantSub, errs)
			}
		})
	}
}

func TestLintEnvelope_L3_CrossFieldChecks(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Envelope)
		wantSub string
	}{
		{
			name: "danger true but risk read",
			mutate: func(e *Envelope) {
				e.Meta.Danger = true
				e.Meta.Risk = "read"
			},
			wantSub: "danger",
		},
		{
			name: "high-risk-write without yes",
			mutate: func(e *Envelope) {
				e.Meta.Risk = "high-risk-write"
				e.Meta.Danger = true
				// no yes injection
			},
			wantSub: "yes",
		},
		{
			name: "yes injected but risk not high-risk-write",
			mutate: func(e *Envelope) {
				e.InputSchema.Properties.Order = []string{"yes"}
				e.InputSchema.Properties.Map["yes"] = Property{Type: "boolean"}
			},
			wantSub: "yes",
		},
		{
			name: "empty access_tokens",
			mutate: func(e *Envelope) {
				e.Meta.AccessTokens = []string{}
			},
			wantSub: "access_tokens",
		},
		{
			name: "invalid access_token value",
			mutate: func(e *Envelope) {
				e.Meta.AccessTokens = []string{"admin"}
			},
			wantSub: "access_tokens",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := validEnvelope()
			tt.mutate(&env)
			errs := lintEnvelope(env)
			if len(errs) == 0 {
				t.Fatalf("expected lint error, got none")
			}
			found := false
			for _, e := range errs {
				if strings.Contains(e.Error(), tt.wantSub) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error containing %q, got: %v", tt.wantSub, errs)
			}
		})
	}
}
