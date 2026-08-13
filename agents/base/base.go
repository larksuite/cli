// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package base exposes the fixed Base assistant through the provider-neutral
// agents SPI. Adapter-specific wire details intentionally stay in this package.
package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
)

// Base Agent scope is checked before every real API operation. Both the Open
// Platform app and the user token must grant it.
const baseAgentExecuteScope = "base:agent:execute"

const baseTokenParamDescription = "Base app token. If the input is a Base-related URL, first run `lark-cli base +url-resolve --url \"<url>\" --as user` and pass the returned `base_token`; if the input is already a Base token, pass it directly. Never pass a URL or Wiki token as `base_token`."

// adapterAgentID is deliberately private: callers always use base:assistant,
// and a future Adapter-side ID change is isolated to this mapping.
const adapterAgentID = "assistant"

type sendParams struct {
	BaseToken     string `param:"base_token"`
	ActiveTableID string `param:"active_table_id"`
}

type baseTokenParams struct {
	BaseToken string `param:"base_token"`
}

type getTaskParams struct {
	BaseToken string `param:"base_token"`
	ContextID string `param:"context_id"`
}

type listTasksParams struct {
	BaseToken string `param:"base_token"`
	State     string `param:"state"`
}

type listContextsParams struct {
	BaseToken string `param:"base_token"`
	Status    string `param:"status"`
}

func baseTokenParam() []iagents.CardParam {
	return []iagents.CardParam{{Name: "base_token", Required: true, Desc: baseTokenParamDescription}}
}

func getTaskParamList() []iagents.CardParam {
	return []iagents.CardParam{
		{Name: "base_token", Required: true, Desc: baseTokenParamDescription},
		{Name: "context_id", Desc: "Optional context override used to retrieve the task's message snapshot"},
	}
}

func sendParamList() []iagents.CardParam {
	return []iagents.CardParam{
		{Name: "base_token", Required: true, Desc: baseTokenParamDescription},
		{Name: "active_table_id", Desc: "Optional active table for automatic routing"},
	}
}

func listTasksParamList() []iagents.CardParam {
	return []iagents.CardParam{
		{Name: "base_token", Required: true, Desc: baseTokenParamDescription},
		{Name: "state", Enum: []string{"running", "done", "failed"}, Desc: "Adapter task state"},
	}
}

func listContextsParamList() []iagents.CardParam {
	return []iagents.CardParam{
		{Name: "base_token", Required: true, Desc: baseTokenParamDescription},
		{Name: "status", Desc: "Adapter context status"},
	}
}

var assistantSpec = iagents.AgentSpec{
	ID:          "assistant",
	Name:        "Base Assistant",
	Description: "Handles multi-component Base construction and restructuring, plus user-facing data retrieval and analysis. Use Base CLI shortcuts for a single atomic edit or record create, update, or delete.",
	Skills: []iagents.CardSkill{
		{
			ID:   "base_assistant",
			Name: "Build and analyze a Base",
			Examples: []string{
				"Create an order table from the provided field list",
				"Build a sales management workflow and dashboard",
				"Analyze recent sales trends and explain the main changes",
			},
		},
	},
	Send:          iagents.SendOp{Params: sendParamList(), Handler: send},
	GetTask:       iagents.TaskGetOp{Params: getTaskParamList(), Handler: getTask},
	ListTasks:     iagents.TaskListOp{Params: listTasksParamList(), Handler: listTasks},
	CancelTask:    iagents.TaskCancelOp{Params: baseTokenParam(), Handler: cancelTask},
	ListContexts:  iagents.ContextListOp{Params: listContextsParamList(), Handler: listContexts},
	GetContext:    iagents.ContextGetOp{Params: baseTokenParam(), Handler: getContext},
	DeleteContext: iagents.ContextDeleteOp{Params: baseTokenParam(), Handler: deleteContext},
	FileInput:     false,
	InputRequired: true,
}

// Provider returns the single offline-discoverable Base assistant.
func Provider() iagents.Provider {
	return iagents.Provider{
		Scheme:        "base",
		Label:         "Base Assistant",
		AgentIDSource: "Use the fixed agent reference base:assistant",
		Identities:    []iagents.IdentitySpec{{Type: iagents.IdentityUser, Scopes: []string{baseAgentExecuteScope}}},
		Catalog:       []iagents.AgentSpec{assistantSpec},
	}
}

func validateSendRuntime(rt iagents.Runtime, in iagents.SendInput) error {
	if rt.IsBot() {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"base:assistant currently supports only user identity").WithParam("--as").
			WithHint("run the command with --as user")
	}
	if len(in.Files) > 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"base:assistant does not support file input").WithParam("--file")
	}
	// The unified contract lets an answer carry a message-level --text remark,
	// but the Base bridge maps CLI answers straight onto the pending clarification
	// card and has no channel for a separate remark: SaveUserMessage rewrites the
	// user message from the card answers alone. Reject the combination explicitly
	// (a Base-only deviation from the framework contract) instead of silently
	// dropping the remark. The backend enforces the same rule for direct HTTP.
	if len(in.Answers) > 0 && strings.TrimSpace(in.Text) != "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"base:assistant does not support a remark when answering input_required").WithParam("--text").
			WithHint("answer the question group with --answer only, then send --text as a follow-up message on the same task")
	}
	return nil
}

func send(ctx context.Context, rt iagents.Runtime, in iagents.SendInput) (*iagents.AgentTask, error) {
	if err := validateSendRuntime(rt, in); err != nil {
		return nil, err
	}
	return sendMessage(ctx, rt, in)
}
