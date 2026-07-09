// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
)

const jsonShorthandUsage = "shorthand for --format json"

func mountTestShortcut(t *testing.T, s Shortcut) *cobra.Command {
	t.Helper()
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	parent := &cobra.Command{Use: "root"}
	s.Mount(parent, f)
	cmd, _, err := parent.Find([]string{s.Command})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	return cmd
}

// 自定义 format 且 Enum 含 json → 注册简写（本次修复的核心行为）
func TestJSONShorthand_CustomFormatWithJSONEnum_Registered(t *testing.T) {
	cmd := mountTestShortcut(t, Shortcut{
		Service: "mail", Command: "+fake-triage", Description: "x",
		Flags:   []Flag{{Name: "format", Default: "table", Enum: []string{"table", "json", "data"}, Desc: "fmt"}},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	})
	fl := cmd.Flags().Lookup("json")
	if fl == nil {
		t.Fatal("--json not registered for custom-format shortcut whose Enum contains json")
	}
	if fl.Usage != jsonShorthandUsage {
		t.Errorf("usage = %q, want %q", fl.Usage, jsonShorthandUsage)
	}
	// 默认输出格式不被改变
	if def := cmd.Flags().Lookup("format").DefValue; def != "table" {
		t.Errorf("format default = %q, want table", def)
	}
}

// 自定义 format 但 Enum 不含 json → 不注册
func TestJSONShorthand_CustomFormatWithoutJSONEnum_NotRegistered(t *testing.T) {
	cmd := mountTestShortcut(t, Shortcut{
		Service: "x", Command: "+no-json", Description: "x",
		Flags:   []Flag{{Name: "format", Default: "csv", Enum: []string{"csv", "table"}, Desc: "fmt"}},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	})
	if cmd.Flags().Lookup("json") != nil {
		t.Fatal("--json must NOT be registered when format Enum lacks json")
	}
}

// 自定义 format 但无 Enum（现状 triage 形态）→ 不注册（Enum 是判定依据）
func TestJSONShorthand_CustomFormatNoEnum_NotRegistered(t *testing.T) {
	cmd := mountTestShortcut(t, Shortcut{
		Service: "x", Command: "+legacy", Description: "x",
		Flags:   []Flag{{Name: "format", Default: "table", Desc: "fmt"}},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	})
	if cmd.Flags().Lookup("json") != nil {
		t.Fatal("--json must NOT be registered when format has no Enum metadata")
	}
}

// 自声明 json flag（subscribe 的 pretty / record-search 的请求体）→ 不覆盖、不 panic、语义保留
func TestJSONShorthand_SelfDeclaredJSON_Preserved(t *testing.T) {
	cmd := mountTestShortcut(t, Shortcut{
		Service: "event", Command: "+fake-subscribe", Description: "x",
		Flags: []Flag{
			{Name: "json", Type: "bool", Desc: "pretty-print JSON instead of NDJSON"},
		},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	})
	fl := cmd.Flags().Lookup("json")
	if fl == nil {
		t.Fatal("self-declared --json missing")
	}
	if fl.Usage != "pretty-print JSON instead of NDJSON" {
		t.Errorf("self-declared --json usage overwritten: %q", fl.Usage)
	}
}

// parseMounted mounts the shortcut and parses args against the command's FlagSet
// (registration side effects included), without executing RunE.
func parseMounted(t *testing.T, s Shortcut, args []string) *cobra.Command {
	t.Helper()
	cmd := mountTestShortcut(t, s)
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags(%v) error = %v", args, err)
	}
	return cmd
}

func customFormatShortcut() Shortcut {
	return Shortcut{
		Service: "mail", Command: "+fake-triage", Description: "x",
		Flags:   []Flag{{Name: "format", Default: "table", Enum: []string{"table", "json", "data"}, Desc: "fmt"}},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	}
}

