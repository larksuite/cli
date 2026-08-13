// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestOpZeroValueNotWired pins the typed-nil implementation constraint: the
// zero-value Op must report unwired. The naive generic check
// `any(o.Handler) != nil` would box a typed nil func into a non-nil interface
// and report every unwired operation as supported — this test kills that
// implementation on sight.
func TestOpZeroValueNotWired(t *testing.T) {
	var s AgentSpec
	for _, o := range s.Ops() {
		if o.Wired {
			t.Errorf("zero-value spec: operation %s must NOT be wired (typed-nil boxing trap)", o.Verb)
		}
	}
	s.Send = SendOp{Handler: func(context.Context, Runtime, SendInput) (*AgentTask, error) { return nil, nil }}
	if op, _ := s.Op(VerbSend); !op.Wired {
		t.Error("a wired Send must report Wired=true")
	}
}

// TestOpsVocabularyMatchesCapabilities is the single-enumeration contract test:
// the --operation verb set must be exactly the capability keys backed by
// operations, plus "send" (file_input / input_required are behavioral flags,
// not verbs). A ninth operation added to one table but not the other fails
// here.
func TestOpsVocabularyMatchesCapabilities(t *testing.T) {
	verbs := map[string]bool{}
	for _, v := range Verbs() {
		verbs[v] = true
	}
	if !verbs[VerbSend] {
		t.Fatal("verb vocabulary must include send")
	}
	for _, capKey := range []string{
		CapTaskGet, CapTaskList, CapTaskCancel,
		CapContextList, CapContextGet, CapContextDelete, CapArtifactDownload,
	} {
		if !verbs[capKey] {
			t.Errorf("operation-backed capability %q missing from the verb vocabulary", capKey)
		}
	}
	if verbs[CapFileInput] || verbs[CapInputRequired] {
		t.Error("behavioral flags (file_input/input_required) must NOT be verbs")
	}
	if len(verbs) != 8 {
		t.Errorf("verb vocabulary should have exactly 8 entries, got %d", len(verbs))
	}
	// Ops() must enumerate the same set, in the Verbs() order.
	var s AgentSpec
	ops := s.Ops()
	if len(ops) != len(Verbs()) {
		t.Fatalf("Ops() should enumerate %d operations, got %d", len(Verbs()), len(ops))
	}
	for i, v := range Verbs() {
		if ops[i].Verb != v {
			t.Errorf("Ops()[%d] should be %s (Verbs() order), got %s", i, v, ops[i].Verb)
		}
	}
}

// fakeParamsRT is a Runtime stub whose Params() returns a fixed map.
type fakeParamsRT struct{ p map[string]string }

func (r fakeParamsRT) AgentID() string           { return "" }
func (r fakeParamsRT) IsBot() bool               { return false }
func (r fakeParamsRT) Params() map[string]string { return r.p }
func (r fakeParamsRT) CallAPI(context.Context, string, string, map[string]string, any) (json.RawMessage, error) {
	return nil, nil
}
func (r fakeParamsRT) CallMultipart(context.Context, string, string, map[string]string, []FilePart) (json.RawMessage, error) {
	return nil, nil
}

// TestBindParams pins the typed consumption seam: tag-driven decode across all
// four supported kinds, zero values for absent optionals, and a typed error on
// declaration/struct drift.
func TestBindParams(t *testing.T) {
	type P struct {
		WS      string  `param:"workspace_id"`
		N       int64   `param:"max_results"`
		Ratio   float64 `param:"ratio"`
		Dry     bool    `param:"dry"`
		Skipped string  // no tag → ignored
	}
	rt := fakeParamsRT{p: map[string]string{
		"workspace_id": "ws_42", "max_results": "50", "ratio": "0.5", "dry": "true",
	}}
	p, err := BindParams[P](rt)
	if err != nil {
		t.Fatalf("BindParams should decode: %v", err)
	}
	if p.WS != "ws_42" || p.N != 50 || p.Ratio != 0.5 || p.Dry != true {
		t.Fatalf("decoded values wrong: %+v", p)
	}

	// absent optional → zero value
	p2, err := BindParams[P](fakeParamsRT{p: map[string]string{}})
	if err != nil || p2.WS != "" || p2.N != 0 {
		t.Fatalf("absent params should decode to zero values: %+v %v", p2, err)
	}

	// declaration/struct drift: int field fed a non-integer → typed error
	type Bad struct {
		N int64 `param:"workspace_id"`
	}
	if _, err := BindParams[Bad](rt); err == nil {
		t.Fatal("type drift should return an error")
	}

	// non-struct T is a typed error, not a panic
	if _, err := BindParams[string](rt); err == nil {
		t.Fatal("non-struct T should error")
	}

	// unexported tagged field is a typed error, not a reflect panic
	type unexported struct {
		ws string `param:"workspace_id"` //nolint:unused // the tag is the point
	}
	if _, err := BindParams[unexported](rt); err == nil {
		t.Fatal("unexported tagged field should return a typed error (reflect cannot Set it)")
	}
}

