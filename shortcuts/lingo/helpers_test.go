// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lingo

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

// lingoTestConfig builds an isolated CliConfig per test so cached state
// (tokens, profile) cannot leak across parallel tests.
func lingoTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	suffix := strings.NewReplacer("/", "-", " ", "-").Replace(strings.ToLower(t.Name()))
	return &core.CliConfig{
		AppID:     "test-lingo-" + suffix,
		AppSecret: "secret-lingo-" + suffix,
		Brand:     core.BrandFeishu,
	}
}

// runLingoShortcut mounts the given shortcut under a fresh "lingo" parent
// and executes it with the given args.
func runLingoShortcut(t *testing.T, s common.Shortcut, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "lingo"}
	s.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

// decodeEnvelope parses the success envelope and returns the data field.
func decodeEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()
	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode output: %v\nraw=%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	if data == nil {
		t.Fatalf("missing data in output envelope: %#v", envelope)
	}
	return data
}
