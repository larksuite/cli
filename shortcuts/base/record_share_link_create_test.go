// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestBaseRecordShareLinkCreateAcceptsSingularAndPluralRecordIDFlags(t *testing.T) {
	factory, stdout, registry := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/records/share_links/batch",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"record_share_links": map[string]interface{}{
					"rec_1": "https://example.com/rec_1",
					"rec_2": "https://example.com/rec_2",
					"rec_3": "https://example.com/rec_3",
				},
			},
		},
	}
	registry.Register(stub)

	err := runShortcut(t, BaseRecordShareLinkCreate, []string{
		"+record-share-link-create",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--record-id", "rec_1",
		"--record-ids", "rec_2,rec_3",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("run shortcut: %v", err)
	}

	body := decodeCapturedJSONBody(t, stub)
	want := []interface{}{"rec_1", "rec_2", "rec_3"}
	if got := body["record_ids"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("record_ids = %#v, want %#v", got, want)
	}
}

func TestBaseRecordShareLinkCreateHelpShowsRecordIDAndHidesRecordIDsAlias(t *testing.T) {
	cmd := mountBaseShortcutFlags(t, BaseRecordShareLinkCreate, "+record-share-link-create")
	usage := cmd.Flags().FlagUsages()
	if !strings.Contains(usage, "--record-id strings") {
		t.Fatalf("help does not show canonical --record-id flag:\n%s", usage)
	}
	if strings.Contains(usage, "--record-ids") {
		t.Fatalf("help exposes hidden --record-ids alias:\n%s", usage)
	}
	if alias := cmd.Flags().Lookup("record-ids"); alias == nil || alias.Name != "record-id" {
		t.Fatalf("Lookup(record-ids) = %#v, want canonical --record-id", alias)
	}
}
