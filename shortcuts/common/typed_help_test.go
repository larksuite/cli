// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
)

type helpDemoTarget struct {
	Chat *string `flag:"chat-id"`
	User *string `flag:"user-id"`
}

func (helpDemoTarget) OneOf() {}

type helpDemoArgs struct {
	Target helpDemoTarget
	Idemp  string `flag:"idempotency-key"`
}

func TestTypedHelp_RendersSections(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cmd := &cobra.Command{Use: "+demo", Short: "demo command"}
	cmdutil.SetRisk(cmd, "high-risk-write")
	cmdutil.SetTips(cmd, []string{"call carefully"})
	specs, err := walkArgs(reflect.TypeOf(&helpDemoArgs{}))
	if err != nil {
		t.Fatalf("walkArgs: %v", err)
	}
	cmd.SetHelpFunc(buildTypedHelp(specs, []HelpExample{{Title: "demo", Cmd: "--chat-id oc_x"}}))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.HelpFunc()(cmd, nil)
	out := buf.String()
	for _, section := range []string{"CHOOSE ONE", "OPTIONAL", "EXAMPLES", "Risk:", "Tips:"} {
		if !strings.Contains(out, section) {
			t.Errorf("help missing %q section; got:\n%s", section, out)
		}
	}
}

// helpReqDefaultsArgs covers two cases the renderer previously botched:
//
//  1. A required flag (--limit) should land under "REQUIRED:" instead of
//     being silently glued into "OPTIONAL:".
//  2. A flag with default (--page-size) should display (default "20").
type helpReqDefaultsArgs struct {
	Limit    string `flag:"limit" required:"true" desc:"max items"`
	PageSize string `flag:"page-size" default:"20" desc:"page size"`
	Verbose  string `flag:"verbose" desc:"output mode"`
}

func TestTypedHelp_RequiredSectionAndDefaults(t *testing.T) {
	cmd := &cobra.Command{Use: "+demo", Short: "demo command"}
	specs, err := walkArgs(reflect.TypeOf(&helpReqDefaultsArgs{}))
	if err != nil {
		t.Fatalf("walkArgs: %v", err)
	}
	cmd.SetHelpFunc(buildTypedHelp(specs, nil))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.HelpFunc()(cmd, nil)
	out := buf.String()

	if !strings.Contains(out, "REQUIRED:") {
		t.Errorf("expected REQUIRED: section, got:\n%s", out)
	}
	// --limit must be under REQUIRED, NOT under OPTIONAL.
	reqIdx := strings.Index(out, "REQUIRED:")
	optIdx := strings.Index(out, "OPTIONAL:")
	limitIdx := strings.Index(out, "--limit")
	if reqIdx < 0 || optIdx < 0 || limitIdx < 0 {
		t.Fatalf("layout markers missing: REQUIRED@%d OPTIONAL@%d --limit@%d\n%s", reqIdx, optIdx, limitIdx, out)
	}
	if !(reqIdx < limitIdx && limitIdx < optIdx) {
		t.Errorf("--limit should appear under REQUIRED before OPTIONAL; got:\n%s", out)
	}

	// Default value must be surfaced for --page-size.
	if !strings.Contains(out, `(default "20")`) {
		t.Errorf("expected default value rendering for --page-size; got:\n%s", out)
	}

	// Plain flag without default or required tag must NOT carry a (default …) suffix.
	verboseLine := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "--verbose") {
			verboseLine = line
			break
		}
	}
	if verboseLine == "" {
		t.Fatalf("expected --verbose flag to be rendered; got:\n%s", out)
	}
	if strings.Contains(verboseLine, "(default") {
		t.Errorf("--verbose has no default, should not carry default suffix: %q", verboseLine)
	}
}
