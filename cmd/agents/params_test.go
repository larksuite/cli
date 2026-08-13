// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	baseprovider "github.com/larksuite/cli/agents/base"
	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
)

// paramSpec builds a spec with a send declaration (required ws + enum/default
// priority + ranged integer) and a task_list declaration sharing ws — the
// cross-operation reverse-lookup and three-way-carry test bed.
func paramSpec() *iagents.AgentSpec {
	ws := iagents.CardParam{Name: "workspace_id", Type: "string", Required: true, Desc: "target workspace"}
	return &iagents.AgentSpec{
		Send: iagents.SendOp{
			Params: []iagents.CardParam{
				ws,
				{Name: "priority", Type: "string", Enum: []string{"low", "normal", "high"}, Default: "normal"},
				{Name: "max_results", Type: "integer", Min: iagents.Float(1), Max: iagents.Float(100), Default: "20"},
			},
			Handler: func(context.Context, iagents.Runtime, iagents.SendInput) (*iagents.AgentTask, error) { return nil, nil },
		},
		GetTask: iagents.TaskGetOp{Handler: func(context.Context, iagents.Runtime, string) (*iagents.AgentTask, error) { return nil, nil }},
		ListTasks: iagents.TaskListOp{
			Params: []iagents.CardParam{ws},
			Handler: func(context.Context, iagents.Runtime, string, iagents.PageParams) ([]iagents.TaskSummary, iagents.PageInfo, error) {
				return nil, iagents.PageInfo{}, nil
			},
		},
	}
}

