// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package shortcuts

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

// agentSupportGaps lists the registered shortcuts the table gives no usable
// verdict: no row, or a row that is neither allowed nor denied. At run time
// such a shortcut stays unknown and dispatches, which is why the gap has to be
// caught before the binary ships rather than when an Agent Employee runs it.
// It lives here because the ship-time check is its only caller.
func agentSupportGaps(registered []common.Shortcut, table agentSupportTable) []string {
	var gaps []string
	for _, sc := range registered {
		key := sc.Service + " " + sc.Command
		switch table.Commands[key].AgentSupport {
		case common.AgentSupportAllowed, common.AgentSupportDenied:
		default:
			gaps = append(gaps, key)
		}
	}
	sort.Strings(gaps)
	return gaps
}

func TestAgentSupportTableIsWellFormed(t *testing.T) {
	table := loadAgentSupportTable()
	if table.Version != 1 {
		t.Errorf("Version = %d, want 1", table.Version)
	}
	for key, entry := range table.Commands {
		switch entry.AgentSupport {
		case common.AgentSupportAllowed, common.AgentSupportDenied:
		default:
			t.Errorf("%s: agentSupport = %q, want allowed or denied", key, entry.AgentSupport)
		}
	}
}

func TestAgentSupportKeysAllResolveToRegisteredShortcuts(t *testing.T) {
	registered := map[string]struct{}{}
	for _, sc := range AllShortcuts() {
		registered[sc.Service+" "+sc.Command] = struct{}{}
	}
	for key := range loadAgentSupportTable().Commands {
		if _, ok := registered[key]; !ok {
			t.Errorf("agent_support.json names %q, which is not a registered shortcut", key)
		}
	}
}

// TestAgentSupportGapsNamesUnrecordedShortcuts pins the helper the coverage
// test below is built on: a registered shortcut with no verdict, or with a
// verdict that is neither allowed nor denied, is a gap.
func TestAgentSupportGapsNamesUnrecordedShortcuts(t *testing.T) {
	registered := []common.Shortcut{
		{Service: "apps", Command: "+deploy"},
		{Service: "im", Command: "+chat-list"},
		{Service: "task", Command: "+create"},
	}
	table := agentSupportTable{Commands: map[string]agentSupportEntry{
		"im +chat-list": {AgentSupport: "allowed"},
		"task +create":  {AgentSupport: "maybe"},
	}}
	got := agentSupportGaps(registered, table)
	want := []string{"apps +deploy", "task +create"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentSupportGaps() = %v, want %v", got, want)
	}
}

// TestEveryRegisteredShortcutHasAgentSupportVerdict is the direction the
// table's own check did not cover. TestAgentSupportKeysAllResolveToRegisteredShortcuts
// says every row names a real shortcut; this says every real shortcut has a
// row. Without it a new shortcut ships unrecorded, and unrecorded means an
// Agent Employee may run it -- which is exactly how three apps commands,
// one of them a deploy, went out open.
func TestEveryRegisteredShortcutHasAgentSupportVerdict(t *testing.T) {
	if gaps := agentSupportGaps(AllShortcuts(), loadAgentSupportTable()); len(gaps) > 0 {
		t.Fatalf("shortcuts without an agent_support.json verdict: %v\n"+
			"decide allowed or denied against the OpenAPI dependencies and record it", gaps)
	}
}

// TestShortcutsDefaultToUnknownAgentSupport guards that no shortcut hand-writes
// an AgentSupport literal, which would bypass the hand-maintained table.
func TestShortcutsDefaultToUnknownAgentSupport(t *testing.T) {
	for _, sc := range AllShortcuts() {
		if sc.AgentSupport != common.AgentSupportUnknown {
			t.Errorf("%s %s: AgentSupport = %q, want unknown (AgentSupport must never be set on a "+
				"shortcut definition; it is only ever assigned by stampAgentSupport from "+
				"shortcuts/agent_support.json)", sc.Service, sc.Command, sc.AgentSupport)
		}
	}
}

// TestRegisterShortcutSnapshotGateRefusesDeniedShortcutOnDispatch mounts a
// denied shortcut through the real registration path and dispatches it via
// cobra, so the assertion sits past the mounting boundary where a dropped gate
// call or a stamp applied after the byService copy would show. The unknown row
// guards against a blanket refusal. It swaps agentSupportCache, so it must not
// run in parallel.
func TestRegisterShortcutSnapshotGateRefusesDeniedShortcutOnDispatch(t *testing.T) {
	for _, tc := range []struct {
		name    string
		verdict common.AgentSupport
		wantRun bool
	}{
		{name: "denied refuses before Execute", verdict: "denied", wantRun: false},
		{name: "unknown still dispatches", verdict: "", wantRun: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = loadAgentSupportTable()
			original := agentSupportCache
			t.Cleanup(func() { agentSupportCache = original })
			commands := map[string]agentSupportEntry{}
			if tc.verdict != "" {
				commands["zz-agent-support-fixture +gate-probe"] =
					agentSupportEntry{AgentSupport: tc.verdict}
			}
			agentSupportCache = agentSupportTable{Version: 1, Commands: commands}

			executed := false
			registered := []common.Shortcut{{
				Service:   "zz-agent-support-fixture",
				Command:   "+gate-probe",
				AuthTypes: []string{"user"},
				Scopes:    []string{},
				Execute: func(_ context.Context, _ *common.RuntimeContext) error {
					executed = true
					return nil
				},
			}}

			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
				AppID:           "cli_test",
				AppSecret:       "secret",
				Brand:           core.BrandFeishu,
				CredentialKind:  core.CredentialAAT,
				PrincipalOpenID: "ou_test",
				UserOpenId:      "ou_test",
			})

			program := &cobra.Command{Use: "root"}
			RegisterShortcutSnapshotWithContext(context.Background(), program, f, registered)

			program.SetArgs([]string{"zz-agent-support-fixture", "+gate-probe", "--as", "user"})
			err := program.ExecuteContext(context.Background())

			if executed != tc.wantRun {
				t.Fatalf("Execute ran = %v, want %v (err: %v)", executed, tc.wantRun, err)
			}
			if tc.wantRun {
				if err != nil {
					t.Fatalf("an unknown verdict must dispatch, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected dispatch to refuse the denied shortcut, got nil error")
			}
			var vErr *errs.ValidationError
			if !errors.As(err, &vErr) || vErr.Subtype != errs.SubtypeFailedPrecondition {
				t.Fatalf("want failed_precondition ValidationError, got %T: %v", err, err)
			}
			if !strings.Contains(vErr.Message, "zz-agent-support-fixture +gate-probe") {
				t.Errorf("message should name the command, got: %s", vErr.Message)
			}
		})
	}
}

