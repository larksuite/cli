// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const copyWorkbookFixture = `{"sheets":[{"sheet_id":"first","index":0},{"sheet_id":"source","index":1},{"sheet_id":"notes","index":2}]}`

func TestSheetCopyIndexMovesOnlyTheNewCopyAndReturnsVerifiedIndex(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, target := range []int{0, 1, 3} {
		t.Run(fmt.Sprint(target), func(t *testing.T) {
			preflight := toolOutputStub(testToken, "read", copyWorkbookFixture)
			copy := toolOutputStub(testToken, "write", `{"sheet_id":"new","sheet_name":"Copy","index":2,"row_count":200,"column_count":20}`)
			move := toolOutputStub(testToken, "write", `{}`)
			verify := toolOutputStub(testToken, "read", fmt.Sprintf(`{"sheets":[{"sheet_id":"new","index":%d}]}`, target))
			out, err := runShortcutWithStubs(t, SheetCopy, []string{"--url", testURL, "--sheet-id", "source", "--title", "Copy", "--index", fmt.Sprint(target)}, preflight, copy, move, verify)
			require.NoError(t, err)
			duplicateInput := gjson.GetBytes(copy.CapturedBody, "input").String()
			require.Equal(t, "duplicate", gjson.Get(duplicateInput, "operation").String())
			require.Equal(t, "source", gjson.Get(duplicateInput, "sheet_id").String())
			require.False(t, gjson.Get(duplicateInput, "target_index").Exists())
			moveInput := gjson.GetBytes(move.CapturedBody, "input").String()
			require.Equal(t, "move", gjson.Get(moveInput, "operation").String())
			require.Equal(t, "new", gjson.Get(moveInput, "sheet_id").String())
			require.Equal(t, int64(2), gjson.Get(moveInput, "source_index").Int())
			require.Equal(t, int64(target), gjson.Get(moveInput, "target_index").Int())
			require.Equal(t, "new", gjson.Get(out, "data.sheet_id").String())
			require.Equal(t, int64(target), gjson.Get(out, "data.index").Int())
			require.Equal(t, int64(200), gjson.Get(out, "data.row_count").Int())
		})
	}
}

func TestSheetCopyDefaultAndAlreadyPlacedDoNotMove(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, explicit := range []bool{false, true} {
		copy := toolOutputStub(testToken, "write", `{"sheet_id":"new","index":2}`)
		read := toolOutputStub(testToken, "read", copyWorkbookFixture)
		read.Optional = !explicit
		move := toolOutputStub(testToken, "write", `{}`)
		move.Optional = true
		args := []string{"--url", testURL, "--sheet-id", "source"}
		if explicit {
			args = append(args, "--index", "2")
		}
		out, err := runShortcutWithStubs(t, SheetCopy, args, read, copy, move)
		require.NoError(t, err)
		require.Equal(t, int64(2), gjson.Get(out, "data.index").Int())
		require.Empty(t, move.CapturedBodies)
		if !explicit {
			require.Empty(t, read.CapturedBodies)
		}
	}
}

func TestSheetCopyIndexRejectsInvalidBoundsBeforeCopy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, target := range []string{"-1", "4"} {
		read := toolOutputStub(testToken, "read", copyWorkbookFixture)
		read.Optional = target == "-1"
		write := toolOutputStub(testToken, "write", `{"sheet_id":"new","index":2}`)
		write.Optional = true
		out, err := runShortcutWithStubs(t, SheetCopy, []string{"--url", testURL, "--sheet-id", "source", "--index", target}, read, write)
		var validation *errs.ValidationError
		require.ErrorAs(t, err, &validation)
		require.Equal(t, "--index", validation.Param)
		require.Empty(t, write.CapturedBodies)
		require.Empty(t, out)
	}
}

func TestSheetCopyMalformedReceiptNeverMovesAnExistingSheet(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, receipt := range []string{`null`, `{}`, `{"sheet_id":"source","index":1}`, `{"sheet_id":"notes","index":2}`, `{"sheet_id":"new"}`, `{"sheet_id":"new","index":"bad"}`} {
		read := toolOutputStub(testToken, "read", copyWorkbookFixture)
		copy := toolOutputStub(testToken, "write", receipt)
		move := toolOutputStub(testToken, "write", `{}`)
		move.Optional = true
		out, err := runShortcutWithStubs(t, SheetCopy, []string{"--url", testURL, "--sheet-id", "source", "--index", "3"}, read, copy, move)
		var internal *errs.InternalError
		require.ErrorAs(t, err, &internal)
		require.Equal(t, errs.SubtypeInvalidResponse, internal.Subtype)
		require.Contains(t, internal.Hint, "instead of copying again")
		require.Empty(t, move.CapturedBodies)
		require.Empty(t, out)
		if receipt == `{"sheet_id":"new","index":"bad"}` {
			require.Error(t, internal.Unwrap())
		}
	}
}

func TestSheetCopyMoveFailurePreservesAPIErrorAndNamesExistingCopy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	read := toolOutputStub(testToken, "read", copyWorkbookFixture)
	copy := toolOutputStub(testToken, "write", `{"sheet_id":"new","index":2}`)
	move := toolOutputStub(testToken, "write", `{}`)
	move.Body = map[string]interface{}{"code": 1310246, "msg": "bad move", "log_id": "copy-move-log"}
	out, err := runShortcutWithStubs(t, SheetCopy, []string{"--url", testURL, "--sheet-id", "source", "--index", "3"}, read, copy, move)
	var api *errs.APIError
	require.ErrorAs(t, err, &api)
	require.Equal(t, errs.SubtypeInvalidParameters, api.Subtype)
	require.Equal(t, 1310246, api.Code)
	require.Equal(t, "copy-move-log", api.LogID)
	require.Contains(t, api.Hint, "--sheet-id new --index 3")
	require.Contains(t, api.Hint, "do not copy again")
	require.Empty(t, out)
}

