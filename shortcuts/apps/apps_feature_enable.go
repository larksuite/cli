// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// featureEnableMaxWait is a variable only so tests can shorten it; production
// always uses featureEnableWaitLimit.
var featureEnableMaxWait = featureEnableWaitLimit

// AppsFeatureEnable enables an app business feature on the server and reports
// the committed switch. The request carries only an idempotent key, which also
// names the operation for later queries. It never edits local files, splits an
// existing database, restarts local processes, pushes code, or publishes.
var AppsFeatureEnable = common.Shortcut{
	Service:     appsService,
	Command:     "+feature-enable",
	Description: "Enable an app business feature on the server (database: the app's PostgreSQL database)",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +feature-enable --feature database --app-id <app_id>",
		"Example: lark-cli apps +feature-enable --feature database --app-id <app_id> --idempotent-key b3d93913-e777-4968-a00c-afd0a1c7ce44",
		"Tip: success means the server committed the feature; follow next_steps in the output: locally run `lark-cli apps +env-pull --app-id <app_id>` and restart the dev process, while in a Miaoda sandbox the platform applies the database by itself.",
	},
	// Resuming and waiting read the operation through the features endpoint,
	// so the command declares both scopes; the enable endpoint itself only
	// needs spark:app:write.
	Scopes:    []string{"spark:app:read", "spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "feature", Desc: "feature to enable", Required: true, Enum: []string{featurePG}},
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "idempotent-key", Desc: "UUID of an earlier attempt to resume the same request (default: a new UUID)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		// The framework enum check skips an empty value, so an explicit
		// --feature "" still needs this guard.
		if rctx.Str("feature") == "" {
			return appsValidationParamError("--feature", "--feature is required")
		}
		appID := strings.TrimSpace(rctx.Str("app-id"))
		if appID == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		if err := validateRealAppID(appID); err != nil {
			return err
		}
		return validateIdempotentKeyFlag(rctx)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		key := canonicalIdempotentKey(rctx.Str("idempotent-key"))
		dr := common.NewDryRunAPI().Desc("Enable an app feature on the server")
		if key == "" {
			key = "<generated at execution time>"
		} else {
			dr.GET(appFeaturesPath(appID)).
				Desc("Read the operation kept for this idempotent_key; a finished or running operation is reported without a new POST").
				Params(map[string]any{"operation_ref": key})
		}
		return dr.
			POST(appFeatureEnablePath(appID, rctx.Str("feature"))).
			Desc("Submit the enable; network failures replay the same idempotent_key").
			Body(map[string]any{"idempotent_key": key}).
			Set("wait", fmt.Sprintf("only when the server answers operation.state=running: poll GET %s?operation_ref=<idempotent_key> for at most %s", appFeaturesPath(appID), featureEnableWaitLimit))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return runFeatureEnable(rctx, strings.TrimSpace(rctx.Str("app-id")), rctx.Str("feature"), resolveIdempotentKey(rctx), rctx.Changed("idempotent-key"))
	},
}

// featureEnableOutput is the success payload: the committed server switch plus
// the key that identifies this request and the local steps still required.
type featureEnableOutput struct {
	AppID         string               `json:"app_id"`
	Feature       appFeatureStatus     `json:"feature"`
	Operation     *appFeatureOperation `json:"operation,omitempty"`
	IdempotentKey string               `json:"idempotent_key"`
	NextSteps     []string             `json:"next_steps"`
}

