// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"sync"
	"testing"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/core"
)

// scriptedHooks scripts a fake provider's behavior per test. Each hook maps to
// one AgentSpec verb; an unset hook that gets called panics — a tripwire against
// a test reaching an unexpected provider path. The command-layer contracts under
// test (envelope shape, watch exit codes, meta.next, pretty rendering, error
// propagation) are provider-neutral, so the scripted hooks ignore the Runtime.
type scriptedHooks struct {
	send             func(in iagents.SendInput) (*iagents.AgentTask, error)
	getTask          func(taskID string) (*iagents.AgentTask, error)
	listTasks        func(contextID string, page iagents.PageParams) ([]iagents.TaskSummary, iagents.PageInfo, error)
	listContexts     func(page iagents.PageParams) ([]iagents.ContextSummary, iagents.PageInfo, error)
	getContext       func(ctxID string) (*iagents.ContextDetail, error)
	deleteContext    func(ctxID string) error
	cancelTask       func(taskID string) error
	downloadArtifact func(taskID, artifactID string) (*iagents.ArtifactData, error)
}

// scripted is the package-level hook set shared by every scripted instance (the
// registered provider is fixed per package run, the hooks can be re-pointed).
var scripted scriptedHooks

// fakeUserOnlyDescribe is a test seam for the user-only provider's optional
// dynamic Card enrichment. Card tests use it to prove an unsupported identity
// gets the static card without invoking Describe, while a supported identity
// may enrich it.
var fakeUserOnlyDescribe func(iagents.Runtime) (*iagents.CardInfo, error)

// setScripted installs the hooks for one test and restores the empty (panic
// tripwire) set on cleanup.
func setScripted(t *testing.T, h scriptedHooks) {
	t.Helper()
	scripted = h
	t.Cleanup(func() { scripted = scriptedHooks{} })
}

// scriptedSpec is the instance template whose capability surface is fixed by
// which hooks are wired: everything the command tests drive is wired (the
// task_cancel unsupported gate is exercised via fakecat:min, whose spec leaves
// it unwired), FileInput=true so the --file gate/confirm path is reachable, and
// InputRequired=true so the --answer capability gate passes (Register requires
// a question-asking spec to wire CancelTask, hence the cancel hook). Each wired
// hook delegates to the per-test hook and panics if it was not set.
func scriptedSpec() *iagents.AgentSpec {
	return &iagents.AgentSpec{
		FileInput:     true,
		InputRequired: true,
		CancelTask: iagents.TaskCancelOp{Handler: func(_ context.Context, _ iagents.Runtime, taskID string) error {
			if scripted.cancelTask == nil {
				panic("scripted provider: CancelTask hook not set")
			}
			return scripted.cancelTask(taskID)
		}},
		Send: iagents.SendOp{Handler: func(_ context.Context, _ iagents.Runtime, in iagents.SendInput) (*iagents.AgentTask, error) {
			if scripted.send == nil {
				panic("scripted provider: Send hook not set")
			}
			return scripted.send(in)
		}},
		GetTask: iagents.TaskGetOp{Handler: func(_ context.Context, _ iagents.Runtime, taskID string) (*iagents.AgentTask, error) {
			if scripted.getTask == nil {
				panic("scripted provider: GetTask hook not set")
			}
			return scripted.getTask(taskID)
		}},
		ListTasks: iagents.TaskListOp{Handler: func(_ context.Context, _ iagents.Runtime, contextID string, page iagents.PageParams) ([]iagents.TaskSummary, iagents.PageInfo, error) {
			if scripted.listTasks == nil {
				panic("scripted provider: ListTasks hook not set")
			}
			return scripted.listTasks(contextID, page)
		}},
		ListContexts: iagents.ContextListOp{Handler: func(_ context.Context, _ iagents.Runtime, page iagents.PageParams) ([]iagents.ContextSummary, iagents.PageInfo, error) {
			if scripted.listContexts == nil {
				panic("scripted provider: ListContexts hook not set")
			}
			return scripted.listContexts(page)
		}},
		GetContext: iagents.ContextGetOp{Handler: func(_ context.Context, _ iagents.Runtime, ctxID string) (*iagents.ContextDetail, error) {
			if scripted.getContext == nil {
				panic("scripted provider: GetContext hook not set")
			}
			return scripted.getContext(ctxID)
		}},
		DeleteContext: iagents.ContextDeleteOp{Handler: func(_ context.Context, _ iagents.Runtime, ctxID string) error {
			if scripted.deleteContext == nil {
				panic("scripted provider: DeleteContext hook not set")
			}
			return scripted.deleteContext(ctxID)
		}},
		DownloadArtifact: iagents.ArtifactDownloadOp{Handler: func(_ context.Context, _ iagents.Runtime, taskID, artifactID string) (*iagents.ArtifactData, error) {
			if scripted.downloadArtifact == nil {
				panic("scripted provider: DownloadArtifact hook not set")
			}
			return scripted.downloadArtifact(taskID, artifactID)
		}},
	}
}

