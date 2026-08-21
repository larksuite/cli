// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/imcontract"
	"github.com/larksuite/cli/internal/output"
)

// TestOutPartialFailure pins the batch / multi-status contract: the result
// rides on stdout as an ok:false envelope (carrying the full payload), and the
// returned error is the typed partial-failure exit signal (ExitAPI), distinct
// from ErrBare (the silent-exit signal).
func TestOutPartialFailure(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	rt := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+push"}, cfg, f, core.AsUser)

	payload := map[string]interface{}{
		"summary": map[string]interface{}{"uploaded": 1, "failed": 1},
		"items": []map[string]interface{}{
			{"rel_path": "a.txt", "action": "uploaded"},
			{"rel_path": "b.txt", "action": "failed", "error": "boom"},
		},
	}

	err := rt.OutPartialFailure(payload, nil)

	// 1) typed partial-failure exit signal
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected *output.PartialFailureError, got %T: %v", err, err)
	}
	if pfErr.Code != output.ExitAPI {
		t.Errorf("exit code = %d, want %d (ExitAPI)", pfErr.Code, output.ExitAPI)
	}

	// 2) stdout envelope reports ok:false but still carries the full payload
	// (both the succeeded and failed items) — consistent with a success Out().
	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal stdout envelope: %v\nstdout: %s", err, stdout.String())
	}
	if env.OK {
		t.Errorf("ok must be false on partial failure, got ok:true\nstdout: %s", stdout.String())
	}
	items, _ := env.Data["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("both succeeded and failed items must ride on stdout, got %d items\nstdout: %s", len(items), stdout.String())
	}
}

func TestNonIMShortcutSuccessOmitsErrorField(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	rt := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+fetch"}, cfg, f, core.AsUser)

	rt.Out(map[string]any{"document_id": "docx_x"}, nil)

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if _, exists := env["error"]; exists {
		t.Fatalf("successful non-IM shortcut emitted error field: %#v", env)
	}
}

func TestIMContractRequiredResultStopsFalseSuccess(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	rt := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+messages-send"}, cfg, f, core.AsUser)
	contract, _ := imcontract.Lookup("im +messages-send")
	rt.contractSession = imcontract.NewSession(contract)

	rt.Out(map[string]any{"message_id": ""}, nil)

	if stdout.Len() != 0 {
		t.Fatalf("false success reached stdout: %s", stdout.String())
	}
	if output.ExitCodeOf(rt.outputErr) != output.ExitInternal {
		t.Fatalf("exit = %d, want 5; err=%v", output.ExitCodeOf(rt.outputErr), rt.outputErr)
	}
}

func TestIMContractPartialWritesOneResultEnvelope(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	rt := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "urgent_app"}, cfg, f, core.AsBot)
	contract, _ := imcontract.Lookup("im messages urgent_app")
	rt.contractSession = imcontract.NewSession(contract)
	rt.contractSession.ObserveRequest(map[string]any{"user_id_list": []any{"ou_a", "ou_b"}})

	rt.Out(map[string]any{"invalid_user_id_list": []any{"ou_b"}}, nil)

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env["ok"] != false || env["hint"] == "" {
		t.Fatalf("unexpected envelope: %#v", env)
	}
	var partial *output.PartialFailureError
	if !errors.As(rt.outputErr, &partial) || partial.Code != output.ExitAPI {
		t.Fatalf("output error = %T %v", rt.outputErr, rt.outputErr)
	}
}

