// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	internalauth "github.com/larksuite/cli/internal/auth"
)

func TestUpdateSelfAcceptsAlternativeGrants(t *testing.T) {
	for _, tc := range []struct {
		name, scopes, flag, value string
		requests                  int
	}{
		{"directory key", "directory:employee:write", "--avatar-key", "img_fixture_avatar", 2},
		{"directory file", "directory:employee:write im:resource", "--avatar-file", "avatar.png", 3},
		{"upload file", "directory:employee.update:write im:resource:upload", "--avatar-file", "avatar.png", 3},
	} {
		for _, identity := range []string{"UAT", "AAT"} {
			t.Run(tc.name+"/"+identity, func(t *testing.T) {
				wire := setupSelfAvatarWire(t, tc.scopes)
				if identity == "AAT" {
					useSelfAvatarAgent(wire)
					t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", "on")
				}
				installSelfAvatarFiles(t, wire)
				data, _, err := executeSelfAvatar(t, wire, tc.flag, tc.value)
				if err != nil {
					t.Fatalf("documented alternative grant rejected: %v", err)
				}
				if len(wire.requests) != tc.requests || len(wire.calls("PATCH")) != 1 || data.EmployeeID != wire.openID {
					t.Fatalf("unexpected self-avatar flow: requests=%+v data=%+v", wire.requests, data)
				}
			})
		}
	}
}

func TestUpdateSelfAgentScopeRecovery(t *testing.T) {
	wire := setupSelfAvatarWire(t, "contact:user.base:readonly")
	useSelfAvatarAgent(wire)
	t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", "on")
	_, output, err := executeSelfAvatar(t, wire, "--avatar-key", "img_fixture_avatar")
	var permission *errs.PermissionError
	if !errors.As(err, &permission) || permission.Subtype != errs.SubtypeMissingScope ||
		!reflect.DeepEqual(permission.MissingScopes, []string{"directory:employee.update:write"}) ||
		!strings.Contains(permission.Hint, "Agent platform") ||
		strings.Contains(permission.Hint, "auth login") {
		t.Fatalf("AAT recovery must request the update scope on the Agent platform: %v", err)
	}
	if output != "" || len(wire.requests) != 0 || wire.credentials.tokenValues != 0 {
		t.Fatalf("known missing scopes must stop before credential resolution or API calls: output=%s requests=%d tokens=%d", output, len(wire.requests), wire.credentials.tokenValues)
	}
}

func TestUpdateSelfMissingUATStopsBeforeRequests(t *testing.T) {
	wire := setupSelfAvatarWire(t, "")
	source := internalauth.NewNeedUserAuthorizationError("ou_self_fixture")
	wire.credentials.tokenError = source
	_, output, err := executeSelfAvatar(t, wire, "--avatar-key", "img_fixture_avatar")
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryAuthentication || problem.Subtype != errs.SubtypeTokenMissing ||
		!errors.Is(err, source) || output != "" || len(wire.requests) != 0 {
		t.Fatalf("missing UAT continued execution or lost its typed error: err=%v output=%s requests=%+v", err, output, wire.requests)
	}
}