func scriptedUserOnlySpec() *iagents.AgentSpec {
	spec := scriptedSpec()
	spec.Describe = func(_ context.Context, rt iagents.Runtime) (*iagents.CardInfo, error) {
		if fakeUserOnlyDescribe == nil {
			panic("scripted user-only provider: Describe hook not set")
		}
		return fakeUserOnlyDescribe(rt)
	}
	return spec
}

// fakescopedAllScopes is the scope set declared on BOTH identities of the
// fakescoped test provider, sorted — the all-or-nothing preflight requires
// every one for any real API verb.
var fakescopedAllScopes = []string{
	"fakescoped:agent_artifact:read",
	"fakescoped:agent_attachment:write",
	"fakescoped:agent_chat:read",
	"fakescoped:agent_chat:write",
}

// fakeflowAgentIDSource is the AgentIDSource text of the fakeflow provider —
// the non-enumerable `agents list <scheme>` error surfaces it as the hint.
const fakeflowAgentIDSource = "get an agent_id from the fakeflow test console (shaped like agt_xxx)"

// minimalSpec is the least-capable legal instance template: only the two core
// verbs are wired (with tripwire handlers — these tests never reach them), so
// every optional verb is honestly unsupported. It is the vehicle for
// unwired-verb shape/ordering tests now that scriptedSpec wires everything.
func minimalSpec() *iagents.AgentSpec {
	return &iagents.AgentSpec{
		Send: iagents.SendOp{Handler: func(_ context.Context, _ iagents.Runtime, _ iagents.SendInput) (*iagents.AgentTask, error) {
			panic("fakemin provider: not callable")
		}},
		GetTask: iagents.TaskGetOp{Handler: func(_ context.Context, _ iagents.Runtime, _ string) (*iagents.AgentTask, error) {
			panic("fakemin provider: not callable")
		}},
	}
}

// fakecatTask is the canned task every fakecat handler answers with.
func fakecatTask(id string) *iagents.AgentTask {
	return &iagents.AgentTask{TaskID: id, State: iagents.StateCompleted, IsTerminal: true}
}

