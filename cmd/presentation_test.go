// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	configpkg "github.com/larksuite/cli/internal/config"
	identitypkg "github.com/larksuite/cli/internal/identity"
	"github.com/larksuite/cli/internal/workspace"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/deprecation"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/skillscheck"
	"github.com/larksuite/cli/internal/surface"
	"github.com/larksuite/cli/internal/update"
)

func registerRestriction(t *testing.T, deny []string, configure func(*platform.Builder) *platform.Builder) {
	t.Helper()
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	builder := platform.NewPlugin("acme", "1.0").
		Restrict(&platform.Rule{Deny: deny})
	if configure != nil {
		builder = configure(builder)
	}
	platform.Register(builder.MustBuild())
}

func TestBuildInternalRestrictDefaultPreservesLegacyContract(t *testing.T) {
	tmpHome(t)
	registerRestriction(t, []string{"skills/read"}, nil)

	runtime, root, _ := buildInternal(context.Background(), buildInvocationForTest(t))
	leaf := findByPath(root, "skills/read")
	if leaf == nil {
		t.Fatal("skills/read not found")
	}
	if got := runtime.surface.State(surface.CommandSkillsRead); got != surface.CommandDeniedVisible {
		t.Fatalf("surface state = %v, want denied-visible", got)
	}
	if _, projected := unavailableHelpMessage(leaf); projected {
		t.Fatal("legacy Restrict unexpectedly received concealment presentation")
	}

	err := leaf.RunE(leaf, nil)
	var validation *errs.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("RunE error = %T %v, want ValidationError", err, err)
	}
	if validation.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q, want failed_precondition", validation.Subtype)
	}
	if !strings.Contains(validation.Hint, "source plugin:acme") ||
		!strings.Contains(validation.Hint, "reason_code") {
		t.Errorf("legacy policy metadata missing from hint: %q", validation.Hint)
	}
	if flag := leaf.Flags().Lookup("json"); flag == nil || flag.Hidden {
		t.Errorf("legacy Restrict must preserve local flag presentation; flag=%+v", flag)
	}

	var help bytes.Buffer
	root.SetOut(&help)
	root.SetErr(&help)
	if err := root.Help(); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"Lark domains:", "Agent tooling:", "CLI management:"} {
		if !strings.Contains(help.String(), title) {
			t.Errorf("default root help lost group %q", title)
		}
	}
}

func TestBuildInternalConcealmentIsExplicitAndKeepsDenialAsCause(t *testing.T) {
	tmpHome(t)
	registerRestriction(t, []string{"skills/read"}, nil)

	runtime, root, _ := buildInternal(
		context.Background(),
		buildInvocationForTest(t),
		ConcealRestrictedCommands(UnavailableMessage("not shipped by acme")),
	)
	leaf := findByPath(root, "skills/read")
	if got := runtime.surface.State(surface.CommandSkillsRead); got != surface.CommandConcealed {
		t.Fatalf("surface state = %v, want concealed", got)
	}

	err := leaf.RunE(leaf, nil)
	var validation *errs.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("RunE error = %T %v, want ValidationError", err, err)
	}
	if validation.Subtype != errs.SubtypeCommandUnavailable ||
		validation.Message != "not shipped by acme" || validation.Hint != "" {
		t.Errorf("concealed error = %+v", validation)
	}
	var denied *platform.CommandDeniedError
	if !errors.As(err, &denied) || denied.Path != "skills/read" ||
		denied.PolicySource != "plugin:acme" {
		t.Errorf("enforcement cause not preserved: %T %+v", err, denied)
	}

	if flag := leaf.Flags().Lookup("json"); flag == nil || !flag.Hidden {
		t.Errorf("concealed command must hide owned flags; flag=%+v", flag)
	}
	if flag := root.PersistentFlags().Lookup("profile"); flag == nil || flag.Hidden {
		t.Errorf("concealing a leaf must not mutate inherited global flags; flag=%+v", flag)
	}
	if args, _ := leaf.ValidArgsFunction(leaf, nil, ""); len(args) != 0 {
		t.Errorf("concealed command completed positionals: %v", args)
	}

	help := findByPath(root, "help")
	if help == nil || help.RunE == nil {
		t.Fatal("concealment-specific help command not installed")
	}
	err = help.RunE(help, []string{"skills", "read"})
	if !errors.As(err, &validation) || validation.Subtype != errs.SubtypeCommandUnavailable {
		t.Errorf("help on concealed command = %v, want command_unavailable", err)
	}

	active := cmdpolicy.GetActive()
	if active == nil || active.DeniedByPath["skills/read"].PolicySource != "plugin:acme" {
		t.Fatalf("presentation overwrote enforcement snapshot: %+v", active)
	}
}

