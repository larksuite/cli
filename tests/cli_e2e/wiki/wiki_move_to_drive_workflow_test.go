// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"fmt"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	drivee2e "github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestWiki_MoveToDriveWorkflow validates the live async round trip, including
// the standalone drive +task_result continuation path.
func TestWiki_MoveToDriveWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	nodeTitle := "lark-cli-e2e-wiki-to-drive-node-" + suffix
	folderToken := drivee2e.CreateDriveFolder(
		t,
		parentT,
		ctx,
		"lark-cli-e2e-wiki-to-drive-"+suffix,
		"bot",
		"",
	)
	_, node := createWikiNodeUnderAnyHost(
		t,
		parentT,
		ctx,
		nodeTitle,
	)
	nodeToken := node.Get("node_token").String()
	require.NotEmpty(t, nodeToken)

	var moveTaskID, movedObjToken, movedObjType string
	// Register fallback cleanup before creating the async task. If the command
	// times out or its success payload omits optional resource fields, find the
	// uniquely named document in the target folder so the folder cleanup does
	// not leak a non-empty tree.
	parentT.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		targets := []wikiMoveToDriveResource{}
		if movedObjToken != "" {
			objType := movedObjType
			if objType == "" {
				objType = "docx"
			}
			targets = append(targets, wikiMoveToDriveResource{Token: movedObjToken, Type: objType})
		} else {
			listed, listResult, listErr := waitForWikiMoveToDriveResources(
				cleanupCtx,
				folderToken,
				nodeTitle,
				moveTaskID,
			)
			if listErr != nil {
				clie2e.ReportCleanupFailure(parentT, "list wiki move-to-drive cleanup targets", listResult, listErr)
				return
			}
			targets = listed
		}

		for _, target := range targets {
			deleteResult, deleteErr := drivee2e.DeleteDriveResourceAndVerify(cleanupCtx, target.Token, target.Type, "bot")
			clie2e.ReportCleanupFailure(parentT, "delete moved Drive document "+target.Token, deleteResult, deleteErr)
		}
	})

	moveResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"wiki", "+move-to-drive",
			"--node-token", nodeToken,
			"--folder-token", folderToken,
		},
		DefaultAs: "bot",
	})
	if moveResult != nil {
		moveTaskID = gjson.Get(moveResult.Stdout, "data.task_id").String()
	}
	require.NoError(t, err)
	moveResult.AssertExitCode(t, 0)
	moveResult.AssertStdoutStatus(t, true)

	moveData := waitWikiMoveToDriveReady(t, ctx, moveResult)
	movedObjToken = moveData.Get("data.obj_token").String()
	movedObjType = moveData.Get("data.obj_type").String()
	require.NotEmpty(t, movedObjToken, "move result must contain obj_token; stdout:\n%s", moveData.Raw)
	require.NotEmpty(t, movedObjType, "move result must contain obj_type; stdout:\n%s", moveData.Raw)
	require.NotEmpty(t, moveData.Get("data.url").String(), "move result must contain url; stdout:\n%s", moveData.Raw)

	err = waitWikiNodeDeleted(ctx, nodeToken)
	require.NoError(t, err, "source wiki node should disappear after move-to-drive")

	err = clie2e.WaitForCondition(ctx, clie2e.WaitOptions{
		Timeout:  45 * time.Second,
		Interval: 3 * time.Second,
		TimeoutError: func() error {
			return fmt.Errorf("moved document %s did not appear in target folder %s", movedObjToken, folderToken)
		},
	}, func() (bool, error) {
		listResult, listErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "files", "list",
				"--folder-token", folderToken,
				"--page-size", "200",
			},
			DefaultAs: "bot",
		})
		if listErr != nil {
			return false, listErr
		}
		if listResult.ExitCode != 0 {
			return false, fmt.Errorf(
				"list target folder failed: exit=%d stdout=%s stderr=%s",
				listResult.ExitCode,
				listResult.Stdout,
				listResult.Stderr,
			)
		}
		match := gjson.Get(listResult.Stdout, `data.files.#(token=="`+movedObjToken+`")`)
		return match.Exists() && match.Get("type").String() == movedObjType, nil
	})
	require.NoError(t, err)
}

type wikiMoveToDriveResource struct {
	Token string
	Type  string
}

