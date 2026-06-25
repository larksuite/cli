// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestValidateDocsV2OnlyIgnoresAPIVersionValues(t *testing.T) {
	for _, apiVersion := range []string{"", "v1", "v2", "v0", "legacy"} {
		t.Run(apiVersion, func(t *testing.T) {
			runtime := docsV2OnlyTestRuntime(t, apiVersion, false)
			if err := validateDocsV2Only(runtime, "+update", []docsLegacyFlag{{Name: "mode", Replacement: "use --command"}}); err != nil {
				t.Fatalf("validateDocsV2Only(%q) error = %v, want nil", apiVersion, err)
			}
		})
	}
}

func TestValidateDocsV2OnlyRejectsChangedLegacyFlags(t *testing.T) {
	runtime := docsV2OnlyTestRuntime(t, "", true)
	err := validateDocsV2Only(runtime, "+update", []docsLegacyFlag{{Name: "mode", Replacement: "use --command"}})
	if err == nil {
		t.Fatal("expected changed legacy flag to be rejected")
	}
	for _, want := range []string{
		"the old v1 interface has been shut down",
		"legacy v1 flag(s) --mode are no longer supported",
		"--mode -> use --command",
		"lark-cli skills read lark-doc references/lark-doc-update.md",
		"lark-cli skills read lark-doc references/lark-doc-xml.md",
		"lark-cli skills read lark-doc references/lark-doc-md.md",
		"MUST NOT grep/open local SKILL.md files",
		"lark-cli docs +update --help",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

func docsV2OnlyTestRuntime(t *testing.T, apiVersion string, legacyMode bool) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "+update"}
	cmd.Flags().String("api-version", "", "")
	cmd.Flags().String("mode", "", "")
	if apiVersion != "" {
		if err := cmd.Flags().Set("api-version", apiVersion); err != nil {
			t.Fatalf("set api-version: %v", err)
		}
	}
	if legacyMode {
		if err := cmd.Flags().Set("mode", "overwrite"); err != nil {
			t.Fatalf("set mode: %v", err)
		}
	}
	return common.TestNewRuntimeContext(cmd, nil)
}