// fakecatMinSpec is the catalog fake's least-capable agent: the core verbs plus
// the read/delete verbs, and nothing else — so task_cancel, artifact_download,
// file_input and input_required are all honestly false. It is the vehicle for the
// capability-gate tests on a CATALOG provider (the fakemin instance provider
// covers the same ground for instance-type).
func fakecatMinSpec() iagents.AgentSpec {
	return iagents.AgentSpec{
		ID:          "min",
		Name:        "minimal catalog agent",
		Description: "Echoes the request back. Minimal capability set.",
		Send: iagents.SendOp{Handler: func(_ context.Context, _ iagents.Runtime, in iagents.SendInput) (*iagents.AgentTask, error) {
			return fakecatTask("task_min"), nil
		}},
		GetTask: iagents.TaskGetOp{Handler: func(_ context.Context, _ iagents.Runtime, taskID string) (*iagents.AgentTask, error) {
			return fakecatTask(taskID), nil
		}},
		ListTasks: iagents.TaskListOp{Handler: func(_ context.Context, _ iagents.Runtime, _ string, _ iagents.PageParams) ([]iagents.TaskSummary, iagents.PageInfo, error) {
			return nil, iagents.PageInfo{}, nil
		}},
		ListContexts: iagents.ContextListOp{Handler: func(_ context.Context, _ iagents.Runtime, _ iagents.PageParams) ([]iagents.ContextSummary, iagents.PageInfo, error) {
			return nil, iagents.PageInfo{}, nil
		}},
		GetContext: iagents.ContextGetOp{Handler: func(_ context.Context, _ iagents.Runtime, ctxID string) (*iagents.ContextDetail, error) {
			return &iagents.ContextDetail{ContextID: ctxID}, nil
		}},
		DeleteContext: iagents.ContextDeleteOp{Handler: func(_ context.Context, _ iagents.Runtime, _ string) error {
			return nil
		}},
	}
}

// fakecatFullSpec is the catalog fake's fully-capable agent: it additionally
// wires CancelTask + DownloadArtifact, declares FileInput, and declares business
// params on send only (2 scalars + 1 object) — the vehicle for the card
// --operation / declared-param / has_parameters contracts. Its CancelTask is
// scoped to feishu, which is what the per-op brand gate is tested against: the
// agent stays visible under both brands, only this op is scoped.
func fakecatFullSpec() iagents.AgentSpec {
	return iagents.AgentSpec{
		ID:          "full",
		Name:        "full catalog agent",
		Description: "Produces a report artifact. Full capability set.",
		FileInput:   true,
		Send: iagents.SendOp{
			Params: []iagents.CardParam{
				{Name: "report_format", Enum: []string{"csv", "xlsx"}, Default: "csv", Desc: "report output format"},
				{Name: "quarters", Type: "integer", Min: iagents.Float(1), Max: iagents.Float(12), Default: "4", Desc: "quarters to look back"},
				{Name: "render", Type: "object", Desc: "render options", Fields: []iagents.CardParam{
					{Name: "theme", Enum: []string{"light", "dark"}, Default: "light", Desc: "color theme"},
					{Name: "watermark", Type: "boolean", Default: "false", Desc: "add a watermark"},
				}},
			},
			Handler: func(_ context.Context, _ iagents.Runtime, _ iagents.SendInput) (*iagents.AgentTask, error) {
				return fakecatTask("task_full"), nil
			},
		},
		GetTask: iagents.TaskGetOp{Handler: func(_ context.Context, _ iagents.Runtime, taskID string) (*iagents.AgentTask, error) {
			return fakecatTask(taskID), nil
		}},
		ListTasks: iagents.TaskListOp{Handler: func(_ context.Context, _ iagents.Runtime, _ string, _ iagents.PageParams) ([]iagents.TaskSummary, iagents.PageInfo, error) {
			return nil, iagents.PageInfo{}, nil
		}},
		ListContexts: iagents.ContextListOp{Handler: func(_ context.Context, _ iagents.Runtime, _ iagents.PageParams) ([]iagents.ContextSummary, iagents.PageInfo, error) {
			return nil, iagents.PageInfo{}, nil
		}},
		GetContext: iagents.ContextGetOp{Handler: func(_ context.Context, _ iagents.Runtime, ctxID string) (*iagents.ContextDetail, error) {
			return &iagents.ContextDetail{ContextID: ctxID}, nil
		}},
		DeleteContext: iagents.ContextDeleteOp{Handler: func(_ context.Context, _ iagents.Runtime, _ string) error {
			return nil
		}},
		CancelTask: iagents.TaskCancelOp{
			Brands: []core.LarkBrand{core.BrandFeishu},
			Handler: func(_ context.Context, _ iagents.Runtime, taskID string) error {
				if taskID != "t1" {
					return errs.NewValidationError(errs.SubtypeInvalidArgument, "unknown task id: "+taskID)
				}
				return nil
			},
		},
		DownloadArtifact: iagents.ArtifactDownloadOp{Handler: func(_ context.Context, _ iagents.Runtime, _, artifactID string) (*iagents.ArtifactData, error) {
			return &iagents.ArtifactData{Name: artifactID + ".csv", Mime: "text/csv", Bytes: []byte("a,b\n1,2\n")}, nil
		}},
	}
}