func waitForWikiMoveToDriveResources(
	ctx context.Context,
	folderToken string,
	name string,
	taskID string,
) ([]wikiMoveToDriveResource, *clie2e.Result, error) {
	const discoveryTimeout = 20 * time.Second
	discoveryCtx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()

	var resources []wikiMoveToDriveResource
	var lastResult *clie2e.Result
	var lastErr error
	err := clie2e.WaitForCondition(discoveryCtx, clie2e.WaitOptions{
		Timeout:  discoveryTimeout,
		Interval: 2 * time.Second,
		TimeoutError: func() error {
			if lastErr != nil {
				return fmt.Errorf("move-to-drive cleanup target did not become visible within %s: %w", discoveryTimeout, lastErr)
			}
			return fmt.Errorf("move-to-drive cleanup target did not become visible within %s", discoveryTimeout)
		},
	}, func() (bool, error) {
		if taskID != "" {
			taskResult, taskErr := clie2e.RunCmd(discoveryCtx, clie2e.Request{
				Args: []string{
					"drive", "+task_result",
					"--scenario", "wiki_move_to_drive",
					"--task-id", taskID,
				},
				DefaultAs: "bot",
			})
			if taskResult != nil {
				lastResult = taskResult
			}
			if taskErr == nil && taskResult != nil && taskResult.ExitCode == 0 {
				parsed := gjson.Parse(taskResult.Stdout)
				if parsed.Get("data.failed").Bool() {
					return true, nil
				}
				if parsed.Get("data.ready").Bool() {
					resource := wikiMoveToDriveResource{
						Token: parsed.Get("data.obj_token").String(),
						Type:  parsed.Get("data.obj_type").String(),
					}
					if resource.Token != "" && resource.Type != "" {
						resources = []wikiMoveToDriveResource{resource}
						return true, nil
					}
				}
			} else if taskErr != nil {
				lastErr = taskErr
			}
		}

		listed, listResult, listErr := findWikiMoveToDriveResources(discoveryCtx, folderToken, name)
		if listResult != nil {
			lastResult = listResult
		}
		if listErr != nil {
			lastErr = listErr
			return false, nil //nolint:nilerr // retry transient cleanup discovery errors until the bounded timeout
		}
		if len(listed) == 0 {
			return false, nil
		}
		resources = listed
		return true, nil
	})
	return resources, lastResult, err
}

func findWikiMoveToDriveResources(
	ctx context.Context,
	folderToken string,
	name string,
) ([]wikiMoveToDriveResource, *clie2e.Result, error) {
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "files", "list",
			"--folder-token", folderToken,
			"--page-size", "200",
		},
		DefaultAs: "bot",
	})
	if err != nil {
		return nil, result, err
	}
	if result.ExitCode != 0 {
		return nil, result, fmt.Errorf(
			"list target folder failed: exit=%d stdout=%s stderr=%s",
			result.ExitCode,
			result.Stdout,
			result.Stderr,
		)
	}

	resources := []wikiMoveToDriveResource{}
	gjson.Get(result.Stdout, "data.files").ForEach(func(_, entry gjson.Result) bool {
		if entry.Get("name").String() != name {
			return true
		}
		resource := wikiMoveToDriveResource{
			Token: entry.Get("token").String(),
			Type:  entry.Get("type").String(),
		}
		if resource.Token != "" && resource.Type != "" {
			resources = append(resources, resource)
		}
		return true
	})
	return resources, result, nil
}

func waitWikiMoveToDriveReady(t *testing.T, ctx context.Context, initial *clie2e.Result) gjson.Result {
	t.Helper()

	current := gjson.Parse(initial.Stdout)
	taskID := current.Get("data.task_id").String()
	require.NotEmpty(t, taskID, "async move result must contain task_id; stdout:\n%s", initial.Stdout)

	queryTask := func() (bool, error) {
		result, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+task_result",
				"--scenario", "wiki_move_to_drive",
				"--task-id", taskID,
			},
			DefaultAs: "bot",
		})
		if runErr != nil {
			return false, runErr
		}
		if result.ExitCode != 0 {
			return false, fmt.Errorf(
				"query wiki move-to-drive task failed: exit=%d stdout=%s stderr=%s",
				result.ExitCode,
				result.Stdout,
				result.Stderr,
			)
		}
		current = gjson.Parse(result.Stdout)
		if current.Get("data.failed").Bool() {
			return false, fmt.Errorf(
				"wiki move-to-drive task %s failed: %s",
				taskID,
				current.Get("data.status_msg").String(),
			)
		}
		return current.Get("data.ready").Bool(), nil
	}

	ready, err := queryTask()
	require.NoError(t, err)
	if ready {
		return current
	}

	err = clie2e.WaitForCondition(ctx, clie2e.WaitOptions{
		Timeout:  90 * time.Second,
		Interval: 3 * time.Second,
		TimeoutError: func() error {
			return fmt.Errorf("wiki move-to-drive task %s did not finish", taskID)
		},
	}, queryTask)
	require.NoError(t, err)
	return current
}