func TestDistributionPresentationNeverConcealsYAMLPolicy(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	leaf := &cobra.Command{Use: "probe", RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(leaf)
	denial := cmdpolicy.Denial{
		Layer:        cmdpolicy.LayerPolicy,
		PolicySource: "yaml:/tmp/policy.yml",
		ReasonCode:   "command_denylisted",
		Reason:       "denied by user policy",
	}
	denied := map[string]cmdpolicy.Denial{"probe": denial}
	cmdpolicy.Apply(root, denied)

	plan, concealed := applyDistributionPresentation(
		root,
		restrictionPresentationConfig{enabled: true},
		denied,
	)
	if concealed {
		t.Fatal("user-owned YAML denial must not be projected as absent")
	}
	if got := plan.State("probe"); got != surface.CommandDeniedVisible {
		t.Fatalf("surface state = %v, want denied-visible", got)
	}
	err := leaf.RunE(leaf, nil)
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || validation.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("YAML denial changed by distribution presentation: %v", err)
	}
}

func TestRootGroupsFollowSurfaceConcealmentNotLegacyHiddenState(t *testing.T) {
	newRoot := func() *cobra.Command {
		root := &cobra.Command{Use: "lark-cli"}
		child := &cobra.Command{
			Use:     "skills",
			GroupID: groupTooling,
			RunE:    func(*cobra.Command, []string) error { return nil },
		}
		root.AddCommand(child)
		return root
	}

	yamlRoot := newRoot()
	yamlChild := findByPath(yamlRoot, "skills")
	yamlChild.Hidden = true
	finalizeRootCommandGroups(yamlRoot, surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandSkills: surface.CommandDeniedVisible,
	}))
	if len(yamlRoot.Groups()) != 1 || yamlRoot.Groups()[0].ID != groupTooling {
		t.Fatalf("legacy/YAML hidden command removed its group: %+v", yamlRoot.Groups())
	}

	concealedRoot := newRoot()
	finalizeRootCommandGroups(concealedRoot, surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandSkills: surface.CommandConcealed,
	}))
	if len(concealedRoot.Groups()) != 0 {
		t.Fatalf("concealed-only group remained visible: %+v", concealedRoot.Groups())
	}
	if got := findByPath(concealedRoot, "skills").GroupID; got != "" {
		t.Fatalf("concealed child retained undefined GroupID %q", got)
	}
}

func TestPresentationDropsRootSkillsFooterWithSkillsRead(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	root.SetUsageTemplate(rootUsageTemplate)
	applyPresentationAffordances(root, surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandSkillsRead: surface.CommandConcealed,
	}))
	if strings.Contains(root.UsageTemplate(), "Skills setup (one-time, humans)") {
		t.Fatalf("concealed skills/read left the root skills footer:\n%s", root.UsageTemplate())
	}
}

