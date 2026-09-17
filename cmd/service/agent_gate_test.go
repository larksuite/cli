// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/agentsupport"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/meta"
	"github.com/spf13/cobra"
)

const unannotatedServiceHint = "this is not something the current profile can fix"

// agentGateConfig is an Agent Employee profile config.
func agentGateConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID:           "cli_agent",
		AppSecret:       "secret",
		Brand:           core.BrandFeishu,
		CredentialKind:  core.CredentialAAT,
		PrincipalOpenID: "ou_agent",
		UserOpenId:      "ou_agent",
	}
}

// agentGateTestService builds an "im" service with "messages create" (never
// agent-marked) and, when includeMarkedMethod, "messages search" (agent-marked).
func agentGateTestService(includeMarkedMethod bool) meta.Service {
	methods := map[string]interface{}{
		"create": map[string]interface{}{
			"path":       "messages",
			"httpMethod": "POST",
		},
	}
	if includeMarkedMethod {
		methods["search"] = map[string]interface{}{
			"path":         "messages/search",
			"httpMethod":   "GET",
			"accessTokens": []interface{}{"user", "agent"},
		}
	}
	return meta.ServiceFromMap(map[string]interface{}{
		"name":        "im",
		"servicePath": "/open-apis/im/v1",
		"resources": map[string]interface{}{
			"messages": map[string]interface{}{
				"methods": methods,
			},
		},
	})
}

// buildAgentGateRoot registers svc through the production registration path.
// Drive the result via root.SetArgs/root.Execute: cobra redirects Execute to
// the root once a command has a parent.
func buildAgentGateRoot(f *cmdutil.Factory, svc meta.Service) *cobra.Command {
	root := &cobra.Command{Use: "lark-cli"}
	registerServiceWithContext(context.Background(), root, svc, f)
	return root
}

// TestServiceMethod_AgentGateRefusesUnmarkedMethodWhenServiceAnnotated pins the
// per-service granularity: one marked sibling drops the unannotated wording.
func TestServiceMethod_AgentGateRefusesUnmarkedMethodWhenServiceAnnotated(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, agentGateConfig())
	root := buildAgentGateRoot(f, agentGateTestService(true))
	root.SetArgs([]string{"im", "messages", "create", "--as", "user", "--dry-run"})

	err := root.Execute()
	var vErr *errs.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("want a refusal (*errs.ValidationError), got %T: %v", err, err)
	}
	if strings.Contains(vErr.Hint, unannotatedServiceHint) {
		t.Errorf("hint must not claim the service is unannotated when it has an agent-marked method, got: %s", vErr.Hint)
	}
}

// TestServiceMethod_AgentGateAllowsMarkedMethod pins that the method's own marker decides allow/deny.
func TestServiceMethod_AgentGateAllowsMarkedMethod(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, agentGateConfig())
	root := buildAgentGateRoot(f, agentGateTestService(true))
	root.SetArgs([]string{"im", "messages", "search", "--as", "user", "--dry-run"})

	if err := root.Execute(); err != nil {
		t.Fatalf("an agent-marked method must be allowed for an Agent Employee profile, got: %v", err)
	}
}

// agentOnlyMethodService builds an "im" service whose only method,
// "messages search", is marked agent-only: accessTokens has no user entry.
func agentOnlyMethodService() meta.Service {
	return meta.ServiceFromMap(map[string]interface{}{
		"name":        "im",
		"servicePath": "/open-apis/im/v1",
		"resources": map[string]interface{}{
			"messages": map[string]interface{}{
				"methods": map[string]interface{}{
					"search": map[string]interface{}{
						"path":         "messages/search",
						"httpMethod":   "GET",
						"accessTokens": []interface{}{"agent"},
					},
				},
			},
		},
	})
}

// TestServiceMethod_AgentOnlyMethodReachesAgentGate pins that an agent-only
// method is not refused at the identity gate with an empty supported list:
// "agent" rides the user wire, so --as user passes dimension 1 and the agent
// gate (dimension 2) then allows the marked method.
func TestServiceMethod_AgentOnlyMethodReachesAgentGate(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, agentGateConfig())
	root := buildAgentGateRoot(f, agentOnlyMethodService())
	root.SetArgs([]string{"im", "messages", "search", "--as", "user", "--dry-run"})

	if err := root.Execute(); err != nil {
		t.Fatalf("an agent-only method must be allowed for an Agent Employee profile, got: %v", err)
	}
}

