// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

// Op is one declared operation: the business parameters it accepts bound to
// the handler that serves it. Attaching Params to the handler (rather than an
// agent-global table) makes "parameters declared on an unimplemented
// operation" impossible by construction. A zero-value Op means the operation
// is not supported.
type Op[H any] struct {
	Params []CardParam
	// Brands scopes this capability to a subset of brands (feishu/lark). Empty
	// means all brands. It is DECLARED at registration (brand-agnostic) and
	// GATED at command time against the resolved brand — the registry stays
	// offline. Register validates every value is feishu|lark.
	Brands  []core.LarkBrand
	Handler H
}

// The eight operation shapes. Each is an alias to an instantiated Op — the
// handler signatures are exactly the former hook signatures, so a provider
// migrates by wrapping `Send: f` into `Send: SendOp{Handler: f}`.
type (
	SendOp             = Op[func(ctx context.Context, rt Runtime, in SendInput) (*AgentTask, error)]
	TaskGetOp          = Op[func(ctx context.Context, rt Runtime, taskID string) (*AgentTask, error)]
	TaskListOp         = Op[func(ctx context.Context, rt Runtime, contextID string, page PageParams) ([]TaskSummary, PageInfo, error)]
	TaskCancelOp       = Op[func(ctx context.Context, rt Runtime, taskID string) error]
	ContextListOp      = Op[func(ctx context.Context, rt Runtime, page PageParams) ([]ContextSummary, PageInfo, error)]
	ContextGetOp       = Op[func(ctx context.Context, rt Runtime, ctxID string) (*ContextDetail, error)]
	ContextDeleteOp    = Op[func(ctx context.Context, rt Runtime, ctxID string) error]
	ArtifactDownloadOp = Op[func(ctx context.Context, rt Runtime, taskID, artifactID string) (*ArtifactData, error)]
)

// wired reports whether the operation has a handler. IMPLEMENTATION
// CONSTRAINT: H is a func type parameter, so `any(o.Handler) != nil` would box
// a typed nil into a non-nil interface and report every unwired operation as
// supported — reflect.Value.IsNil is the only correct generic path (verified
// by test on the zero value).
func (o Op[H]) wired() bool {
	v := reflect.ValueOf(o.Handler)
	return v.Kind() == reflect.Func && !v.IsNil()
}

func (o Op[H]) params() []CardParam { return o.Params }

func (o Op[H]) brands() []core.LarkBrand { return o.Brands }

// Verb constants: the operation vocabulary is the capability key set plus
// "send" — the AI reads one set of words for both "which verbs exist"
// (capabilities) and "what parameters each verb takes" (--operation).
const (
	VerbSend             = "send"
	VerbTaskGet          = CapTaskGet          // "task_get"
	VerbTaskList         = CapTaskList         // "task_list"
	VerbTaskCancel       = CapTaskCancel       // "task_cancel"
	VerbContextList      = CapContextList      // "context_list"
	VerbContextGet       = CapContextGet       // "context_get"
	VerbContextDelete    = CapContextDelete    // "context_delete"
	VerbArtifactDownload = CapArtifactDownload // "artifact_download" (task get --artifact)
)

// OpInfo is one operation's declaration as seen by framework consumers
// (card --operation, has_parameters, per-verb validation).
type OpInfo struct {
	Verb   string
	Wired  bool
	Params []CardParam
	// Brands is the operation's brand scope (empty = all brands), surfaced so
	// the command layer can gate a wired-but-brand-excluded capability.
	Brands []core.LarkBrand
}

// opDecl is the single-enumeration seam: every Op instantiation satisfies it,
// so Ops() is the one place that knows the verb↔field mapping. All framework
// consumers (capabilities, has_parameters, --operation, validation) enumerate
// through it — adding a ninth operation means extending exactly this table.
type opDecl interface {
	wired() bool
	params() []CardParam
	brands() []core.LarkBrand
}

