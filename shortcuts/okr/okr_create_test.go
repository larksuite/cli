// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"
)

func createTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	return &core.CliConfig{
		AppID:     "test-okr-create",
		AppSecret: patchTestValue(),
		Brand:     core.BrandFeishu,
	}
}

func runCreateShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	OKRCreate.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func runCreateShortcutWithStdin(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, stdin string, args []string) error {
	t.Helper()
	f.IOStreams.In = strings.NewReader(stdin)
	return runCreateShortcut(t, f, stdout, args)
}

const (
	validCreateSimpleJSON     = `{"text":"test objective","mention":["ou_123"]}`
	validCreateRichTextJSON   = `{"blocks":[{"block_element_type":"paragraph","paragraph":{"elements":[{"paragraph_element_type":"textRun","text_run":{"text":"test content"}}]}}]}`
	emptyCreateRichTextJSON   = `{"blocks":[]}`
	blankCreateRichTextJSON   = `{"blocks":[{"block_element_type":"paragraph","paragraph":{"elements":[]}}]}`
	validCreateObjectiveArgs1 = "+create"
)

func TestCreateValidate_MissingLevel(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		validCreateObjectiveArgs1,
		"--cycle-id", "123",
		"--content", validCreateSimpleJSON,
	})
	if err == nil || !strings.Contains(err.Error(), "level") {
		t.Fatalf("expected --level required error, got: %v", err)
	}
}

func TestCreateValidate_InvalidLevel(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "invalid",
		"--cycle-id", "123",
		"--content", validCreateSimpleJSON,
	})
	if err == nil {
		t.Fatal("expected invalid level error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected typed invalid argument error, got: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--level" {
		t.Fatalf("expected param --level, got: %v", err)
	}
}

func TestCreateValidate_MissingCycleIDForObjective(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--content", validCreateSimpleJSON,
	})
	if err == nil {
		t.Fatal("expected missing cycle-id error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected typed invalid argument error, got: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--cycle-id" {
		t.Fatalf("expected param --cycle-id, got: %v", err)
	}
}

func TestCreateValidate_InvalidCycleID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "abc",
		"--content", validCreateSimpleJSON,
	})
	if err == nil {
		t.Fatal("expected invalid cycle-id error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected typed invalid argument error, got: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--cycle-id" {
		t.Fatalf("expected param --cycle-id, got: %v", err)
	}
}

func TestCreateValidate_MissingObjectiveIDForKR(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "key-result",
		"--content", validCreateSimpleJSON,
	})
	if err == nil {
		t.Fatal("expected missing objective-id error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected typed invalid argument error, got: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--objective-id" {
		t.Fatalf("expected param --objective-id, got: %v", err)
	}
}

func TestCreateValidate_RejectObjectiveIDForObjective(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--objective-id", "456",
		"--content", validCreateSimpleJSON,
	})
	if err == nil {
		t.Fatal("expected objective-id rejection")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--objective-id" {
		t.Fatalf("expected param --objective-id, got: %v", err)
	}
}

func TestCreateValidate_RejectCycleIDForKeyResult(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "key-result",
		"--cycle-id", "123",
		"--objective-id", "456",
		"--content", validCreateSimpleJSON,
	})
	if err == nil {
		t.Fatal("expected cycle-id rejection")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--cycle-id" {
		t.Fatalf("expected param --cycle-id, got: %v", err)
	}
}

func TestCreateValidate_RejectNotesForKeyResult(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "key-result",
		"--objective-id", "456",
		"--content", validCreateSimpleJSON,
		"--notes", `{"text":"objective only notes"}`,
	})
	if err == nil {
		t.Fatal("expected notes rejection for key-result")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--notes" {
		t.Fatalf("expected param --notes, got: %v", err)
	}
}

func TestCreateValidate_RejectCategoryIDForKeyResult(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "key-result",
		"--objective-id", "456",
		"--content", validCreateSimpleJSON,
		"--category-id", "123",
	})
	if err == nil {
		t.Fatal("expected category-id rejection for key-result")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--category-id" {
		t.Fatalf("expected param --category-id, got: %v", err)
	}
}