func TestPresentationProjectsEveryFrameworkOwnedRootHelpTarget(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli", Long: rootLong}
	root.SetUsageTemplate(rootUsageTemplate)
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		rootHelpAPI:            surface.CommandConcealed,
		surface.CommandSchema:  surface.CommandConcealed,
		rootHelpCalendarAgenda: surface.CommandConcealed,
		rootHelpMailList:       surface.CommandConcealed,
	})

	applyPresentationAffordances(root, plan)

	for _, dead := range []string{
		"lark-cli api ",
		"lark-cli schema ",
		"lark-cli calendar +agenda",
		"lark-cli mail user_mailbox.messages list",
	} {
		if strings.Contains(root.Long, dead) || strings.Contains(root.UsageTemplate(), dead) {
			t.Errorf("concealed root-help target %q survived:\nLong:\n%s\nTemplate:\n%s",
				dead, root.Long, root.UsageTemplate())
		}
	}
	if !strings.Contains(root.Long, "Browse commands:") ||
		!strings.Contains(root.UsageTemplate(), "lark-cli <command>") {
		t.Fatalf("target-independent root guidance was removed:\nLong:\n%s\nTemplate:\n%s",
			root.Long, root.UsageTemplate())
	}
}

func TestFrameworkOwnedRootHelpTargetsExistInDefaultTree(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	_, root, _ := buildInternal(
		context.Background(),
		buildInvocationForTest(t),
		WithoutPlugins(),
	)
	var fragments []rootHelpFragment
	for _, section := range rootLongSections {
		fragments = append(fragments, section.fragments...)
	}
	fragments = append(fragments, rootUsageSynopsis...)
	for _, fragment := range fragments {
		if fragment.target == "" {
			continue
		}
		if command := findByPath(root, string(fragment.target)); command == nil {
			t.Errorf("root-help target %q does not resolve in the default command tree", fragment.target)
		}
	}
}

func TestPresentationKeepsDefaultRootHelpByteStable(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli", Long: rootLong}
	root.SetUsageTemplate(rootUsageTemplate)
	wantLong, wantUsage := root.Long, root.UsageTemplate()

	applyPresentationAffordances(root, nil)

	if root.Long != wantLong {
		t.Fatalf("default root Long changed:\nwant:\n%s\n\ngot:\n%s", wantLong, root.Long)
	}
	if root.UsageTemplate() != wantUsage {
		t.Fatalf("default root usage template changed:\nwant:\n%s\n\ngot:\n%s", wantUsage, root.UsageTemplate())
	}
}

func TestHelpRejectsDescendantOfConcealedParent(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	parent := &cobra.Command{Use: "apps"}
	child := &cobra.Command{Use: "+db-execute", RunE: func(*cobra.Command, []string) error { return nil }}
	parent.AddCommand(child)
	root.AddCommand(parent)
	installUnavailableProjection(parent, "apps", cmdpolicy.Denial{}, "not shipped")
	installHelpCommand(root)

	help := findByPath(root, "help")
	if help == nil || help.RunE == nil {
		t.Fatal("help command not installed")
	}
	err := help.RunE(help, []string{"apps", "+db-execute"})
	var validation *errs.ValidationError
	if !errors.As(err, &validation) ||
		validation.Subtype != errs.SubtypeCommandUnavailable ||
		validation.Message != "not shipped" {
		t.Fatalf("help descendant error = %#v, want command_unavailable inherited from parent", err)
	}
}

func TestHidePolicyDiagnosticsIsHostPresentationOnly(t *testing.T) {
	tmpHome(t)
	registerRestriction(t, []string{"config/**"}, nil)

	runtime, root, _ := buildInternal(
		context.Background(),
		buildInvocationForTest(t),
		ConcealRestrictedCommands(HidePolicyDiagnostics()),
	)
	for _, path := range []string{
		"config",
		"config/policy",
		"config/policy/show",
		"config/plugins",
		"config/plugins/show",
	} {
		cmd := findByPath(root, path)
		if cmd == nil || cmd.RunE == nil {
			t.Fatalf("%s missing unavailable projection", path)
		}
		err := cmd.RunE(cmd, nil)
		var validation *errs.ValidationError
		if !errors.As(err, &validation) ||
			validation.Subtype != errs.SubtypeCommandUnavailable {
			t.Errorf("%s error = %v, want command_unavailable", path, err)
		}
		if !runtime.surface.IsConcealed(surface.CommandID(path)) {
			t.Errorf("%s not recorded in build-local surface", path)
		}
	}

	// Synthetic presentation decisions must not be reported as policy facts.
	active := cmdpolicy.GetActive()
	if active == nil {
		t.Fatal("missing active enforcement policy")
	}
	if _, exists := active.DeniedByPath["config/policy/show"]; exists {
		t.Errorf("presentation-only diagnostic concealment leaked into ActivePolicy: %+v", active)
	}
}