// Ops enumerates the spec's eight operations in fixed verb order.
func (s *AgentSpec) Ops() []OpInfo {
	decls := []struct {
		verb string
		op   opDecl
	}{
		{VerbSend, s.Send},
		{VerbTaskGet, s.GetTask},
		{VerbTaskList, s.ListTasks},
		{VerbTaskCancel, s.CancelTask},
		{VerbContextList, s.ListContexts},
		{VerbContextGet, s.GetContext},
		{VerbContextDelete, s.DeleteContext},
		{VerbArtifactDownload, s.DownloadArtifact},
	}
	out := make([]OpInfo, 0, len(decls))
	for _, d := range decls {
		out = append(out, OpInfo{Verb: d.verb, Wired: d.op.wired(), Params: d.op.params(), Brands: d.op.brands()})
	}
	return out
}

// Op looks up one operation by verb (ok=false for a word outside the verb
// vocabulary).
func (s *AgentSpec) Op(verb string) (OpInfo, bool) {
	for _, o := range s.Ops() {
		if o.Verb == verb {
			return o, true
		}
	}
	return OpInfo{}, false
}

// Verbs returns the fixed verb vocabulary (declaration order of Ops).
func Verbs() []string {
	return []string{
		VerbSend, VerbTaskGet, VerbTaskList, VerbTaskCancel,
		VerbContextList, VerbContextGet, VerbContextDelete, VerbArtifactDownload,
	}
}

// OpAvailableForBrand reports whether an operation whose declaration lists
// `brands` is available under `brand`: an empty declaration means all brands,
// otherwise `brand` must be one of the listed values. It is the per-capability
// sibling of SpecAvailableForBrand (whole-agent) and is reused by both
// DeriveCapabilities (card matrix) and the command layer's per-verb brand gate.
func OpAvailableForBrand(brands []core.LarkBrand, brand core.LarkBrand) bool {
	if len(brands) == 0 {
		return true
	}
	for _, b := range brands {
		if b == brand {
			return true
		}
	}
	return false
}

// Float is a literal helper for CardParam.Min/Max.
func Float(v float64) *float64 { return &v }

// Int is a literal helper for ContextDetail.TaskCount.
func Int(v int) *int { return &v }

// ── Typed parameter access for provider handlers ──
//
// The framework validates every parameter against its declaration BEFORE a
// handler runs (rt.Params() contract), so the typed helpers treat a parse
// failure as provider coding drift, not user error.

// ParamInt returns the named integer parameter (ok=false when absent). The
// framework has already validated the value against Type "integer", so a parse
// failure is a programmer error (reading a non-integer param as int) and panics
// like Register does.
func ParamInt(rt Runtime, name string) (int64, bool) {
	raw, ok := rt.Params()[name]
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("agent: ParamInt(%q) on a non-integer value %q — declaration/consumption drift", name, raw))
	}
	return n, true
}

// ParamBool returns the named boolean parameter (ok=false when absent).
func ParamBool(rt Runtime, name string) (bool, bool) {
	raw, ok := rt.Params()[name]
	if !ok {
		return false, false
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		panic(fmt.Sprintf("agent: ParamBool(%q) on a non-boolean value %q — declaration/consumption drift", name, raw))
	}
	return b, true
}

// BindParams decodes rt.Params() into a provider struct via `param:"name"`
// tags, so the consumption side is compile-checked instead of stringly map
// lookups. Absent optional parameters leave the zero value (required ones are
// guaranteed present by the rt.Params() contract). Supported field kinds:
// string, int/int64, float64, bool. A conversion failure or unsupported field
// kind indicates declaration/struct drift (a provider coding error) and
// returns a typed internal error; agenttest.CheckParamsBinding catches the
// same drift in CI before it can happen at runtime.
func BindParams[T any](rt Runtime) (T, error) {
	var out T
	v := reflect.ValueOf(&out).Elem()
	if v.Kind() != reflect.Struct {
		return out, errs.NewInternalError(errs.SubtypeUnknown, "BindParams: %s is not a struct", v.Type())
	}
	if err := bindStruct(v, rt.Params(), ""); err != nil {
		return out, err
	}
	return out, nil
}

