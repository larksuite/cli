// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
)

func TestRuntimeContextPresentErrorUsesResolvedShortcutDeclaredScopes(t *testing.T) {
	const (
		userScope = "calendar:calendar.event:read"
		botScope  = "calendar:calendar.event:read:bot"
	)
	tests := []struct {
		name      string
		identity  string
		concealed bool
		wantScope string
	}{
		{name: "user visible", identity: "user", wantScope: userScope},
		{name: "user concealed", identity: "user", concealed: true, wantScope: userScope},
		{name: "bot keeps bot recovery", identity: "bot"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTestFactory()
			var plan *surface.Plan
			if tt.concealed {
				plan = surface.NewPlan(map[surface.CommandID]surface.CommandState{
					surface.CommandAuthLogin: surface.CommandConcealed,
				})
				f.Recovery = recovery.NewProjector(func() *surface.Plan { return plan })
			}

			var source *errs.PermissionError
			var presented *errs.PermissionError
			shortcut := &Shortcut{
				Service:               "test",
				Command:               "+present-error",
				AuthTypes:             []string{"user", "bot"},
				ConditionalUserScopes: []string{userScope},
				ConditionalBotScopes:  []string{botScope},
				Execute: func(_ context.Context, runtime *RuntimeContext) error {
					err := errclass.BuildAPIError(
						map[string]any{"code": 230027, "msg": "operation unauthorized"},
						errclass.ClassifyContext{Identity: string(runtime.As())},
					)
					if !errors.As(err, &source) {
						t.Fatalf("source = %T, want *errs.PermissionError", err)
					}
					rendered := runtime.PresentError(err)
					permission, ok := rendered.(*errs.PermissionError)
					if !ok {
						t.Fatalf("presented = %T, want *errs.PermissionError", rendered)
					}
					presented = permission
					return nil
				},
			}
			cmd := newTestShortcutCmd(shortcut, f)
			if err := cmd.Flags().Set("as", tt.identity); err != nil {
				t.Fatal(err)
			}
			if err := runShortcut(cmd, f, shortcut, false); err != nil {
				t.Fatalf("runShortcut() error = %v", err)
			}

			if source == nil || presented == nil {
				t.Fatal("shortcut did not present its error")
			}
			if source == presented || source.Identity != tt.identity || presented.Identity != tt.identity {
				t.Fatalf("identity/clone mismatch: source=%+v presented=%+v", source, presented)
			}
			if len(source.MissingScopes) != 0 || len(presented.MissingScopes) != 0 {
				t.Fatalf("presentation fabricated missing_scopes: source=%v presented=%v", source.MissingScopes, presented.MissingScopes)
			}

			want := errclass.PermissionRecovery(nil, tt.identity, errs.SubtypeUserUnauthorized, "").Render(plan)
			if tt.wantScope != "" {
				want = errclass.PermissionRecovery([]string{tt.wantScope}, tt.identity, errs.SubtypeUserUnauthorized, "").Render(plan)
			}
			if presented.Hint != want {
				t.Fatalf("presented recovery = %q, want %q", presented.Hint, want)
			}
			if tt.identity == "bot" {
				if strings.Contains(presented.Hint, "auth login") || strings.Contains(presented.Hint, botScope) {
					t.Fatalf("bot recovery used user/scoped OAuth guidance: %q", presented.Hint)
				}
			} else if tt.concealed {
				if strings.Contains(presented.Hint, "auth login") || !strings.Contains(presented.Hint, userScope) {
					t.Fatalf("concealed user recovery leaked command or lost scope: %q", presented.Hint)
				}
			} else if !strings.Contains(presented.Hint, `auth login --scope "`+userScope+`"`) {
				t.Fatalf("visible user recovery did not use declared scope: %q", presented.Hint)
			}
		})
	}
}

func TestNewRuntimeContextDeclaredScopesFollowResolvedIdentity(t *testing.T) {
	// The recovery hint built from declaredScopes must name the lane the call
	// actually used. An Agent Employee runs a bot-only shortcut on the user
	// lane whenever the recorded verdict allows it, and telling it to acquire
	// a bot scope there would send it somewhere that cannot help.
	newBotOnlyShortcut := func() *Shortcut {
		return &Shortcut{
			Service:    "test",
			Command:    "+bot-only",
			AuthTypes:  []string{"bot"},
			UserScopes: []string{"user:scope"},
			BotScopes:  []string{"bot:scope"},
			Execute:    func(context.Context, *RuntimeContext) error { return nil },
		}
	}
	for _, tc := range []struct {
		name       string
		as         core.Identity
		wantAs     core.Identity
		wantScopes []string
	}{
		{
			name:       "resolved user lane wins over the build-time bot-only flag",
			as:         core.AsUser,
			wantAs:     core.AsUser,
			wantScopes: []string{"user:scope"},
		},
		{
			name:       "resolved bot lane keeps bot scopes",
			as:         core.AsBot,
			wantAs:     core.AsBot,
			wantScopes: []string{"bot:scope"},
		},
		{
			// botOnly still decides when nothing was resolved, which is what
			// keeps a caller that never ran identity resolution on the bot lane.
			name:       "unresolved identity falls back to bot-only",
			as:         "",
			wantAs:     core.AsBot,
			wantScopes: []string{"bot:scope"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shortcut := newBotOnlyShortcut()
			f := newTestFactory()
			config, err := f.Config()
			if err != nil {
				t.Fatal(err)
			}
			cmd := newTestShortcutCmd(shortcut, f)
			runtime, err := newRuntimeContext(cmd, f, shortcut, config, tc.as, true)
			if err != nil {
				t.Fatalf("newRuntimeContext() error = %v", err)
			}
			if runtime.As() != tc.wantAs {
				t.Fatalf("runtime.As() = %q, want %q", runtime.As(), tc.wantAs)
			}
			if got := runtime.declaredScopes; !reflect.DeepEqual(got, tc.wantScopes) {
				t.Fatalf("declared scopes = %v, want %v", got, tc.wantScopes)
			}
		})
	}
}
