// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/larksuite/cli/cmd/event/render"
	"github.com/larksuite/cli/errs"
	appconsume "github.com/larksuite/cli/internal/event/application/consume"
	"github.com/larksuite/cli/internal/event/catalog"
)

const blockedTestKey = "im.message.receive_v1"

// blockingPreflight reports one blocked precondition, the shape a real
// preflight produces when credentials cannot be used.
type blockingPreflight struct {
	name     string
	blockErr error
}

func (p *blockingPreflight) Read(context.Context, *catalog.Entry, string) ([]appconsume.Precondition, error) {
	return []appconsume.Precondition{
		{Name: "console_event_published", Status: appconsume.PreconditionOK},
		{
			Name:     p.name,
			Status:   appconsume.PreconditionBlocked,
			Detail:   p.blockErr.Error(),
			BlockErr: p.blockErr,
		},
	}, nil
}

type fixedIdentity string

func (i fixedIdentity) Resolve(context.Context, *catalog.Entry) (string, error) {
	return string(i), nil
}

// spyRunner records whether the delivery stream was ever started.
type spyRunner struct{ started bool }

func (r *spyRunner) Run(context.Context, appconsume.PrepareFunc) error {
	r.started = true
	return nil
}

func blockedDecisionFixture(t *testing.T) (*catalog.Entry, *appconsume.Service, *appconsume.Decision, error, error) {
	t.Helper()
	snap := compileCatalog()
	entry, ok := snap.Resolve(blockedTestKey)
	if !ok {
		t.Fatalf("catalog has no %s", blockedTestKey)
	}
	blockErr := errs.NewValidationError(errs.SubtypeMissingScope,
		"missing scopes for %s", blockedTestKey).
		WithHint("run `lark-cli auth login --scope im:message` and retry")

	svc := &appconsume.Service{
		Strategies: appconsume.DefaultRegistry(),
		Identity:   fixedIdentity("bot"),
		Preflight:  &blockingPreflight{name: "credentials_available", blockErr: blockErr},
	}
	decision, err := svc.Decide(context.Background(), entry, appconsume.Request{EventKey: blockedTestKey}, appconsume.ExecutionContext{})
	return entry, svc, decision, blockErr, err
}

// The dry-run envelope for a blocked decision is a contract an orchestrator
// reads: ok stays true and the exit code stays 0, because the preview itself
// succeeded, and "would this run" is answered inside the payload. Anything that
// only checks ok or the exit code would treat a refusal as a green light, so
// the status and the blocked precondition must be present and named.
func TestBlockedDecision_DryRunEnvelopeStatesTheRefusal(t *testing.T) {
	_, _, decision, _, err := blockedDecisionFixture(t)
	if err != nil {
		t.Fatalf("deciding must succeed even when a precondition blocks, got: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := render.WriteDecisionJSON(&stdout, &stderr, "bot", decision.View()); err != nil {
		t.Fatalf("render: %v", err)
	}

	var envelope struct {
		OK     bool `json:"ok"`
		DryRun bool `json:"dry_run"`
		Data   struct {
			Decision struct {
				Status        string `json:"status"`
				Preconditions []struct {
					Name   string `json:"name"`
					Status string `json:"status"`
					Detail string `json:"detail"`
				} `json:"preconditions"`
				WouldWrite []string `json:"would_write"`
			} `json:"decision"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not a decision envelope: %v\n%s", err, stdout.String())
	}

	if !envelope.OK || !envelope.DryRun {
		t.Errorf("the preview succeeded, so ok and dry_run stay true; got: %s", stdout.String())
	}
	if envelope.Data.Decision.Status != "blocked" {
		t.Errorf("status = %q, want \"blocked\": this is the only field that tells a caller the real run would refuse",
			envelope.Data.Decision.Status)
	}

	var blocked *struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Detail string `json:"detail"`
	}
	for i := range envelope.Data.Decision.Preconditions {
		if envelope.Data.Decision.Preconditions[i].Status == "blocked" {
			blocked = &envelope.Data.Decision.Preconditions[i]
		}
	}
	if blocked == nil {
		t.Fatalf("a blocked decision must name the precondition that blocks it, got: %s", stdout.String())
	}
	if blocked.Name != "credentials_available" {
		t.Errorf("blocked precondition = %q, want the one the preflight reported", blocked.Name)
	}
	if blocked.Detail == "" {
		t.Error("a blocked precondition must carry a detail; without it the caller knows only that something failed")
	}
	// Declared write side effects stay declarations in a preview.
	if len(envelope.Data.Decision.WouldWrite) == 0 {
		t.Error("the preview must still declare what a real run would write")
	}
}

// Executing a blocked decision returns the preflight's own error and never
// starts the stream. Everything the command sets up for a live run — the stdin
// EOF watcher included — hangs off the runner for this reason: started earlier,
// it announced "stdin closed — shutting down" on a run that was actually
// refused for an unmet precondition.
func TestBlockedDecision_ExecuteNeverStartsTheStream(t *testing.T) {
	entry, svc, decision, blockErr, err := blockedDecisionFixture(t)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	runner := &spyRunner{}
	execErr := svc.Execute(context.Background(), entry, decision, runner, appconsume.ExecutionContext{})

	if runner.started {
		t.Error("a blocked decision must not start the delivery stream")
	}
	problem, ok := errs.ProblemOf(execErr)
	if !ok {
		t.Fatalf("executing a blocked decision must return the preflight's typed error, got: %v", execErr)
	}
	if problem.Subtype != errs.SubtypeMissingScope {
		t.Errorf("subtype = %q, want the preflight's own subtype preserved", problem.Subtype)
	}
	// The caller must receive the preflight's own error, not a copy of its
	// message: the hint is what tells an operator how to recover, and a rewrap
	// would drop it.
	if !errors.Is(execErr, blockErr) {
		t.Errorf("execute returned %v, want the preflight's own error", execErr)
	}
	if problem.Hint == "" {
		t.Error("the recovery hint the preflight attached must survive to the caller")
	}
}
