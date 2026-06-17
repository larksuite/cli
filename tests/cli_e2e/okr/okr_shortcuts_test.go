// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// --- Dry-run E2E tests for +batch-create, +reorder, +weight ---

// TestOKR_BatchCreateDryRun validates +batch-create dry-run output contains expected API paths.
func TestOKR_BatchCreateDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+batch-create",
			"--cycle-id", "123456",
			"--input", `[{"text":"Objective 1","krs":[{"text":"KR 1"}]}]`,
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v2/cycles/123456/objectives"), "dry-run should contain objective API path, got: %s", output)
	assert.True(t, strings.Contains(output, "/open-apis/okr/v2/objectives/"), "dry-run should contain KR API path prefix, got: %s", output)
	assert.True(t, strings.Contains(output, "123456"), "dry-run should contain cycle-id, got: %s", output)
}

// TestOKR_BatchCreateDryRun_WithUserIDType validates +batch-create dry-run with --user-id-type.
func TestOKR_BatchCreateDryRun_WithUserIDType(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+batch-create",
			"--cycle-id", "123456",
			"--input", `[{"text":"Objective 1","krs":[{"text":"KR 1"}]}]`,
			"--user-id-type", "user_id",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "user_id"), "dry-run should contain user-id-type, got: %s", output)
}

// TestOKR_ReorderDryRun validates +reorder dry-run output contains expected API paths.
func TestOKR_ReorderDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+reorder",
			"--cycle-id", "123456",
			"--level", "objective",
			"--ops", `[{"id":"obj_1","position":2},{"id":"obj_2","position":1}]`,
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v2/cycles/123456/objectives"), "dry-run should contain objective API path, got: %s", output)
	assert.True(t, strings.Contains(output, "123456"), "dry-run should contain cycle-id, got: %s", output)
}

// TestOKR_ReorderDryRun_KR validates +reorder dry-run with --level=key-result.
func TestOKR_ReorderDryRun_KR(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+reorder",
			"--cycle-id", "123456",
			"--objective-id", "789",
			"--level", "key-result",
			"--ops", `[{"id":"1001","position":2},{"id":"1002","position":1}]`,
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v2/objectives/789/key_results"), "dry-run should contain KR API path, got: %s", output)
	assert.True(t, strings.Contains(output, "789"), "dry-run should contain objective-id, got: %s", output)
}

// TestOKR_WeightDryRun validates +weight dry-run output contains expected API paths.
func TestOKR_WeightDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+weight",
			"--cycle-id", "123456",
			"--level", "objective",
			"--weights", `[{"id":"obj_1","weight":0.6},{"id":"obj_2","weight":0.4}]`,
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v2/cycles/123456/objectives"), "dry-run should contain objective API path, got: %s", output)
	assert.True(t, strings.Contains(output, "123456"), "dry-run should contain cycle-id, got: %s", output)
}

// TestOKR_WeightDryRun_KR validates +weight dry-run with --level=key-result.
func TestOKR_WeightDryRun_KR(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+weight",
			"--cycle-id", "123456",
			"--objective-id", "789",
			"--level", "key-result",
			"--weights", `[{"id":"1001","weight":0.5},{"id":"1002","weight":0.5}]`,
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := result.Stdout
	assert.True(t, strings.Contains(output, "/open-apis/okr/v2/objectives/789/key_results"), "dry-run should contain KR API path, got: %s", output)
	assert.True(t, strings.Contains(output, "789"), "dry-run should contain objective-id, got: %s", output)
}

// --- Live E2E tests (require user token, skip otherwise) ---

// getTestCycleID returns the test cycle ID from env var, or skips the test.
func getTestCycleID(t *testing.T) string {
	t.Helper()
	cycleID := os.Getenv("OKR_TEST_CYCLE_ID")
	if cycleID == "" {
		t.Skip("OKR_TEST_CYCLE_ID not set; set to a valid cycle ID for live E2E tests")
	}
	return cycleID
}

// liveTestCreated tracks resources created during a live test for cleanup.
type liveTestCreated struct {
	ObjectiveID string
	KRIDs       []string
}