// TestShortcutPrincipalGateRoutesOnCredentialKind pins the routing that makes
// the recorded verdict the only thing that decides an Agent Employee's access:
// AuthTypes is not consulted for an AAT profile, and the verdict is not
// consulted for anyone else. Each case dispatches through the real mounting
// path so the assertion sits past the boundary where a dropped gate would show.
// It swaps agentSupportCache, so it must not run in parallel.
func TestShortcutPrincipalGateRoutesOnCredentialKind(t *testing.T) {
	const path = "zz-principal-fixture +gate-probe"
	for _, tc := range []struct {
		name       string
		kind       core.CredentialKind
		authTypes  []string
		verdict    common.AgentSupport
		as         string
		wantRun    bool
		wantAs     core.Identity
		wantErrHas string
	}{
		{
			// The point of the change: a bot-only lane no longer speaks for an
			// Agent Employee, because an AAT has no bot lane to compare against.
			name:      "AAT runs a bot-only shortcut the verdict allows",
			kind:      core.CredentialAAT,
			authTypes: []string{"bot"},
			verdict:   "allowed",
			as:        "user",
			wantRun:   true,
			wantAs:    core.AsUser,
		},
		{
			name:       "AAT is refused by the verdict on its own lane",
			kind:       core.CredentialAAT,
			authTypes:  []string{"user"},
			verdict:    "denied",
			as:         "user",
			wantErrHas: "not available for an Agent Employee profile",
		},
		{
			// The verdict replaces the lane check for an Agent Employee, not the
			// check that --as names an identity at all. Without it, --dry-run
			// answers ok:true with an identity that does not exist.
			name:       "AAT is refused an --as value that is not an identity",
			kind:       core.CredentialAAT,
			authTypes:  []string{"user"},
			verdict:    "allowed",
			as:         "foo",
			wantErrHas: "only supports: user",
		},
		{
			// The other half: routing must not loosen the lane check for the
			// tenant profiles that bot-only mode actually runs on.
			name:       "TAT is still refused by the declared lane",
			kind:       core.CredentialTAT,
			authTypes:  []string{"user"},
			verdict:    "allowed",
			as:         "bot",
			wantErrHas: "only supports: user",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = loadAgentSupportTable()
			original := agentSupportCache
			t.Cleanup(func() { agentSupportCache = original })
			agentSupportCache = agentSupportTable{Version: 1, Commands: map[string]agentSupportEntry{
				path: {AgentSupport: tc.verdict},
			}}

			executed := false
			var gotAs core.Identity
			registered := []common.Shortcut{{
				Service:   "zz-principal-fixture",
				Command:   "+gate-probe",
				AuthTypes: tc.authTypes,
				Scopes:    []string{},
				Execute: func(_ context.Context, rctx *common.RuntimeContext) error {
					executed = true
					gotAs = rctx.As()
					return nil
				},
			}}

			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			cfg := &core.CliConfig{
				AppID:          "cli_test",
				AppSecret:      "secret",
				Brand:          core.BrandFeishu,
				CredentialKind: tc.kind,
			}
			if tc.kind == core.CredentialAAT {
				cfg.PrincipalOpenID = "ou_test"
				cfg.UserOpenId = "ou_test"
			}
			f, _, _, _ := cmdutil.TestFactory(t, cfg)

			program := &cobra.Command{Use: "root"}
			RegisterShortcutSnapshotWithContext(context.Background(), program, f, registered)
			program.SetArgs([]string{"zz-principal-fixture", "+gate-probe", "--as", tc.as})
			err := program.ExecuteContext(context.Background())

			if executed != tc.wantRun {
				t.Fatalf("Execute ran = %v, want %v (err: %v)", executed, tc.wantRun, err)
			}
			if tc.wantRun {
				if err != nil {
					t.Fatalf("expected dispatch to succeed, got: %v", err)
				}
				// Guards the companion change in RuntimeContext.As(): the
				// build-time botOnly flag must not put an AAT invocation on
				// the bot lane, where it would look for a tenant token.
				if gotAs != tc.wantAs {
					t.Errorf("rctx.As() = %q, want %q", gotAs, tc.wantAs)
				}
				return
			}
			if err == nil {
				t.Fatal("expected dispatch to be refused, got nil error")
			}
			var vErr *errs.ValidationError
			if !errors.As(err, &vErr) {
				t.Fatalf("want a ValidationError, got %T: %v", err, err)
			}
			if !strings.Contains(vErr.Message, tc.wantErrHas) {
				t.Errorf("message = %q, want it to contain %q", vErr.Message, tc.wantErrHas)
			}
		})
	}
}