func TestIMContractPartialPresentationFallbackKeepsCountsWithoutItems(t *testing.T) {
	const secret = "SECRET_MARKER"
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, stdout, stderr, _ := cmdutil.TestFactory(t, cfg)
	rt := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "urgent_app"}, cfg, f, core.AsBot)
	rt.JqExpr = `.data.completion | .status, error("SECRET_MARKER")`
	contract, _ := imcontract.Lookup("im messages urgent_app")
	rt.contractSession = imcontract.NewSession(contract)
	if err := rt.contractSession.ObserveRequest(map[string]any{"user_id_list": []any{"ou_a", secret}}); err != nil {
		t.Fatal(err)
	}

	rt.Out(map[string]any{"invalid_user_id_list": []any{secret}}, nil)

	if output.ExitCodeOf(rt.outputErr) != output.ExitAPI {
		t.Fatalf("output error = %T %v", rt.outputErr, rt.outputErr)
	}
	if !strings.Contains(stderr.String(), "error: jq projection failed after the IM write completed; inspect --jq") {
		t.Fatalf("stderr did not identify the jq failure: %q", stderr.String())
	}
	if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) ||
		strings.Contains(rt.outputErr.Error(), secret) {
		t.Fatalf("fallback leaked item or jq detail: stdout=%q stderr=%q err=%v", stdout.String(), stderr.String(), rt.outputErr)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("fallback is not JSON: %v\n%s", err, stdout.String())
	}
	data, _ := env["data"].(map[string]any)
	completion, _ := data["completion"].(map[string]any)
	if completion["status"] != "partial" ||
		completion["requested_count"] != float64(2) ||
		completion["succeeded_count"] != float64(1) ||
		completion["failed_count"] != float64(1) ||
		completion["pending_count"] != float64(0) ||
		completion["retry_scope"] != "failed_items_only" {
		t.Fatalf("completion = %#v", completion)
	}
	for _, forbidden := range []string{"succeeded_items", "failed_items", "pending_items"} {
		if _, exists := completion[forbidden]; exists {
			t.Fatalf("completion copied %s: %#v", forbidden, completion)
		}
	}
}