// TestServiceMethod_AgentOnlyMethodRefusedForUserProfile pins the human side
// of the vocabulary: an agent-only method is not callable by a human profile,
// and the refusal says why instead of listing an empty identity set.
func TestServiceMethod_AgentOnlyMethodRefusedForUserProfile(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID:          "cli_user",
		AppSecret:      "secret",
		Brand:          core.BrandFeishu,
		CredentialKind: core.CredentialUAT,
		UserOpenId:     "ou_user",
	})
	root := buildAgentGateRoot(f, agentOnlyMethodService())
	root.SetArgs([]string{"im", "messages", "search", "--as", "user", "--dry-run"})

	err := root.Execute()
	var vErr *errs.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("want a refusal (*errs.ValidationError), got %T: %v", err, err)
	}
	if !strings.Contains(vErr.Message, "Agent Employee") {
		t.Errorf("refusal must say the method is reserved for an Agent Employee, got: %s", vErr.Message)
	}
	if strings.Contains(vErr.Message, "only supports:") {
		t.Errorf("refusal must not list an empty identity set, got: %s", vErr.Message)
	}
}

// tenantOnlyMethodService builds an "im" service whose only method accepts a
// tenant token and nothing else.
func tenantOnlyMethodService() meta.Service {
	return meta.ServiceFromMap(map[string]interface{}{
		"name":        "im",
		"servicePath": "/open-apis/im/v1",
		"resources": map[string]interface{}{
			"messages": map[string]interface{}{
				"methods": map[string]interface{}{
					"create": map[string]interface{}{
						"path":         "messages",
						"httpMethod":   "POST",
						"accessTokens": []interface{}{"tenant"},
					},
				},
			},
		},
	})
}

// TestServiceMethod_AgentGateSpeaksBeforeTheLaneCheck pins the order of the two
// gates for an Agent Employee. A tenant-only method fails both; the refusal
// the user reads must be the one about the agent marker, because "only
// supports: bot" sends an Agent Employee to a lane it does not have.
func TestServiceMethod_AgentGateSpeaksBeforeTheLaneCheck(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, agentGateConfig())
	root := buildAgentGateRoot(f, tenantOnlyMethodService())
	root.SetArgs([]string{"im", "messages", "create", "--as", "user", "--dry-run"})

	err := root.Execute()
	var vErr *errs.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("want a refusal (*errs.ValidationError), got %T: %v", err, err)
	}
	if !strings.Contains(vErr.Message, "not available for an Agent Employee profile") {
		t.Errorf("want the agent eligibility refusal, got: %s", vErr.Message)
	}
}

// scopedAgentTokenResolver returns a token with a caller-controlled scope so
// checkServiceScopes can be exercised without a token exchange.
type scopedAgentTokenResolver struct {
	scopes string
}

func (r *scopedAgentTokenResolver) ResolveToken(_ context.Context, _ credential.TokenSpec) (*credential.TokenResult, error) {
	return &credential.TokenResult{Token: "test-token", Scopes: r.scopes}, nil
}

// agentMarkedScopedMethod builds an "im messages search" method that is both
// agent-marked and has a required scope.
func agentMarkedScopedMethod(requiredScope string) meta.Service {
	return meta.ServiceFromMap(map[string]interface{}{
		"name":        "im",
		"servicePath": "/open-apis/im/v1",
		"resources": map[string]interface{}{
			"messages": map[string]interface{}{
				"methods": map[string]interface{}{
					"search": map[string]interface{}{
						"path":           "messages/search",
						"httpMethod":     "GET",
						"accessTokens":   []interface{}{"user", "agent"},
						"requiredScopes": []interface{}{requiredScope},
					},
				},
			},
		},
	})
}

// TestServiceMethod_AgentGateScopePrecheckRefusesMarkedMethodMissingScope pins
// that a marked method passes the gate and is then refused by the scope pre-check.
func TestServiceMethod_AgentGateScopePrecheckRefusesMarkedMethodMissingScope(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, agentGateConfig())
	f.Credential = credential.NewCredentialProvider(nil, nil, &scopedAgentTokenResolver{scopes: "im:chat:read"}, nil)

	root := buildAgentGateRoot(f, agentMarkedScopedMethod("im:message:readonly"))
	root.SetArgs([]string{"im", "messages", "search", "--as", "user", "--dry-run"})

	err := root.Execute()
	var permErr *errs.PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("want *errs.PermissionError, got %T: %v", err, err)
	}
	if permErr.Subtype != errs.SubtypeMissingScope {
		t.Errorf("subtype = %q, want %q", permErr.Subtype, errs.SubtypeMissingScope)
	}
	if strings.Contains(permErr.Hint, "auth login") {
		t.Errorf("Agent Employee hint must not mention auth login, got: %s", permErr.Hint)
	}
}