// registerScripted registers the scripted schemes exactly once (Register panics
// on duplicates). All but fakecat are instance-type (agent_id is arbitrary) and
// not enumerable (no ListAgents hook). They leak into the package-level registry
// for the rest of this package run — so no test may assert an exact provider set.
//
//   - fakeflow: no scopes on either identity (preflight always passes) — the
//     workhorse.
//   - fakescoped: the same 4-scope set on both identities, for the
//     scope-preflight tests.
//   - fakemin: the same 4-scope set on the minimal spec — the vehicle for
//     unwired-verb gating (its capability gate must answer before preflight).
//   - fakeuseronly: no scopes, user identity only.
//   - fakesplit: asymmetric per-identity scopes — user declares one scope, bot
//     declares none — pinning that the preflight resolves the required set from
//     the RESOLVED identity's declaration.
//   - fakecat: the only CATALOG-type fake (agents are enumerable data), carrying
//     a minimal and a fully-capable agent so the catalog-specific paths
//     (unknown-agent-id, per-agent capability differences, declared params,
//     per-op brand scoping) are exercisable offline.
var registerScriptedOnce sync.Once

func registerScripted() {
	registerScriptedOnce.Do(func() {
		iagents.Register(iagents.Provider{
			Scheme:        "fakeflow",
			Label:         "test fake (scripted flow)",
			AgentIDSource: fakeflowAgentIDSource,
			Identities:    []iagents.IdentitySpec{{Type: iagents.IdentityUser}, {Type: iagents.IdentityBot}},
			Instance:      scriptedSpec(),
		})
		iagents.Register(iagents.Provider{
			Scheme:        "fakescoped",
			Label:         "test fake (scoped)",
			AgentIDSource: "test only",
			Identities: []iagents.IdentitySpec{
				{Type: iagents.IdentityUser, Scopes: fakescopedAllScopes},
				{Type: iagents.IdentityBot, Scopes: fakescopedAllScopes},
			},
			Instance: scriptedSpec(),
		})
		iagents.Register(iagents.Provider{
			Scheme:        "fakemin",
			Label:         "test fake (minimal caps)",
			AgentIDSource: "test only",
			Identities: []iagents.IdentitySpec{
				{Type: iagents.IdentityUser, Scopes: fakescopedAllScopes},
				{Type: iagents.IdentityBot, Scopes: fakescopedAllScopes},
			},
			Instance: minimalSpec(),
		})
		iagents.Register(iagents.Provider{
			Scheme:        "fakeuseronly",
			Label:         "test fake (user only)",
			AgentIDSource: "test only",
			Identities:    []iagents.IdentitySpec{{Type: iagents.IdentityUser}},
			Instance:      scriptedUserOnlySpec(),
		})
		iagents.Register(iagents.Provider{
			Scheme:        "fakesplit",
			Label:         "test fake (split scopes)",
			AgentIDSource: "test only",
			Identities: []iagents.IdentitySpec{
				{Type: iagents.IdentityUser, Scopes: []string{"fakesplit:user:read"}},
				{Type: iagents.IdentityBot}, // no scopes: the bot preflight (and its tenant-scope fetch) must be skipped
			},
			Instance: scriptedSpec(),
		})
		iagents.Register(iagents.Provider{
			Scheme:        "fakecat",
			Label:         "test fake (catalog)",
			AgentIDSource: "test only",
			Identities:    []iagents.IdentitySpec{{Type: iagents.IdentityUser}, {Type: iagents.IdentityBot}},
			Catalog:       []iagents.AgentSpec{fakecatMinSpec(), fakecatFullSpec()},
		})
	})
}
