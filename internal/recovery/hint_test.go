// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package recovery

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/surface"
)

func TestUserAuthorizationGolden(t *testing.T) {
	tests := []struct {
		name      string
		hint      Hint
		visible   string
		concealed string
	}{
		{
			name: "no scopes",
			hint: UserAuthorization(),
			visible: "run `lark-cli auth login --recommend --no-wait --json` to get device_code and verification_url; " +
				"present verification_url to the user exactly and end this turn; after the user confirms authorization, " +
				"run `lark-cli auth login --device-code <device_code>` in a later turn to finish login",
			concealed: "obtain or refresh a user credential through this distribution's supported authorization flow, have the user complete authorization, then retry",
		},
		{
			name: "multiple scopes",
			hint: UserAuthorization("docx:document", "drive:drive"),
			visible: "run `lark-cli auth login --scope \"docx:document drive:drive\" --no-wait --json` to get device_code and verification_url; " +
				"present verification_url to the user exactly and end this turn; after the user confirms authorization, " +
				"run `lark-cli auth login --device-code <device_code>` in a later turn to finish login",
			concealed: "obtain or refresh a user credential through this distribution's supported authorization flow, have the user complete authorization, then retry\n" +
				"current command requires scope(s): docx:document, drive:drive",
		},
	}

	concealedPlan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandAuthLogin: surface.CommandConcealed,
	})
	deniedVisiblePlan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandAuthLogin: surface.CommandDeniedVisible,
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.hint.String(); got != tt.visible {
				t.Fatalf("visible hint = %q, want %q", got, tt.visible)
			}
			if got := tt.hint.Render(deniedVisiblePlan); got != tt.visible {
				t.Fatalf("denied-visible hint = %q, want %q", got, tt.visible)
			}
			if got := tt.hint.Render(concealedPlan); got != tt.concealed {
				t.Fatalf("concealed hint = %q, want %q", got, tt.concealed)
			}
			for _, dead := range []string{"auth login", "verification_url", "device_code"} {
				if got := tt.hint.Render(concealedPlan); strings.Contains(got, dead) {
					t.Errorf("concealed hint %q contains dead command detail %q", got, dead)
				}
			}
		})
	}
}

func TestUserAuthorizationUsesBuildLocalProfileForBothCommands(t *testing.T) {
	hint := UserAuthorization("docx:document", "drive:drive")
	projector := NewProjectorWithContext(nil, RenderContext{Profile: "team-beta"})

	want := "run `lark-cli auth login --profile='team-beta' --scope \"docx:document drive:drive\" --no-wait --json` to get device_code and verification_url; " +
		"present verification_url to the user exactly and end this turn; after the user confirms authorization, " +
		"run `lark-cli auth login --profile='team-beta' --device-code <device_code>` in a later turn to finish login"
	if got := projector.RenderHint(hint); got != want {
		t.Fatalf("profile-aware hint = %q, want %q", got, want)
	}

	// The producer owns no invocation state: its default form remains byte-for-byte
	// pinned by TestUserAuthorizationGolden after another build renders it.
	if strings.Contains(hint.String(), "--profile") {
		t.Fatalf("producer hint was polluted with build-local profile: %q", hint.String())
	}

	concealed := NewProjectorWithContext(func() *surface.Plan {
		return surface.NewPlan(map[surface.CommandID]surface.CommandState{
			surface.CommandAuthLogin: surface.CommandConcealed,
		})
	}, RenderContext{Profile: "team-beta"}).RenderHint(hint)
	if strings.Contains(concealed, "team-beta") || strings.Contains(concealed, "auth login") {
		t.Fatalf("concealed recovery leaked profile or command: %q", concealed)
	}
}

func TestRenderContextShellQuotesProfileAsOneArgument(t *testing.T) {
	context := RenderContext{Profile: "team'$(touch /tmp/should-not-run)"}
	want := `lark-cli auth login --profile='team'"'"'$(touch /tmp/should-not-run)' --device-code <code>`
	if got := context.AuthLoginCommand("--device-code <code>"); got != want {
		t.Fatalf("AuthLoginCommand() = %q, want %q", got, want)
	}
}

func TestHintRenderFiltersOnlyUnreferenceableTargets(t *testing.T) {
	hint := Join("; ",
		Command(TargetConfigInit, "run `lark-cli config init`"),
		Command(TargetAuthLogin, "run `lark-cli auth login`"),
		Text("inspect the local logs"),
	)
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		"config/init": surface.CommandConcealed,
		"auth/login":  surface.CommandDeniedVisible,
	})

	if got, want := hint.String(), "run `lark-cli config init`; run `lark-cli auth login`; inspect the local logs"; got != want {
		t.Fatalf("Hint.String() = %q, want %q", got, want)
	}
	if got, want := hint.Render(plan), "run `lark-cli auth login`; inspect the local logs"; got != want {
		t.Fatalf("Hint.Render() = %q, want %q", got, want)
	}
	// Rendering is immutable and repeatable for another command tree.
	if got, want := hint.String(), "run `lark-cli config init`; run `lark-cli auth login`; inspect the local logs"; got != want {
		t.Fatalf("Hint.String() after filtering = %q, want %q", got, want)
	}
}

func TestHintRenderDoesNotLeaveDanglingSeparator(t *testing.T) {
	hint := Join("; ",
		Command(TargetConfigInit, "run config init"),
		Text(""),
		Text("inspect logs"),
	)
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		"config": surface.CommandConcealed,
	})

	if got, want := hint.Render(plan), "inspect logs"; got != want {
		t.Fatalf("Hint.Render() = %q, want %q", got, want)
	}
}

func TestCommandOnlyHintUsesFallbackOnlyWhenTargetIsConcealed(t *testing.T) {
	hint := Join("", Command(TargetAuthLogin, "run auth")).
		WithFallback("use the supported authorization flow")
	if got := hint.String(); got != "run auth" {
		t.Fatalf("visible hint = %q", got)
	}
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandAuthLogin: surface.CommandConcealed,
	})
	if got := hint.Render(plan); got != "use the supported authorization flow" {
		t.Fatalf("concealed hint = %q", got)
	}
}

func TestAnnotatePreservesErrorChain(t *testing.T) {
	sentinel := errors.New("sentinel")
	typed := errs.NewConfigError(errs.SubtypeNotConfigured, "not configured").
		WithCause(sentinel)

	annotated := Annotate(typed, Join("", Text("hint")))
	if !errors.Is(annotated, sentinel) {
		t.Error("Annotate broke errors.Is traversal")
	}
	if problem, ok := errs.ProblemOf(annotated); !ok || problem != &typed.Problem {
		t.Error("Annotate hid the underlying typed error")
	}
	if Annotate(nil, Join("", Text("hint"))) != nil {
		t.Error("Annotate(nil) must return nil")
	}
}
