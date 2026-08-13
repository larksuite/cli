// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// brandFactory builds a test Factory whose resolved Config.Brand is the given
// brand, so the command-layer brand gates exercise both feishu and lark.
func brandFactory(t *testing.T, brand core.LarkBrand) *cmdutil.Factory {
	t.Helper()
	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: brand}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	return f
}

// TestCardBrandScoped pins that `agents card fakecat:full` renders a
// brand-scoped card: under feishu task_cancel is true and data.brand=="feishu";
// under lark the feishu-only task_cancel op flips to false and
// data.brand=="lark". The agent itself stays visible under both brands (only the
// op is scoped), so the card renders in both cases.
func TestCardBrandScoped(t *testing.T) {
	for _, tc := range []struct {
		brand          core.LarkBrand
		wantTaskCancel bool
	}{
		{core.BrandFeishu, true},
		{core.BrandLark, false},
	} {
		t.Run(string(tc.brand), func(t *testing.T) {
			f := brandFactory(t, tc.brand)
			opts := &cardOptions{Factory: f, Cmd: resolveCmd(t, true, "bot"), Ref: "fakecat:full", As: "bot", Format: "json"}
			out := f.IOStreams.Out.(interface{ Bytes() []byte })

			if err := agentCardRun(opts); err != nil {
				t.Fatalf("card should render under %s: %v", tc.brand, err)
			}
			var env output.Envelope
			if err := json.Unmarshal(out.Bytes(), &env); err != nil {
				t.Fatalf("card output should be valid envelope JSON: %v", err)
			}
			data, ok := env.Data.(map[string]interface{})
			if !ok {
				t.Fatalf("data should be a card object, got %T", env.Data)
			}
			if data["brand"] != string(tc.brand) {
				t.Errorf("card.brand should be %q, got %v", tc.brand, data["brand"])
			}
			caps, ok := data["capabilities"].(map[string]interface{})
			if !ok {
				t.Fatalf("capabilities should be an object, got %T", data["capabilities"])
			}
			if caps["task_cancel"] != tc.wantTaskCancel {
				t.Errorf("%s: task_cancel should be %v, got %v", tc.brand, tc.wantTaskCancel, caps["task_cancel"])
			}
		})
	}
}