// runFeatureEnable performs one logical enable. resume is true when the caller
// passed --idempotent-key: the operation kept for that key is read first, so a
// request that already finished or is still running is reported as it is
// instead of being re-sent.
func runFeatureEnable(rctx *common.RuntimeContext, appID, feature, key string, resume bool) error {
	if resume {
		kept, err := readKeptFeatureOperation(rctx, appID, feature, key)
		if err != nil {
			if confirmFeatureAPIMissing(rctx, appID, err) {
				return explainFeatureAPIMissing(err, appID)
			}
			return err
		}
		// A retryable failure may continue: it is re-sent below with the same
		// key so the server resumes its known resource steps.
		if kept != nil && !(kept.Operation.State == "failed" && kept.Operation.Retryable != nil && *kept.Operation.Retryable) {
			return followKeptFeatureOperation(rctx, kept, appID, feature, key)
		}
	}

	body := map[string]any{"idempotent_key": key}
	data, err := callWithIdempotentReplay(rctx, "POST", appFeatureEnablePath(appID, feature), nil, body)
	if err != nil {
		if confirmFeatureAPIMissing(rctx, appID, err) {
			return explainFeatureAPIMissing(err, appID)
		}
		// The server answers 40913 while any enable holds the app, including
		// this key's own earlier attempt that a network replay raced. If the
		// server keeps an operation for this key, that operation is the answer.
		if featureStudioCode(problemCode(err)) == featureCodeKeyConflict {
			if kept, readErr := readKeptFeatureOperation(rctx, appID, feature, key); readErr == nil && kept != nil {
				return followKeptFeatureOperation(rctx, kept, appID, feature, key)
			}
		}
		return setFeatureRecoveryHint(err, appID, key)
	}
	result, err := parseFeatureEnableResult(data)
	if err == nil {
		err = checkEnableResultIdentity(result.AppID, result.Feature.Name, result.Operation, appID, feature, key)
	}
	if err != nil {
		return withUnverifiedAnswerHint(err, appID, key)
	}

	if result.Operation != nil && result.Operation.State == "running" {
		result, err = waitFeatureOperation(rctx, appID, feature, key)
		if err != nil {
			return err
		}
	}
	return finishFeatureEnable(rctx, result, key)
}

// followKeptFeatureOperation reports the operation the server keeps for key,
// waiting first while it is still running.
func followKeptFeatureOperation(rctx *common.RuntimeContext, kept *appFeatureEnableResult, appID, feature, key string) error {
	if kept.Operation.State != "running" {
		return finishFeatureEnable(rctx, kept, key)
	}
	result, err := waitFeatureOperation(rctx, appID, feature, key)
	if err != nil {
		return err
	}
	return finishFeatureEnable(rctx, result, key)
}

func problemCode(err error) int {
	if p, ok := errs.ProblemOf(err); ok {
		return p.Code
	}
	return 0
}

// readKeptFeatureOperation reads the operation the server keeps for key. It
// returns nil when the server reports no operation, so the caller re-sends the
// same request under the same key.
func readKeptFeatureOperation(rctx *common.RuntimeContext, appID, feature, key string) (*appFeatureEnableResult, error) {
	data, err := callWithIdempotentReplay(rctx, "GET", appFeaturesPath(appID), map[string]any{"operation_ref": key}, nil)
	if err != nil {
		return nil, setKeptOperationReadHint(err, appID, key)
	}
	current, err := parseFeaturesResult(data)
	if err != nil {
		return nil, err
	}
	status, ok := findFeature(current.Features, feature)
	if !ok {
		return nil, featureResponseError("features does not list %q", feature)
	}
	if current.Operation == nil {
		return nil, nil
	}
	if err := checkEnableResultIdentity(current.AppID, status.Name, current.Operation, appID, feature, key); err != nil {
		return nil, err
	}
	return &appFeatureEnableResult{AppID: current.AppID, Feature: status, Operation: current.Operation}, nil
}

// setKeptOperationReadHint replaces the hint of a failed kept-operation read.
// The message stays as the server or transport reported it. Only the server's
// explicit operation_unavailable answer (40915, or 40913 led by that reason)
// says the key is not kept, so only it may point to a new attempt; any other
// failure leaves the operation's outcome unknown and keeps the caller on the
// same key. Authentication, authorization and local validation failures keep
// their own guidance, as in setFeatureRecoveryHint.
func setKeptOperationReadHint(err error, appID, key string) error {
	p, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	if authErr, ok := featureAuthFailure(err); ok {
		return withFeatureAuthRecovery(authErr)
	}
	switch p.Category {
	case errs.CategoryAuthorization, errs.CategoryConfig, errs.CategoryValidation, errs.CategoryPolicy:
		return err
	}
	if explainFeatureKeyNotKept(err, appID, key) {
		return err
	}
	if featureStudioCode(p.Code) == featureCodeKeyConflict {
		switch {
		case strings.Contains(p.Message, featureReasonOperationUnavailable):
			p.Subtype = errs.SubtypeConflict
			p.Hint = featureKeyNotKeptHint(appID, key)
			return err
		case !strings.Contains(p.Message, featureReasonOperationInProgress):
			// Without the in-progress reason (including the gateway's fixed
			// label) the same key may never be answered differently, so the
			// hint must not send the caller round the same request again.
			p.Subtype = errs.SubtypeConflict
			p.Hint = featureConflictHint(appID, key)
			return err
		}
	}
	p.Hint = fmt.Sprintf("reading the operation of idempotent_key=%s failed, so its outcome is unknown; retry `%s` with the same key, or check it with `%s`; do not start a new request with another key while it is unknown", key, featureEnableRetryCommand(appID, key), featureQueryCommand(appID, key))
	return err
}

