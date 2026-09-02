// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package commandtest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/extension/command/commandtest"
)

type outcomeProbeArgs struct {
	ID string `flag:"id" schema:"required;minLength=1" doc:"identifier"`
}

type outcomeProbeData struct {
	Content string `json:"content" schema:"required" doc:"document content"`
}

// missingOutcomeDefinition models the mistake worth catching: Execute reports
// success by returning a bare Result instead of routing through Success.
func missingOutcomeDefinition() command.Definition[outcomeProbeArgs, outcomeProbeData] {
	return command.Definition[outcomeProbeArgs, outcomeProbeData]{
		Metadata: command.CommandMetadata{
			Service: "docs", Command: "+business-missing-outcome", Description: "Probe", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{"docx:document:readonly"}},
			}},
		},
		Hooks: command.Hooks[outcomeProbeArgs, outcomeProbeData]{
			Execute: func(context.Context, command.CommandContext, *outcomeProbeArgs) (command.Result[outcomeProbeData], error) {
				return command.Result[outcomeProbeData]{}, nil
			},
		},
	}
}

func missingOutcomePageDefinition() command.Definition[outcomeProbeArgs, command.Page[outcomeProbeData]] {
	return command.Definition[outcomeProbeArgs, command.Page[outcomeProbeData]]{
		Metadata: command.CommandMetadata{
			Service: "docs", Command: "+business-missing-outcome-page", Description: "Probe", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{"docx:document:readonly"}},
			}},
		},
		Hooks: command.Hooks[outcomeProbeArgs, command.Page[outcomeProbeData]]{
			Execute: func(context.Context, command.CommandContext, *outcomeProbeArgs) (command.Result[command.Page[outcomeProbeData]], error) {
				return command.Result[command.Page[outcomeProbeData]]{}, nil
			},
		},
	}
}

func assertMissingOutcome(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want the missing-outcome protocol failure")
	}
	if !strings.Contains(err.Error(), "without an outcome") {
		t.Fatalf("error = %v, want the missing-outcome protocol failure", err)
	}
	if !errs.IsInternal(err) {
		t.Errorf("error %v is not an internal error", err)
	}
}

func TestExecuteRejectsResultWithoutOutcome(t *testing.T) {
	recorder := commandtest.New(t)
	_, err := commandtest.Execute(
		context.Background(), recorder, command.IdentityUser,
		missingOutcomeDefinition(), &outcomeProbeArgs{ID: "doc_1"},
	)
	assertMissingOutcome(t, err)
}

func TestRunWithFlagsRejectsResultWithoutOutcome(t *testing.T) {
	recorder := commandtest.New(t)
	_, err := commandtest.RunWithFlags(
		context.Background(), recorder, command.IdentityUser,
		missingOutcomePageDefinition(), &outcomeProbeArgs{ID: "doc_1"}, "--page-all",
	)
	assertMissingOutcome(t, err)
}
