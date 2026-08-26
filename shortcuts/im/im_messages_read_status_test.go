// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func newMessagesReadStatusTestRuntime(t *testing.T, messageIDs string) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("message-ids", "", "")
	if err := cmd.Flags().Set("message-ids", messageIDs); err != nil {
		t.Fatalf("Flags().Set() error = %v", err)
	}
	return &common.RuntimeContext{Cmd: cmd}
}

func TestBuildMessagesReadStatusBody(t *testing.T) {
	runtime := newMessagesReadStatusTestRuntime(t, "om_one, om_two")

	got, err := buildMessagesReadStatusBody(runtime)
	if err != nil {
		t.Fatalf("buildMessagesReadStatusBody() error = %v", err)
	}
	want := map[string]interface{}{
		"message_ids": []string{"om_one", "om_two"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildMessagesReadStatusBody() = %#v, want %#v", got, want)
	}
}

func TestBuildMessagesReadStatusBodyAllowsMaximum(t *testing.T) {
	runtime := newMessagesReadStatusTestRuntime(t, strings.Join(makeReadStatusMessageIDs(50), ","))

	if _, err := buildMessagesReadStatusBody(runtime); err != nil {
		t.Fatalf("buildMessagesReadStatusBody() error = %v", err)
	}
}

func TestBuildMessagesReadStatusBodyRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		messageIDs string
	}{
		{name: "empty", messageIDs: ""},
		{name: "invalid prefix", messageIDs: "oc_not_message"},
		{name: "more than fifty", messageIDs: strings.Join(makeReadStatusMessageIDs(51), ",")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := newMessagesReadStatusTestRuntime(t, tt.messageIDs)
			_, err := buildMessagesReadStatusBody(runtime)
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("errs.ProblemOf() ok = false, err = %v", err)
			}
			if problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("problem.Subtype = %q, want %q", problem.Subtype, errs.SubtypeInvalidArgument)
			}
			if problem.Category != errs.CategoryValidation {
				t.Fatalf("problem.Category = %q, want %q", problem.Category, errs.CategoryValidation)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("errors.As(*errs.ValidationError) = false, err = %v", err)
			}
			if validationErr.Param != "--message-ids" {
				t.Fatalf("validationErr.Param = %q, want --message-ids", validationErr.Param)
			}
		})
	}
}

func TestMessagesReadStatusShortcutContract(t *testing.T) {
	if !reflect.DeepEqual(ImMessagesReadStatus.AuthTypes, []string{"user"}) {
		t.Fatalf("AuthTypes = %v, want [user]", ImMessagesReadStatus.AuthTypes)
	}
	if got := ImMessagesReadStatus.ScopesForIdentity("user"); !reflect.DeepEqual(got, []string{"im:message:readonly"}) {
		t.Fatalf("user preflight scopes = %v, want [im:message:readonly]", got)
	}
	if got := ImMessagesReadStatus.DeclaredScopesForIdentity("user"); !reflect.DeepEqual(got, []string{"im:message:readonly"}) {
		t.Fatalf("declared user scopes = %v, want [im:message:readonly]", got)
	}
	if ImMessagesReadStatus.Risk != "read" {
		t.Fatalf("Risk = %q, want read", ImMessagesReadStatus.Risk)
	}
}

func makeReadStatusMessageIDs(count int) []string {
	ids := make([]string, count)
	for i := range ids {
		ids[i] = fmt.Sprintf("om_%d", i)
	}
	return ids
}