func checkEnableResultIdentity(gotApp, gotFeature string, op *appFeatureOperation, appID, feature, key string) error {
	if gotApp != appID {
		return featureResponseError("app_id %q does not match the requested app %q", gotApp, appID)
	}
	if gotFeature != feature {
		return featureResponseError("feature %q does not match the requested feature %q", gotFeature, feature)
	}
	if op != nil && op.Ref != key {
		return featureResponseError("operation.ref %q does not match idempotent_key %q", op.Ref, key)
	}
	return nil
}

// withUnverifiedAnswerHint keeps an invalid-response classification but tells
// the caller the request itself may have been applied.
func withUnverifiedAnswerHint(err error, appID, key string) error {
	if p, ok := errs.ProblemOf(err); ok {
		p.Hint = fmt.Sprintf("the enable request was sent but its answer could not be verified (idempotent_key=%s); check the state with `%s` before any new attempt", key, featureQueryCommand(appID, key))
	}
	return err
}

// waitFeatureOperation polls the read-only features endpoint while the server
// reports the operation as running. It never re-sends the write; stopping
// early leaves the server-side operation untouched.
func waitFeatureOperation(rctx *common.RuntimeContext, appID, feature, key string) (*appFeatureEnableResult, error) {
	waitCtx, cancel := context.WithTimeout(rctx.Ctx(), featureEnableMaxWait)
	defer cancel()
	fmt.Fprintf(rctx.IO().ErrOut, "-> %s enable is running (idempotent_key=%s); waiting up to %s\n", feature, key, featureEnableMaxWait)
	params := map[string]any{"operation_ref": key}
	for {
		select {
		case <-waitCtx.Done():
			return nil, featureWaitStoppedError(waitCtx.Err(), appID, feature, key)
		case <-time.After(featureEnablePollInterval):
		}
		data, err := callWithIdempotentReplay(rctx, "GET", appFeaturesPath(appID), params, nil)
		if err != nil {
			// The operation is already running on the server, so a failed
			// read leaves its outcome unknown rather than rejecting a request.
			return nil, setKeptOperationReadHint(err, appID, key)
		}
		res, err := parseFeaturesResult(data)
		if err != nil {
			return nil, withUnverifiedAnswerHint(err, appID, key)
		}
		status, ok := findFeature(res.Features, feature)
		if !ok {
			return nil, withUnverifiedAnswerHint(featureResponseError("features does not list %q", feature), appID, key)
		}
		op := res.Operation
		if op == nil {
			return nil, errs.NewAPIError(errs.SubtypeUnknown, "operation %s is no longer reported for app %s; its result is unknown", key, appID).
				WithHint("check the current state with `lark-cli apps +features --app-id %s`; do not assume the enable succeeded or was rolled back", appID)
		}
		if err := checkEnableResultIdentity(res.AppID, status.Name, op, appID, feature, key); err != nil {
			return nil, withUnverifiedAnswerHint(err, appID, key)
		}
		if op.State == "running" {
			continue
		}
		return &appFeatureEnableResult{AppID: res.AppID, Feature: status, Operation: op}, nil
	}
}

func featureWaitStoppedError(cause error, appID, feature, key string) error {
	query := featureQueryCommand(appID, key)
	resume := featureEnableRetryCommand(appID, key)
	if errors.Is(cause, context.DeadlineExceeded) {
		return errs.NewNetworkError(errs.SubtypeNetworkTimeout,
			"%s enable (idempotent_key=%s) is still running after %s; its result is not known yet", feature, key, featureEnableMaxWait).
			WithHint("the operation was not cancelled or rolled back; check it with `%s`, or resume waiting with `%s`", query, resume).
			WithCause(cause)
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport,
		"stopped waiting for %s enable (idempotent_key=%s): %v", feature, key, cause).
		WithHint("the server-side operation was not cancelled or rolled back; check it with `%s`", query).
		WithCause(cause)
}

