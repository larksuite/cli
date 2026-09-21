// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/spf13/cobra"
)

type businessArgs struct {
	ChatID string `flag:"chat-id" schema:"required;minLength=1" doc:"chat identifier"`
}

type businessData struct {
	ChatID string `json:"chat_id" schema:"required" doc:"chat identifier"`
}

func businessCommand(name string, executed *bool) command.Command {
	return businessCommandIn(command.DomainIm, name, executed)
}

func businessCommandIn(service command.DomainName, name string, executed *bool) command.Command {
	return command.Define(command.Definition[businessArgs, businessData]{
		Metadata: command.CommandMetadata{
			Service: service, Command: name, Description: "Business command", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{"im:chat:read"}},
			}},
		},
		Hooks: command.Hooks[businessArgs, businessData]{
			DryRun: func(_ context.Context, _ command.CommandContext, args *businessArgs) *command.DryRun {
				return command.NewDryRun(command.GET("/open-apis/im/v1/chats/" + args.ChatID))
			},
			Execute: func(_ context.Context, _ command.CommandContext, args *businessArgs) (command.Result[businessData], error) {
				if executed != nil {
					*executed = true
				}
				return command.Success(businessData{ChatID: args.ChatID}), nil
			},
		},
	})
}

// acmeHR declares a domain lark-cli does not ship and mounts commands under it.
func acmeHR(commands ...command.Command) command.Set {
	return command.Set{
		Domain:   command.NewDomain(command.DomainSpec{Name: "acme-hr", Description: "Acme HR records"}),
		Commands: commands,
	}
}

func TestWithCommandSetsInIsolatedProcesses(t *testing.T) {
	for _, scenario := range []string{
		"official", "mount", "atomic", "governance", "surface",
		"newdomain", "newdomain-conflict", "newdomain-governance",
	} {
		t.Run(scenario, func(t *testing.T) {
			process := exec.Command(os.Args[0], "-test.run=^TestCommandSetSubprocess$", "-test.v")
			process.Env = append(os.Environ(), "LARK_CLI_COMMAND_SET_SCENARIO="+scenario)
			output, err := process.CombinedOutput()
			if err != nil {
				t.Fatalf("scenario %s failed: %v\n%s", scenario, err, output)
			}
		})
	}
}

func TestFailedBuildDoesNotAffectNextBuild(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	platform.Register(&failingPlugin{
		name: "command-set-failure",
		caps: platform.Capabilities{FailurePolicy: platform.FailClosed},
		err:  errors.New("install failure after command assembly"),
	})

	failed := Build(context.Background(), buildInvocationForTest(t),
		WithCommandSets(command.Set{
			Domain:   command.ExtendDomain(command.DomainIm),
			Commands: []command.Command{businessCommand("+business-failed-build", nil)},
		}),
		WithoutStrictMode(), WithoutServiceCommands(),
	)
	if findCommand(failed, "im +business-failed-build") == nil || failed.PersistentPreRunE == nil {
		t.Fatal("failed build did not reach the post-mount plugin guard")
	}

	platform.ResetForTesting()
	clean := Build(context.Background(), buildInvocationForTest(t), WithoutPlugins(), WithoutStrictMode(), WithoutServiceCommands())
	if findCommand(clean, "im +business-failed-build") != nil {
		t.Fatal("clean build contains a command from an earlier failed build")
	}
}

