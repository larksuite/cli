// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

func TestTypedClassifyContextPreservesCommandPath(t *testing.T) {
	root := &cobra.Command{Use: "lark"}
	service := &cobra.Command{Use: "fixture"}
	command := &cobra.Command{Use: "+typed"}
	root.AddCommand(service)
	service.AddCommand(command)

	ctx := typedCommandContext{runtime: &RuntimeContext{
		Cmd:        command,
		Config:     &core.CliConfig{AppID: "cli_test", Brand: core.BrandFeishu},
		resolvedAs: core.AsBot,
	}}
	classify := typedClassifyContext(ctx)
	if classify.AppID != "cli_test" || classify.Identity != string(core.AsBot) || classify.LarkCmd != "fixture +typed" {
		t.Fatalf("classify context = %#v", classify)
	}
}