// TestServiceMethod_AgentGateOnByDefaultRefusesUnmarkedMethod pins the shipped
// default with nothing in the environment: generated commands filter by
// identity, an Agent Employee is a distinct identity, and a method that does
// not declare it is refused before any request goes out.
func TestServiceMethod_AgentGateOnByDefaultRefusesUnmarkedMethod(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_IDENTITY_GATE", "")
	if err := os.Unsetenv("LARKSUITE_CLI_AGENT_IDENTITY_GATE"); err != nil {
		t.Fatal(err)
	}
	f, _, _, _ := cmdutil.TestFactory(t, agentGateConfig())
	root := buildAgentGateRoot(f, agentGateTestService(false))
	root.SetArgs([]string{"im", "messages", "create", "--as", "user", "--dry-run"})

	var vErr *errs.ValidationError
	if err := root.Execute(); !errors.As(err, &vErr) {
		t.Fatalf("want a refusal (*errs.ValidationError), got %T: %v", err, err)
	}
}

// findMethodCmd walks a registered service tree down to one method command.
func findMethodCmd(root *cobra.Command, names ...string) *cobra.Command {
	cmd := root
	for _, name := range names {
		var next *cobra.Command
		for _, c := range cmd.Commands() {
			if c.Name() == name {
				next = c
				break
			}
		}
		if next == nil {
			return nil
		}
		cmd = next
	}
	return cmd
}

// TestServiceMethod_StampsAgentVerdictForTreePruning pins that registration
// records what the run-time gate would decide, which is what lets the
// command-tree build withhold an unusable method from help instead of
// advertising it and failing on dispatch.
func TestServiceMethod_StampsAgentVerdictForTreePruning(t *testing.T) {
	for _, tc := range []struct {
		name          string
		serviceMarked bool
		method        string
		wantVerdict   agentsupport.Verdict
		wantAnnotated bool
	}{
		{
			name:          "unmarked method is stamped denied",
			serviceMarked: false,
			method:        "create",
			wantVerdict:   agentsupport.Denied,
			wantAnnotated: false,
		},
		{
			name:          "unmarked method in an annotated service keeps the grant wording",
			serviceMarked: true,
			method:        "create",
			wantVerdict:   agentsupport.Denied,
			wantAnnotated: true,
		},
		{
			name:          "agent-marked method is not stamped at all",
			serviceMarked: true,
			method:        "search",
			wantVerdict:   "",
			wantAnnotated: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, agentGateConfig())
			root := buildAgentGateRoot(f, agentGateTestService(tc.serviceMarked))
			cmd := findMethodCmd(root, "im", "messages", tc.method)
			if cmd == nil {
				t.Fatalf("method %q not registered", tc.method)
			}
			if got := cmdutil.GetAgentSupport(cmd); got != tc.wantVerdict {
				t.Errorf("agent verdict = %q, want %q", got, tc.wantVerdict)
			}
			if got := cmdutil.AgentServiceAnnotated(cmd); got != tc.wantAnnotated {
				t.Errorf("service-annotated = %v, want %v", got, tc.wantAnnotated)
			}
		})
	}
}

// TestServiceMethod_GateOffLeavesNoVerdictToPruneOn pins that turning the gate
// off keeps commands in the tree. With the gate off the run-time check passes
// everything, so stamping a denial would hide from help exactly what the gate
// just chose to allow.
func TestServiceMethod_GateOffLeavesNoVerdictToPruneOn(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_IDENTITY_GATE", "off")
	f, _, _, _ := cmdutil.TestFactory(t, agentGateConfig())
	root := buildAgentGateRoot(f, agentGateTestService(false))
	cmd := findMethodCmd(root, "im", "messages", "create")
	if cmd == nil {
		t.Fatal("method not registered")
	}
	if got := cmdutil.GetAgentSupport(cmd); got != "" {
		t.Errorf("agent verdict = %q, want none while the gate is off", got)
	}
}
