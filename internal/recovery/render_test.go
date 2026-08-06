// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package recovery

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/surface"
)

func TestRenderClonesEveryConcreteTypedErrorAndPreservesWireExtensions(t *testing.T) {
	sentinel := errors.New("sentinel")
	hint := Join("; ",
		Command(TargetConfigInit, "run `lark-cli config init`"),
		Text("inspect logs"),
	)
	problem := func(category errs.Category, subtype errs.Subtype) errs.Problem {
		return errs.Problem{
			Category:       category,
			Subtype:        subtype,
			Code:           123,
			Message:        "message",
			Hint:           hint.String(),
			LogID:          "log-id",
			Troubleshooter: "https://example.test/troubleshooter",
			Retryable:      true,
		}
	}

	tests := []struct {
		name     string
		original error
	}{
		{
			name: "problem",
			original: &errs.Problem{
				Category:       errs.CategoryInternal,
				Subtype:        errs.SubtypeUnknown,
				Code:           123,
				Message:        "message",
				Hint:           hint.String(),
				LogID:          "log-id",
				Troubleshooter: "https://example.test/troubleshooter",
				Retryable:      true,
			},
		},
		{
			name: "validation",
			original: &errs.ValidationError{
				Problem: problem(errs.CategoryValidation, errs.SubtypeInvalidArgument),
				Param:   "--name",
				Params: []errs.InvalidParam{{
					Name:        "--name",
					Reason:      "invalid",
					Suggestions: []string{"--title"},
				}},
				Cause: sentinel,
			},
		},
		{
			name: "authentication",
			original: &errs.AuthenticationError{
				Problem:    problem(errs.CategoryAuthentication, errs.SubtypeTokenExpired),
				UserOpenID: "ou_test",
				Cause:      sentinel,
			},
		},
		{
			name: "permission",
			original: &errs.PermissionError{
				Problem:         problem(errs.CategoryAuthorization, errs.SubtypeMissingScope),
				MissingScopes:   []string{"scope.missing"},
				RequestedScopes: []string{"scope.requested"},
				GrantedScopes:   []string{"scope.granted"},
				Identity:        "user",
				ConsoleURL:      "https://example.test/console",
				Cause:           sentinel,
			},
		},
		{
			name: "config",
			original: &errs.ConfigError{
				Problem: problem(errs.CategoryConfig, errs.SubtypeNotConfigured),
				Field:   "app_id",
				Cause:   sentinel,
			},
		},
		{
			name: "network",
			original: &errs.NetworkError{
				Problem: problem(errs.CategoryNetwork, errs.SubtypeNetworkTimeout),
				Cause:   sentinel,
			},
		},
		{
			name: "api",
			original: &errs.APIError{
				Problem:           problem(errs.CategoryAPI, errs.SubtypeRateLimit),
				RetryAfterSeconds: 7,
				Cause:             sentinel,
			},
		},
		{
			name: "security_policy",
			original: &errs.SecurityPolicyError{
				Problem:      problem(errs.CategoryPolicy, errs.SubtypeChallengeRequired),
				ChallengeURL: "https://example.test/challenge",
				Cause:        sentinel,
			},
		},
		{
			name: "content_safety",
			original: &errs.ContentSafetyError{
				Problem: problem(errs.CategoryPolicy, errs.SubtypeContentSafety),
				Rules:   []string{"secret"},
				Cause:   sentinel,
			},
		},
		{
			name: "internal",
			original: &errs.InternalError{
				Problem: problem(errs.CategoryInternal, errs.SubtypeFileIO),
				Cause:   sentinel,
			},
		},
		{
			name: "confirmation",
			original: &errs.ConfirmationRequiredError{
				Problem: problem(errs.CategoryConfirmation, errs.SubtypeConfirmationRequired),
				Risk:    errs.RiskHighRiskWrite,
				Action:  "delete",
				Cause:   sentinel,
			},
		},
	}

	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		"config/init": surface.CommandConcealed,
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rendered := Render(Annotate(tt.original, hint), plan)

			if reflect.TypeOf(rendered) != reflect.TypeOf(tt.original) {
				t.Fatalf("rendered concrete type = %T, want %T", rendered, tt.original)
			}
			if reflect.ValueOf(rendered).Pointer() == reflect.ValueOf(tt.original).Pointer() {
				t.Fatal("Render returned the producer's typed error instead of a clone")
			}
			if originalProblem, ok := errs.ProblemOf(tt.original); !ok || originalProblem.Hint != hint.String() {
				t.Fatalf("Render mutated source hint: %#v", originalProblem)
			}
			renderedProblem, ok := errs.ProblemOf(rendered)
			if !ok {
				t.Fatalf("rendered %T is not a typed error", rendered)
			}
			if got, want := renderedProblem.Hint, "inspect logs"; got != want {
				t.Fatalf("rendered hint = %q, want %q", got, want)
			}
			if tt.name != "problem" && !errors.Is(rendered, sentinel) {
				t.Error("typed cause was not preserved")
			}
			assertSameWireExceptHint(t, tt.original, rendered, "inspect logs")
		})
	}
}

