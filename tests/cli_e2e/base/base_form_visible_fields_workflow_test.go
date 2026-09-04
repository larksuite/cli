// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseFormVisibleFieldsWorkflow(t *testing.T) {
	if os.Getenv("LARK_CLI_E2E_BASE_FORM_VISIBLE_FIELDS_READY") != "1" {
		t.Skip("set LARK_CLI_E2E_BASE_FORM_VISIBLE_FIELDS_READY=1 after Form visible_fields target-state support is deployed")
	}
	clie2e.SkipWithoutTenantAccessToken(t)

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-form-visible-fields-"+clie2e.GenerateSuffix())
	tableID, _, _ := createTableWithRetry(
		t,
		parentT,
		ctx,
		baseToken,
		"Form visible fields "+clie2e.GenerateSuffix(),
		`[{"name":"Question A","type":"text"},{"name":"Question B","type":"text"},{"name":"Question C","type":"text"}]`,
		`{"name":"Main","type":"grid"}`,
	)

	formCreate, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+form-create",
			"--base-token", baseToken,
			"--table-id", tableID,
			"--name", "Visible fields form " + clie2e.GenerateSuffix(),
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	formCreate.AssertExitCode(t, 0)
	formCreate.AssertStdoutStatus(t, true)
	formID := gjson.Get(formCreate.Stdout, "data.id").String()
	require.NotEmpty(t, formID, formCreate.Stdout)

	var initialQuestionIDs []string
	var initialQuestionNames []string
	waitErr := clie2e.WaitForCondition(ctx, clie2e.WaitOptions{
		Timeout:  90 * time.Second,
		Interval: 2 * time.Second,
		TimeoutError: func() error {
			return fmt.Errorf("Form questions were not readable after create")
		},
	}, func() (bool, error) {
		listed, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+form-questions-list",
				"--base-token", baseToken,
				"--table-id", tableID,
				"--form-id", formID,
			},
			DefaultAs: "bot",
		})
		if runErr != nil || listed.ExitCode != 0 {
			return false, nil
		}
		questions := gjson.Get(listed.Stdout, "data.questions").Array()
		if len(questions) != 3 {
			return false, nil
		}
		initialQuestionIDs = initialQuestionIDs[:0]
		initialQuestionNames = initialQuestionNames[:0]
		for _, question := range questions {
			id := question.Get("id").String()
			name := question.Get("title").String()
			if id == "" || name == "" {
				return false, nil
			}
			initialQuestionIDs = append(initialQuestionIDs, id)
			initialQuestionNames = append(initialQuestionNames, name)
		}
		return true, nil
	})
	require.NoError(t, waitErr)

	setVisibleFields := func(fieldIDs []string) {
		t.Helper()
		body, marshalErr := json.Marshal(map[string][]string{"visible_fields": fieldIDs})
		require.NoError(t, marshalErr)
		result, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+view-set-visible-fields",
				"--base-token", baseToken,
				"--table-id", tableID,
				"--view-id", formID,
				"--json", string(body),
			},
			DefaultAs: "bot",
		})
		require.NoError(t, runErr)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	}
	readVisibleFields := func() ([]string, error) {
		result, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+view-get-visible-fields",
				"--base-token", baseToken,
				"--table-id", tableID,
				"--view-id", formID,
			},
			DefaultAs: "bot",
		})
		if runErr != nil {
			return nil, runErr
		}
		if result.ExitCode != 0 {
			return nil, fmt.Errorf("get visible fields failed: stdout=%s stderr=%s", result.Stdout, result.Stderr)
		}
		items := gjson.Get(result.Stdout, "data.visible_fields").Array()
		names := make([]string, 0, len(items))
		for _, item := range items {
			names = append(names, item.String())
		}
		return names, nil
	}
	assertVisibleFieldsEventually := func(expected []string) {
		t.Helper()
		var actual []string
		pollErr := clie2e.WaitForCondition(ctx, clie2e.WaitOptions{
			Timeout:  90 * time.Second,
			Interval: 2 * time.Second,
			TimeoutError: func() error {
				return fmt.Errorf("visible fields did not reach target: got=%v want=%v", actual, expected)
			},
		}, func() (bool, error) {
			var readErr error
			actual, readErr = readVisibleFields()
			if readErr != nil {
				return false, nil
			}
			return slices.Equal(actual, expected), nil
		})
		require.NoError(t, pollErr)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := cleanupContext()
		defer cleanupCancel()
		body, marshalErr := json.Marshal(map[string][]string{"visible_fields": initialQuestionIDs})
		if marshalErr != nil {
			parentT.Errorf("marshal Form visible_fields cleanup target: %v", marshalErr)
			return
		}
		result, cleanupErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{
				"base", "+view-set-visible-fields",
				"--base-token", baseToken,
				"--table-id", tableID,
				"--view-id", formID,
				"--json", string(body),
			},
			DefaultAs: "bot",
		})
		if cleanupErr != nil || result == nil || result.ExitCode != 0 {
			reportCleanupFailure(parentT, "restore Form visible_fields", result, cleanupErr)
		}
	})

	hiddenTargetIDs := []string{initialQuestionIDs[2], initialQuestionIDs[0]}
	hiddenTargetNames := []string{initialQuestionNames[2], initialQuestionNames[0]}
	setVisibleFields(hiddenTargetIDs)
	assertVisibleFieldsEventually(hiddenTargetNames)

	restoredTargetIDs := []string{initialQuestionIDs[0], initialQuestionIDs[1], initialQuestionIDs[2]}
	restoredTargetNames := []string{initialQuestionNames[0], initialQuestionNames[1], initialQuestionNames[2]}
	setVisibleFields(restoredTargetIDs)
	assertVisibleFieldsEventually(restoredTargetNames)
}