func TestBuildForArgsRecognizesExternalOnlyShortcutDomain(t *testing.T) {
	tmpHome(t)
	declaration := command.Define(command.Definition[businessArgs, businessData]{
		Metadata: command.CommandMetadata{
			Service: command.DomainApproval, Command: "+business-target", Description: "Targeted business command", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{"approval:approval:readonly"}},
			}},
		},
		Hooks: command.Hooks[businessArgs, businessData]{
			Execute: func(_ context.Context, _ command.CommandContext, args *businessArgs) (command.Result[businessData], error) {
				return command.Success(businessData{ChatID: args.ChatID}), nil
			},
		},
	})

	root, err := buildRootForArgs(context.Background(), buildInvocationForTest(t), []string{"approval", "+business-target"},
		WithCommandSets(command.Set{
			Domain:   command.ExtendDomain(command.DomainApproval),
			Commands: []command.Command{declaration},
		}),
		WithoutPlugins(), WithoutStrictMode(), WithoutServiceCommands(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if findCommand(root, "approval +business-target") == nil {
		t.Fatal("target-aware assembly omitted the external approval command")
	}
	if findCommand(root, "im") != nil {
		t.Fatal("target-aware assembly mounted unrelated im shortcuts")
	}
}

// Target-aware assembly routes on the new domain's stub, which is also the
// command its description must already be on: expansion has no registry entry
// to read a description from.
func TestBuildForArgsRoutesToNewDomain(t *testing.T) {
	tmpHome(t)
	root, err := buildRootForArgs(context.Background(), buildInvocationForTest(t), []string{"acme-hr", "+employee-get"},
		WithCommandSets(acmeHR(businessCommandIn("acme-hr", "+employee-get", nil))),
		WithoutPlugins(), WithoutStrictMode(), WithoutServiceCommands(),
	)
	if err != nil {
		t.Fatal(err)
	}
	domain := findCommand(root, "acme-hr")
	if domain == nil {
		t.Fatal("target-aware assembly omitted the new domain")
	}
	if domain.Short != "Acme HR records" {
		t.Fatalf("routed new domain summary = %q, want the declared description", domain.Short)
	}
	if findCommand(root, "acme-hr +employee-get") == nil {
		t.Fatal("target-aware assembly omitted the new domain command")
	}
	if findCommand(root, "im") != nil {
		t.Fatal("target-aware assembly mounted unrelated im shortcuts")
	}
}

// A declared domain may not take a root command's alias either: Cobra routes
// aliases exactly like names. Root commands carry no aliases today, so a
// synthetic tree pins the rule.
func TestRejectShadowingDomainsCoversRootAliases(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	root.AddCommand(&cobra.Command{Use: "whoami", Aliases: []string{"me"}})
	if err := rejectShadowingDomains(root, map[string]string{"me": "Acme"}); err == nil || !strings.Contains(err.Error(), `"whoami"`) {
		t.Fatalf("alias shadowing error = %v", err)
	}
	if err := rejectShadowingDomains(root, nil); err != nil {
		t.Fatalf("a contribution without declared domains must pass: %v", err)
	}
}

// No declared domain may take the name of a root command a standard build
// mounts, which holds only while each is mounted before the tree check. The
// standard tree is the census, so a root command added in a later assembly step
// fails here instead of leaving a contribution to hang under it.
func TestNewDomainCannotShadowAnyRootCommand(t *testing.T) {
	tmpHome(t)
	standard := Build(context.Background(), buildInvocationForTest(t), WithoutPlugins(), WithoutStrictMode(), WithoutServiceCommands())
	for _, framework := range standard.Commands() {
		if isLarkDomain(framework) {
			continue
		}
		for _, name := range append([]string{framework.Name()}, framework.Aliases...) {
			shadow := command.DomainName(name)
			root := Build(context.Background(), buildInvocationForTest(t),
				WithCommandSets(command.Set{
					Domain:   command.NewDomain(command.DomainSpec{Name: shadow, Description: "Shadow"}),
					Commands: []command.Command{businessCommandIn(shadow, "+employee-shadow", nil)},
				}),
				WithoutPlugins(), WithoutStrictMode(), WithoutServiceCommands(),
			)
			if root.PersistentPreRunE == nil {
				t.Errorf("NewDomain(%q) built without a startup guard", name)
				continue
			}
			if err := root.PersistentPreRunE(root, nil); err == nil || !strings.Contains(err.Error(), "shadows the lark-cli") {
				t.Errorf("NewDomain(%q) startup error = %v, want the shadowing rejection", name, err)
			}
		}
	}
}

// loginDomainVocabulary reads the domains auth login offers, the observable
// projection of the shortcut snapshot auth was built from.
func loginDomainVocabulary(t *testing.T, root *cobra.Command) []string {
	t.Helper()
	login := findCommand(root, "auth login")
	if login == nil {
		t.Fatal("auth login is missing")
	}
	_, list, ok := strings.Cut(login.Flag("domain").Usage, "available: ")
	if !ok {
		t.Fatalf("auth login --domain usage = %q", login.Flag("domain").Usage)
	}
	return strings.Split(list, ", ")
}

func TestCommandSetSubprocess(t *testing.T) {
	scenario := os.Getenv("LARK_CLI_COMMAND_SET_SCENARIO")
	if scenario == "" {
		t.Skip("subprocess helper")
	}
	tmpHome(t)
	switch scenario {
	case "official":
		root := Build(context.Background(), buildInvocationForTest(t), WithoutPlugins(), WithoutStrictMode(), WithoutServiceCommands())
		if findCommand(root, "im +business-official") != nil {
			t.Fatal("official command tree contains an external command")
		}
	case "mount":
		commands := []command.Command{businessCommand("+business-captured", nil)}
		option := WithCommandSets(command.Set{Domain: command.ExtendDomain(command.DomainIm), Commands: commands})
		commands[0] = businessCommand("+business-mutated", nil)
		root := Build(context.Background(), buildInvocationForTest(t), option, WithoutPlugins(), WithoutStrictMode(), WithoutServiceCommands())
		leaf := findCommand(root, "im +business-captured")
		if leaf == nil {
			t.Fatal("captured business command is missing")
		}
		if findCommand(root, "im +business-mutated") != nil {
			t.Fatal("mutation after WithCommandSets changed the command tree")
		}
		leaf.InitDefaultHelpFlag()
		var help strings.Builder
		leaf.SetOut(&help)
		leaf.SetErr(&help)
		if err := leaf.Help(); err != nil {
			t.Fatal(err)
		}
		if rendered := help.String(); !strings.Contains(rendered, "Risk: read") {
			t.Fatalf("business metadata is missing from help:\n%s", rendered)
		}
	case "atomic":
		set := command.Set{Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{
			businessCommand("+business-valid", nil), businessCommand("+business-valid", nil),
		}}
		root := Build(context.Background(), buildInvocationForTest(t), WithCommandSets(set), WithoutPlugins(), WithoutStrictMode(), WithoutServiceCommands())
		if findCommand(root, "im +business-valid") != nil {
			t.Fatal("part of an invalid contribution was mounted")
		}
		if root.PersistentPreRunE == nil {
			t.Fatal("invalid contribution did not install a startup guard")
		}
		err := root.PersistentPreRunE(root, nil)
		var validation *errs.ValidationError
		if !errors.As(err, &validation) || validation.Subtype != errs.SubtypeFailedPrecondition {
			t.Fatalf("startup error = %T %v", err, err)
		}
		if !strings.Contains(err.Error(), "conflicts") {
			t.Fatalf("startup error omitted command conflict: %v", err)
		}
	case "governance":
		executed := false
		registerRestriction(t, []string{"im/+business-governed"}, nil)
		root := Build(context.Background(), buildInvocationForTest(t),
			WithCommandSets(command.Set{
				Domain:   command.ExtendDomain(command.DomainIm),
				Commands: []command.Command{businessCommand("+business-governed", &executed)},
			}),
			WithoutStrictMode(), WithoutServiceCommands(),
		)
		leaf := findCommand(root, "im +business-governed")
		if leaf == nil || leaf.RunE == nil {
			t.Fatal("governed business command is missing")
		}
		err := leaf.RunE(leaf, nil)
		var validation *errs.ValidationError
		if !errors.As(err, &validation) || validation.Subtype != errs.SubtypeFailedPrecondition {
			t.Fatalf("governance error = %T %v", err, err)
		}
		if executed {
			t.Fatal("governance denial reached business Execute")
		}
	case "surface":
		var stdout, stderr bytes.Buffer
		root := Build(context.Background(), buildInvocationForTest(t),
			WithIO(strings.NewReader(""), &stdout, &stderr),
			WithCommandSets(command.Set{
				Domain:   command.ExtendDomain(command.DomainIm),
				Commands: []command.Command{businessCommand("+business-surface", nil)},
			}),
			WithoutPlugins(), WithoutStrictMode(), WithoutServiceCommands(),
		)
		root.SetArgs([]string{"__complete", "im", "+"})
		if _, err := root.ExecuteC(); err != nil {
			t.Fatalf("complete external command: %v\nstderr: %s", err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "+business-surface") {
			t.Fatalf("external command is missing from shell completion: %s", stdout.String())
		}
		// The schema command serves the generated API catalog only; mounted
		// shortcuts (external commands included) must stay invisible to it.
		stdout.Reset()
		stderr.Reset()
		root.SetArgs([]string{"__complete", "schema", "im", "+business-"})
		if _, err := root.ExecuteC(); err != nil {
			t.Fatalf("complete schema path: %v\nstderr: %s", err, stderr.String())
		}
		if strings.Contains(stdout.String(), "+business-surface") {
			t.Fatalf("schema completion leaked an external command: %s", stdout.String())
		}
		stdout.Reset()
		stderr.Reset()
		root.SetArgs([]string{"schema", "im", "+business-surface"})
		if _, err := root.ExecuteC(); err == nil {
			t.Fatalf("schema resolved an external command: %s", stdout.String())
		}
		if strings.Contains(stdout.String(), "inputSchema") {
			t.Fatalf("schema rendered an external command contract: %s", stdout.String())
		}
	case "newdomain":
		var stdout, stderr bytes.Buffer
		root := Build(context.Background(), buildInvocationForTest(t),
			WithIO(strings.NewReader(""), &stdout, &stderr),
			WithCommandSets(acmeHR(businessCommandIn("acme-hr", "+employee-get", nil))),
			WithoutPlugins(), WithoutStrictMode(), WithoutServiceCommands(),
		)
		domain := findCommand(root, "acme-hr")
		if domain == nil {
			t.Fatal("new domain is not mounted as a root command")
		}
		if domain.Short != "Acme HR records" {
			t.Fatalf("new domain summary = %q, want the declared description", domain.Short)
		}
		if domain.GroupID != groupDomains {
			t.Fatalf("new domain root-help group = %q, want %q", domain.GroupID, groupDomains)
		}
		leaf := findCommand(root, "acme-hr +employee-get")
		if leaf == nil {
			t.Fatal("new domain command is missing")
		}
		// Policy path globs, ByDomain selectors and domain help all read this tag.
		if got := cmdmeta.Domain(leaf); got != "acme-hr" {
			t.Fatalf("new domain command is tagged %q, want acme-hr", got)
		}
		root.SetArgs([]string{"__complete", "acme-hr", "+"})
		if _, err := root.ExecuteC(); err != nil {
			t.Fatalf("complete new domain command: %v\nstderr: %s", err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "+employee-get") {
			t.Fatalf("new domain command is missing from shell completion: %s", stdout.String())
		}
	case "newdomain-conflict":
		// Each name fails the whole contribution: nothing mounts, the framework
		// command keeps its place, and the startup guard names the reason. "auth"
		// is the case that matters most: without the check, mountDomainStubs
		// would treat the existing auth command as a shared root and hang the
		// business command under it. "help" exists only once Cobra executes, and
		// "slide" would capture the alias that routes to slides.
		for _, test := range []struct {
			name, want string
			// rebuilt marks a name the tree check rejects: auth was built from
			// the contribution's snapshot by then, so the rebuilt auth command
			// must not offer the name.
			rebuilt bool
		}{
			{name: "auth", want: `shadows the lark-cli "auth" command`, rebuilt: true},
			{name: "help", want: `shadows the lark-cli "help" command`},
			{name: "all", want: "auth login --domain"},
			{name: "im", want: "use ExtendDomain"},
			{name: "slide", want: "alias of the built-in domain"},
		} {
			shadow := command.DomainName(test.name)
			root := Build(context.Background(), buildInvocationForTest(t),
				WithCommandSets(command.Set{
					Domain:   command.NewDomain(command.DomainSpec{Name: shadow, Description: "Shadow"}),
					Commands: []command.Command{businessCommandIn(shadow, "+employee-shadow", nil)},
				}),
				WithoutPlugins(), WithoutStrictMode(), WithoutServiceCommands(),
			)
			if findCommand(root, test.name+" +employee-shadow") != nil {
				t.Fatalf("%s: a rejected contribution mounted a command under %q", test.name, test.name)
			}
			if findCommand(root, "auth login") == nil {
				t.Fatalf("%s: the framework auth command is missing after the rejection", test.name)
			}
			if test.rebuilt && slices.Contains(loginDomainVocabulary(t, root), test.name) {
				t.Fatalf("%s: rejected domain survives in auth login --domain", test.name)
			}
			if root.PersistentPreRunE == nil {
				t.Fatalf("%s: invalid contribution did not install a startup guard", test.name)
			}
			err := root.PersistentPreRunE(root, nil)
			var validation *errs.ValidationError
			if !errors.As(err, &validation) || validation.Subtype != errs.SubtypeFailedPrecondition {
				t.Fatalf("%s: startup error = %T %v", test.name, err, err)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("%s: startup error = %v, want %q", test.name, err, test.want)
			}
		}
	case "newdomain-governance":
		executed := false
		registerRestriction(t, []string{"acme-hr/**"}, nil)
		root := Build(context.Background(), buildInvocationForTest(t),
			WithCommandSets(acmeHR(businessCommandIn("acme-hr", "+employee-governed", &executed))),
			WithoutStrictMode(), WithoutServiceCommands(),
		)
		leaf := findCommand(root, "acme-hr +employee-governed")
		if leaf == nil || leaf.RunE == nil {
			t.Fatal("governed new domain command is missing")
		}
		err := leaf.RunE(leaf, nil)
		var validation *errs.ValidationError
		if !errors.As(err, &validation) || validation.Subtype != errs.SubtypeFailedPrecondition {
			t.Fatalf("governance error = %T %v", err, err)
		}
		if executed {
			t.Fatal("governance denial reached business Execute")
		}
	default:
		t.Fatalf("unknown scenario %q", scenario)
	}
}