func TestConcealedBuildOmitsEmptyRootGroup(t *testing.T) {
	tmpHome(t)
	registerRestriction(t, []string{
		"auth", "auth/**",
		"config", "config/**",
		"profile", "profile/**",
		"doctor",
		"update",
	}, nil)

	_, root, _ := buildInternal(
		context.Background(),
		buildInvocationForTest(t),
		ConcealRestrictedCommands(HidePolicyDiagnostics()),
	)
	var help bytes.Buffer
	root.SetOut(&help)
	root.SetErr(&help)
	if err := root.Help(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help.String(), "CLI management:") {
		t.Errorf("empty management group leaked into help:\n%s", help.String())
	}
	if !strings.Contains(help.String(), "Agent tooling:") {
		t.Errorf("non-empty tooling group disappeared:\n%s", help.String())
	}

	// Cobra's Execute path validates GroupID definitions before parsing flags.
	// Calling root.Help directly does not exercise this invariant.
	help.Reset()
	root.SetOut(&help)
	root.SetErr(&help)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("concealed root Execute --help: %v", err)
	}
	if strings.Contains(help.String(), "CLI management:") {
		t.Errorf("empty management group leaked through Execute:\n%s", help.String())
	}
}

func TestRecoveryRenderingUsesExactBuildLocalSurfaceAndDoesNotMutate(t *testing.T) {
	tmpHome(t)
	previousWorkspace := workspace.CurrentWorkspace()
	workspace.SetCurrentWorkspace(workspace.WorkspaceLocal)
	t.Cleanup(func() { workspace.SetCurrentWorkspace(previousWorkspace) })

	registerRestriction(t, []string{"config/init"}, nil)
	concealedRuntime, _, _ := buildInternal(
		context.Background(),
		buildInvocationForTest(t),
		ConcealRestrictedCommands(),
	)

	platform.ResetForTesting()
	defaultRuntime, _, _ := buildInternal(
		context.Background(),
		buildInvocationForTest(t),
		WithoutPlugins(),
	)

	if concealedRuntime.surface.CanReference(surface.CommandConfigInit) {
		t.Fatal("config/init should be concealed")
	}
	if !concealedRuntime.surface.CanReference(surface.CommandConfigStrictMode) {
		t.Fatal("exact leaf concealment incorrectly removed config/strict-mode")
	}

	original := configpkg.NotConfiguredError()
	originalProblem, ok := errs.ProblemOf(original)
	if !ok || originalProblem.Hint == "" {
		t.Fatalf("invalid test error: %v", original)
	}
	wantHint := originalProblem.Hint

	concealed := concealedRuntime.recovery.Render(original)
	concealedProblem, _ := errs.ProblemOf(concealed)
	if strings.Contains(concealedProblem.Hint, "config init") ||
		!strings.Contains(concealedProblem.Hint, "configure this distribution") {
		t.Errorf("concealed tree did not use target-free recovery fallback: %q", concealedProblem.Hint)
	}
	if originalProblem.Hint != wantHint {
		t.Fatalf("rendering mutated source hint: %q -> %q", wantHint, originalProblem.Hint)
	}

	visible := defaultRuntime.recovery.Render(original)
	visibleProblem, _ := errs.ProblemOf(visible)
	if visibleProblem.Hint != wantHint {
		t.Errorf("default tree lost recovery after second Build: %q", visibleProblem.Hint)
	}
}

