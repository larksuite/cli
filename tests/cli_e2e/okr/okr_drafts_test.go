// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOKR_DraftFlagsDryRun(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		flag   string
		fields []string
	}{
		{"read", []string{"+cycle-detail", "--cycle-id", "123"}, "prefer-draft", []string{"api.0.params.prefer_draft"}},
		{"create_objective", []string{"+create", "--level", "objective", "--cycle-id", "123", "--content", `{"text":"O"}`}, "apply-to-draft", []string{"api.0.body.apply_to_draft"}},
		{"create_key_result", []string{"+create", "--level", "key-result", "--objective-id", "123", "--content", `{"text":"KR"}`}, "apply-to-draft", []string{"api.0.body.apply_to_draft"}},
		{"batch_create", []string{"+batch-create", "--cycle-id", "123", "--input", `[{"text":"O","krs":[{"text":"KR"}]}]`}, "apply-to-draft", []string{"api.0.body.apply_to_draft", "api.1.body.apply_to_draft"}},
		{"patch_objective", []string{"+patch", "--level", "objective", "--target-id", "123", "--content", `{"text":"O"}`}, "apply-to-draft", []string{"api.0.body.apply_to_draft"}},
		{"patch_key_result", []string{"+patch", "--level", "key-result", "--target-id", "123", "--content", `{"text":"KR"}`}, "apply-to-draft", []string{"api.0.body.apply_to_draft"}},
		{"indicator_objective", []string{"+indicator-update", "--level", "objective", "--id", "123", "--value", "1"}, "apply-to-draft", []string{"api.0.params.prefer_draft", "api.1.body.apply_to_draft"}},
		{"indicator_key_result", []string{"+indicator-update", "--level", "key-result", "--id", "123", "--value", "1"}, "apply-to-draft", []string{"api.0.params.prefer_draft", "api.1.body.apply_to_draft"}},
		{"key_result_weight", []string{"+weight", "--level", "key-result", "--cycle-id", "123", "--objective-id", "456", "--weights", `[{"id":"789","weight":1}]`}, "apply-to-draft", []string{"api.1.body.apply_to_draft"}},
	}
	for _, tc := range cases {
		for _, mode := range []string{"default", "true", "false"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				setDryRunConfigEnv(t)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				t.Cleanup(cancel)
				args := append([]string{"okr"}, tc.args...)
				if mode != "default" {
					args = append(args, "--"+tc.flag+"="+mode)
				}
				args = append(args, "--dry-run")
				result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
				require.NoError(t, err)
				result.AssertExitCode(t, 0)
				result.AssertStdoutStatus(t, true)
				for _, field := range tc.fields {
					value := clie2e.DryRunGet(result.Stdout, field)
					require.True(t, value.Type == gjson.True || value.Type == gjson.False, "%s must be a JSON boolean: %s", field, result.Stdout)
					assert.Equal(t, mode != "false", value.Bool(), field)
				}
			})
		}
	}
}

func TestOKR_PatchDraftScoreRejectedDryRun(t *testing.T) {
	for _, level := range []string{"objective", "key-result"} {
		for _, mode := range []string{"default", "true"} {
			t.Run(level+"/"+mode, func(t *testing.T) {
				setDryRunConfigEnv(t)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				t.Cleanup(cancel)
				args := []string{"okr", "+patch", "--level", level, "--target-id", "123", "--score", "0.5", "--dry-run"}
				if mode == "true" {
					args = append(args, "--apply-to-draft=true")
				}
				result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
				require.NoError(t, err)
				result.AssertExitCode(t, 2)
				assert.Empty(t, result.Stdout)
				assert.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
				assert.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
				assert.Equal(t, "--score", gjson.Get(result.Stderr, "error.param").String())
			})
		}
	}
}