func TestCreateValidate_ContentAndNotesCannotBothReadStdin(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcutWithStdin(t, f, stdout, `{"text":"stdin content"}`, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--content", "-",
		"--notes", "-",
	})
	if err == nil {
		t.Fatal("expected duplicate stdin error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--notes" {
		t.Fatalf("expected param --notes, got: %v", err)
	}
	if !strings.Contains(err.Error(), "stdin (-) can only be used by one flag") {
		t.Fatalf("expected duplicate stdin error, got: %v", err)
	}
}

func TestCreateValidate_InvalidObjectiveID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "key-result",
		"--objective-id", "0",
		"--content", validCreateSimpleJSON,
	})
	if err == nil {
		t.Fatal("expected invalid objective-id error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected typed invalid argument error, got: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--objective-id" {
		t.Fatalf("expected param --objective-id, got: %v", err)
	}
}

func TestCreateValidate_InvalidStyle(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--style", "invalid",
		"--content", validCreateSimpleJSON,
	})
	if err == nil {
		t.Fatal("expected invalid style error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--style" {
		t.Fatalf("expected param --style, got: %v", err)
	}
}

func TestCreateValidate_InvalidUserIDType(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--content", validCreateSimpleJSON,
		"--user-id-type", "invalid",
	})
	if err == nil {
		t.Fatal("expected invalid user-id-type error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--user-id-type" {
		t.Fatalf("expected param --user-id-type, got: %v", err)
	}
}

func TestCreateValidate_MissingContent(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
	})
	if err == nil || !strings.Contains(err.Error(), "content") {
		t.Fatalf("expected required content error, got: %v", err)
	}
}

func TestCreateValidate_InvalidSimpleContentJSON(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--style", "simple",
		"--content", "not-json",
	})
	if err == nil {
		t.Fatal("expected invalid simple json error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--content" {
		t.Fatalf("expected param --content, got: %v", err)
	}
}

func TestCreateValidate_EmptySimpleText(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--style", "simple",
		"--content", `{"text":"   "}`,
	})
	if err == nil {
		t.Fatal("expected empty simple text error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--content" {
		t.Fatalf("expected param --content, got: %v", err)
	}
}

func TestCreateValidate_EmptySimpleMention(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--style", "simple",
		"--content", `{"text":"test","mention":[""]}`,
	})
	if err == nil {
		t.Fatal("expected empty simple mention error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--content" {
		t.Fatalf("expected param --content, got: %v", err)
	}
}

func TestCreateValidate_SimpleContentRejectsDocsImages(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--style", "simple",
		"--content", `{"text":"test","docs":[{"title":"doc","url":"https://example.com"}],"images":["img"]}`,
	})
	if err == nil {
		t.Fatal("expected docs/images rejection")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--content" {
		t.Fatalf("expected param --content, got: %v", err)
	}
}

func TestCreateValidate_SimpleContentRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--style", "simple",
		"--content", `{"text":"test","mentions":["ou_123"]}`,
	})
	if err == nil {
		t.Fatal("expected unknown simple content field error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--content" {
		t.Fatalf("expected param --content, got: %v", err)
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got: %v", err)
	}
}

func TestCreateValidate_InvalidRichTextJSON(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--style", "richtext",
		"--content", "not-json",
	})
	if err == nil {
		t.Fatal("expected invalid richtext json error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--content" {
		t.Fatalf("expected param --content, got: %v", err)
	}
}

func TestCreateValidate_RichTextRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--style", "richtext",
		"--content", `{"blocks":[{"block_element_type":"paragraph","paragraph":{"elements":[{"paragraph_element_type":"textRun","text_run":{"text":"test content"}}]}}],"mentions":["ou_123"]}`,
	})
	if err == nil {
		t.Fatal("expected unknown richtext content field error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--content" {
		t.Fatalf("expected param --content, got: %v", err)
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got: %v", err)
	}
}

func TestCreateValidate_EmptyRichTextContent(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	for _, content := range []string{emptyCreateRichTextJSON, blankCreateRichTextJSON} {
		err := runCreateShortcut(t, f, stdout, []string{
			"+create",
			"--level", "objective",
			"--cycle-id", "123",
			"--style", "richtext",
			"--content", content,
		})
		if err == nil {
			t.Fatalf("expected empty richtext error for %s", content)
		}
		var ve *errs.ValidationError
		if !errors.As(err, &ve) || ve.Param != "--content" {
			t.Fatalf("expected param --content, got: %v", err)
		}
	}
}

func TestCreateDryRun_Objective(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--content", validCreateSimpleJSON,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if got := gjson.Get(output, "data.api.0.method").String(); got != "POST" {
		t.Fatalf("dry-run method = %q, want POST; output: %s", got, output)
	}
	if got := gjson.Get(output, "data.api.0.url").String(); got != "/open-apis/okr/v2/cycles/123/objectives" {
		t.Fatalf("dry-run url = %q, want objective create path; output: %s", got, output)
	}
	if gjson.Get(output, "data.api.0.params.cycle_id").String() != "123" {
		t.Fatalf("expected query params in dry-run, got: %s", output)
	}
	if gjson.Get(output, "data.api.0.params.user_id_type").String() != "open_id" {
		t.Fatalf("expected default user-id-type in dry-run, got: %s", output)
	}
	if got := gjson.Get(output, "data.api.0.body.content.blocks.0.paragraph.elements.0.text_run.text").String(); got != "test objective" {
		t.Fatalf("dry-run content text = %q, want test objective; output: %s", got, output)
	}
	if got := gjson.Get(output, "data.api.0.body.content.blocks.0.paragraph.elements.1.mention.user_id").String(); got != "ou_123" {
		t.Fatalf("dry-run mention user_id = %q, want ou_123; output: %s", got, output)
	}
}

