// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/extension/platform"
)

type businessArgs struct {
	ChatID string `flag:"chat-id" schema:"required;minLength=1" doc:"chat identifier"`
}

type businessData struct {
	ChatID string `json:"chat_id" schema:"required" doc:"chat identifier"`
}

func businessCommand(name string, executed *bool) command.Command {
	return command.Define(command.Definition[businessArgs, businessData]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: name, Description: "Business command", Risk: command.RiskRead,
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

func TestWithCommandSetsInIsolatedProcesses(t *testing.T) {
	for _, scenario := range []string{"official", "mount", "atomic", "governance", "surface"} {
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
	default:
		t.Fatalf("unknown scenario %q", scenario)
	}
}
