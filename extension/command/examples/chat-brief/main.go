// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Command chat-brief is a runnable lark-cli distribution that contributes
// two business commands to the existing im domain via WithCommandSets:
//
//	im +chat-brief   single read: Validate, shared DryRun request, CallJSON
//	im +chat-brief-list  list read: Page[T] Data auto-installs --page-all/--page-limit/--page-delay
//
// Unlike the test fixture under testdata/wrapper, this example keeps the
// real distribution shape: plugins, strict mode, and service commands all
// stay enabled; only the command sets are added.
//
// Build & run:
//
//	cd extension/command/examples/chat-brief
//	go build -o chat-brief-cli .
//	./chat-brief-cli im +chat-brief --help                     # Tips + flags from tags
//	./chat-brief-cli im +chat-brief --chat-id oc_xxx --dry-run # offline request preview
//	./chat-brief-cli im +chat-brief --chat-id oc_xxx           # real call (requires auth login)
//	./chat-brief-cli im +chat-brief-list --page-all --page-limit 2   # framework pagination flags
//	./chat-brief-cli auth login --domain im                    # aggregates business scopes too
package main

import (
	"context"
	"os"
	"strings"

	defaultaffordance "github.com/larksuite/cli/affordance"
	"github.com/larksuite/cli/cmd"
	"github.com/larksuite/cli/extension/command"
	defaultskills "github.com/larksuite/cli/skills"

	_ "github.com/larksuite/cli/extension/credential/env" // activate env credential provider
)

type chatBriefArgs struct {
	ChatID string `flag:"chat-id" schema:"required;minLength=1" doc:"chat ID (oc_xxx)"`
	IDType string `flag:"user-id-type" schema:"optional;default=\"open_id\";enum=open_id|union_id|user_id" doc:"user ID type"`
}

type chatBriefData struct {
	ChatID string `json:"chat_id" schema:"required" doc:"chat ID"`
	Name   string `json:"name" schema:"required" doc:"chat name"`
	Owner  string `json:"owner_id" schema:"required" doc:"owner open ID"`
}

// chatWire holds the OpenAPI response fields this command projects.
type chatWire struct {
	Name  string `json:"name"`
	Owner string `json:"owner_id"`
}

// chatRequest is shared by Execute and DryRun so the preview cannot drift
// from the real call. User input concatenated into the path goes through
// PathSegment.
func chatRequest(args *chatBriefArgs) command.Request {
	return command.GET("/open-apis/im/v1/chats/" + command.PathSegment(args.ChatID)).
		Set("user_id_type", args.IDType)
}

var chatBrief = command.Define(command.Definition[chatBriefArgs, chatBriefData]{
	Metadata: command.CommandMetadata{
		Service:     "im",
		Command:     "+chat-brief",
		Description: "Get a concise chat projection",
		Risk:        command.RiskRead,
		Tips: []string{
			"Example: chat-brief-cli im +chat-brief --chat-id oc_xxx",
		},
		Authorization: command.AuthorizationDefinition{
			Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{"im:chat:read"}},
			},
		},
	},
	Hooks: command.Hooks[chatBriefArgs, chatBriefData]{
		Validate: func(_ context.Context, _ command.CommandContext, args *chatBriefArgs) error {
			if !strings.HasPrefix(args.ChatID, "oc_") {
				return command.ValidationErrorf("--chat-id must start with oc_")
			}
			return nil
		},
		DryRun: func(_ context.Context, _ command.CommandContext, args *chatBriefArgs) *command.DryRun {
			return command.Preview(chatRequest(args))
		},
		Execute: func(ctx context.Context, c command.CommandContext, args *chatBriefArgs) (command.Result[chatBriefData], error) {
			chat, err := command.CallJSON[chatWire](ctx, c, chatRequest(args))
			if err != nil {
				return command.Result[chatBriefData]{}, err
			}
			return command.Success(chatBriefData{
				ChatID: args.ChatID,
				Name:   chat.Name,
				Owner:  chat.Owner,
			}), nil
		},
	},
})

type chatListArgs struct {
	PageSize int `flag:"page-size" schema:"optional;default=20;minimum=1;maximum=100" doc:"items per page"`
}

type chatItem struct {
	ChatID string `json:"chat_id" schema:"required" doc:"chat ID"`
	Name   string `json:"name" schema:"required" doc:"chat name"`
}

func chatListRequest(args *chatListArgs) command.Request {
	return command.GET("/open-apis/im/v1/chats").Set("page_size", args.PageSize)
}

// chatList declares Page[T] as its Data, so the compiler installs the
// framework pagination flags; the Args stay free of paging fields.
var chatList = command.Define(command.Definition[chatListArgs, command.Page[chatItem]]{
	Metadata: command.CommandMetadata{
		Service:     "im",
		Command:     "+chat-brief-list",
		Description: "List visible chats",
		Risk:        command.RiskRead,
		Tips: []string{
			"Example: chat-brief-cli im +chat-brief-list --page-all --page-limit 2",
		},
		Authorization: command.AuthorizationDefinition{
			Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{"im:chat:read"}},
			},
		},
	},
	Hooks: command.Hooks[chatListArgs, command.Page[chatItem]]{
		DryRun: func(_ context.Context, _ command.CommandContext, args *chatListArgs) *command.DryRun {
			return command.Preview(chatListRequest(args))
		},
		Execute: func(ctx context.Context, c command.CommandContext, args *chatListArgs) (command.Result[command.Page[chatItem]], error) {
			page, err := command.CollectPages[chatItem](ctx, c, chatListRequest(args))
			if err != nil {
				return command.Result[command.Page[chatItem]]{}, err
			}
			return command.Success(page), nil
		},
	},
})

func main() {
	// A wrapper main has no implicit embedded content; reuse the repository
	// defaults so official command guidance stays available.
	cmd.SetEmbeddedSkillContent(defaultskills.DefaultFS())
	cmd.SetEmbeddedAffordanceContent(defaultaffordance.DefaultFS())
	os.Exit(cmd.ExecuteWithOptions(
		cmd.WithCommandSets(command.Set{
			Domain:   command.ExtendDomain(command.DomainIm),
			Commands: []command.Command{chatBrief, chatList},
		}),
	))
}