// TestTaskCancelBrandGatedUnderLark pins the per-capability brand gate: under
// lark, `agents task cancel fakecat:full` (task_cancel is feishu-only) is
// rejected offline with the unavailable_for_brand validation error (exit 2)
// before any request — the CancelTask handler IS wired, so this is a brand gate,
// not an unsupported_capability gate.
func TestTaskCancelBrandGatedUnderLark(t *testing.T) {
	f := brandFactory(t, core.BrandLark)
	err := agentTaskCancelRun(&taskOptions{
		Factory: f, Cmd: taskCmdCtx(t, "cancel"), Ref: "fakecat:full", TaskID: "t1", As: "bot",
	})
	if err == nil {
		t.Fatal("task cancel under lark should be gated (unavailable_for_brand)")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("want a validation error, got %T", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeUnavailableForBrand {
		t.Fatalf("subtype should be unavailable_for_brand, got %+v", p)
	}
	if output.ExitCodeOf(err) != output.ExitValidation {
		t.Fatalf("exit should be %d, got %d", output.ExitValidation, output.ExitCodeOf(err))
	}
}

// TestTaskCancelReachesHandlerUnderFeishu pins the sibling of the gate: under
// feishu the feishu-scoped task_cancel is live, so the command passes both brand
// gates and reaches the provider handler — for an unknown task the fake handler
// returns invalid_argument (unknown task id), never unavailable_for_brand.
func TestTaskCancelReachesHandlerUnderFeishu(t *testing.T) {
	f := brandFactory(t, core.BrandFeishu)
	err := agentTaskCancelRun(&taskOptions{
		Factory: f, Cmd: taskCmdCtx(t, "cancel"), Ref: "fakecat:full", TaskID: "nope_task", As: "bot",
	})
	if err == nil {
		t.Fatal("cancel of an unknown task should error from the handler")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("want a typed problem, got %T: %v", err, err)
	}
	if p.Subtype == errs.SubtypeUnavailableForBrand {
		t.Fatal("under feishu the brand gate must NOT fire — the handler should run")
	}
	if p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected the handler's unknown-task invalid_argument, got %+v", p)
	}
}

// TestListCatalogIncludesReporterBothBrands pins that an op-level brand tag does
// NOT hide the whole agent: fakecat:full appears in the catalog listing under
// both feishu and lark (only its task_cancel capability differs by brand).
func TestListCatalogIncludesReporterBothBrands(t *testing.T) {
	prov, ok := iagents.Info("fakecat")
	if !ok {
		t.Fatal("fakecat provider should be registered")
	}
	for _, brand := range []core.LarkBrand{core.BrandFeishu, core.BrandLark} {
		found := false
		for _, a := range prov.ListCatalog(brand) {
			if a.AgentRef == "fakecat:full" {
				found = true
			}
		}
		if !found {
			t.Errorf("fakecat:full should be listed under %s (op-level tag must not hide the agent)", brand)
		}
	}
}

// registerBrandHiddenOnce registers the feishu-only catalog agent exactly once
// (Register panics on dup). Its ListTasks is deliberately UNWIRED so the
// whole-agent brand gate can be tested against a verb the agent does not even
// implement — the ordering assertion behind fix #1.
var registerBrandHiddenOnce sync.Once

func registerBrandHidden() {
	registerBrandHiddenOnce.Do(func() {
		task := func(context.Context, iagents.Runtime, string) (*iagents.AgentTask, error) {
			return &iagents.AgentTask{TaskID: "t", State: iagents.StateCompleted}, nil
		}
		iagents.Register(iagents.Provider{
			Scheme:        "brandhidden",
			Label:         "test fake (feishu-only agent)",
			AgentIDSource: "test only",
			Identities:    []iagents.IdentitySpec{{Type: iagents.IdentityUser}, {Type: iagents.IdentityBot}},
			Catalog: []iagents.AgentSpec{{
				ID:     "x",
				Name:   "hidden demo",
				Brands: []core.LarkBrand{core.BrandFeishu},
				Send: iagents.SendOp{Handler: func(_ context.Context, _ iagents.Runtime, _ iagents.SendInput) (*iagents.AgentTask, error) {
					return &iagents.AgentTask{TaskID: "t", State: iagents.StateCompleted}, nil
				}},
				GetTask: iagents.TaskGetOp{Handler: task},
				// ListTasks intentionally UNWIRED.
			}},
		})
	})
}

// TestWholeAgentBrandGatedUnderLark pins the whole-agent brand gate AND its
// ordering: a feishu-only agent (spec.Brands=[feishu]) reports
// unavailable_for_brand under lark for EVERY verb — including task list, whose
// handler is unwired. If the capability nil-gate ran first, task list would
// misreport unsupported_capability; the whole-agent brand gate must fire before
// it. Under feishu the agent is visible and its card renders.
func TestWholeAgentBrandGatedUnderLark(t *testing.T) {
	registerBrandHidden()
	lark := brandFactory(t, core.BrandLark)

	errCard := agentCardRun(&cardOptions{Factory: lark, Cmd: resolveCmd(t, true, "bot"), Ref: "brandhidden:x", As: "bot", Format: "json"})
	assertUnavailableWholeAgent(t, errCard, "card")

	errList := agentTaskListRun(&taskOptions{Factory: lark, Cmd: taskCmdCtx(t, "list"), Ref: "brandhidden:x", As: "bot", Format: "json"})
	assertUnavailableWholeAgent(t, errList, "task list (unwired verb)")

	feishu := brandFactory(t, core.BrandFeishu)
	out := feishu.IOStreams.Out.(interface{ Bytes() []byte })
	if err := agentCardRun(&cardOptions{Factory: feishu, Cmd: resolveCmd(t, true, "bot"), Ref: "brandhidden:x", As: "bot", Format: "json"}); err != nil {
		t.Fatalf("card should render under feishu (agent visible): %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("card output should be valid JSON: %v", err)
	}
	if data, _ := env.Data.(map[string]interface{}); data["brand"] != "feishu" {
		t.Errorf("under feishu card.brand should be feishu, got %v", data["brand"])
	}
}

// assertUnavailableWholeAgent checks err is the WHOLE-AGENT unavailable_for_brand
// form: subtype unavailable_for_brand, naming the lark brand, and with NO verb
// named (the op form reads "does not offer '<verb>'"; the whole-agent form
// omits the verb since the entire agent is hidden).
func assertUnavailableWholeAgent(t *testing.T, err error, where string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s under lark should be gated", where)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeUnavailableForBrand {
		t.Fatalf("%s: subtype should be unavailable_for_brand, got %+v", where, p)
	}
	if strings.Contains(p.Message, "does not offer") {
		t.Errorf("%s: expected the whole-agent message (no verb named), got %q", where, p.Message)
	}
	if !strings.Contains(p.Message, "is unavailable under the lark brand") {
		t.Errorf("%s: message should name the lark brand, got %q", where, p.Message)
	}
}

// TestResolvedBrandDefaults pins resolvedBrand's resolution + offline default:
// nil Factory and an empty configured Brand both fall back to feishu (consistent
// with core.ParseBrand); an explicit brand is returned as-is.
func TestResolvedBrandDefaults(t *testing.T) {
	if got := resolvedBrand(nil); got != core.BrandFeishu {
		t.Errorf("resolvedBrand(nil) should default to feishu, got %q", got)
	}
	if got := resolvedBrand(brandFactory(t, "")); got != core.BrandFeishu {
		t.Errorf("resolvedBrand with empty Brand should default to feishu, got %q", got)
	}
	if got := resolvedBrand(brandFactory(t, core.BrandLark)); got != core.BrandLark {
		t.Errorf("resolvedBrand should return the configured lark brand, got %q", got)
	}
	if got := resolvedBrand(brandFactory(t, core.BrandFeishu)); got != core.BrandFeishu {
		t.Errorf("resolvedBrand should return the configured feishu brand, got %q", got)
	}
}
