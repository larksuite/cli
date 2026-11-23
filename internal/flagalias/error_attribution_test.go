// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package flagalias

import (
	"errors"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
)

func TestInvalidValueAttributionOf(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		managed   bool
		shorthand bool
		wrap      bool
		want      InvalidValueAttribution
		wantOK    bool
	}{
		{
			name:    "alias separate value",
			args:    []string{"--page-size", "bad"},
			managed: true,
			want:    InvalidValueAttribution{Canonical: "limit", Source: "page-size"},
			wantOK:  true,
		},
		{
			name:    "alias equals value",
			args:    []string{"--page-size=bad"},
			managed: true,
			want:    InvalidValueAttribution{Canonical: "limit", Source: "page-size"},
			wantOK:  true,
		},
		{
			name:    "canonical separate value",
			args:    []string{"--limit", "bad"},
			managed: true,
			want:    InvalidValueAttribution{Canonical: "limit", Source: "limit"},
			wantOK:  true,
		},
		{
			name:    "canonical equals value",
			args:    []string{"--limit=bad"},
			managed: true,
			want:    InvalidValueAttribution{Canonical: "limit", Source: "limit"},
			wantOK:  true,
		},
		{
			name:    "alias fails after canonical",
			args:    []string{"--limit=10", "--page-size=bad"},
			managed: true,
			want:    InvalidValueAttribution{Canonical: "limit", Source: "page-size"},
			wantOK:  true,
		},
		{
			name:    "canonical fails after alias",
			args:    []string{"--page-size=10", "--limit=bad"},
			managed: true,
			want:    InvalidValueAttribution{Canonical: "limit", Source: "limit"},
			wantOK:  true,
		},
		{
			name:   "ordinary flag",
			args:   []string{"--limit=bad"},
			wantOK: false,
		},
		{
			name:      "shorthand source is ambiguous",
			args:      []string{"-l", "bad"},
			managed:   true,
			shorthand: true,
			wantOK:    false,
		},
		{
			name:      "alias remains exact when canonical has shorthand",
			args:      []string{"--page-size=bad"},
			managed:   true,
			shorthand: true,
			want:      InvalidValueAttribution{Canonical: "limit", Source: "page-size"},
			wantOK:    true,
		},
		{
			name:    "wrapped pflag error",
			args:    []string{"--page-size=bad"},
			managed: true,
			wrap:    true,
			want:    InvalidValueAttribution{Canonical: "limit", Source: "page-size"},
			wantOK:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "demo"}
			if test.shorthand {
				cmd.Flags().IntP("limit", "l", 10, "")
			} else {
				cmd.Flags().Int("limit", 10, "")
			}
			if test.managed {
				if err := Bind(cmd, []Spec{{Canonical: "limit", Aliases: []string{"page-size"}}}); err != nil {
					t.Fatal(err)
				}
			}
			parseErr := cmd.ParseFlags(test.args)
			if parseErr == nil {
				t.Fatal("ParseFlags() succeeded, want invalid integer error")
			}
			if test.wrap {
				parseErr = fmt.Errorf("parse flags: %w", parseErr)
			}

			got, ok := InvalidValueAttributionOf(parseErr)
			if ok != test.wantOK || got != test.want {
				t.Fatalf("InvalidValueAttributionOf() = (%+v, %v), want (%+v, %v)", got, ok, test.want, test.wantOK)
			}
		})
	}
}

func TestInvalidValueAttributionOfRejectsOtherErrors(t *testing.T) {
	if got, ok := InvalidValueAttributionOf(errors.New("flag needs an argument: --limit")); ok {
		t.Fatalf("InvalidValueAttributionOf() = (%+v, true), want no attribution", got)
	}
}
