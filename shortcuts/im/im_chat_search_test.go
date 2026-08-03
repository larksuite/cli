// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
)

func newSearchTestRT(t *testing.T, stringFlags map[string]string) *common.RuntimeContext {
	t.Helper()
	if stringFlags == nil {
		stringFlags = map[string]string{}
	}
	if _, ok := stringFlags["query"]; !ok {
		stringFlags["query"] = "team"
	}
	rt := newChatSearchTestRuntimeContext(t, stringFlags, nil)
	rt.Factory = &cmdutil.Factory{
		IOStreams: &cmdutil.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		},
	}
	return rt
}

func TestChatSearch_SortMapping(t *testing.T) {
	cases := []struct{ sort, want string }{
		{"create_time", "create_time_desc"},
		{"update_time", "update_time_desc"},
		{"member_count", "member_count_desc"},
	}
	for _, c := range cases {
		t.Run(c.sort, func(t *testing.T) {
			rt := newSearchTestRT(t, map[string]string{"sort": c.sort})
			body := buildSearchChatBody(rt)
			if body["sorter"] != c.want {
				t.Fatalf("sort=%s -> sorter=%v, want %s", c.sort, body["sorter"], c.want)
			}
		})
	}
}

// TestChatSearch_SortOmittedWhenUnset: no --sort and no --sort-by -> sorter omitted.
func TestChatSearch_SortOmittedWhenUnset(t *testing.T) {
	rt := newSearchTestRT(t, nil)
	body := buildSearchChatBody(rt)
	if _, present := body["sorter"]; present {
		t.Fatalf("sorter should be omitted when neither --sort nor --sort-by set")
	}
}

// TestChatSearch_SortAliasParity: hidden --sort-by value is already the upstream
// sorter (pass-through), so it must equal the mapped new --sort body.
func TestChatSearch_SortAliasParity(t *testing.T) {
	pairs := []struct{ newVal, oldVal string }{
		{"create_time", "create_time_desc"},
		{"update_time", "update_time_desc"},
		{"member_count", "member_count_desc"},
	}
	for _, p := range pairs {
		t.Run(p.newVal, func(t *testing.T) {
			newBody := buildSearchChatBody(newSearchTestRT(t, map[string]string{"sort": p.newVal}))
			oldBody := buildSearchChatBody(newSearchTestRT(t, map[string]string{"sort-by": p.oldVal}))
			if newBody["sorter"] != oldBody["sorter"] {
				t.Fatalf("alias parity: new sorter=%v, old sorter=%v", newBody["sorter"], oldBody["sorter"])
			}
		})
	}
}

func TestChatSearch_SortNewWins(t *testing.T) {
	rt := newSearchTestRT(t, map[string]string{"sort": "member_count", "sort-by": "create_time_desc"})
	body := buildSearchChatBody(rt)
	if body["sorter"] != "member_count_desc" {
		t.Fatalf("new should win: sorter=%v, want member_count_desc", body["sorter"])
	}
}

func TestChatSearch_SortFlagSurface(t *testing.T) {
	var sortFlag, aliasFlag *common.Flag
	for i := range ImChatSearch.Flags {
		switch ImChatSearch.Flags[i].Name {
		case "sort":
			sortFlag = &ImChatSearch.Flags[i]
		case "sort-by":
			aliasFlag = &ImChatSearch.Flags[i]
		}
	}
	if sortFlag == nil || aliasFlag == nil {
		t.Fatalf("expected both --sort and --sort-by flags declared")
	}
	if sortFlag.Default != "" {
		t.Errorf("--sort must have no default (sorter omitted when unset), got %q", sortFlag.Default)
	}
	if got := strings.Join(sortFlag.Enum, ","); got != "create_time,update_time,member_count" {
		t.Errorf("--sort Enum = %q", got)
	}
	if !aliasFlag.Hidden {
		t.Errorf("--sort-by must be Hidden")
	}
	if len(aliasFlag.Enum) != 0 {
		// Enforced by validateAliasEnum in Validate; a declared enum would be
		// framework-validated before canonical-wins resolution runs.
		t.Errorf("--sort-by (hidden alias) must not declare an Enum, got %q", aliasFlag.Enum)
	}
}

