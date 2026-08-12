// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"

	defaultaffordance "github.com/larksuite/cli/affordance"
	"github.com/larksuite/cli/cmd"
	"github.com/larksuite/cli/extension/command"
	defaultskills "github.com/larksuite/cli/skills"

	_ "github.com/larksuite/cli/extension/credential/env"
)

type readArgs struct {
	ID string `flag:"id" schema:"required;minLength=1" doc:"resource identifier"`
}

type readData struct {
	ID string `json:"id" schema:"required" doc:"resource identifier"`
}

var readCommand = command.Define(command.Definition[readArgs, readData]{
	Metadata: command.CommandMetadata{
		Service: command.DomainIm, Command: "+wrapper-read", Description: "Read one wrapper resource", Risk: command.RiskRead,
		Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
			command.IdentityUser: {RequiredScopes: []string{"im:chat:read"}},
		}},
	},
	Hooks: command.Hooks[readArgs, readData]{
		DryRunE: func(_ context.Context, _ command.CommandContext, args *readArgs) (*command.DryRun, error) {
			return command.Preview(command.GET("/open-apis/im/v1/chats/" + args.ID)), nil
		},
		Execute: func(ctx context.Context, commandContext command.CommandContext, args *readArgs) (command.Result[readData], error) {
			data, err := command.CallJSON[readData](ctx, commandContext, command.GET("/open-apis/im/v1/chats/"+args.ID))
			if err != nil {
				return command.Result[readData]{}, err
			}
			return command.Success(data), nil
		},
	},
})

func main() {
	cmd.SetEmbeddedSkillContent(defaultskills.DefaultFS())
	cmd.SetEmbeddedAffordanceContent(defaultaffordance.DefaultFS())
	os.Exit(cmd.ExecuteWithOptions(
		cmd.WithCommandSets(command.Set{
			Domain:   command.ExtendDomain(command.DomainIm),
			Commands: []command.Command{readCommand},
		}),
		cmd.WithoutPlugins(),
		cmd.WithoutStrictMode(),
		cmd.WithoutServiceCommands(),
	))
}