func TestCreateDryRun_ObjectiveWithNotes(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--content", validCreateSimpleJSON,
		"--notes", `{"text":"objective notes","mention":["ou_note"]}`,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if got := gjson.Get(output, "data.api.0.body.notes.blocks.0.paragraph.elements.0.text_run.text").String(); got != "objective notes" {
		t.Fatalf("dry-run notes text = %q, want objective notes; output: %s", got, output)
	}
	if got := gjson.Get(output, "data.api.0.body.notes.blocks.0.paragraph.elements.1.mention.user_id").String(); got != "ou_note" {
		t.Fatalf("dry-run notes mention user_id = %q, want ou_note; output: %s", got, output)
	}
}

func TestCreateDryRun_ObjectiveWithCategoryID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--content", validCreateSimpleJSON,
		"--category-id", "7249339036661170180",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if got := gjson.Get(output, "data.api.0.body.category_id").String(); got != "7249339036661170180" {
		t.Fatalf("dry-run category_id = %q, want 7249339036661170180; output: %s", got, output)
	}
}

func TestCreateDryRun_KeyResult(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "key-result",
		"--objective-id", "456",
		"--style", "richtext",
		"--content", validCreateRichTextJSON,
		"--user-id-type", "union_id",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if got := gjson.Get(output, "data.api.0.method").String(); got != "POST" {
		t.Fatalf("dry-run method = %q, want POST; output: %s", got, output)
	}
	if got := gjson.Get(output, "data.api.0.url").String(); got != "/open-apis/okr/v2/objectives/456/key_results" {
		t.Fatalf("dry-run url = %q, want key result create path; output: %s", got, output)
	}
	if gjson.Get(output, "data.api.0.params.objective_id").String() != "456" {
		t.Fatalf("expected objective-id query param in dry-run, got: %s", output)
	}
	if gjson.Get(output, "data.api.0.params.user_id_type").String() != "union_id" {
		t.Fatalf("expected query params in dry-run, got: %s", output)
	}
	if got := gjson.Get(output, "data.api.0.body.content.blocks.0.paragraph.elements.0.text_run.text").String(); got != "test content" {
		t.Fatalf("dry-run richtext content = %q, want test content; output: %s", got, output)
	}
}

func TestCreateExecute_ObjectiveSuccess(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, createTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/cycles/123/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"objective_id": "1001",
			},
		},
	})
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--content", validCreateSimpleJSON,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	level, _ := data["level"].(string)
	if level != "objective" {
		t.Fatalf("expected level objective, got %v", data["level"])
	}
	if data["objective_id"] != "1001" {
		t.Fatalf("expected objective_id=1001, got %v", data["objective_id"])
	}
}

func TestCreateExecute_KeyResultSuccess(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, createTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/objectives/456/key_results",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"key_result_id": "2001",
			},
		},
	})
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "key-result",
		"--objective-id", "456",
		"--content", validCreateSimpleJSON,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	level, _ := data["level"].(string)
	if level != "key-result" {
		t.Fatalf("expected level key-result, got %v", data["level"])
	}
	if data["key_result_id"] != "2001" {
		t.Fatalf("expected key_result_id=2001, got %v", data["key_result_id"])
	}
	if data["objective_id"] != "456" {
		t.Fatalf("expected objective_id=456, got %v", data["objective_id"])
	}
}

func TestCreateExecute_ObjectiveAPITypedErrorPassThrough(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, createTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/okr/v2/cycles/123/objectives",
		Status: 400,
		Body: map[string]interface{}{
			"code": 1001001,
			"msg":  "invalid parameters",
		},
	})
	err := runCreateShortcut(t, f, stdout, []string{
		"+create",
		"--level", "objective",
		"--cycle-id", "123",
		"--content", validCreateSimpleJSON,
	})
	if err == nil {
		t.Fatal("expected API error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryAPI {
		t.Fatalf("expected typed API error, got: %v", err)
	}
}

func TestCreateExecute_KeyResultRawErrorWrappedAsNetworkError(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, createTestConfig(t))
	raw := errors.New("dial tcp: i/o timeout")
	got := wrapOkrNetworkErr(raw, "failed to create key result")
	problem, ok := errs.ProblemOf(got)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("expected network transport error, got: %v", got)
	}
	if !errors.Is(got, raw) {
		t.Fatal("expected wrapped raw error to be preserved")
	}
	if stdout.String() != "" || f == nil {
		// keep the test factory referenced so the helper wiring stays exercised
	}
}