// createTestObjectives creates test objectives using +batch-create and returns the created IDs.
func createTestObjectives(t *testing.T, ctx context.Context, cycleID string, suffix string) []liveTestCreated {
	t.Helper()

	input := []map[string]interface{}{
		{
			"text": fmt.Sprintf("E2E Test Objective A %s", suffix),
			"krs": []map[string]interface{}{
				{"text": fmt.Sprintf("E2E Test KR A1 %s", suffix)},
				{"text": fmt.Sprintf("E2E Test KR A2 %s", suffix)},
			},
		},
		{
			"text": fmt.Sprintf("E2E Test Objective B %s", suffix),
			"krs": []map[string]interface{}{
				{"text": fmt.Sprintf("E2E Test KR B1 %s", suffix)},
			},
		},
	}
	inputJSON, _ := json.Marshal(input)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+batch-create",
			"--cycle-id", cycleID,
			"--input", string(inputJSON),
		},
	})
	require.NoError(t, err, "failed to create test objectives")
	result.AssertExitCode(t, 0)

	var created []liveTestCreated
	createdArr := gjson.Get(result.Stdout, "data.created").Array()
	for _, obj := range createdArr {
		objectiveID := obj.Get("objective_id").String()
		var krIDs []string
		for _, kr := range obj.Get("krs").Array() {
			krIDs = append(krIDs, kr.String())
		}
		created = append(created, liveTestCreated{
			ObjectiveID: objectiveID,
			KRIDs:       krIDs,
		})
	}

	require.Len(t, created, 2, "expected 2 objectives created")
	require.Len(t, created[0].KRIDs, 2, "expected 2 KRs for first objective")
	require.Len(t, created[1].KRIDs, 1, "expected 1 KR for second objective")
	require.NotEmpty(t, created[0].ObjectiveID, "objective_id should not be empty")
	require.NotEmpty(t, created[0].KRIDs[0], "kr_id should not be empty")

	return created
}

// cleanupLiveTest deletes KRs first, then objectives, using the raw API service commands.
func cleanupLiveTest(t *testing.T, created []liveTestCreated) {
	t.Helper()
	cleanupCtx, cleanupCancel := clie2e.CleanupContext()
	defer cleanupCancel()

	// Delete in reverse order: KRs first, then objectives
	for i := len(created) - 1; i >= 0; i-- {
		obj := created[i]
		// Delete KRs first (reverse order)
		for j := len(obj.KRIDs) - 1; j >= 0; j-- {
			krID := obj.KRIDs[j]
			result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args: []string{
					"okr", "v2/key_results", "delete",
					"--key-result-id", krID,
					"--yes",
				},
			})
			clie2e.ReportCleanupFailure(t, fmt.Sprintf("delete KR %s", krID), result, err)
			select {
			case <-cleanupCtx.Done():
				return
			case <-time.After(200 * time.Millisecond):
			}
		}
		// Then delete the objective
		result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{
				"okr", "v2/objectives", "delete",
				"--objective-id", obj.ObjectiveID,
				"--yes",
			},
		})
		clie2e.ReportCleanupFailure(t, fmt.Sprintf("delete objective %s", obj.ObjectiveID), result, err)
		if i > 0 {
			select {
			case <-cleanupCtx.Done():
				return
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
}

// TestOKR_BatchCreateLive validates +batch-create with real API calls: create, verify, cleanup.
func TestOKR_BatchCreateLive(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)
	cycleID := getTestCycleID(t)
	suffix := clie2e.GenerateSuffix()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	// Create test objectives
	created := createTestObjectives(t, ctx, cycleID, suffix)

	// Register cleanup immediately after create to ensure resources are cleaned up even if later code fails
	t.Cleanup(func() {
		cleanupLiveTest(t, created)
	})

	// Verify: call +cycle-detail to confirm objectives exist
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+cycle-detail",
			"--cycle-id", cycleID,
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	objectives := gjson.Get(result.Stdout, "data.objectives").Array()
	foundCount := 0
	for _, obj := range objectives {
		objID := obj.Get("id").String()
		for _, c := range created {
			if objID == c.ObjectiveID {
				foundCount++
				// Verify KRs exist under this objective
				krs := obj.Get("key_results").Array()
				krIDs := make(map[string]bool)
				for _, kr := range krs {
					krIDs[kr.Get("id").String()] = true
				}
				for _, expectedKR := range c.KRIDs {
					assert.True(t, krIDs[expectedKR], "expected KR %s to exist under objective %s", expectedKR, objID)
				}
			}
		}
	}
	assert.Equal(t, len(created), foundCount, "all created objectives should be found in cycle detail")
}