func TestRecoveryRenderingKeepsExplicitProfilesIsolatedAcrossBuilds(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	alphaInvocation := buildInvocationForTest(t)
	alphaInvocation.Profile = "alpha"
	alphaRuntime, _, _ := buildInternal(context.Background(), alphaInvocation, WithoutPlugins())

	betaInvocation := buildInvocationForTest(t)
	betaInvocation.Profile = "beta"
	betaRuntime, _, _ := buildInternal(context.Background(), betaInvocation, WithoutPlugins())

	source := recovery.Attach(
		errs.NewPermissionError(errs.SubtypeMissingScope, "missing scope").
			WithMissingScopes("docx:document").
			WithIdentity("user"),
		recovery.UserAuthorization("docx:document"),
	)
	sourceProblem, _ := errs.ProblemOf(source)
	if strings.Contains(sourceProblem.Hint, "--profile") {
		t.Fatalf("producer hint unexpectedly owns an invocation profile: %q", sourceProblem.Hint)
	}

	assertProfile := func(name string, runtime *buildRuntime, want, unwanted string) {
		t.Helper()
		rendered := runtime.recovery.Render(source)
		problem, ok := errs.ProblemOf(rendered)
		if !ok {
			t.Fatalf("%s rendered error = %T, want typed error", name, rendered)
		}
		for _, command := range []string{
			"lark-cli auth login --profile='" + want + "' --scope \"docx:document\" --no-wait --json",
			"lark-cli auth login --profile='" + want + "' --device-code <device_code>",
		} {
			if !strings.Contains(problem.Hint, command) {
				t.Errorf("%s recovery missing %q: %q", name, command, problem.Hint)
			}
		}
		if strings.Contains(problem.Hint, "--profile='"+unwanted+"'") {
			t.Errorf("%s recovery leaked profile %q: %q", name, unwanted, problem.Hint)
		}
	}

	assertProfile("alpha before beta", alphaRuntime, "alpha", "beta")
	assertProfile("beta", betaRuntime, "beta", "alpha")
	assertProfile("alpha after beta", alphaRuntime, "alpha", "beta")
	if strings.Contains(sourceProblem.Hint, "--profile") {
		t.Fatalf("build-local rendering mutated source hint: %q", sourceProblem.Hint)
	}
}

func TestConcurrentBuildsKeepIndependentSurfacePlans(t *testing.T) {
	tmpHome(t)
	registerRestriction(t, []string{"config/init"}, nil)
	inv := buildInvocationForTest(t)

	const pairs = 4
	type result struct {
		concealed bool
		state     surface.CommandState
	}
	results := make(chan result, pairs*2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < pairs; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			runtime, _, _ := buildInternal(
				context.Background(),
				inv,
				ConcealRestrictedCommands(),
			)
			results <- result{concealed: true, state: runtime.surface.State(surface.CommandConfigInit)}
		}()
		go func() {
			defer wg.Done()
			<-start
			runtime, _, _ := buildInternal(
				context.Background(),
				inv,
				WithoutPlugins(),
			)
			results <- result{state: runtime.surface.State(surface.CommandConfigInit)}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	for got := range results {
		want := surface.CommandAvailable
		if got.concealed {
			want = surface.CommandConcealed
		}
		if got.state != want {
			t.Errorf("concealed=%v state=%v, want %v", got.concealed, got.state, want)
		}
	}
}

func TestUpdateAffordancesDisappearWithoutDroppingIndependentRecovery(t *testing.T) {
	update.SetPending(&update.UpdateInfo{Current: "1.0.0", Latest: "2.0.0"})
	skillscheck.SetPending(&skillscheck.StaleNotice{Current: "1.0.0", Target: "2.0.0"})
	deprecation.SetPending(&deprecation.Notice{
		Command:     "+read",
		Replacement: "+cells-get",
		Skill:       "lark-sheets",
	})
	t.Cleanup(func() {
		update.SetPending(nil)
		skillscheck.SetPending(nil)
		deprecation.SetPending(nil)
	})

	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandUpdate: surface.CommandConcealed,
	})
	got := composePendingNotice(plan)
	if got == nil {
		t.Fatal("independent deprecation recovery was dropped")
	}
	if _, exists := got["update"]; exists {
		t.Errorf("update notice survived concealed update: %+v", got)
	}
	if _, exists := got["skills"]; exists {
		t.Errorf("skills drift notice survived concealed update: %+v", got)
	}
	entry, ok := got["deprecated_command"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing deprecated_command: %+v", got)
	}
	if entry["replacement"] != "+cells-get" || entry["skill"] != "lark-sheets" {
		t.Errorf("independent deprecation fields lost: %+v", entry)
	}
	if _, exists := entry["action"]; exists {
		t.Errorf("unavailable update action survived: %+v", entry)
	}
	if strings.Contains(entry["message"].(string), "lark-cli update") {
		t.Errorf("dead update pointer survived in message: %+v", entry)
	}
}

