// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"errors"
	"net/url"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/smartystreets/goconvey/convey"
)

func TestShortcutsRegistration(t *testing.T) {
	convey.Convey("Shortcuts() returns all commands", t, func() {
		list := Shortcuts()
		convey.So(len(list), convey.ShouldBeGreaterThan, 0)
	})
}

func TestParseTaskGUID(t *testing.T) {
	t.Run("accepts GUIDs and task applinks", func(t *testing.T) {
		tests := []struct {
			name  string
			input string
			want  string
		}{
			{name: "opaque GUID", input: "task-guid-123", want: "task-guid-123"},
			{name: "trimmed GUID", input: "  task-guid-123  ", want: "task-guid-123"},
			{
				name:  "task applink",
				input: "https://applink.larksuite.com/client/todo/detail?guid=task-guid-123",
				want:  "task-guid-123",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := parseTaskGUID(tt.input)
				if err != nil {
					t.Fatalf("parseTaskGUID(%q) error = %v", tt.input, err)
				}
				if got != tt.want {
					t.Fatalf("parseTaskGUID(%q) = %q, want %q", tt.input, got, tt.want)
				}
			})
		}
	})

	t.Run("rejects unusable task identifiers", func(t *testing.T) {
		for _, input := range []string{
			"",
			"https://applink.larksuite.com/client/todo/detail",
			"https://%",
			"t12345",
		} {
			t.Run(input, func(t *testing.T) {
				_, err := parseTaskGUID(input)
				if err == nil {
					t.Fatalf("parseTaskGUID(%q) error = nil, want typed validation error", input)
				}

				problem, ok := errs.ProblemOf(err)
				if !ok {
					t.Fatalf("parseTaskGUID(%q) error type = %T, want typed error", input, err)
				}
				if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
					t.Fatalf("problem = %s/%s, want %s/%s", problem.Category, problem.Subtype, errs.CategoryValidation, errs.SubtypeInvalidArgument)
				}
				if problem.Hint == "" {
					t.Fatal("problem hint is empty")
				}

				var validationErr *errs.ValidationError
				if !errors.As(err, &validationErr) {
					t.Fatalf("error type = %T, want *errs.ValidationError", err)
				}
				if validationErr.Param != "--task-id" {
					t.Fatalf("param = %q, want %q", validationErr.Param, "--task-id")
				}
			})
		}
	})

	t.Run("preserves applink parse cause", func(t *testing.T) {
		_, err := parseTaskGUID("https://%")
		if err == nil {
			t.Fatal("parseTaskGUID() error = nil, want URL parse error")
		}

		var urlErr *url.Error
		if !errors.As(err, &urlErr) {
			t.Fatalf("error chain = %T %v, want *url.Error cause", err, err)
		}
	})
}