func TestChatSearch_TypesGroupMatchesChatModesGroup(t *testing.T) {
	for _, typesValue := range []string{"group", "group,group"} {
		t.Run(typesValue, func(t *testing.T) {
			typesRT := newSearchTestRT(t, map[string]string{"types": typesValue})
			if err := ImChatSearch.Validate(context.Background(), typesRT); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			canonicalRT := newSearchTestRT(t, map[string]string{"chat-modes": "group"})

			typesBody := buildSearchChatBody(typesRT)
			canonicalBody := buildSearchChatBody(canonicalRT)
			if !reflect.DeepEqual(typesBody, canonicalBody) {
				t.Fatalf("--types body = %#v, --chat-modes body = %#v", typesBody, canonicalBody)
			}
			filter, _ := typesBody["filter"].(map[string]interface{})
			if got := filter["chat_modes"]; !reflect.DeepEqual(got, []string{"default"}) {
				t.Fatalf("filter.chat_modes = %#v, want []string{\"default\"}", got)
			}

			stderr := typesRT.IO().ErrOut.(*bytes.Buffer).String()
			if stderr != "note: --types on +chat-search maps to --chat-modes\n" {
				t.Fatalf("stderr = %q", stderr)
			}
			if stdout := typesRT.IO().Out.(*bytes.Buffer).String(); stdout != "" {
				t.Fatalf("mapping note leaked to stdout: %q", stdout)
			}
		})
	}
}

func TestChatSearch_TypesP2PReturnsActionableValidationError(t *testing.T) {
	for _, typesValue := range []string{"p2p", "group,p2p"} {
		t.Run(typesValue, func(t *testing.T) {
			rt := newSearchTestRT(t, map[string]string{"types": typesValue})
			err := ImChatSearch.Validate(context.Background(), rt)
			assertAliasValidationError(t, err, "--types", "im +chat-list --types p2p")
			if !strings.Contains(err.Error(), "service does not support p2p") {
				t.Fatalf("error = %q, want service p2p limitation", err)
			}
		})
	}
}

func TestChatSearch_TypesUnknownListsCanonicalValueDomains(t *testing.T) {
	rt := newSearchTestRT(t, map[string]string{"types": "xxx"})
	err := ImChatSearch.Validate(context.Background(), rt)
	assertAliasValidationError(t, err, "--types", "--chat-modes (group|topic)")
	if !strings.Contains(err.Error(), "--search-types (private|external|public_joined|public_not_joined)") {
		t.Fatalf("error = %q, want --search-types values", err)
	}
}

func TestChatSearch_ChatModesWinsOverTypes(t *testing.T) {
	rt := newSearchTestRT(t, map[string]string{
		"types":      "p2p",
		"chat-modes": "topic",
	})
	if err := ImChatSearch.Validate(context.Background(), rt); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	body := buildSearchChatBody(rt)
	filter, _ := body["filter"].(map[string]interface{})
	if got := filter["chat_modes"]; !reflect.DeepEqual(got, []string{"thread"}) {
		t.Fatalf("filter.chat_modes = %#v, want []string{\"thread\"}", got)
	}
	if stderr := rt.IO().ErrOut.(*bytes.Buffer).String(); stderr != "" {
		t.Fatalf("ignored --types emitted stderr: %q", stderr)
	}
}

func TestChatSearch_TypesFlagIsHiddenAndHasNoEnum(t *testing.T) {
	var typesFlag *common.Flag
	for i := range ImChatSearch.Flags {
		if ImChatSearch.Flags[i].Name == "types" {
			typesFlag = &ImChatSearch.Flags[i]
			break
		}
	}
	if typesFlag == nil {
		t.Fatal("--types flag is missing")
	}
	if !typesFlag.Hidden {
		t.Fatal("--types must be hidden")
	}
	if len(typesFlag.Enum) != 0 {
		t.Fatalf("--types enum = %v, want custom validation", typesFlag.Enum)
	}
}