func TestSheetCopyPostCreationFailuresCannotRetryTheWholeCommand(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, stage := range []string{"move", "readback"} {
		for _, kind := range []string{"rate_limit", "api_with_delay", "network_with_delay"} {
			t.Run(stage+"/"+kind, func(t *testing.T) {
				preflight := toolOutputStub(testToken, "read", copyWorkbookFixture)
				duplicate := toolOutputStub(testToken, "write", `{"sheet_id":"new","index":2}`)
				move := toolOutputStub(testToken, "write", `{}`)
				readback := toolOutputStub(testToken, "read", `{}`)
				failure := move
				if stage == "readback" {
					failure = readback
				} else {
					readback.Optional = true
				}
				failure.Reusable = true
				cause := errors.New("upstream failure")
				failure.Body = map[string]interface{}{"code": 99991400, "msg": "rate limited", "log_id": "copy-rate-log"}
				switch kind {
				case "api_with_delay":
					failure.Error = errs.NewAPIError(errs.SubtypeRateLimit, "rate limited").WithCode(99991400).
						WithLogID("copy-rate-log").WithRetryable().WithRetryAfterSeconds(7).WithCause(cause)
				case "network_with_delay":
					failure.Error = errs.NewNetworkError(errs.SubtypeNetworkTimeout, "request timed out").WithCode(503).
						WithLogID("copy-network-log").WithRetryable().WithRetryAfterSeconds(7).WithCause(cause)
				case "rate_limit":
					// Exercise the real API response classifier's retryable signal.
				default:
					t.Fatal("unknown error kind")
				}
				out, err := runShortcutWithStubs(t, SheetCopy, []string{"--url", testURL, "--sheet-id", "source", "--index", "3"}, preflight, duplicate, move, readback)
				require.Error(t, err)
				require.Empty(t, out)
				require.Len(t, duplicate.CapturedBodies, 1)
				require.False(t, errs.IsRetryable(err))
				_, hasDelay := errs.RetryAfter(err)
				require.False(t, hasDelay)
				problem, ok := errs.ProblemOf(err)
				require.True(t, ok)
				require.Contains(t, problem.Hint, "--sheet-id new --index 3")
				if kind == "network_with_delay" {
					var network *errs.NetworkError
					require.ErrorAs(t, err, &network)
					require.Equal(t, errs.SubtypeNetworkTimeout, network.Subtype)
					require.Equal(t, 503, network.Code)
					require.Equal(t, "copy-network-log", network.LogID)
					require.Zero(t, network.RetryAfterSeconds)
				} else {
					var api *errs.APIError
					require.ErrorAs(t, err, &api)
					require.Equal(t, errs.SubtypeRateLimit, api.Subtype)
					require.Equal(t, 99991400, api.Code)
					require.Equal(t, "copy-rate-log", api.LogID)
					require.Zero(t, api.RetryAfterSeconds)
				}
				if kind != "rate_limit" {
					require.ErrorIs(t, err, cause)
				}
				var wire bytes.Buffer
				require.True(t, output.WriteTypedErrorEnvelope(&wire, err, "user"))
				require.False(t, gjson.Get(wire.String(), "error.retryable").Exists())
				require.False(t, gjson.Get(wire.String(), "error.retry_after_seconds").Exists())
			})
		}
	}
}

func TestSheetCopyPreflightFailuresKeepRetryMetadataBeforeCreation(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cause := errors.New("read failed before any copy")
	read := toolOutputStub(testToken, "read", `{}`)
	read.Error = errs.NewAPIError(errs.SubtypeRateLimit, "rate limited").WithRetryable().WithRetryAfterSeconds(7).WithCause(cause)
	duplicate := toolOutputStub(testToken, "write", `{"sheet_id":"new","index":2}`)
	duplicate.Optional = true
	out, err := runShortcutWithStubs(t, SheetCopy, []string{"--url", testURL, "--sheet-id", "source", "--index", "3"}, read, duplicate)
	require.ErrorIs(t, err, cause)
	require.Empty(t, out)
	require.Empty(t, duplicate.CapturedBodies)
	require.True(t, errs.IsRetryable(err))
	_, hasDelay := errs.RetryAfter(err)
	require.True(t, hasDelay)
}

func TestSheetCopyDoesNotReportSuccessWhenMoveReadbackDisagrees(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	read := toolOutputStub(testToken, "read", copyWorkbookFixture)
	copy := toolOutputStub(testToken, "write", `{"sheet_id":"new","index":2}`)
	move := toolOutputStub(testToken, "write", `{}`)
	verify := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"new","index":2}]}`)
	out, err := runShortcutWithStubs(t, SheetCopy, []string{"--url", testURL, "--sheet-id", "source", "--index", "3"}, read, copy, move, verify)
	var internal *errs.InternalError
	require.ErrorAs(t, err, &internal)
	require.Equal(t, errs.SubtypeInvalidResponse, internal.Subtype)
	require.Contains(t, internal.Hint, "--sheet-id new --index 3")
	require.Empty(t, out)
}

func TestSheetCopyPositionIsExplicitlyUnsupportedInsideBatch(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	out, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+batch-update"), []string{"--url", testURL, "--operations", `[{"shortcut":"+sheet-copy","input":{"sheet_id":"source","index":3}}]`, "--dry-run"})
	var validation *errs.ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, "--operations", validation.Param)
	require.Contains(t, validation.Hint, "standalone +sheet-copy --index")
	require.Empty(t, out)
}