func TestOKR_DraftActionsDryRun(t *testing.T) {
	for _, action := range []string{"publish", "drop"} {
		t.Run(action, func(t *testing.T) {
			setDryRunConfigEnv(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"okr", "+" + action + "-draft", "--objective-id", "123", "--dry-run"}, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			result.AssertStdoutStatus(t, true)
			assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			assert.Equal(t, "/open-apis/okr/v2/objectives/123/draft_"+action, clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		})
	}
}

// Requires a bot with access to OKR_TEST_DRAFT_CYCLE_ID in an environment with draft APIs.
func TestOKR_DraftLifecycleLive(t *testing.T) {
	cycleID := os.Getenv("OKR_TEST_DRAFT_CYCLE_ID")
	if cycleID == "" {
		t.Skip("OKR_TEST_DRAFT_CYCLE_ID not set; requires a bot-accessible cycle in an environment with draft APIs")
	}
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	run := func(args ...string) string {
		t.Helper()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: append([]string{"okr"}, args...), DefaultAs: "bot"})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		return result.Stdout
	}
	suffix := clie2e.GenerateSuffix()
	text := "okr-e2e-draft-" + suffix
	created := run("+create", "--level", "objective", "--cycle-id", cycleID, "--content", fmt.Sprintf(`{"text":%q}`, text))
	objectiveID := gjson.Get(created, "data.objective_id").String()
	require.NotEmpty(t, objectiveID)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{Args: []string{"okr", "objectives", "delete"}, Params: map[string]string{"objective_id": objectiveID}, DefaultAs: "bot", Yes: true})
		clie2e.ReportCleanupFailure(t, "delete draft lifecycle objective", result, err)
	})
	krCreated := run("+create", "--level", "key-result", "--objective-id", objectiveID, "--content", `{"text":"okr-e2e-key-result"}`)
	krID := gjson.Get(krCreated, "data.key_result_id").String()
	require.NotEmpty(t, krID)
	read := func(preferDraft bool) gjson.Result {
		t.Helper()
		output := run("+cycle-detail", "--cycle-id", cycleID, fmt.Sprintf("--prefer-draft=%t", preferDraft))
		for _, objective := range gjson.Get(output, "data.objectives").Array() {
			if objective.Get("id").String() == objectiveID {
				return objective
			}
		}
		return gjson.Result{}
	}
	check := func(preferDraft, isDraft bool, expectedText, expectedKRText string) {
		t.Helper()
		objective := read(preferDraft)
		require.True(t, objective.Exists(), "created objective must be visible")
		require.True(t, objective.Get("is_draft").Exists())
		assert.Equal(t, isDraft, objective.Get("is_draft").Bool())
		assert.Equal(t, expectedText, objective.Get("content.text").String())
		krs := objective.Get("key_results").Array()
		require.Len(t, krs, 1)
		assert.Equal(t, krID, krs[0].Get("id").String())
		require.True(t, krs[0].Get("is_draft").Exists())
		assert.Equal(t, isDraft, krs[0].Get("is_draft").Bool())
		assert.Equal(t, expectedKRText, krs[0].Get("content.text").String())
	}
	check(true, true, text, "okr-e2e-key-result")
	assert.False(t, read(false).Exists(), "draft-only objective must be absent from published view")
	published := run("+publish-draft", "--objective-id", objectiveID)
	assert.Equal(t, objectiveID, gjson.Get(published, "data.objective_id").String())
	check(false, false, text, "okr-e2e-key-result")
	run("+patch", "--level", "objective", "--target-id", objectiveID, "--content", `{"text":"okr-e2e-edited-draft"}`)
	run("+patch", "--level", "key-result", "--target-id", krID, "--content", `{"text":"okr-e2e-edited-key-result"}`)
	check(true, true, "okr-e2e-edited-draft", "okr-e2e-edited-key-result")
	check(false, false, text, "okr-e2e-key-result")
	dropped := run("+drop-draft", "--objective-id", objectiveID, "--yes")
	assert.Equal(t, objectiveID, gjson.Get(dropped, "data.objective_id").String())
	check(true, false, text, "okr-e2e-key-result")
	check(false, false, text, "okr-e2e-key-result")
	run("+patch", "--level", "objective", "--target-id", objectiveID, "--content", `{"text":"okr-e2e-published-edit"}`, "--apply-to-draft=false")
	check(true, false, "okr-e2e-published-edit", "okr-e2e-key-result")
	check(false, false, "okr-e2e-published-edit", "okr-e2e-key-result")
}