// TestParamObjectAndNestedBind pins the object consumption seam: ParamObject
// assembles "name.*" leaves; a nested tagged struct in BindParams does the
// same inline; ok=false when no leaf exists.
func TestParamObjectAndNestedBind(t *testing.T) {
	type Filter struct {
		Region    string  `param:"region"`
		MinAmount float64 `param:"min_amount"`
		Active    bool    `param:"active"`
	}
	rt := fakeParamsRT{p: map[string]string{
		"workspace_id": "ws_42", "filter.region": "east", "filter.min_amount": "100", "filter.active": "true",
	}}
	f, ok, err := ParamObject[Filter](rt, "filter")
	if err != nil || !ok {
		t.Fatalf("ParamObject should assemble: ok=%v err=%v", ok, err)
	}
	if f.Region != "east" || f.MinAmount != 100 || !f.Active {
		t.Fatalf("assembled values wrong: %+v", f)
	}
	if _, ok, _ := ParamObject[Filter](rt, "absent_obj"); ok {
		t.Error("ParamObject on an absent object should be ok=false")
	}

	type Top struct {
		WS     string `param:"workspace_id"`
		Filter Filter `param:"filter"`
	}
	top, err := BindParams[Top](rt)
	if err != nil {
		t.Fatalf("nested BindParams should decode: %v", err)
	}
	if top.WS != "ws_42" || top.Filter.Region != "east" || top.Filter.MinAmount != 100 {
		t.Fatalf("nested decode wrong: %+v", top)
	}
}

// TestRegisterObjectRules table-drives the object declaration rules.
func TestRegisterObjectRules(t *testing.T) {
	mk := func(params []CardParam) Provider {
		return Provider{
			Scheme: "objrules", Label: "x", AgentIDSource: "x",
			Identities: []IdentitySpec{{Type: IdentityUser}},
			Instance: &AgentSpec{
				Send:    SendOp{Params: params, Handler: func(context.Context, Runtime, SendInput) (*AgentTask, error) { return nil, nil }},
				GetTask: TaskGetOp{Handler: func(context.Context, Runtime, string) (*AgentTask, error) { return nil, nil }},
			},
		}
	}
	cases := []struct {
		name   string
		params []CardParam
		panics string
	}{
		{"object without fields", []CardParam{{Name: "f", Type: "object"}}, "non-empty Fields"},
		{"object with required", []CardParam{{Name: "f", Type: "object", Required: true,
			Fields: []CardParam{{Name: "a"}}}}, "must not set Required"},
		{"object with default", []CardParam{{Name: "f", Type: "object", Default: "x",
			Fields: []CardParam{{Name: "a"}}}}, "must not set Required/Enum/Default"},
		{"nested object", []CardParam{{Name: "f", Type: "object",
			Fields: []CardParam{{Name: "g", Type: "object", Fields: []CardParam{{Name: "a"}}}}}}, "nested object"},
		{"fields on scalar", []CardParam{{Name: "s", Fields: []CardParam{{Name: "a"}}}}, "only valid on Type"},
		{"leaf rules recurse", []CardParam{{Name: "f", Type: "object",
			Fields: []CardParam{{Name: "a", Type: "integer", Enum: []string{"x"}}}}}, "must parse as integer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("Register should panic (%s)", tc.panics)
				}
				if msg, _ := r.(string); !strings.Contains(msg, tc.panics) {
					t.Fatalf("panic should contain %q, got %v", tc.panics, r)
				}
			}()
			Register(mk(tc.params))
		})
	}
	// legal object registers fine (fresh scheme per test binary run)
	Register(mk([]CardParam{{Name: "render", Type: "object",
		Fields: []CardParam{{Name: "theme", Enum: []string{"light", "dark"}, Default: "light"}}}}))
}