// TestValidateParams_CollectAll pins the batch contract: every violation in ONE
// error — two missing requireds are impossible on one decl set, so mix missing
// required + unknown key + enum violation and assert all three violations
// surface with self-contained specs.
func TestValidateParams_CollectAll(t *testing.T) {
	spec := paramSpec()
	_, err := validateParams(
		[]string{"priority=urgent", "bogus=1"},
		spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if err == nil {
		t.Fatal("should fail with collected violations")
	}
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if len(verr.Params) != 3 {
		t.Fatalf("want 3 violations (enum + unknown + missing required), got %d: %+v", len(verr.Params), verr.Params)
	}
	byName := map[string]errs.InvalidParam{}
	for _, v := range verr.Params {
		byName[v.Name] = v
	}
	// enum violation lists the full set and embeds the spec
	if v := byName["priority"]; !strings.Contains(v.Reason, "low|normal|high") || v.Spec == nil {
		t.Errorf("priority violation should list the enum set and embed spec, got %+v", v)
	}
	// unknown key lists this operation's available params
	if v := byName["bogus"]; !strings.Contains(v.Reason, "workspace_id") {
		t.Errorf("unknown-key violation should list available params, got %+v", v)
	}
	// missing required embeds the full declaration so the caller can fix without
	// a discovery round-trip
	v := byName["workspace_id"]
	if !strings.Contains(v.Reason, "missing required parameter") || v.Spec == nil {
		t.Fatalf("missing-required violation should embed spec, got %+v", v)
	}
	if sp, ok := v.Spec.(iagents.CardParam); !ok || sp.Desc != "target workspace" {
		t.Errorf("embedded spec should be the full CardParam, got %+v", v.Spec)
	}
	// multi-violation message is a count summary; hint points at --operation
	if !strings.Contains(verr.Message, "3 problems") {
		t.Errorf("multi-violation message should carry the count, got %q", verr.Message)
	}
	if !strings.Contains(verr.Hint, "--operation send") {
		t.Errorf("hint should point at card --operation send, got %q", verr.Hint)
	}
}

func TestBaseTaskGetAcceptsContextID(t *testing.T) {
	provider := baseprovider.Provider()
	if len(provider.Catalog) != 1 {
		t.Fatalf("base catalog=%d", len(provider.Catalog))
	}
	spec := &provider.Catalog[0]
	got, err := validateParams(
		[]string{"base_token=b1", "context_id=7663083417936891420"},
		spec.GetTask.Params,
		iagents.VerbTaskGet,
		spec,
		"base:assistant",
	)
	if err != nil {
		t.Fatalf("task_get context_id should pass provider parameter validation: %v", err)
	}
	want := map[string]string{"base_token": "b1", "context_id": "7663083417936891420"}
	if !reflect.DeepEqual(got.Resolved, want) {
		t.Fatalf("resolved=%v want %v", got.Resolved, want)
	}
}

// TestValidateParams_CrossOpReverseLookup pins the "declared on" teaching error: a
// param declared on send but passed to task_get names where it lives.
func TestValidateParams_CrossOpReverseLookup(t *testing.T) {
	spec := paramSpec()
	_, err := validateParams([]string{"priority=high"}, spec.GetTask.Params, iagents.VerbTaskGet, spec, "acme:reporter")
	if err == nil || !strings.Contains(err.Error(), "does not apply to task_get") || !strings.Contains(err.Error(), "declared on: send") {
		t.Fatalf("cross-op teaching error expected, got %v", err)
	}
}

// TestValidateParams_RulesTable covers the remaining violation kinds one by one.
func TestValidateParams_RulesTable(t *testing.T) {
	spec := paramSpec()
	base := []string{"workspace_id=ws_42"}
	cases := []struct {
		name string
		kvs  []string
		want string
	}{
		{"duplicate", append(base, "workspace_id=ws_43"), "given more than once"},
		{"empty required", []string{"workspace_id="}, "must not be empty"},
		{"malformed", append(base, "noequals"), "key=value"},
		{"type mismatch", append(base, "max_results=abc"), "integer"},
		{"range violation", append(base, "max_results=500"), "1..100"},
		{"zero-param op given a param", nil, ""},
	}
	for _, tc := range cases[:5] {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateParams(tc.kvs, spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
			if err == nil || !strings.Contains(err.Error()+errHint(err), tc.want) {
				t.Fatalf("want %q in error, got %v", tc.want, err)
			}
		})
	}
	// value containing '=' splits on the first '=' only
	vp, err := validateParams(append(base, "priority=high"), spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if err != nil || vp.Given["workspace_id"] != "ws_42" {
		t.Fatalf("valid set should pass: %v %v", vp, err)
	}
}

// TestValidateParams_EmptyOptionalTreatedAsAbsent pins the review fix (blocker):
// `k=` on an OPTIONAL param counts as not provided — no violation, no entry in
// Given, and the declared Default still backfills Resolved, so no unvalidated
// "" can ever reach a hook (the rt.Params() contract).
func TestValidateParams_EmptyOptionalTreatedAsAbsent(t *testing.T) {
	spec := paramSpec()
	vp, err := validateParams([]string{"workspace_id=ws_42", "max_results="}, spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if err != nil {
		t.Fatalf("empty optional should not violate: %v", err)
	}
	if got := vp.Resolved["max_results"]; got != "20" {
		t.Errorf("empty optional must not shadow the default (backfill still applies), got %q", got)
	}
	if _, ok := vp.Given["max_results"]; ok {
		t.Errorf("empty optional must not enter Given, got %v", vp.Given)
	}
	// empty on a declared optional with default: default wins in Resolved
	vp2, err := validateParams([]string{"workspace_id=ws_42", "priority="}, spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if err != nil {
		t.Fatalf("empty optional should not violate: %v", err)
	}
	if vp2.Resolved["priority"] != "normal" {
		t.Errorf("empty optional must not shadow the default, got %q", vp2.Resolved["priority"])
	}
	// duplicate detection still sees the empty occurrence
	_, err = validateParams([]string{"workspace_id=ws_42", "priority=", "priority=high"}, spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if err == nil || !strings.Contains(err.Error()+errHint(err), "given more than once") {
		t.Fatalf("duplicate after empty occurrence must be reported, got %v", err)
	}
}

// TestValidateParams_NoFalseMissingOnInvalidValue pins the review fix: a
// required param given an INVALID value reports exactly the value violation —
// never an additional contradictory "missing required parameter"; and a duplicate after an
// invalid first value is reported as duplicate, not as the same violation twice.
func TestValidateParams_NoFalseMissingOnInvalidValue(t *testing.T) {
	spec := paramSpec()
	// make workspace_id enum-constrained for this test via a local declaration
	decl := []iagents.CardParam{{Name: "mode", Type: "string", Required: true, Enum: []string{"a", "b"}}}
	_, err := validateParams([]string{"mode=zzz"}, decl, iagents.VerbSend, spec, "acme:reporter")
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want validation error, got %T", err)
	}
	if len(verr.Params) != 1 {
		t.Fatalf("invalid value must yield exactly 1 violation (no false missing-required), got %d: %+v", len(verr.Params), verr.Params)
	}
	if !strings.Contains(verr.Params[0].Reason, "a|b") {
		t.Errorf("the one violation should be the enum violation, got %+v", verr.Params[0])
	}
	// duplicate after invalid first value → enum violation + duplicate violation
	_, err = validateParams([]string{"mode=zzz", "mode=zzz"}, decl, iagents.VerbSend, spec, "acme:reporter")
	if !errors.As(err, &verr) {
		t.Fatalf("want validation error, got %T", err)
	}
	if len(verr.Params) != 2 {
		t.Fatalf("want enum violation + duplicate violation, got %d: %+v", len(verr.Params), verr.Params)
	}
	kinds := verr.Params[0].Reason + verr.Params[1].Reason
	if !strings.Contains(kinds, "a|b") || !strings.Contains(kinds, "given more than once") {
		t.Errorf("want one enum + one duplicate violation, got %+v", verr.Params)
	}
}

// objSpec is the object-param test bed: send declares a filter object
// (required enum leaf + optional ranged leaf + defaulted bool leaf) and a
// NoCarry trace param shared with task_get.
func objSpec() *iagents.AgentSpec {
	trace := iagents.CardParam{Name: "trace_tag", NoCarry: true, Required: true, Desc: "call-chain tag (fresh value per call)"}
	return &iagents.AgentSpec{
		Send: iagents.SendOp{
			Params: []iagents.CardParam{
				trace,
				{Name: "filter", Type: "object", Desc: "filter conditions", Fields: []iagents.CardParam{
					{Name: "region", Enum: []string{"east", "west"}, Required: true},
					{Name: "min_amount", Type: "number", Min: iagents.Float(0)},
					{Name: "active", Type: "boolean", Default: "true"},
				}},
			},
			Handler: func(context.Context, iagents.Runtime, iagents.SendInput) (*iagents.AgentTask, error) { return nil, nil },
		},
		GetTask: iagents.TaskGetOp{
			Params:  []iagents.CardParam{trace},
			Handler: func(context.Context, iagents.Runtime, string) (*iagents.AgentTask, error) { return nil, nil },
		},
	}
}

// TestValidateParams_ObjectDottedChannel pins the primary object transport:
// dotted leaves validate with leaf rules, defaults backfill per leaf, and the
// canonical Resolved form is flat dotted keys.
func TestValidateParams_ObjectDottedChannel(t *testing.T) {
	spec := objSpec()
	vp, err := validateParams(
		[]string{"trace_tag=t1", "filter.region=east", "filter.min_amount=100"},
		spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if err != nil {
		t.Fatalf("valid dotted set should pass: %v", err)
	}
	if vp.Resolved["filter.region"] != "east" || vp.Resolved["filter.min_amount"] != "100" {
		t.Errorf("dotted leaves should land flat in Resolved, got %v", vp.Resolved)
	}
	if vp.Resolved["filter.active"] != "true" {
		t.Errorf("leaf default should backfill, got %v", vp.Resolved)
	}

	// leaf teaching errors carry the dotted path
	_, err = validateParams([]string{"trace_tag=t1", "filter.region=north"}, spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if err == nil || !strings.Contains(err.Error(), "filter.region") || !strings.Contains(err.Error(), "east|west") {
		t.Fatalf("leaf enum violation should carry the dotted path + full set, got %v", err)
	}
	// unknown leaf lists the object's field set
	_, err = validateParams([]string{"trace_tag=t1", "filter.region=east", "filter.regoin=east"}, spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if err == nil || !strings.Contains(err.Error()+errHint(err), "filter accepts") {
		t.Fatalf("unknown leaf should list the field set, got %v", err)
	}
	// missing required leaf reported with dotted name
	_, err = validateParams([]string{"trace_tag=t1"}, spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if err == nil || !strings.Contains(err.Error(), "filter.region") {
		t.Fatalf("missing required leaf should be reported by dotted name, got %v", err)
	}
}

// TestValidateParams_ObjectJSONChannel pins the fallback transport: a JSON
// value validates per leaf and NORMALIZES into the same flat dotted keys — the
// provider cannot tell which channel the caller used. Mixing channels for one
// object is rejected.
func TestValidateParams_ObjectJSONChannel(t *testing.T) {
	spec := objSpec()
	vp, err := validateParams(
		[]string{"trace_tag=t1", `filter={"region":"east","min_amount":100}`},
		spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if err != nil {
		t.Fatalf("valid JSON object should pass: %v", err)
	}
	if vp.Resolved["filter.region"] != "east" || vp.Resolved["filter.min_amount"] != "100" {
		t.Errorf("JSON members should normalize to flat dotted keys (numbers literal), got %v", vp.Resolved)
	}
	if vp.Resolved["filter.active"] != "true" {
		t.Errorf("leaf default should backfill on the JSON channel too, got %v", vp.Resolved)
	}

	// invalid JSON → teaching error pointing at the dotted alternative. With
	// several violations the summary is in message and the detail in params[], so
	// assert via listReasons.
	_, err = validateParams([]string{"trace_tag=t1", "filter={not json"}, spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if err == nil || !strings.Contains(listReasons(err), "is not valid JSON") {
		t.Fatalf("bad JSON should teach, got %v", err)
	}
	if !strings.Contains(listReasons(err), "pass fields one by one") {
		t.Fatalf("bad JSON error should point at the dotted alternative, got %v", listReasons(err))
	}
	// member enum violation carries the dotted path
	_, err = validateParams([]string{"trace_tag=t1", `filter={"region":"north"}`}, spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if err == nil || !strings.Contains(err.Error(), "filter.region") {
		t.Fatalf("JSON member violation should carry the dotted path, got %v", err)
	}
	// unknown member listed against the field set
	_, err = validateParams([]string{"trace_tag=t1", `filter={"region":"east","foo":1}`}, spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if err == nil || !strings.Contains(err.Error()+errHint(err), "filter accepts") {
		t.Fatalf("unknown JSON member should list fields, got %v", err)
	}
	// channel mixing rejected
	_, err = validateParams([]string{"trace_tag=t1", `filter={"region":"east"}`, "filter.active=false"}, spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if err == nil || !strings.Contains(err.Error()+listReasons(err), "mixes the JSON and dotted-path forms") {
		t.Fatalf("channel mixing should be rejected, got %v", err)
	}
}

// listReasons flattens all violation reasons for containment asserts.
func listReasons(err error) string {
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		return ""
	}
	var b strings.Builder
	for _, v := range verr.Params {
		b.WriteString(v.Reason)
	}
	return b.String()
}

// TestParamArgsFor_ObjectAndNoCarry pins the carry semantics: object leaves
// carry as ordinary scalars; NoCarry params never ride literally — required
// ones degrade to placeholders so the caller supplies a FRESH value.
func TestParamArgsFor_ObjectAndNoCarry(t *testing.T) {
	spec := objSpec()
	given := map[string]string{"trace_tag": "t1", "filter.region": "east", "filter.min_amount": "100"}
	args, tpl := paramArgsFor(spec, iagents.VerbSend, given)
	if strings.Contains(args, "trace_tag=t1") {
		t.Errorf("NoCarry param must never ride literally, got %q", args)
	}
	if !strings.Contains(args, "--param trace_tag=<trace_tag>") || !tpl {
		t.Errorf("required NoCarry should degrade to a placeholder, got %q tpl=%v", args, tpl)
	}
	if !strings.Contains(args, "--param filter.region=east") || !strings.Contains(args, "--param filter.min_amount=100") {
		t.Errorf("object leaves should carry as ordinary scalars, got %q", args)
	}
	// target verb without the object (task_get) → only its own declaration carries
	args, _ = paramArgsFor(spec, iagents.VerbTaskGet, given)
	if strings.Contains(args, "filter") {
		t.Errorf("params undeclared on the target verb must not carry, got %q", args)
	}
}

func errHint(err error) string {
	if p, ok := errs.ProblemOf(err); ok {
		return p.Hint
	}
	return ""
}

// TestValidateParams_DefaultBackfill pins Resolved vs Given: defaults land in
// Resolved (what the hook sees) but never in Given (what meta.next carries).
func TestValidateParams_DefaultBackfill(t *testing.T) {
	spec := paramSpec()
	vp, err := validateParams([]string{"workspace_id=ws_42"}, spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if err != nil {
		t.Fatalf("should pass: %v", err)
	}
	if vp.Resolved["priority"] != "normal" || vp.Resolved["max_results"] != "20" {
		t.Errorf("defaults should backfill Resolved, got %v", vp.Resolved)
	}
	if _, ok := vp.Given["priority"]; ok {
		t.Errorf("defaults must NOT appear in Given (meta.next noise), got %v", vp.Given)
	}
	// an explicitly provided value overrides the default in Resolved
	vp2, _ := validateParams([]string{"workspace_id=ws_42", "priority=high"}, spec.Send.Params, iagents.VerbSend, spec, "acme:reporter")
	if vp2.Resolved["priority"] != "high" || vp2.Given["priority"] != "high" {
		t.Errorf("explicit value should override default, got %v / %v", vp2.Resolved, vp2.Given)
	}
}

// TestParamArgsFor pins the three-way carry rule.
func TestParamArgsFor(t *testing.T) {
	spec := paramSpec()
	// 1) given + whitelisted → literal carry (declaration order)
	args, tpl := paramArgsFor(spec, iagents.VerbSend, map[string]string{"workspace_id": "ws_42", "priority": "high"})
	if args != " --param workspace_id=ws_42 --param priority=high" || tpl {
		t.Errorf("literal carry wrong: %q tpl=%v", args, tpl)
	}
	// 2) given but whitelist-failing → required degrades to placeholder,
	//    optional drops
	args, tpl = paramArgsFor(spec, iagents.VerbSend, map[string]string{"workspace_id": "ws 42; rm", "priority": "value with spaces"})
	if !strings.Contains(args, "--param workspace_id=<workspace_id>") || strings.Contains(args, "priority") || !tpl {
		t.Errorf("degrade rule wrong: %q tpl=%v", args, tpl)
	}
	// 3) absent but required on the target verb → placeholder (cross-verb hole)
	args, tpl = paramArgsFor(spec, iagents.VerbTaskList, map[string]string{})
	if args != " --param workspace_id=<workspace_id>" || !tpl {
		t.Errorf("required-absent placeholder wrong: %q tpl=%v", args, tpl)
	}
	// nil spec / unknown verb carry nothing
	if a, _ := paramArgsFor(nil, iagents.VerbSend, nil); a != "" {
		t.Errorf("nil spec should carry nothing, got %q", a)
	}
}

// TestNextForTaskCarriesParams pins the wired outcome: a send with given params
// yields a poll hint carrying them literally.
func TestNextForTaskCarriesParams(t *testing.T) {
	spec := paramSpec()
	task := &iagents.AgentTask{TaskID: "task_1", State: iagents.StateWorking}
	// task_get declares no params on this spec → nothing to carry for the poll
	next := nextForTask("acme:reporter", task, spec, map[string]string{"workspace_id": "ws_42"}, iagents.VerbSend)
	if len(next) != 1 || strings.Contains(next[0].Command, "--param") {
		t.Fatalf("task_get declares no params, poll hint should carry none: %+v", next)
	}
	// give task_get a required param → the poll hint must carry it
	spec.GetTask.Params = []iagents.CardParam{{Name: "workspace_id", Type: "string", Required: true}}
	next = nextForTask("acme:reporter", task, spec, map[string]string{"workspace_id": "ws_42"}, iagents.VerbSend)
	if !strings.Contains(next[0].Command, "--param workspace_id=ws_42") {
		t.Fatalf("poll hint should carry the given required param: %+v", next)
	}
	// absent → placeholder + template
	next = nextForTask("acme:reporter", task, spec, nil, iagents.VerbSend)
	if !strings.Contains(next[0].Command, "--param workspace_id=<workspace_id>") || !next[0].Template {
		t.Fatalf("absent required should degrade to placeholder+template: %+v", next)
	}
}

// TestArtifactNext pins the per-artifact download hints: terminal task +
// wired DownloadArtifact → one template hint per whitelisted artifact id;
// whitelist-failing ids are skipped (never interpolated).
func TestArtifactNext(t *testing.T) {
	spec := paramSpec()
	spec.DownloadArtifact = iagents.ArtifactDownloadOp{
		Params:  []iagents.CardParam{{Name: "workspace_id", Type: "string", Required: true}},
		Handler: func(context.Context, iagents.Runtime, string, string) (*iagents.ArtifactData, error) { return nil, nil },
	}
	task := &iagents.AgentTask{
		TaskID: "task_1", State: iagents.StateCompleted, IsTerminal: true,
		Artifacts: []iagents.Artifact{{ID: "art_1"}, {ID: "bad;id"}, {ID: "art_2"}},
	}
	next := nextForTask("acme:reporter", task, spec, map[string]string{"workspace_id": "ws_42"}, iagents.VerbSend)
	var downloads []string
	for _, n := range next {
		if strings.Contains(n.Command, "--artifact") {
			downloads = append(downloads, n.Command)
			if !n.Template {
				t.Errorf("download hint has a -o placeholder, must be template: %+v", n)
			}
		}
	}
	if len(downloads) != 2 {
		t.Fatalf("want 2 download hints (bad;id skipped), got %d: %v", len(downloads), downloads)
	}
	for _, c := range downloads {
		if !strings.Contains(c, "--param workspace_id=ws_42") || !strings.Contains(c, "-o <save_path>") {
			t.Errorf("download hint should carry params and the -o placeholder: %q", c)
		}
		if strings.Contains(c, "bad;id") {
			t.Errorf("whitelist-failing artifact id leaked: %q", c)
		}
	}
	// unwired DownloadArtifact → no hints
	spec.DownloadArtifact = iagents.ArtifactDownloadOp{}
	if n := artifactNext("acme:reporter", task, spec, nil); n != nil {
		t.Errorf("unwired artifact_download should produce no hints, got %+v", n)
	}
}

// TestCardOperationSubquery pins `card --operation <verb>` against the real
// catalog provider: fakecat:full's send contract carries command + parameters;
// unknown verb lists the vocabulary; unwired verb answers supported:false; a
// wired zero-param verb answers parameters:[].
func TestCardOperationSubquery(t *testing.T) {
	decode := func(t *testing.T, opts *cardOptions) map[string]any {
		t.Helper()
		out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })
		if err := agentCardRun(opts); err != nil {
			t.Fatalf("card --operation should not error: %v", err)
		}
		var env struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(out.Bytes(), &env); err != nil {
			t.Fatalf("invalid envelope: %v", err)
		}
		return env.Data
	}

	opts, _ := cardTestOpts(t, "fakecat:full")
	opts.Operation = "send"
	data := decode(t, opts)
	if data["operation"] != "send" || data["supported"] != true {
		t.Fatalf("send contract wrong: %v", data)
	}
	if cmdStr, _ := data["command"].(string); !strings.Contains(cmdStr, "lark-cli agents send") {
		t.Errorf("contract should carry the command template, got %v", data["command"])
	}
	params, _ := data["parameters"].([]any)
	if len(params) != 3 {
		t.Fatalf("fakecat:full send declares 3 demo params (2 scalars + render object), got %v", data["parameters"])
	}
	first, _ := params[0].(map[string]any)
	if first["name"] != "report_format" || first["default"] != "csv" {
		t.Errorf("first param should be report_format with default csv, got %v", first)
	}

	// unwired verb → supported:false
	opts2, _ := cardTestOpts(t, "fakecat:min")
	opts2.Operation = "task_cancel"
	data = decode(t, opts2)
	if data["supported"] != false {
		t.Errorf("fakecat:min task_cancel should be supported:false, got %v", data)
	}

	// wired zero-param verb → parameters []
	opts3, _ := cardTestOpts(t, "fakecat:min")
	opts3.Operation = "context_delete"
	data = decode(t, opts3)
	if data["supported"] != true {
		t.Fatalf("fakecat:min context_delete should be supported, got %v", data)
	}
	if ps, ok := data["parameters"].([]any); !ok || len(ps) != 0 {
		t.Errorf("zero-param op should answer parameters:[], got %v", data["parameters"])
	}

	// unknown verb → invalid_argument listing the vocabulary
	opts4, _ := cardTestOpts(t, "fakecat:min")
	opts4.Operation = "sennd"
	err := agentCardRun(opts4)
	if err == nil || !strings.Contains(err.Error(), "task_get") || !strings.Contains(err.Error(), "all") {
		t.Fatalf("unknown verb should list the vocabulary, got %v", err)
	}
	if p, ok := errs.ProblemOf(err); !ok || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("unknown verb should be invalid_argument, got %+v", p)
	}
}

// TestCardOperationInstanceShape pins the review fix: on an INSTANCE provider
// (fakeflow), the single-verb --operation output reuses the struct — an
// unwired verb carries NO command key (omitempty, not command:"") and every
// response carries parameters_source:"template".
func TestCardOperationInstanceShape(t *testing.T) {
	registerScripted()
	opts, _ := cardTestOpts(t, "fakemin:agt_x")
	opts.Operation = "task_cancel" // minimalSpec leaves CancelTask unwired
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })
	if err := agentCardRun(opts); err != nil {
		t.Fatalf("card --operation should not error: %v", err)
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("invalid envelope: %v", err)
	}
	if env.Data["supported"] != false {
		t.Fatalf("task_cancel should be unsupported on the scripted spec, got %v", env.Data)
	}
	if _, present := env.Data["command"]; present {
		t.Errorf("unwired verb must not carry a command key (omitempty), got %v", env.Data["command"])
	}
	if env.Data["parameters_source"] != "template" {
		t.Errorf("instance provider --operation should carry parameters_source:template, got %v", env.Data)
	}
}

// TestCardOperationAll pins the one-shot full map: every verb present, wired
// ones carrying command+parameters.
func TestCardOperationAll(t *testing.T) {
	opts, _ := cardTestOpts(t, "fakecat:full")
	opts.Operation = "all"
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })
	if err := agentCardRun(opts); err != nil {
		t.Fatalf("card --operation all should not error: %v", err)
	}
	var env struct {
		Data struct {
			Operations map[string]map[string]any `json:"operations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("invalid envelope: %v", err)
	}
	if len(env.Data.Operations) != 8 {
		t.Fatalf("all should enumerate 8 operations, got %d", len(env.Data.Operations))
	}
	send := env.Data.Operations["send"]
	if send["supported"] != true {
		t.Errorf("fakecat:full send should be supported, got %v", send)
	}
	if ps, _ := send["parameters"].([]any); len(ps) != 3 {
		t.Errorf("fakecat:full send should carry its 3 demo params, got %v", send["parameters"])
	}
}

// TestCardLeanHasParameters pins the lean card cue on fakecat:full: send
// appears in has_parameters (it declares demo params), context_delete does not.
func TestCardLeanHasParameters(t *testing.T) {
	opts, _ := cardTestOpts(t, "fakecat:full")
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })
	if err := agentCardRun(opts); err != nil {
		t.Fatalf("card should not error: %v", err)
	}
	var env struct {
		Data struct {
			HasParameters []string `json:"has_parameters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("invalid envelope: %v", err)
	}
	if len(env.Data.HasParameters) != 1 || env.Data.HasParameters[0] != "send" {
		t.Fatalf("fakecat:full has_parameters should be [send], got %v", env.Data.HasParameters)
	}
}

// TestSendValidatesDeclaredParams drives the full send path against the
// fakecat:full declaration: enum violation fails offline; a valid --param passes
// through to dry-run with defaults backfilled.
func TestSendValidatesDeclaredParams(t *testing.T) {
	opts := sendTestOpts(t)
	opts.Ref = "fakecat:full"
	opts.Text = "report"
	opts.Params = []string{"report_format=pdf"}
	err := agentSendRun(opts)
	if err == nil || !strings.Contains(err.Error(), "csv|xlsx") {
		t.Fatalf("enum violation should fail offline listing the set, got %v", err)
	}

	opts2 := sendTestOpts(t)
	opts2.Ref = "fakecat:full"
	opts2.Text = "report"
	opts2.Params = []string{"report_format=xlsx"}
	opts2.DryRun = true
	out := opts2.Factory.IOStreams.Out.(interface{ Bytes() []byte })
	if err := agentSendRun(opts2); err != nil {
		t.Fatalf("valid param should pass: %v", err)
	}
	var env struct {
		Data struct {
			WouldSend struct {
				Params map[string]string `json:"params"`
			} `json:"would_send"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("invalid envelope: %v", err)
	}
	if env.Data.WouldSend.Params["report_format"] != "xlsx" || env.Data.WouldSend.Params["quarters"] != "4" {
		t.Fatalf("dry-run should show the resolved params (default quarters=4 backfilled), got %v", env.Data.WouldSend.Params)
	}
}

// TestListRejectsParams pins the two list guards: --param without a scheme is
// rejected outright; --param on a catalog scheme validates against the empty
// set with the list-specific hint.
func TestListRejectsParams(t *testing.T) {
	opts, _ := listFactory()
	opts.Params = []string{"env=boe"}
	err := agentListRun(opts)
	if err == nil || !strings.Contains(err.Error(), "only means something with agents list <scheme>") {
		t.Fatalf("no-scheme --param should be rejected, got %v", err)
	}

	opts2, _ := listFactory()
	opts2.Scheme = "base"
	opts2.Params = []string{"env=boe"}
	err = agentListRun(opts2)
	if err == nil {
		t.Fatal("catalog scheme with --param should be rejected (zero-param op)")
	}
	if p, ok := errs.ProblemOf(err); !ok || !strings.Contains(p.Hint, "list_parameters") {
		t.Fatalf("list param error hint should point at providers[].list_parameters, got %+v", p)
	}
}