func TestRenderProjectsStructuredMessageWithoutMutatingSource(t *testing.T) {
	message := Join("",
		Text("operation failed."),
		Command(TargetSkillsRead, " Run `lark-cli skills read lark-doc`."),
		Text(" Run `lark-cli docs --help`."),
	)
	original := AnnotateMessage(
		errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", message.String()),
		message,
	)
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandSkillsRead: surface.CommandConcealed,
	})

	rendered := Render(original, plan)
	renderedProblem, ok := errs.ProblemOf(rendered)
	if !ok {
		t.Fatalf("Render returned %T, want typed error", rendered)
	}
	if got, want := renderedProblem.Message,
		"operation failed. Run `lark-cli docs --help`."; got != want {
		t.Fatalf("rendered message = %q, want %q", got, want)
	}
	originalProblem, _ := errs.ProblemOf(original)
	if got, want := originalProblem.Message, message.String(); got != want {
		t.Fatalf("source message mutated to %q, want %q", got, want)
	}
}

func TestRenderPreservesProducerEnrichmentAddedAfterAnnotation(t *testing.T) {
	hint := Join("; ",
		Command(TargetConfigInit, "run `lark-cli config init`"),
		Text("inspect logs"),
	)
	typed := errs.NewConfigError(errs.SubtypeNotConfigured, "not configured").
		WithHint("%s", hint.String())
	annotated := Annotate(typed, hint)
	typed.Hint = "completed_steps=[create]\n" + typed.Hint + "\nrollback_event_id=evt-123"

	visible := Render(annotated, nil)
	visibleProblem, _ := errs.ProblemOf(visible)
	if got, want := visibleProblem.Hint, typed.Hint; got != want {
		t.Fatalf("visible hint = %q, want enriched producer hint %q", got, want)
	}

	concealed := Render(annotated, surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandConfigInit: surface.CommandConcealed,
	}))
	concealedProblem, _ := errs.ProblemOf(concealed)
	if got, want := concealedProblem.Hint,
		"completed_steps=[create]\ninspect logs\nrollback_event_id=evt-123"; got != want {
		t.Fatalf("concealed hint = %q, want %q", got, want)
	}
}

func TestRenderDoesNotBorrowNestedTypedErrorAnnotation(t *testing.T) {
	innerHint := Join("", Command(TargetAuthLogin, "run `lark-cli auth login`"))
	inner := Annotate(
		errs.NewAuthenticationError(errs.SubtypeTokenMissing, "inner").
			WithHint("%s", innerHint.String()),
		innerHint,
	)
	outer := errs.NewInternalError(errs.SubtypeUnknown, "outer").
		WithHint("outer recovery").
		WithCause(inner)

	rendered := Render(outer, surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandAuthLogin: surface.CommandConcealed,
	}))
	problem, _ := errs.ProblemOf(rendered)
	if got, want := problem.Hint, "outer recovery"; got != want {
		t.Fatalf("outer hint = %q, want %q", got, want)
	}
}

func TestProjectorUsesLazyBuildLocalPlan(t *testing.T) {
	var plan *surface.Plan
	projector := NewProjector(func() *surface.Plan { return plan })
	if !projector.CanReference(TargetUpdate) {
		t.Fatal("nil plan should be fully visible")
	}
	plan = surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandUpdate: surface.CommandConcealed,
	})
	if projector.CanReference(TargetUpdate) {
		t.Fatal("projector did not observe its completed build-local plan")
	}
}