// TestParamHelpers pins ParamInt/ParamBool presence semantics and the
// programmer-error panic on drift.
func TestParamHelpers(t *testing.T) {
	rt := fakeParamsRT{p: map[string]string{"n": "7", "b": "true", "s": "x"}}
	if n, ok := ParamInt(rt, "n"); !ok || n != 7 {
		t.Errorf("ParamInt: want 7,true got %d,%v", n, ok)
	}
	if _, ok := ParamInt(rt, "absent"); ok {
		t.Error("ParamInt on absent key should be ok=false")
	}
	if b, ok := ParamBool(rt, "b"); !ok || !b {
		t.Errorf("ParamBool: want true,true got %v,%v", b, ok)
	}
	defer func() {
		if recover() == nil {
			t.Error("ParamInt on a non-integer value should panic (declaration/consumption drift)")
		}
	}()
	ParamInt(rt, "s")
}

// TestValidateValue pins the shared Type/Enum/Min-Max checker.
func TestValidateValue(t *testing.T) {
	intp := CardParam{Name: "n", Type: "integer", Min: Float(1), Max: Float(100)}
	if err := ValidateValue(intp, "50"); err != nil {
		t.Errorf("50 in 1..100 should pass: %v", err)
	}
	if err := ValidateValue(intp, "500"); err == nil || !strings.Contains(err.Error(), "1..100") {
		t.Errorf("500 should violate range with the bounds in the message, got %v", err)
	}
	if err := ValidateValue(intp, "abc"); err == nil || !strings.Contains(err.Error(), "integer") {
		t.Errorf("abc should violate type, got %v", err)
	}
	enump := CardParam{Name: "p", Type: "string", Enum: []string{"low", "high"}}
	if err := ValidateValue(enump, "mid"); err == nil || !strings.Contains(err.Error(), "low|high") {
		t.Errorf("enum violation should list the full set, got %v", err)
	}
	nump := CardParam{Name: "r", Type: "number", Min: Float(0), Max: Float(1)}
	for _, bad := range []string{"NaN", "Inf", "-Inf"} {
		if err := ValidateValue(nump, bad); err == nil {
			t.Errorf("%s should be rejected as non-finite (would sail past range checks)", bad)
		}
	}
	boolp := CardParam{Name: "b", Type: "boolean"}
	if err := ValidateValue(boolp, "yes"); err == nil {
		t.Error("'yes' is not a Go bool literal, should fail")
	}
	if err := ValidateValue(boolp, "true"); err != nil {
		t.Errorf("'true' should pass: %v", err)
	}
}