func TestSetupNoticesDoesNoProviderWorkWhenUpdateIsConcealed(t *testing.T) {
	oldCheck, oldRefresh, oldSkills := checkCachedUpdate, refreshUpdateCache, initializeSkillsCheck
	oldPending := output.PendingNotice
	t.Cleanup(func() {
		checkCachedUpdate, refreshUpdateCache, initializeSkillsCheck = oldCheck, oldRefresh, oldSkills
		output.PendingNotice = oldPending
	})

	var checks, refreshes, skillChecks int
	checkCachedUpdate = func(string) *update.UpdateInfo {
		checks++
		return nil
	}
	refreshUpdateCache = func(string) { refreshes++ }
	initializeSkillsCheck = func(string) { skillChecks++ }

	setupNotices(surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandUpdate: surface.CommandConcealed,
	}))
	if checks != 0 || refreshes != 0 || skillChecks != 0 {
		t.Fatalf("concealed update performed provider work: cache=%d refresh=%d skills=%d",
			checks, refreshes, skillChecks)
	}
}

func TestExecuteProfileBootstrapPreservesDefaultAndDefersOnlyForOptIn(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER", "1")
	t.Setenv("LARKSUITE_CLI_NO_SKILLS_NOTIFIER", "1")

	t.Run("default remains plain exit one", func(t *testing.T) {
		tmpHome(t)
		platform.ResetForTesting()
		t.Cleanup(platform.ResetForTesting)

		code, stdout, stderr := executeWithCapturedOS(t, nil, "--profile")
		if code != 1 || stdout != "" ||
			stderr != "Error: flag needs an argument: --profile\n" {
			t.Fatalf("default --profile: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
	})

	t.Run("opt-in concealed profile is an unknown flag", func(t *testing.T) {
		tmpHome(t)
		registerRestriction(t, []string{"profile", "profile/**"}, nil)

		code, _, stderr := executeWithCapturedOS(
			t,
			[]BuildOption{ConcealRestrictedCommands()},
			"--profile",
		)
		if code != 2 ||
			!strings.Contains(stderr, `"subtype": "invalid_argument"`) ||
			!strings.Contains(stderr, `unknown flag \"--profile\"`) {
			t.Fatalf("concealed --profile: exit=%d stderr=%s", code, stderr)
		}
	})
}

func TestExecuteEnvironmentProfileConcealmentFailsBeforeLifecycle(t *testing.T) {
	tmpHome(t)
	t.Setenv(envvars.CliProfile, "session")
	t.Setenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER", "1")
	t.Setenv("LARKSUITE_CLI_NO_SKILLS_NOTIFIER", "1")

	var startups, shutdowns int
	registerRestriction(t, []string{"profile", "profile/**"}, func(builder *platform.Builder) *platform.Builder {
		return builder.
			On(platform.Startup, "startup", func(context.Context, *platform.LifecycleContext) error {
				startups++
				return nil
			}).
			On(platform.Shutdown, "shutdown", func(context.Context, *platform.LifecycleContext) error {
				shutdowns++
				return nil
			})
	})

	code, stdout, stderr := executeWithCapturedOS(
		t,
		[]BuildOption{ConcealRestrictedCommands()},
		"--version",
	)
	if code != output.ExitValidation || stdout != "" {
		t.Fatalf("environment profile gate: exit=%d stdout=%q stderr=%s", code, stdout, stderr)
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Type    errs.Category `json:"type"`
			Subtype errs.Subtype  `json:"subtype"`
			Message string        `json:"message"`
			Hint    string        `json:"hint"`
			Param   string        `json:"param"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v\n%s", err, stderr)
	}
	if envelope.OK ||
		envelope.Error.Type != errs.CategoryValidation ||
		envelope.Error.Subtype != errs.SubtypeInvalidArgument ||
		envelope.Error.Param != envvars.CliProfile {
		t.Errorf("envelope = %+v, want ok=false validation/invalid_argument param=%s", envelope, envvars.CliProfile)
	}
	if want := `environment variable "` + envvars.CliProfile + `" is not supported by this build`; !strings.Contains(envelope.Error.Message, want) {
		t.Errorf("message = %q, missing %q", envelope.Error.Message, want)
	}
	if want := "remove " + envvars.CliProfile + " from the process environment and retry"; !strings.Contains(envelope.Error.Hint, want) {
		t.Errorf("hint = %q, missing %q", envelope.Error.Hint, want)
	}
	if startups != 0 || shutdowns != 0 {
		t.Fatalf("invalid environment profile emitted lifecycle events: startup=%d shutdown=%d", startups, shutdowns)
	}
}

func TestExecuteWithOptionsAppliesEachBuildOptionOnce(t *testing.T) {
	tmpHome(t)
	t.Setenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER", "1")
	t.Setenv("LARKSUITE_CLI_NO_SKILLS_NOTIFIER", "1")
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	var applied int
	option := BuildOption(func(*buildConfig) { applied++ })
	code, _, stderr := executeWithCapturedOS(t, []BuildOption{option}, "--version")
	if code != 0 {
		t.Fatalf("--version exit=%d stderr=%s", code, stderr)
	}
	if applied != 1 {
		t.Fatalf("BuildOption applied %d times, want exactly once", applied)
	}
}

func TestConcealmentHelpIsOutsideBusinessHooks(t *testing.T) {
	tmpHome(t)
	var observed, wrapped int
	registerRestriction(t, []string{"skills/read"}, func(builder *platform.Builder) *platform.Builder {
		return builder.
			Observer(platform.Before, "observe", platform.All(), func(context.Context, platform.Invocation) {
				observed++
			}).
			Wrap("wrap", platform.All(), func(next platform.Handler) platform.Handler {
				return func(ctx context.Context, inv platform.Invocation) error {
					wrapped++
					return next(ctx, inv)
				}
			})
	})

	_, root, _ := buildInternal(
		context.Background(),
		buildInvocationForTest(t),
		ConcealRestrictedCommands(),
	)
	help := findByPath(root, "help")
	err := help.RunE(help, []string{"skills", "read"})
	var validation *errs.ValidationError
	if !errors.As(err, &validation) ||
		validation.Subtype != errs.SubtypeCommandUnavailable {
		t.Fatalf("help error = %v", err)
	}
	if observed != 0 || wrapped != 0 {
		t.Fatalf("help entered business hooks: observed=%d wrapped=%d", observed, wrapped)
	}
}

func TestWrapperCannotSwallowConcealedCommandEnforcement(t *testing.T) {
	tmpHome(t)
	registerRestriction(t, []string{"skills/read"}, func(builder *platform.Builder) *platform.Builder {
		return builder.Wrap("swallow", platform.All(), func(platform.Handler) platform.Handler {
			return func(context.Context, platform.Invocation) error { return nil }
		})
	})

	_, root, _ := buildInternal(
		context.Background(),
		buildInvocationForTest(t),
		ConcealRestrictedCommands(),
	)
	leaf := findByPath(root, "skills/read")
	err := leaf.RunE(leaf, nil)
	var validation *errs.ValidationError
	if !errors.As(err, &validation) ||
		validation.Subtype != errs.SubtypeCommandUnavailable {
		t.Fatalf("wrapper swallowed denial: %v", err)
	}
}

func TestConcealedCommandLeavesFlagAndPositionalCompletion(t *testing.T) {
	tmpHome(t)
	registerRestriction(t, []string{"skills/read"}, nil)
	_, root, _ := buildInternal(
		context.Background(),
		buildInvocationForTest(t),
		ConcealRestrictedCommands(),
	)

	for _, args := range [][]string{
		{"__complete", "skills", "read", "--"},
		{"__complete", "skills", "read", ""},
	} {
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		_ = root.Execute()
		if strings.Contains(out.String(), "--json") || strings.Contains(out.String(), "lark-") {
			t.Errorf("%v exposed concealed completion:\n%s", args, out.String())
		}
	}
}

func TestApplyStrictStubWinsOverPluginDenial(t *testing.T) {
	root := newTestTree()
	pruneForStrictMode(root, identitypkg.StrictModeBot)
	stub := findCmd(root, "auth", "login")
	if stub == nil {
		t.Fatal("auth/login strict stub missing")
	}

	cmdpolicy.Apply(root, map[string]cmdpolicy.Denial{
		"auth/login": {
			Layer:        cmdpolicy.LayerPolicy,
			PolicySource: "plugin:acme",
		},
	})
	if got := stub.Annotations[cmdpolicy.AnnotationDenialLayer]; got != cmdpolicy.LayerStrictMode {
		t.Fatalf("denial layer = %q, want strict_mode", got)
	}
	err := stub.RunE(stub, nil)
	if err == nil || !strings.Contains(err.Error(), "strict mode") {
		t.Errorf("double-restricted command lost strict-mode error: %v", err)
	}
}

func TestPluginConcealmentProjectsStrictStubWithoutRelabelingEnforcement(t *testing.T) {
	root := newTestTree()
	pruneForStrictMode(root, identitypkg.StrictModeBot)
	stub := findCmd(root, "auth", "login")
	if stub == nil {
		t.Fatal("auth/login strict stub missing")
	}

	denial := cmdpolicy.Denial{
		Layer:        cmdpolicy.LayerPolicy,
		PolicySource: "plugin:acme",
		ReasonCode:   "command_denylisted",
		Reason:       "not shipped by this distribution",
	}
	denied := map[string]cmdpolicy.Denial{"auth/login": denial}
	cmdpolicy.Apply(root, denied)

	plan, concealed := applyDistributionPresentation(
		root,
		restrictionPresentationConfig{enabled: true},
		denied,
	)
	if !concealed {
		t.Fatal("plugin-restricted strict stub was not concealed")
	}
	if got := plan.State(surface.CommandAuthLogin); got != surface.CommandConcealed {
		t.Fatalf("surface state = %v, want concealed", got)
	}
	if got := stub.Annotations[cmdpolicy.AnnotationDenialLayer]; got != cmdpolicy.LayerStrictMode {
		t.Fatalf("presentation relabeled enforcement layer = %q, want strict_mode", got)
	}
	if !stub.Hidden {
		t.Fatal("concealed strict stub remained visible")
	}

	err := stub.RunE(stub, nil)
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || validation.Subtype != errs.SubtypeCommandUnavailable {
		t.Fatalf("concealed strict stub error = %v, want command_unavailable", err)
	}
	var cause *platform.CommandDeniedError
	if !errors.As(err, &cause) || cause.PolicySource != "plugin:acme" {
		t.Fatalf("presentation cause = %#v, want plugin:acme denial", cause)
	}
}

func executeWithCapturedOS(
	t *testing.T,
	opts []BuildOption,
	args ...string,
) (int, string, string) {
	t.Helper()
	oldArgs, oldStdout, oldStderr := os.Args, os.Stdout, os.Stderr
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		os.Args, os.Stdout, os.Stderr = oldArgs, oldStdout, oldStderr
	}
	defer restore()

	os.Args = append([]string{"e2e-cli"}, args...)
	os.Stdout, os.Stderr = stdout, stderr
	code := ExecuteWithOptions(opts...)
	restore()

	if err := stdout.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stderr.Close(); err != nil {
		t.Fatal(err)
	}
	stdoutData, err := os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatal(err)
	}
	stderrData, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	return code, string(stdoutData), string(stderrData)
}
