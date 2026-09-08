// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"testing"
)

func TestParseFieldGroupBodiesAcceptsWrappedAndBare(t *testing.T) {
	rt := newBaseTestRuntime(nil, nil, nil)
	pc := newParseCtx(rt)

	wrapped := `{"field_groups":[{"name":"G1","children":[{"type":"field","id":"fldA"}]}]}`
	groups, err := parseFieldGroupBodies(pc, wrapped)
	if err != nil {
		t.Fatalf("wrapped body: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("wrapped body groups = %d, want 1", len(groups))
	}

	bare := `[{"name":"G1","children":[{"type":"field","id":"fldA"}]}]`
	groups, err = parseFieldGroupBodies(pc, bare)
	if err != nil {
		t.Fatalf("bare array: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("bare array groups = %d, want 1", len(groups))
	}
}

func TestParseFieldGroupBodiesRejectsBadShapes(t *testing.T) {
	rt := newBaseTestRuntime(nil, nil, nil)
	pc := newParseCtx(rt)

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"not json", `nope`, "--json"},
		{"empty array", `[]`, "at least one field group"},
		{"object without field_groups", `{"groups":[]}`, "field_groups"},
		{"group without name", `[{"children":[{"type":"field","id":"fldA"}]}]`, "non-empty name"},
		{"group without children", `[{"name":"G1"}]`, "at least one child"},
		{"child without id", `[{"name":"G1","children":[{"type":"field"}]}]`, "type and id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseFieldGroupBodies(pc, tc.raw)
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q missing %q", err.Error(), tc.want)
			}
		})
	}
}

func TestParseFieldGroupBodiesRejectsDuplicateFieldAcrossGroups(t *testing.T) {
	rt := newBaseTestRuntime(nil, nil, nil)
	pc := newParseCtx(rt)

	raw := `[{"name":"G1","children":[{"type":"field","id":"fldA"}]},{"name":"G2","children":[{"type":"field","id":"fldA"}]}]`
	_, err := parseFieldGroupBodies(pc, raw)
	if err == nil {
		t.Fatal("expected duplicate field error")
	}
	if !strings.Contains(err.Error(), "only one field group") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDryRunFieldGroupCreate(t *testing.T) {
	ctx := context.Background()
	rt := newBaseTestRuntime(map[string]string{
		"base-token": "app_x",
		"table-id":   "tbl_1",
		"json":       `{"field_groups":[{"name":"Customer Info","children":[{"type":"field","id":"fldA"}]}]}`,
	}, nil, nil)

	sc := BaseFieldGroupCreate
	assertDryRunContains(t, sc.DryRun(ctx, rt),
		"POST /open-apis/bitable/v1/apps/app_x/tables/tbl_1/field_groups",
		`"name":"Customer Info"`,
		`"children":[{"id":"fldA","type":"field"}]`)
}