// TestRegisterParamChecks table-drives the Register fail-fast rules for
// parameter declarations.
func TestRegisterParamChecks(t *testing.T) {
	specWith := func(params []CardParam) Provider {
		return Provider{
			Scheme: "regcheck", Label: "x", AgentIDSource: "x",
			Identities: []IdentitySpec{{Type: IdentityUser}},
			Instance: &AgentSpec{
				Send:    SendOp{Params: params, Handler: func(context.Context, Runtime, SendInput) (*AgentTask, error) { return nil, nil }},
				GetTask: TaskGetOp{Handler: func(context.Context, Runtime, string) (*AgentTask, error) { return nil, nil }},
			},
		}
	}
	cases := []struct {
		name   string
		mut    func(*Provider)
		panics string
	}{
		{"bad name charset", func(p *Provider) { p.Instance.Send.Params = []CardParam{{Name: "Bad-Name"}} }, "param name"},
		{"dup name", func(p *Provider) {
			p.Instance.Send.Params = []CardParam{{Name: "a"}, {Name: "a"}}
		}, "duplicate param name"},
		{"bad type", func(p *Provider) { p.Instance.Send.Params = []CardParam{{Name: "a", Type: "float"}} }, "Type must be one of"},
		{"enum on boolean", func(p *Provider) {
			p.Instance.Send.Params = []CardParam{{Name: "a", Type: "boolean", Enum: []string{"true"}}}
		}, "Enum is only valid"},
		{"enum+range mutex", func(p *Provider) {
			p.Instance.Send.Params = []CardParam{{Name: "a", Type: "integer", Enum: []string{"1"}, Min: Float(0)}}
		}, "mutually exclusive"},
		{"integer enum member not int", func(p *Provider) {
			p.Instance.Send.Params = []CardParam{{Name: "a", Type: "integer", Enum: []string{"x"}}}
		}, "must parse as integer"},
		{"default+required mutex", func(p *Provider) {
			p.Instance.Send.Params = []CardParam{{Name: "a", Required: true, Default: "v"}}
		}, "Default and Required"},
		{"default violates enum", func(p *Provider) {
			p.Instance.Send.Params = []CardParam{{Name: "a", Enum: []string{"x"}, Default: "y"}}
		}, "Default violates"},
		{"min>max", func(p *Provider) {
			p.Instance.Send.Params = []CardParam{{Name: "a", Type: "integer", Min: Float(2), Max: Float(1)}}
		}, "Min must be <= Max"},
		{"range on string", func(p *Provider) {
			p.Instance.Send.Params = []CardParam{{Name: "a", Min: Float(1)}}
		}, "Min/Max are only valid"},
		{"params on unwired op", func(p *Provider) {
			p.Instance.ListTasks = TaskListOp{Params: []CardParam{{Name: "a"}}}
		}, "unwired operation"},
		{"listparams without listagents", func(p *Provider) {
			p.ListParams = []CardParam{{Name: "env"}}
		}, "ListParams without a ListAgents"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := specWith(nil)
			tc.mut(&p)
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("Register should panic (%s)", tc.panics)
				}
				if msg, _ := r.(string); !strings.Contains(msg, tc.panics) {
					t.Fatalf("panic message should contain %q, got %v", tc.panics, r)
				}
			}()
			Register(p)
		})
	}
}

// TestRegisterNormalizesType pins the empty-Type ⇒ "string" normalization: the
// most common declaration shape must not force every param to spell Type out.
func TestRegisterNormalizesType(t *testing.T) {
	p := Provider{
		Scheme: "normtype", Label: "x", AgentIDSource: "x",
		Identities: []IdentitySpec{{Type: IdentityUser}},
		Instance: &AgentSpec{
			Send: SendOp{
				Params:  []CardParam{{Name: "plain"}},
				Handler: func(context.Context, Runtime, SendInput) (*AgentTask, error) { return nil, nil },
			},
			GetTask: TaskGetOp{Handler: func(context.Context, Runtime, string) (*AgentTask, error) { return nil, nil }},
		},
	}
	Register(p)
	prov, _ := Info("normtype")
	if got := prov.Instance.Send.Params[0].Type; got != "string" {
		t.Errorf("empty Type should normalize to string, got %q", got)
	}
}

// TestHasParameters pins the card cue derivation: only wired operations with a
// non-empty declaration appear, in fixed verb order, never nil.
func TestHasParameters(t *testing.T) {
	s := AgentSpec{
		Send: SendOp{
			Params:  []CardParam{{Name: "a"}},
			Handler: func(context.Context, Runtime, SendInput) (*AgentTask, error) { return nil, nil },
		},
		GetTask: TaskGetOp{Handler: func(context.Context, Runtime, string) (*AgentTask, error) { return nil, nil }},
		// unwired op with params is a Register error; here simulate wired+empty
		ListTasks: TaskListOp{Handler: func(context.Context, Runtime, string, PageParams) ([]TaskSummary, PageInfo, error) {
			return nil, PageInfo{}, nil
		}},
	}
	got := HasParameters(&s)
	if len(got) != 1 || got[0] != VerbSend {
		t.Errorf("has_parameters should be [send], got %v", got)
	}
	if HasParameters(&AgentSpec{}) == nil {
		t.Error("has_parameters must never be nil")
	}
}
