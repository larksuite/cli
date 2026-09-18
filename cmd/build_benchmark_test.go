// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

func BenchmarkBuildForArgs(b *testing.B) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "None", args: []string{"--version"}},
		{name: "TargetDrive", args: []string{"drive", "files", "list"}},
		{name: "Full", args: []string{"--help"}},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := buildRootForArgs(
					context.Background(),
					cmdutil.InvocationContext{},
					tt.args,
					WithoutPlugins(),
				); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkBuildWithInvocationArgsDrive(b *testing.B) {
	args := []string{"drive", "files", "list"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Build(
			context.Background(),
			cmdutil.InvocationContext{},
			WithInvocationArgs(args),
			WithoutPlugins(),
		)
	}
}