// TestOKR_ReorderLive validates +reorder with real API calls: create, reorder, verify, cleanup.
func TestOKR_ReorderLive(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)
	cycleID := getTestCycleID(t)
	suffix := clie2e.GenerateSuffix()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	// Create test objectives (A, then B)
	created := createTestObjectives(t, ctx, cycleID, suffix)

	// Register cleanup immediately after create to ensure resources are cleaned up even if later code fails
	t.Cleanup(func() {
		cleanupLiveTest(t, created)
	})

	objA := created[0].ObjectiveID
	objB := created[1].ObjectiveID

	// Reorder: swap positions (B at position 1, A at position 2)
	ops := []map[string]interface{}{
		{"id": objB, "position": 1},
		{"id": objA, "position": 2},
	}
	opsJSON, _ := json.Marshal(ops)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+reorder",
			"--cycle-id", cycleID,
			"--level", "objective",
			"--ops", string(opsJSON),
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	// Verify order via +cycle-detail
	result, err = clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+cycle-detail",
			"--cycle-id", cycleID,
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	objectives := gjson.Get(result.Stdout, "data.objectives").Array()
	var foundIDs []string
	for _, obj := range objectives {
		objID := obj.Get("id").String()
		if objID == objA || objID == objB {
			foundIDs = append(foundIDs, objID)
		}
	}

	require.Len(t, foundIDs, 2, "should find both test objectives")
	assert.Equal(t, objB, foundIDs[0], "after reorder, objective B should be first")
	assert.Equal(t, objA, foundIDs[1], "after reorder, objective A should be second")
}

// TestOKR_WeightLive validates +weight with real API calls: create, set weights, verify sum=1.0, cleanup.
func TestOKR_WeightLive(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)
	cycleID := getTestCycleID(t)
	suffix := clie2e.GenerateSuffix()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	// Create test objectives
	created := createTestObjectives(t, ctx, cycleID, suffix)

	// Register cleanup immediately after create to ensure resources are cleaned up even if later code fails
	t.Cleanup(func() {
		cleanupLiveTest(t, created)
	})

	objA := created[0].ObjectiveID
	objB := created[1].ObjectiveID

	// Set weights: A=0.6, B=0.4 (sum=1.0)
	weights := []map[string]interface{}{
		{"id": objA, "weight": 0.6},
		{"id": objB, "weight": 0.4},
	}
	weightsJSON, _ := json.Marshal(weights)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+weight",
			"--cycle-id", cycleID,
			"--level", "objective",
			"--weights", string(weightsJSON),
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	// Verify weights via +cycle-detail
	result, err = clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"okr", "+cycle-detail",
			"--cycle-id", cycleID,
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	objectives := gjson.Get(result.Stdout, "data.objectives").Array()
	var weightA, weightB float64
	for _, obj := range objectives {
		objID := obj.Get("id").String()
		if objID == objA {
			weightA = obj.Get("weight").Float()
		} else if objID == objB {
			weightB = obj.Get("weight").Float()
		}
	}

	// Verify weights are set correctly (allowing for floating point tolerance)
	assert.InDelta(t, 0.6, weightA, 0.001, "objective A weight should be 0.6")
	assert.InDelta(t, 0.4, weightB, 0.001, "objective B weight should be 0.4")

	// Verify sum = 1.0
	sumWeights := weightA + weightB
	assert.InDelta(t, 1.0, sumWeights, 0.001, "sum of weights should be 1.0, got %.6f", sumWeights)
}