func finishFeatureEnable(rctx *common.RuntimeContext, result *appFeatureEnableResult, key string) error {
	appID, op := result.AppID, result.Operation
	if op != nil {
		query := featureQueryCommand(appID, key)
		reason := reasonOrUnknown(op.ErrorReason)
		switch op.State {
		case "failed":
			e := errs.NewAPIError(errs.SubtypeUnknown, "%s enable failed for app %s: %s", result.Feature.Name, appID, reason)
			if op.Retryable != nil && *op.Retryable {
				return e.WithRetryable().WithHint("the feature was not committed (idempotent_key=%s); resume the same request with `%s`", key, featureEnableRetryCommand(appID, key))
			}
			return e.WithHint("the feature was not committed (idempotent_key=%s); check the state with `%s`", key, query)
		case "unknown":
			return errs.NewAPIError(errs.SubtypeUnknown, "%s enable result is unknown for app %s: %s", result.Feature.Name, appID, reason).
				WithHint("the resource request outcome is unknown (idempotent_key=%s); check it with `%s`; do not start a new request with another key while it is unknown", key, query)
		case "conflicted":
			return errs.NewAPIError(errs.SubtypeConflict, "%s enable conflicted with a newer change for app %s: %s", result.Feature.Name, appID, reason).
				WithHint("the app's features changed concurrently (idempotent_key=%s); review `lark-cli apps +features --app-id %s` and decide again; nothing was overwritten", key, appID)
		}
	}
	if !result.Feature.Enabled {
		return withUnverifiedAnswerHint(featureResponseError("the enable answered success but feature %s is not enabled", result.Feature.Name), appID, key)
	}
	out := &featureEnableOutput{
		AppID:         appID,
		Feature:       result.Feature,
		Operation:     op,
		IdempotentKey: key,
		NextSteps:     featureEnableNextSteps(appID),
	}
	rctx.OutFormat(out, nil, func(w io.Writer) {
		fmt.Fprintf(w, "✓ %s enabled on the server for %s\n", out.Feature.Name, appID)
		fmt.Fprintln(w, "Next:")
		for _, step := range out.NextSteps {
			fmt.Fprintf(w, "  - %s\n", step)
		}
	})
	return nil
}

// envKeyMiaodaAuthnCode carries the credential the Miaoda sandbox build of
// lark-cli authenticates with. Every Miaoda sandbox has it and a local install
// does not, which makes it the marker for where this command runs.
const envKeyMiaodaAuthnCode = "MIAODA_AUTHN_CODE"

func inMiaodaSandbox() bool {
	return strings.TrimSpace(os.Getenv(envKeyMiaodaAuthnCode)) != ""
}

// featureEnableNextSteps states what remains after the server commit. In a
// Miaoda sandbox the platform applies the new database to the app, and the
// sandbox build has no +env-pull. Locally the commit alone never makes a
// running process use the database, and an enabled switch does not mean a dev
// database exists for local development: +env-pull reports that through the
// env API.
func featureEnableNextSteps(appID string) []string {
	if inMiaodaSandbox() {
		return []string{
			"in a Miaoda sandbox the platform applies the new database settings to the app by itself; nothing needs to be pulled or restarted by hand",
			"wait a moment, then verify one real database read/write; if the database is not reachable yet, wait a little and try again before reporting a problem",
		}
	}
	return []string{
		fmt.Sprintf("in the local project run `lark-cli apps +env-pull --app-id %s`", appID),
		"restart the whole local dev process yourself (stop it, then `npm run dev`) so the new database settings are loaded; nothing restarts it automatically",
		fmt.Sprintf("if +env-pull reports the app has no dev database environment, do not use the online database locally; creating dev splits the existing database and is irreversible, so only after the user confirms, preview `lark-cli apps +db-env-create --app-id %s --environment dev --dry-run`", appID),
		"verify one real database read/write before reporting the database as ready",
	}
}

func reasonOrUnknown(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "no reason given"
	}
	return reason
}