// --json 单独使用 → format 归一化为 json
func TestApplyJSONShorthand_JSONAlone_SetsFormatJSON(t *testing.T) {
	s := customFormatShortcut()
	cmd := parseMounted(t, s, []string{"--json"})
	applyJSONShorthand(cmd, &s)
	if got := cmd.Flags().Lookup("format").Value.String(); got != "json" {
		t.Fatalf("format = %q, want json", got)
	}
}

// 显式 --format 优先于 --json 简写：--format table --json → table
func TestApplyJSONShorthand_ExplicitFormatWins(t *testing.T) {
	s := customFormatShortcut()
	cmd := parseMounted(t, s, []string{"--format", "table", "--json"})
	applyJSONShorthand(cmd, &s)
	if got := cmd.Flags().Lookup("format").Value.String(); got != "table" {
		t.Fatalf("format = %q, want table (explicit --format must win)", got)
	}
}

// --format json --json → json（一致，无冲突）
func TestApplyJSONShorthand_ExplicitJSONFormatConsistent(t *testing.T) {
	s := customFormatShortcut()
	cmd := parseMounted(t, s, []string{"--format", "json", "--json"})
	applyJSONShorthand(cmd, &s)
	if got := cmd.Flags().Lookup("format").Value.String(); got != "json" {
		t.Fatalf("format = %q, want json", got)
	}
}

// 均不传 → 默认值不变
func TestApplyJSONShorthand_NoFlags_DefaultUntouched(t *testing.T) {
	s := customFormatShortcut()
	cmd := parseMounted(t, s, nil)
	applyJSONShorthand(cmd, &s)
	if got := cmd.Flags().Lookup("format").Value.String(); got != "table" {
		t.Fatalf("format = %q, want table (default untouched)", got)
	}
}

// 自声明 string 型 --json（record-search 形态：format+json 双声明）→ 归一化跳过
func TestApplyJSONShorthand_SelfDeclaredStringJSON_Skipped(t *testing.T) {
	s := Shortcut{
		Service: "base", Command: "+fake-record-search", Description: "x",
		Flags: []Flag{
			{Name: "format", Default: "markdown", Enum: []string{"markdown", "json"}, Desc: "fmt"},
			{Name: "json", Desc: "request body JSON object"},
		},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	}
	cmd := parseMounted(t, s, []string{"--json", `{"keyword":"Alice"}`})
	applyJSONShorthand(cmd, &s)
	if got := cmd.Flags().Lookup("format").Value.String(); got != "markdown" {
		t.Fatalf("format = %q, want markdown (self-declared json must not normalize)", got)
	}
	if got := cmd.Flags().Lookup("json").Value.String(); got != `{"keyword":"Alice"}` {
		t.Fatalf("request-body --json corrupted: %q", got)
	}
}

// 自声明 bool 型 --json（subscribe 形态：无自定义 format，框架注入 format）→ 归一化跳过
func TestApplyJSONShorthand_SelfDeclaredBoolJSON_Skipped(t *testing.T) {
	s := Shortcut{
		Service: "event", Command: "+fake-subscribe", Description: "x",
		Flags: []Flag{
			{Name: "json", Type: "bool", Desc: "pretty-print JSON instead of NDJSON"},
		},
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	}
	cmd := parseMounted(t, s, []string{"--json"})
	applyJSONShorthand(cmd, &s)
	// 注入的 format 默认即 json；这里断言的是 Changed 状态未被归一化污染
	if cmd.Flags().Changed("format") {
		t.Fatal("normalization must not touch format for shortcuts declaring their own --json")
	}
}

// 无自定义 format（普通命令）→ 注入默认 format + 简写（现状回归）
func TestJSONShorthand_DefaultInjectedFormat_StillRegistered(t *testing.T) {
	cmd := mountTestShortcut(t, Shortcut{
		Service: "im", Command: "+plain", Description: "x",
		Execute: func(context.Context, *RuntimeContext) error { return nil },
	})
	fl := cmd.Flags().Lookup("json")
	if fl == nil {
		t.Fatal("--json missing on default-format shortcut (regression)")
	}
	if fl.Usage != jsonShorthandUsage {
		t.Errorf("usage = %q, want %q", fl.Usage, jsonShorthandUsage)
	}
}
