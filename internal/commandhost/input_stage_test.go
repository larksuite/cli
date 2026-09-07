// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package commandhost

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/internal/commandbridge"
)

// stubHost embeds the interface so only the methods inputStageContext actually
// reads need an implementation; anything else it reached for would panic and
// name itself in the failure.
type stubHost struct {
	commandbridge.RuntimeContext
}

func (stubHost) Identity() command.Identity               { return command.IdentityUser }
func (stubHost) IsDryRun() bool                           { return false }
func (stubHost) RequireConditionalScopes(...string) error { return nil }

// Normalize and Validate run before the high-risk confirmation gate (see the
// documented hook order), so they must not reach the API: a high-risk command
// calling out from Validate would leave remote side effects behind before the
// user was ever asked to confirm.
func TestInputStageContextRefusesNetworkBeforeConfirmation(t *testing.T) {
	commandContext := inputStageContext(stubHost{})

	_, err := command.CallJSON[map[string]any](context.Background(), commandContext,
		command.POST("/open-apis/im/v1/chats"))
	if err == nil {
		t.Fatal("CallJSON from an input-stage hook returned no error")
	}
	if !strings.Contains(err.Error(), "unavailable in Normalize and Validate") {
		t.Fatalf("CallJSON error = %v", err)
	}

	_, err = command.CollectAllPages[map[string]any](context.Background(), commandContext,
		command.GET("/open-apis/im/v1/chats"))
	if err == nil {
		t.Fatal("CollectAllPages from an input-stage hook returned no error")
	}
	if !strings.Contains(err.Error(), "unavailable in Normalize and Validate") {
		t.Fatalf("CollectAllPages error = %v", err)
	}

	// Conditional scope checks stay available: they read the resolved token and
	// are what a Validate hook legitimately needs.
	if err := command.PreflightScopes(commandContext, "im:chat:read"); err != nil {
		t.Fatalf("PreflightScopes from an input-stage hook = %v", err)
	}
}

// The guard holds even when a host wires the callbacks anyway, so the staging
// rule does not depend on every future adapter remembering to omit them.
func TestInputStageGuardOutranksWiredCallbacks(t *testing.T) {
	commandContext := command.NewCommandContext(command.ContextOptions{
		InputStage: true,
		CallJSON: func(context.Context, command.Request) (map[string]any, error) {
			t.Fatal("input-stage CallJSON reached the host callback")
			return nil, nil
		},
	})
	if _, err := command.CallJSON[map[string]any](context.Background(), commandContext,
		command.GET("/open-apis/im/v1/chats")); err == nil {
		t.Fatal("wired input-stage CallJSON returned no error")
	}
}

// The Execute context keeps what the input stage gives up, so this is a staging
// rule rather than a wholesale removal.
func TestExecuteContextKeepsNetworkCapability(t *testing.T) {
	commandContext := command.NewCommandContext(command.ContextOptions{
		CallJSON: func(context.Context, command.Request) (map[string]any, error) {
			return map[string]any{"ok": true}, nil
		},
	})
	data, err := command.CallJSON[map[string]any](context.Background(), commandContext,
		command.GET("/open-apis/im/v1/chats"))
	if err != nil {
		t.Fatal(err)
	}
	if data["ok"] != true {
		t.Fatalf("execute-stage CallJSON = %#v", data)
	}
}
