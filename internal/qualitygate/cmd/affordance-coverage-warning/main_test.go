// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/meta"
)

func TestReportCoverage(t *testing.T) {
	tests := []struct {
		name       string
		methods    []apicatalog.MethodRef
		documented map[string]bool
		want       string
	}{
		{
			name: "no metadata",
			want: "WARNING: no embedded API metadata; run make fetch_meta before checking coverage.\n",
		},
		{
			name: "reports sorted missing native methods and skips shortcuts",
			methods: []apicatalog.MethodRef{
				testMethod("drive", "files.copy"),
				testMethod("calendar", "calendar.get"),
				testMethod("drive", "+copy-file"),
			},
			documented: map[string]bool{"drive/files.copy": true},
			want:       "WARNING: 1 native command(s) have no affordance document:\ncalendar/calendar.get\n",
		},
		{
			name: "reports complete coverage",
			methods: []apicatalog.MethodRef{
				testMethod("drive", "files.copy"),
			},
			documented: map[string]bool{"drive/files.copy": true},
			want:       "Affordance coverage is complete.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			reportCoverage(&out, tt.methods, func(service, methodID string) bool {
				return tt.documented[service+"/"+methodID]
			})
			if got := out.String(); got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func testMethod(service, methodID string) apicatalog.MethodRef {
	return apicatalog.MethodRef{
		Service: meta.Service{Name: service},
		Method:  meta.Method{ID: methodID},
	}
}