// ParamObject assembles an object parameter's leaves (the flat "name.field"
// keys in rt.Params()) into a typed struct via `param:"field"` tags. ok=false
// when no leaf of the object is present at all (the object was not provided
// and no field declares a Default). Same contract as BindParams: values were
// validated leaf-by-leaf before the handler ran; a conversion failure means
// declaration/struct drift.
func ParamObject[T any](rt Runtime, name string) (T, bool, error) {
	var out T
	v := reflect.ValueOf(&out).Elem()
	if v.Kind() != reflect.Struct {
		return out, false, errs.NewInternalError(errs.SubtypeUnknown, "ParamObject: %s is not a struct", v.Type())
	}
	prefix := name + "."
	sub := map[string]string{}
	for k, val := range rt.Params() {
		if strings.HasPrefix(k, prefix) {
			sub[strings.TrimPrefix(k, prefix)] = val
		}
	}
	if len(sub) == 0 {
		return out, false, nil
	}
	if err := bindStruct(v, sub, name+"."); err != nil {
		return out, true, err
	}
	return out, true, nil
}

// bindStruct is the shared tag-driven decoder: params keys are matched against
// `param` tags; a nested struct field with a tag recurses with its "tag."
// prefix stripped (object params). where prefixes error messages with the
// dotted path context.
func bindStruct(v reflect.Value, params map[string]string, where string) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("param")
		if tag == "" || tag == "-" {
			continue
		}
		if !f.IsExported() {
			// reflect cannot Set an unexported field — surface the coding error as
			// a typed error instead of a runtime panic (CheckParamsBinding flags
			// the same mistake in CI).
			return errs.NewInternalError(errs.SubtypeUnknown,
				"BindParams: field %s carries a param tag but is unexported (cannot be set)", f.Name)
		}
		// Nested struct = object param: bind its leaves from the "tag." prefix.
		if f.Type.Kind() == reflect.Struct {
			prefix := tag + "."
			sub := map[string]string{}
			for k, val := range params {
				if strings.HasPrefix(k, prefix) {
					sub[strings.TrimPrefix(k, prefix)] = val
				}
			}
			if len(sub) > 0 {
				if err := bindStruct(v.Field(i), sub, where+prefix); err != nil {
					return err
				}
			}
			continue
		}
		raw, ok := params[tag]
		if !ok {
			continue // absent optional → zero value
		}
		full := where + tag
		switch f.Type.Kind() {
		case reflect.String:
			v.Field(i).SetString(raw)
		case reflect.Int, reflect.Int64:
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return errs.NewInternalError(errs.SubtypeUnknown,
					"BindParams: parameter %s value %q cannot be parsed as %s (declaration and consumption drifted)", full, raw, f.Type.Kind()).WithCause(err)
			}
			v.Field(i).SetInt(n)
		case reflect.Float64:
			fl, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return errs.NewInternalError(errs.SubtypeUnknown,
					"BindParams: parameter %s value %q cannot be parsed as float64 (declaration and consumption drifted)", full, raw).WithCause(err)
			}
			v.Field(i).SetFloat(fl)
		case reflect.Bool:
			b, err := strconv.ParseBool(raw)
			if err != nil {
				return errs.NewInternalError(errs.SubtypeUnknown,
					"BindParams: parameter %s value %q cannot be parsed as bool (declaration and consumption drifted)", full, raw).WithCause(err)
			}
			v.Field(i).SetBool(b)
		default:
			return errs.NewInternalError(errs.SubtypeUnknown,
				"BindParams: field %s has unsupported type %s (supported: string/int/int64/float64/bool or a nested struct)", f.Name, f.Type.Kind())
		}
	}
	return nil
}
