// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/command"
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
			Tips: []string{"Uses the distribution-specific chat policy."},
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{"im:chat:read"}},
			}},
		},
		Hooks: command.Hooks[businessArgs, businessData]{
			DryRun: func(_ context.Context, _ command.CommandContext, args *businessArgs) *command.DryRun {
				return command.Preview(command.GET("/open-apis/im/v1/chats/" + args.ChatID))
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
	for _, scenario := range []string{"official", "mount", "atomic", "governance"} {
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
		option := WithCommandSets(command.Set{Domain: command.ExtendDomain(command.DomainIM), Commands: commands})
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
		if !strings.Contains(help.String(), "distribution-specific chat policy") {
			t.Fatalf("business tip is missing from help:\n%s", help.String())
		}
	case "atomic":
		set := command.Set{Domain: command.ExtendDomain(command.DomainIM), Commands: []command.Command{
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
				Domain:   command.ExtendDomain(command.DomainIM),
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
	default:
		t.Fatalf("unknown scenario %q", scenario)
	}
}
