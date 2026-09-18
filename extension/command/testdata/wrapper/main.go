// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"

	"github.com/larksuite/cli/cmd"
	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/extension/download"

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

type noteArgs struct {
	ChatID  string `flag:"chat-id" schema:"required;minLength=1" doc:"chat ID"`
	Content string `flag:"content" schema:"required;minLength=1" doc:"note body"`
}

type noteData struct {
	MessageID string `json:"message_id" schema:"required" doc:"created message ID"`
}

func noteRequest(args *noteArgs) command.Request {
	return command.POST("/open-apis/im/v1/messages").
		Set("receive_id_type", "chat_id").
		Body(map[string]any{"receive_id": args.ChatID, "content": args.Content})
}

// noteCommand carries a body large enough to hit shell quoting limits, so it
// declares the file and stdin sources: --content @./note.xml and --content -
// both reach Execute already substituted with the content.
var noteCommand = command.Define(command.Definition[noteArgs, noteData]{
	Metadata: command.CommandMetadata{
		Service: command.DomainIm, Command: "+wrapper-note", Description: "Post one note from inline text, @file or stdin", Risk: command.RiskWrite,
		Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
			command.IdentityUser: {RequiredScopes: []string{"im:message:send_as_bot"}},
		}},
	},
	Input: command.InputDefinition{Fields: []command.InputField{{
		Name: "content",
		CLI: command.CLIInput{ValueSources: []command.ValueSource{
			command.SourceFlag, command.SourceFile, command.SourceStdin,
		}},
	}}},
	Hooks: command.Hooks[noteArgs, noteData]{
		DryRun: func(_ context.Context, _ command.CommandContext, args *noteArgs) *command.DryRun {
			return command.NewDryRun(noteRequest(args))
		},
		Execute: func(ctx context.Context, commandContext command.CommandContext, args *noteArgs) (command.Result[noteData], error) {
			data, err := command.CallJSON[noteData](ctx, commandContext, noteRequest(args))
			if err != nil {
				return command.Result[noteData]{}, err
			}
			return command.Success(data), nil
		},
	},
})

func main() {
	os.Exit(cmd.ExecuteWithOptions(
		cmd.WithCommandSets(
			command.Set{Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{readCommand, noteCommand}},
			command.Set{Domain: command.ExtendDomain(command.DomainDrive), Commands: []command.Command{backupCommand}},
		),
		cmd.WithoutPlugins(),
		cmd.WithoutStrictMode(),
		cmd.WithoutServiceCommands(),
	))
}