func TestRenderDeepClonesSliceExtensions(t *testing.T) {
	validation := &errs.ValidationError{
		Problem: errs.Problem{Message: "message"},
		Params: []errs.InvalidParam{{
			Name:        "--name",
			Suggestions: []string{"--title"},
		}},
	}
	permission := &errs.PermissionError{
		Problem:         errs.Problem{Message: "message"},
		MissingScopes:   []string{"missing"},
		RequestedScopes: []string{"requested"},
		GrantedScopes:   []string{"granted"},
	}
	content := &errs.ContentSafetyError{
		Problem: errs.Problem{Message: "message"},
		Rules:   []string{"rule"},
	}

	renderedValidation := Render(validation, nil).(*errs.ValidationError)
	renderedPermission := Render(permission, nil).(*errs.PermissionError)
	renderedContent := Render(content, nil).(*errs.ContentSafetyError)
	renderedValidation.Params[0].Suggestions[0] = "changed"
	renderedPermission.MissingScopes[0] = "changed"
	renderedPermission.RequestedScopes[0] = "changed"
	renderedPermission.GrantedScopes[0] = "changed"
	renderedContent.Rules[0] = "changed"

	if validation.Params[0].Suggestions[0] != "--title" {
		t.Error("ValidationError suggestions alias the source")
	}
	if permission.MissingScopes[0] != "missing" ||
		permission.RequestedScopes[0] != "requested" ||
		permission.GrantedScopes[0] != "granted" {
		t.Error("PermissionError scope slices alias the source")
	}
	if content.Rules[0] != "rule" {
		t.Error("ContentSafetyError rules alias the source")
	}
}

func TestRenderLeavesUntypedAndRawErrorsUnchanged(t *testing.T) {
	untyped := errors.New("untyped")
	if got := Render(untyped, nil); got != untyped {
		t.Error("untyped error must pass through unchanged")
	}

	typed := errs.NewConfigError(errs.SubtypeNotConfigured, "not configured").
		WithHint("run config init")
	raw := errs.MarkRaw(Annotate(typed,
		Join("", Command(TargetConfigInit, "run config init")),
	))
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		"config/init": surface.CommandConcealed,
	})
	if got := Render(raw, plan); got != raw {
		t.Error("raw error must pass through unchanged")
	}
	if typed.Hint != "run config init" {
		t.Error("raw error hint was rewritten")
	}
}

func TestRenderWrappedTypedErrorReturnsConcreteClone(t *testing.T) {
	typed := errs.NewConfigError(errs.SubtypeNotConfigured, "not configured").
		WithHint("run config init").
		WithField("app_id")
	wrapped := &testWrapper{err: typed}

	rendered := Render(wrapped, nil)
	config, ok := rendered.(*errs.ConfigError)
	if !ok {
		t.Fatalf("Render() type = %T, want *errs.ConfigError", rendered)
	}
	if config == typed || config.Field != "app_id" {
		t.Fatalf("Render() did not preserve a concrete cloned ConfigError: %#v", config)
	}
}

type testWrapper struct {
	err error
}

func (e *testWrapper) Error() string { return "wrapped: " + e.err.Error() }
func (e *testWrapper) Unwrap() error { return e.err }

func assertSameWireExceptHint(t *testing.T, original, rendered error, wantHint string) {
	t.Helper()

	var want, got map[string]any
	originalJSON, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal original: %v", err)
	}
	renderedJSON, err := json.Marshal(rendered)
	if err != nil {
		t.Fatalf("marshal rendered: %v", err)
	}
	if err := json.Unmarshal(originalJSON, &want); err != nil {
		t.Fatalf("unmarshal original: %v", err)
	}
	if err := json.Unmarshal(renderedJSON, &got); err != nil {
		t.Fatalf("unmarshal rendered: %v", err)
	}
	want["hint"] = wantHint
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wire fields changed beyond hint:\n got: %s\nwant: %s", renderedJSON, mustJSON(t, want))
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal expected value: %v", err)
	}
	return data
}
