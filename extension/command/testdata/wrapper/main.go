// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"

	defaultaffordance "github.com/larksuite/cli/affordance"
	"github.com/larksuite/cli/cmd"
	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/extension/download"
	defaultskills "github.com/larksuite/cli/skills"

	_ "github.com/larksuite/cli/extension/credential/env"
)

type readArgs struct {
	ID string `flag:"id" schema:"required;minLength=1" doc:"resource identifier"`
}

type readData struct {
	ID string `json:"id" schema:"required" doc:"resource identifier"`
}

// readRequest is shared by DryRun and Execute so the preview cannot drift from
// the real call. User input concatenated into the path goes through PathSegment.
func readRequest(args *readArgs) command.Request {
	return command.GET("/open-apis/im/v1/chats/" + command.PathSegment(args.ID))
}

var readCommand = command.Define(command.Definition[readArgs, readData]{
	Metadata: command.CommandMetadata{
		Service: command.DomainIm, Command: "+wrapper-read", Description: "Read one wrapper resource", Risk: command.RiskRead,
		Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
			command.IdentityUser: {RequiredScopes: []string{"im:chat:read"}},
		}},
	},
	Hooks: command.Hooks[readArgs, readData]{
		DryRun: func(_ context.Context, _ command.CommandContext, args *readArgs) *command.DryRun {
			return command.NewDryRun(readRequest(args))
		},
		Execute: func(ctx context.Context, commandContext command.CommandContext, args *readArgs) (command.Result[readData], error) {
			data, err := command.CallJSON[readData](ctx, commandContext, readRequest(args))
			if err != nil {
				return command.Result[readData]{}, err
			}
			return command.Success(data), nil
		},
	},
})

type backupArgs struct {
	FileToken string `flag:"file-token" schema:"required;minLength=1" doc:"file token"`
	Output    string `flag:"output" schema:"required;minLength=1" doc:"logical output name"`
}

type backupDescriptor struct {
	DownloadURL string `json:"download_url"`
}

type backupData struct {
	FileToken string           `json:"file_token" schema:"required" doc:"backed-up file token"`
	Artifact  command.Artifact `json:"artifact" schema:"required" doc:"saved backup artifact"`
}

func backupDescriptorRequest(args *backupArgs) command.Request {
	return command.GET("/open-apis/drive/v1/files/" + command.PathSegment(args.FileToken) + "/download_url")
}

func backupTarget(args *backupArgs) command.FileTarget {
	return command.FileTarget{Name: args.Output}
}

var backupCommand = command.Define(command.Definition[backupArgs, backupData]{
	Metadata: command.CommandMetadata{
		Service: command.DomainDrive, Command: "+wrapper-backup", Description: "Resolve and save one file backup", Risk: command.RiskWrite,
		Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
			command.IdentityUser: {RequiredScopes: []string{"drive:drive.metadata:readonly", "drive:file:download"}},
		}},
	},
	Hooks: command.Hooks[backupArgs, backupData]{
		DryRun: func(_ context.Context, _ command.CommandContext, args *backupArgs) *command.DryRun {
			return command.NewDryRun(backupDescriptorRequest(args).Desc("resolve a short-lived download URL")).
				File(backupTarget(args).Intent("file content returned by the resolved URL"))
		},
		Execute: func(ctx context.Context, commandContext command.CommandContext, args *backupArgs) (command.Result[backupData], error) {
			descriptor, err := command.CallJSON[backupDescriptor](ctx, commandContext, backupDescriptorRequest(args))
			if err != nil {
				return command.Result[backupData]{}, err
			}
			artifact, err := command.DownloadURL(ctx, commandContext, descriptor.DownloadURL, backupTarget(args), command.DownloadOptions{
				Representation: download.Immutable,
			})
			if err != nil {
				return command.Result[backupData]{}, err
			}
			return command.Success(backupData{FileToken: args.FileToken, Artifact: artifact}), nil
		},
	},
})

func main() {
	cmd.SetEmbeddedSkillContent(defaultskills.DefaultFS())
	cmd.SetEmbeddedAffordanceContent(defaultaffordance.DefaultFS())
	os.Exit(cmd.ExecuteWithOptions(
		cmd.WithCommandSets(
			command.Set{Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{readCommand}},
			command.Set{Domain: command.ExtendDomain(command.DomainDrive), Commands: []command.Command{backupCommand}},
		),
		cmd.WithoutPlugins(),
		cmd.WithoutStrictMode(),
		cmd.WithoutServiceCommands(),
	))
}
