// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"errors"
	identitypkg "github.com/larksuite/cli/internal/identity"
	"testing"

	eventlib "github.com/larksuite/cli/internal/event"
	appconsume "github.com/larksuite/cli/internal/event/application/consume"
)

func preconditionByName(list []appconsume.Precondition, name string) *appconsume.Precondition {
	for i := range list {
		if list[i].Name == name {
			return &list[i]
		}
	}
	return nil
}

// An unusable credential blocks the decision and carries the exact error a
// real run would have returned, so both paths refuse for the same reason.
func TestReadPreconditions_TokenErrorBlocksWithTheSameError(t *testing.T) {
	tokenErr := errors.New("no tenant token available")
	pf := &preflightCtx{
		appID:    "cli_test",
		identity: identitypkg.AsBot,
		keyDef:   &eventlib.KeyDefinition{Key: "demo.thing.updated_v1"},
	}
	got := readPreconditions(context.Background(), pf, nil, tokenErr)

	cred := preconditionByName(got, "credentials_available")
	if cred == nil {
		t.Fatal("credentials_available precondition missing")
	}
	if cred.Status != appconsume.PreconditionBlocked || !errors.Is(cred.BlockErr, tokenErr) {
		t.Errorf("token failure must block with the original error, got %+v", cred)
	}
}

// A scope ledger nobody could read is reported as unknown — never as ok.
func TestReadPreconditions_UnreadableScopesAreUnknown(t *testing.T) {
	pf := &preflightCtx{
		appID:    "cli_test",
		identity: identitypkg.AsBot,
		keyDef: &eventlib.KeyDefinition{
			Key:    "demo.thing.updated_v1",
			Scopes: []string{"demo:read"},
		},
		appVer: nil, // no published version: the bot scope ledger is unreadable
	}
	got := readPreconditions(context.Background(), pf, nil, nil)

	scopes := preconditionByName(got, "scopes_granted")
	if scopes == nil {
		t.Fatal("scopes_granted precondition missing")
	}
	if scopes.Status != appconsume.PreconditionUnknown {
		t.Errorf("an unreadable ledger must report unknown, got %q", scopes.Status)
	}
}