func TestIMContractFlagCancelPendingLayerIsPartial(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	rt := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+flag-cancel"}, cfg, f, core.AsUser)
	contract, _ := imcontract.Lookup("im +flag-cancel")
	rt.contractSession = imcontract.NewSession(contract)
	rt.RecordContractFact(imcontract.Fact{Kind: imcontract.FactFlagFeedLayerPending})

	rt.Out(map[string]any{"results": []any{
		map[string]any{"flag_type": "message", "status": "ok"},
	}}, nil)

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Completion imcontract.Completion `json:"completion"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.OK || env.Data.Completion.PendingCount != 1 ||
		len(env.Data.Completion.PendingItems) != 1 || env.Data.Completion.PendingItems[0] != "feed" {
		t.Fatalf("unexpected pending ledger: %#v", env)
	}
}

func TestRunShortcutAppliesIMReplayPolicy(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x", AppSecret: "secret"}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	parent := &cobra.Command{Use: "im"}
	shortcut := Shortcut{
		Service:     "im",
		Command:     "+flag-create",
		Description: "test",
		Risk:        "write",
		AuthTypes:   []string{"bot"},
		Execute: func(_ context.Context, runtime *RuntimeContext) error {
			runtime.RecordContractFact(imcontract.Fact{Kind: imcontract.FactWriteAttempted})
			return errs.NewNetworkError(errs.SubtypeNetworkTransport, "request failed").WithRetryable()
		},
	}
	shortcut.Mount(parent, f)
	parent.SetArgs([]string{"+flag-create", "--as", "bot"})

	err := parent.Execute()
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if p.Retryable || p.Hint != "The write result is unknown. Do not replay the original request." {
		t.Fatalf("problem = %#v", p)
	}
}

func TestRunShortcutAppliesIMReadRetryPolicy(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x", AppSecret: "secret"}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	parent := &cobra.Command{Use: "im"}
	shortcut := Shortcut{
		Service:     "im",
		Command:     "+chat-list",
		Description: "test",
		Risk:        "read",
		AuthTypes:   []string{"bot"},
		Execute: func(_ context.Context, _ *RuntimeContext) error {
			return errs.NewAPIError(errs.SubtypeServerError, "server unavailable")
		},
	}
	shortcut.Mount(parent, f)
	parent.SetArgs([]string{"+chat-list", "--as", "bot"})

	err := parent.Execute()
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if problem.Category != errs.CategoryAPI || problem.Subtype != errs.SubtypeServerError ||
		!problem.Retryable {
		t.Fatalf("problem = %#v", problem)
	}
}

func TestMessagesSearchExplicitUnlimitedLimitRequiresCompleteRead(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x", AppSecret: "secret"}
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	parent := &cobra.Command{Use: "im"}
	shortcut := Shortcut{
		Service:     "im",
		Command:     "+messages-search",
		Description: "test",
		Risk:        "read",
		AuthTypes:   []string{"bot"},
		Flags: []Flag{
			{Name: "page-all", Type: "bool"},
			{Name: "page-limit", Type: "int", Default: "40"},
		},
		Execute: func(_ context.Context, runtime *RuntimeContext) error {
			runtime.RecordPagination(client.PaginationStatus{
				PagesFetched: 1,
				StopReason:   client.StopReasonServerTruncation,
			})
			runtime.RecordMaterialization(imcontract.MaterializationStatus{})
			runtime.Out(map[string]any{"messages": []any{}}, nil)
			return nil
		},
	}
	shortcut.Mount(parent, f)
	parent.SetArgs([]string{"+messages-search", "--as", "bot", "--page-limit", "0"})

	err := parent.Execute()
	if output.ExitCodeOf(err) != output.ExitAPI {
		t.Fatalf("error = %T %v, exit=%d want %d", err, err, output.ExitCodeOf(err), output.ExitAPI)
	}
	var envelope map[string]any
	if jsonErr := json.Unmarshal(stdout.Bytes(), &envelope); jsonErr != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", jsonErr, stdout.String())
	}
	meta, _ := envelope["meta"].(map[string]any)
	pagination, _ := meta["pagination"].(map[string]any)
	notice, _ := envelope["_notice"].(map[string]any)
	readNotice, _ := notice["im_read"].(map[string]any)
	if envelope["ok"] != false || pagination["complete"] != false ||
		readNotice["stop_reason"] != string(client.StopReasonServerTruncation) {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestIMContractAlsoAppliesToPrettyOutput(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	rt := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+chat-create"}, cfg, f, core.AsUser)
	rt.Format = "pretty"
	contract, _ := imcontract.Lookup("im +chat-create")
	rt.contractSession = imcontract.NewSession(contract)

	rt.OutFormat(map[string]any{"chat_id": ""}, nil, func(w io.Writer) {
		fmt.Fprintln(w, "Group created successfully")
	})

	if stdout.Len() != 0 {
		t.Fatalf("false pretty success reached stdout: %s", stdout.String())
	}
	if output.ExitCodeOf(rt.outputErr) != output.ExitInternal {
		t.Fatalf("exit = %d, want 5; err=%v", output.ExitCodeOf(rt.outputErr), rt.outputErr)
	}
}

func TestIMReadLateFailureWritesOneSelfContainedJSONEnvelope(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, stdout, stderr, _ := cmdutil.TestFactory(t, cfg)
	rt := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+chat-list"}, cfg, f, core.AsUser)
	contract, _ := imcontract.Lookup("im +chat-list")
	rt.readSession, _ = imcontract.NewReadSession(contract, imcontract.ReadOptions{FullRead: true})
	cause := errs.NewNetworkError(errs.SubtypeNetworkTransport, "request failed").WithRetryable()
	rt.RecordPagination(client.PaginationStatus{
		PagesFetched: 1, HasMore: true, NextPageToken: "next",
		StopReason: client.StopReasonTransportError, Cause: cause,
	})

	rt.Out(map[string]any{"items": []any{"kept"}}, nil)

	if stderr.Len() != 0 {
		t.Fatalf("stderr must stay empty for unprojected JSON, got %s", stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	meta := env["meta"].(map[string]any)
	pagination := meta["pagination"].(map[string]any)
	notice := env["_notice"].(map[string]any)
	readNotice := notice["im_read"].(map[string]any)
	rawProblem, exists := env["error"]
	if !exists {
		t.Fatalf("late failure omitted structured error: %#v", env)
	}
	problem, ok := rawProblem.(map[string]any)
	if !ok {
		t.Fatalf("late failure error = %T, want object: %#v", rawProblem, env)
	}
	if env["ok"] != false || pagination["complete"] != false ||
		readNotice["stop_reason"] != "transport_error" || problem["type"] != "network" {
		t.Fatalf("unexpected envelope: %#v", env)
	}
	var partial *output.PartialFailureError
	if !errors.As(rt.outputErr, &partial) || partial.Code != output.ExitNetwork {
		t.Fatalf("output error = %T %v", rt.outputErr, rt.outputErr)
	}
}

func TestIMReadLateFailureKeepsPresentationAndTypedErrorOutsideJSON(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, stdout, stderr, _ := cmdutil.TestFactory(t, cfg)
	rt := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+chat-list"}, cfg, f, core.AsUser)
	rt.Format = "pretty"
	contract, _ := imcontract.Lookup("im +chat-list")
	rt.readSession, _ = imcontract.NewReadSession(contract, imcontract.ReadOptions{FullRead: true})
	cause := errs.NewNetworkError(errs.SubtypeNetworkTransport, "request failed").WithRetryable()
	rt.RecordPagination(client.PaginationStatus{
		PagesFetched: 1, HasMore: true, NextPageToken: "next",
		StopReason: client.StopReasonTransportError, Cause: cause,
	})

	rt.OutFormat(map[string]any{"items": []any{"kept"}}, nil, func(w io.Writer) {
		fmt.Fprintln(w, "kept")
	})

	wantStdout := "kept\n\nPagination: incomplete (1 page(s), 1 item(s)); resume token: \"next\"\n"
	if stdout.String() != wantStdout {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "hint: The read is incomplete") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if !errors.Is(rt.outputErr, cause) {
		t.Fatalf("output error = %T %v, want original cause", rt.outputErr, rt.outputErr)
	}
}

func TestMergeIMReadMetaHandlesNilInputsAndPreservesBaseFields(t *testing.T) {
	if got := mergeIMReadMeta(nil, nil); got != nil {
		t.Fatalf("mergeIMReadMeta(nil, nil) = %#v, want nil", got)
	}
	base := &output.Meta{Count: 7, Rollback: "undo-token"}
	baseOnly := mergeIMReadMeta(base, nil)
	if baseOnly == nil || baseOnly.Count != 7 || baseOnly.Rollback != "undo-token" {
		t.Fatalf("base-only meta = %#v", baseOnly)
	}
	contract := &output.Meta{
		Pagination: &output.PaginationMeta{Complete: false, Pages: 1, Items: 2, NextToken: "next"},
	}
	contractOnly := mergeIMReadMeta(nil, contract)
	if contractOnly == nil || contractOnly.Pagination == nil || contractOnly.Pagination.Complete ||
		contractOnly.Pagination.Pages != 1 || contractOnly.Pagination.Items != 2 {
		t.Fatalf("contract-only meta = %#v", contractOnly)
	}
	merged := mergeIMReadMeta(base, contract)
	if merged.Count != 7 || merged.Rollback != "undo-token" ||
		merged.Pagination == nil || merged.Pagination.Complete || merged.Pagination.NextToken != "next" {
		t.Fatalf("merged meta = %#v", merged)
	}
}

func TestIMChatMembersReadPreservesCountMeta(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, stdout, _, _ := cmdutil.TestFactory(t, cfg)
	rt := TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+chat-members-list"}, cfg, f, core.AsUser)
	contract, _ := imcontract.Lookup("im +chat-members-list")
	rt.readSession, _ = imcontract.NewReadSession(contract, imcontract.ReadOptions{})
	rt.RecordPagination(client.PaginationStatus{
		PagesFetched: 1, HasMore: true, NextPageToken: "next", StopReason: client.StopReasonSinglePage,
	})

	rt.Out(map[string]any{"users": []any{"ou_a"}, "bots": []any{"cli_a"}}, &output.Meta{Count: 2})

	var env struct {
		Meta   output.Meta            `json:"meta"`
		Notice map[string]interface{} `json:"_notice"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	readNotice, _ := env.Notice["im_read"].(map[string]interface{})
	if env.Meta.Count != 2 || env.Meta.Pagination == nil || env.Meta.Pagination.Complete ||
		readNotice["stop_reason"] != "single_page" {
		t.Fatalf("meta = %#v, want count plus incomplete contract fields", env.Meta)
	}
}
