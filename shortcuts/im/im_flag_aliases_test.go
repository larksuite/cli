// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func newMountedIMRuntime(t *testing.T, shortcut *common.Shortcut, args ...string) (*common.RuntimeContext, *bytes.Buffer) {
	t.Helper()
	config := &core.CliConfig{}
	factory, _, stderr, _ := cmdutil.TestFactory(t, config)
	parent := &cobra.Command{Use: "root"}
	shortcut.Mount(parent, factory)
	cmd, _, err := parent.Find([]string{shortcut.Command})
	if err != nil {
		t.Fatalf("Find(%s) error = %v", shortcut.Command, err)
	}
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags(%v) error = %v", args, err)
	}
	return &common.RuntimeContext{Cmd: cmd, Factory: factory, Config: config}, stderr
}

func TestIMDeclarativeFlagAliases(t *testing.T) {
	tests := []struct {
		shortcut  *common.Shortcut
		canonical string
		aliases   []string
	}{
		{&ImChatMessageList, "start", []string{"start-time"}},
		{&ImChatMessageList, "end", []string{"end-time"}},
		{&ImChatMessageList, "order", []string{"sort-order"}},
		{&ImChatMessageList, "page-size", []string{"limit"}},
		{&ImChatMembersList, "page-size", []string{"limit"}},
		{&ImThreadsMessagesList, "thread", []string{"thread-id"}},
		{&ImMessagesMGet, "message-ids", []string{"message-id"}},
		{&ImMessagesSearch, "query", []string{"keyword"}},
		{&ImMessagesSearch, "page-size", []string{"limit"}},
	}

	for _, test := range tests {
		t.Run(test.shortcut.Command+"/"+test.canonical, func(t *testing.T) {
			canonical := findIMFlag(t, test.shortcut, test.canonical)
			if !slices.Equal(canonical.Aliases, test.aliases) {
				t.Fatalf("--%s aliases = %v, want %v", test.canonical, canonical.Aliases, test.aliases)
			}
			for _, alias := range test.aliases {
				for _, declared := range test.shortcut.Flags {
					if declared.Name == alias {
						t.Fatalf("--%s must not be declared as an independent flag", alias)
					}
				}
			}
		})
	}
}

func TestIMFlagAliasesProduceCanonicalRequests(t *testing.T) {
	aliasRT, _ := newMountedIMRuntime(t, &ImChatMessageList,
		"--chat-id", "oc_test",
		"--start-time", "2026-07-27T00:00:00+08:00",
		"--end-time", "1785254400",
		"--sort-order", "asc",
		"--limit", "25",
	)
	canonicalRT, _ := newMountedIMRuntime(t, &ImChatMessageList,
		"--chat-id", "oc_test",
		"--start", "2026-07-27T00:00:00+08:00",
		"--end", "1785254400",
		"--order", "asc",
		"--page-size", "25",
	)
	aliasParams, err := buildChatMessageListRequest(aliasRT, "oc_test")
	if err != nil {
		t.Fatal(err)
	}
	canonicalParams, err := buildChatMessageListRequest(canonicalRT, "oc_test")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(aliasParams, canonicalParams) {
		t.Fatalf("chat-message alias request = %#v, canonical request = %#v", aliasParams, canonicalParams)
	}

	aliasRT, _ = newMountedIMRuntime(t, &ImChatMembersList,
		"--chat-id", "oc_test", "--limit", "25", "--page-all",
	)
	canonicalRT, _ = newMountedIMRuntime(t, &ImChatMembersList,
		"--chat-id", "oc_test", "--page-size", "25", "--page-all",
	)
	aliasMemberParams, err := buildChatMembersParams(aliasRT, "")
	if err != nil {
		t.Fatal(err)
	}
	canonicalMemberParams, err := buildChatMembersParams(canonicalRT, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(aliasMemberParams, canonicalMemberParams) {
		t.Fatalf("chat-members alias request = %#v, canonical request = %#v", aliasMemberParams, canonicalMemberParams)
	}

	aliasRT, _ = newMountedIMRuntime(t, &ImMessagesSearch, "--keyword", "project", "--limit", "30")
	canonicalRT, _ = newMountedIMRuntime(t, &ImMessagesSearch, "--query", "project", "--page-size", "30")
	aliasSearch, err := buildMessagesSearchRequest(aliasRT)
	if err != nil {
		t.Fatal(err)
	}
	canonicalSearch, err := buildMessagesSearchRequest(canonicalRT)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(aliasSearch, canonicalSearch) {
		t.Fatalf("message-search alias request = %#v, canonical request = %#v", aliasSearch, canonicalSearch)
	}
}

func TestIMAliasValidationReportsCanonicalFlag(t *testing.T) {
	runtime, _ := newMountedIMRuntime(t, &ImChatMessageList, "--chat-id", "oc_test", "--start-time", "bad-time")
	_, err := buildChatMessageListRequest(runtime, "oc_test")
	assertIMValidationError(t, err, "--start", "--start: cannot parse time")

	runtime, _ = newMountedIMRuntime(t, &ImThreadsMessagesList, "--thread-id", "not-a-thread")
	err = ImThreadsMessagesList.Validate(context.Background(), runtime)
	assertIMValidationError(t, err, "--thread", `invalid --thread "not-a-thread"`)

	runtime, _ = newMountedIMRuntime(t, &ImMessagesMGet, "--message-id", "not-om")
	err = ImMessagesMGet.Validate(context.Background(), runtime)
	assertIMValidationError(t, err, "--message-ids", `invalid message ID "not-om"`)
}

func findIMFlag(t *testing.T, shortcut *common.Shortcut, name string) *common.Flag {
	t.Helper()
	for i := range shortcut.Flags {
		if shortcut.Flags[i].Name == name {
			return &shortcut.Flags[i]
		}
	}
	t.Fatalf("%s is missing --%s", shortcut.Command, name)
	return nil
}

func assertIMValidationError(t *testing.T, err error, wantParam, wantMessage string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error is not typed: %T %v", err, err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %#v", problem)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error is not *errs.ValidationError: %T %v", err, err)
	}
	if validationErr.Param != wantParam {
		t.Fatalf("param = %q, want %q", validationErr.Param, wantParam)
	}
	if !strings.Contains(err.Error(), wantMessage) {
		t.Fatalf("error = %q, want substring %q", err, wantMessage)
	}
}
