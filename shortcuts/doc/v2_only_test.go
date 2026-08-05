// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/skillref"
	"github.com/larksuite/cli/internal/surface"
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
	runtime := docsV2OnlyTestRuntimeWithSkills(t, true, nil, "lark-doc")
	err := validateDocsV2Only(runtime, "+update", []docsLegacyFlag{{Name: "mode", Replacement: "use --command"}})
	if err == nil {
		t.Fatal("expected changed legacy flag to be rejected")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T, want typed problem", err)
	}
	if got, want := problem.Message, "docs +update is v2-only; the old v1 interface has been shut down; legacy v1 flag(s) --mode are no longer supported; --mode -> use --command"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want *errs.ValidationError", err)
	}
	if got, want := validationErr.Param, "--mode"; got != want {
		t.Fatalf("param = %q, want %q", got, want)
	}
	if got, want := problem.Hint, "run `lark-cli docs +update --help` for the latest command flags; read the version-matched embedded guidance before retrying: `lark-cli skills read lark-doc`, `lark-cli skills read lark-doc/references/lark-doc-update.md`, `lark-cli skills read lark-doc/references/lark-doc-xml.md`, `lark-cli skills read lark-doc/references/lark-doc-md.md`; do not inspect another local SKILL.md copy"; got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
}

func TestValidateDocsV2OnlyOmitsConcealedSkillsReadRecovery(t *testing.T) {
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandSkillsRead: surface.CommandConcealed,
	})
	runtime := docsV2OnlyTestRuntimeWithSkills(t, true, plan, "lark-doc")
	err := validateDocsV2Only(runtime, "+update", []docsLegacyFlag{{Name: "mode", Replacement: "use --command"}})
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T, want typed problem", err)
	}
	if got, want := problem.Hint, "run `lark-cli docs +update --help` for the latest command flags"; got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
}

func TestValidateDocsV2OnlyUsesRemappedSkillReferences(t *testing.T) {
	runtime := docsV2OnlyTestRuntimeWithSkills(t, true, nil, "acme-doc")
	err := validateDocsV2Only(runtime, "+update", []docsLegacyFlag{{Name: "mode", Replacement: "use --command"}})
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T, want typed problem", err)
	}
	for _, want := range []string{
		"lark-cli skills read acme-doc",
		"lark-cli skills read acme-doc/references/lark-doc-update.md",
		"lark-cli skills read acme-doc/references/lark-doc-xml.md",
		"lark-cli skills read acme-doc/references/lark-doc-md.md",
	} {
		if !strings.Contains(problem.Hint, want) {
			t.Fatalf("hint missing %q: %s", want, problem.Hint)
		}
	}
	if strings.Contains(problem.Hint, "skills read lark-doc") {
		t.Fatalf("hint retained canonical skill name after remap: %s", problem.Hint)
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

func docsV2OnlyTestRuntimeWithSkills(t *testing.T, legacyMode bool, plan *surface.Plan, runtimeSkill string) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "+update"}
	cmd.Flags().String("api-version", "", "")
	cmd.Flags().String("mode", "", "")
	if legacyMode {
		if err := cmd.Flags().Set("mode", "overwrite"); err != nil {
			t.Fatalf("set mode: %v", err)
		}
	}
	cmdmeta.SetAffordanceRef(cmd, "docs", "+update")
	affordance.SetSource(fstest.MapFS{
		"docs.md": {Data: []byte(`# docs
> skill: lark-doc

## +update
### Skills
- lark-doc/references/lark-doc-update.md
- lark-doc/references/lark-doc-xml.md
- lark-doc/references/lark-doc-md.md
`)},
	})
	t.Cleanup(func() { affordance.SetSource(nil) })

	content := fstest.MapFS{
		runtimeSkill + "/SKILL.md":                      {Data: []byte("skill")},
		runtimeSkill + "/references/lark-doc-update.md": {Data: []byte("update")},
		runtimeSkill + "/references/lark-doc-xml.md":    {Data: []byte("xml")},
		runtimeSkill + "/references/lark-doc-md.md":     {Data: []byte("markdown")},
	}
	var mappings []skillref.Mapping
	if runtimeSkill != "lark-doc" {
		from, err := skillref.Parse("lark-doc")
		if err != nil {
			t.Fatal(err)
		}
		to, err := skillref.Parse(runtimeSkill)
		if err != nil {
			t.Fatal(err)
		}
		mappings = append(mappings, skillref.Mapping{From: from, To: to})
	}
	resolver, err := skillref.New(content, mappings)
	if err != nil {
		t.Fatalf("skillref.New(): %v", err)
	}
	factory := &cmdutil.Factory{
		SkillContent:    content,
		SkillReferences: resolver,
		Recovery: recovery.NewProjector(func() *surface.Plan {
			return plan
		}),
	}
	return common.TestNewRuntimeContextForAPI(context.Background(), cmd, &core.CliConfig{}, factory, core.AsUser)
}
