// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// featurePG is the only feature +feature-enable accepts in this release: the
// app's database (PostgreSQL). The public name says what the app gets, not
// which engine backs it.
const featurePG = "database"

// featureEnableWaitLimit bounds how long +feature-enable waits for an operation
// the server reports as running. The server executes the enable synchronously,
// so running only appears when a persistent executor really took the work over.
const featureEnableWaitLimit = 60 * time.Second

// featureEnablePollInterval and featureRetryBackoff are variables only so tests
// can run without real sleeps.
var (
	featureEnablePollInterval = 2 * time.Second
	// featureRetryBackoff is the delay before each replay of a request that
	// failed on the network. The replay carries the identical body, including
	// idempotent_key, so the server deduplicates it instead of acting twice.
	featureRetryBackoff = []time.Duration{time.Second, 2 * time.Second}
)

// idempotentKeyPattern is the canonical hyphenated UUID form the server stores.
var idempotentKeyPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

var featureOperationStates = []string{"running", "succeeded", "failed", "unknown", "conflicted"}

// Studio business codes of the app feature API. They select the subtype, the
// wording and the recovery the commands report; the category and exit code
// still come from errclass.
const (
	featureCodeInvalidArgument      = 40001 // request parameters rejected; nothing was changed
	featureCodeFeatureUnsupported   = 40009 // the server does not offer this feature name
	featureCodeRevisionChanged      = 40912 // the app configuration changed while enabling
	featureCodeKeyConflict          = 40913 // operation conflict; the reason leads the message
	featureCodeEnableUnsupported    = 40914 // the app cannot enable features (not a new-contract app)
	featureCodeOperationUnavailable = 40915 // the key names no operation this caller can follow
	featureCodeRateLimited          = 42900 // per-API rate limit, checked before anything else
	featureCodeResultUnknown        = 50008 // resource request outcome is unknown
)

// featureCodeGatewayInternal is the OpenAPI gateway's own "Internal Error"
// (HTTP 500): the upstream call failed without a business answer. A server
// that does not implement the app features API answers every call this way,
// and so can a passing upstream failure; the response carries nothing that
// tells them apart, so only a read-only re-check failing the same way does.
// A gateway that has no route for the API at all answers a bare HTTP 404
// instead; see isFeatureAPIAbsentAnswer.
const featureCodeGatewayInternal = 2200

// featureGatewayCodeBase is what the OpenAPI gateway adds to a Studio code:
// 40914 reaches the CLI as 3340914. A bare Studio code is read the same way.
const featureGatewayCodeBase = 3300000

// featureStudioCode returns the Studio code behind a gateway code.
func featureStudioCode(code int) int {
	if code > featureGatewayCodeBase && code < featureGatewayCodeBase+100000 {
		return code - featureGatewayCodeBase
	}
	return code
}

// Reasons the server writes at the start of a 40913 or 40914 message. The
// gateway may answer with its fixed label instead, so a reason only sharpens
// the wording; a message without one gets guidance that holds for all of them.
const (
	featureReasonOperationInProgress = "operation_in_progress"
	featureReasonOperationSuperseded = "operation_superseded"
	// featureReasonOperationUnavailable means the queried operation_ref is
	// superseded, another caller's, or was never created for this app. Studio
	// answers it as 40915; a 40913 led by this reason means the same.
	featureReasonOperationUnavailable = "operation_unavailable"
	featureReasonAppIdentityMissing   = "app_identity_missing"
)

// appFeatureStatus is the public switch of one app feature: its name and
// whether it is enabled. It says nothing about resources or local readiness —
// enabled=true does not mean a dev database exists for local development.
type appFeatureStatus struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// appFeatureOperation projects the one enable intent the server still keeps.
type appFeatureOperation struct {
	Ref         string `json:"ref"`
	State       string `json:"state"`
	ErrorReason string `json:"error_reason,omitempty"`
	Retryable   *bool  `json:"retryable,omitempty"`
}

// appFeaturesResult is GET /apps/:app_id/features.
type appFeaturesResult struct {
	AppID     string               `json:"app_id"`
	Features  []appFeatureStatus   `json:"features"`
	Operation *appFeatureOperation `json:"operation,omitempty"`
}

// appFeatureEnableResult is POST /apps/:app_id/features/:feature_name/enable.
type appFeatureEnableResult struct {
	AppID     string               `json:"app_id"`
	Feature   appFeatureStatus     `json:"feature"`
	Operation *appFeatureOperation `json:"operation,omitempty"`
}

// Wire shapes: pointers distinguish an absent field from its zero value, so a
// missing bool is a malformed answer instead of a silent false. Fields the
// server may still send beyond name/enabled are dropped, never projected.
type wireFeatureStatus struct {
	Name    *string `json:"name"`
	Enabled *bool   `json:"enabled"`
}

type wireFeatureOperation struct {
	Ref         *string `json:"ref"`
	State       *string `json:"state"`
	ErrorReason *string `json:"error_reason"`
	Retryable   *bool   `json:"retryable"`
}

type wireFeaturesResult struct {
	AppID     *string               `json:"app_id"`
	Features  *[]wireFeatureStatus  `json:"features"`
	Operation *wireFeatureOperation `json:"operation"`
}

type wireFeatureEnableResult struct {
	AppID     *string               `json:"app_id"`
	Feature   *wireFeatureStatus    `json:"feature"`
	Operation *wireFeatureOperation `json:"operation"`
}

func featureResponseError(format string, args ...any) *errs.InternalError {
	return appsSubprocessEnvelopeError("app features response: "+format, args...).
		WithHint("the server answer does not match the app features API; nothing was retried or replaced by another request — report the log_id to the app platform")
}

// decodeFeatureWire re-decodes the classified "data" object into a wire struct
// so a field of the wrong type fails instead of being coerced.
func decodeFeatureWire(data map[string]any, dst any) error {
	if data == nil {
		return featureResponseError("data object is missing")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return featureResponseError("cannot re-encode data: %v", err).WithCause(err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return featureResponseError("unexpected field type: %v", err).WithCause(err)
	}
	return nil
}

func requireNonEmptyString(field string, v *string) (string, error) {
	if v == nil {
		return "", featureResponseError("%s is missing", field)
	}
	if strings.TrimSpace(*v) == "" {
		return "", featureResponseError("%s is empty", field)
	}
	return *v, nil
}

func projectFeatureStatus(field string, w *wireFeatureStatus) (appFeatureStatus, error) {
	if w == nil {
		return appFeatureStatus{}, featureResponseError("%s is missing", field)
	}
	name, err := requireNonEmptyString(field+".name", w.Name)
	if err != nil {
		return appFeatureStatus{}, err
	}
	if w.Enabled == nil {
		return appFeatureStatus{}, featureResponseError("%s.enabled is missing", field)
	}
	return appFeatureStatus{Name: name, Enabled: *w.Enabled}, nil
}

func projectFeatureList(w *[]wireFeatureStatus) ([]appFeatureStatus, error) {
	if w == nil {
		return nil, featureResponseError("features is missing")
	}
	out := make([]appFeatureStatus, 0, len(*w))
	for i := range *w {
		status, err := projectFeatureStatus(fmt.Sprintf("features[%d]", i), &(*w)[i])
		if err != nil {
			return nil, err
		}
		out = append(out, status)
	}
	return out, nil
}

func projectFeatureOperation(w *wireFeatureOperation) (*appFeatureOperation, error) {
	if w == nil {
		return nil, nil
	}
	ref, err := requireNonEmptyString("operation.ref", w.Ref)
	if err != nil {
		return nil, err
	}
	state, err := requireNonEmptyString("operation.state", w.State)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(featureOperationStates, state) {
		return nil, featureResponseError("operation.state %q is not one of %s", state, strings.Join(featureOperationStates, ", "))
	}
	op := &appFeatureOperation{Ref: ref, State: state, Retryable: w.Retryable}
	if w.ErrorReason != nil {
		op.ErrorReason = *w.ErrorReason
	}
	return op, nil
}

func parseFeaturesResult(data map[string]any) (*appFeaturesResult, error) {
	var w wireFeaturesResult
	if err := decodeFeatureWire(data, &w); err != nil {
		return nil, err
	}
	appID, err := requireNonEmptyString("app_id", w.AppID)
	if err != nil {
		return nil, err
	}
	features, err := projectFeatureList(w.Features)
	if err != nil {
		return nil, err
	}
	op, err := projectFeatureOperation(w.Operation)
	if err != nil {
		return nil, err
	}
	return &appFeaturesResult{AppID: appID, Features: features, Operation: op}, nil
}

func parseFeatureEnableResult(data map[string]any) (*appFeatureEnableResult, error) {
	var w wireFeatureEnableResult
	if err := decodeFeatureWire(data, &w); err != nil {
		return nil, err
	}
	appID, err := requireNonEmptyString("app_id", w.AppID)
	if err != nil {
		return nil, err
	}
	feature, err := projectFeatureStatus("feature", w.Feature)
	if err != nil {
		return nil, err
	}
	op, err := projectFeatureOperation(w.Operation)
	if err != nil {
		return nil, err
	}
	return &appFeatureEnableResult{AppID: appID, Feature: feature, Operation: op}, nil
}

func findFeature(features []appFeatureStatus, name string) (appFeatureStatus, bool) {
	for _, f := range features {
		if f.Name == name {
			return f, true
		}
	}
	return appFeatureStatus{}, false
}

// resolveIdempotentKey returns the caller's --idempotent-key in canonical
// form, or a fresh UUID when the flag is absent. A generated key is fixed for
// the whole command, so every in-command replay sends the same value.
func resolveIdempotentKey(rctx *common.RuntimeContext) string {
	if key := canonicalIdempotentKey(rctx.Str("idempotent-key")); key != "" {
		return key
	}
	return uuid.NewString()
}

// canonicalIdempotentKey lower-cases a key the way the server stores it, so
// the operation ref it echoes back matches the key this command sent.
func canonicalIdempotentKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func validateIdempotentKeyFlag(rctx *common.RuntimeContext) error {
	if !rctx.Changed("idempotent-key") {
		return nil
	}
	if !idempotentKeyPattern.MatchString(strings.TrimSpace(rctx.Str("idempotent-key"))) {
		return appsValidationParamError("--idempotent-key", "--idempotent-key must be a UUID such as 0e54c2cd-211f-48e2-88cb-736930c0d001").
			WithHint("reuse the idempotent_key printed by the failed attempt, or omit the flag to start a new request")
	}
	return nil
}

// callWithIdempotentReplay issues one logical request and replays the identical
// body after a network failure. Only network failures are replayed; a server
// answer (including a conflict) is returned as is. ctx bounds every attempt and
// every backoff, so a caller's deadline also covers slow requests and replays.
func callWithIdempotentReplay(ctx context.Context, rctx *common.RuntimeContext, method, url string, params map[string]any, body any) (map[string]any, error) {
	data, _, err := callWithIdempotentReplayAttempts(ctx, rctx, method, url, params, body)
	return data, err
}

// callWithIdempotentReplayAttempts is callWithIdempotentReplay that also
// reports how many requests it sent. More than one means an earlier attempt
// failed on the network after it may already have reached the server.
func callWithIdempotentReplayAttempts(ctx context.Context, rctx *common.RuntimeContext, method, url string, params map[string]any, body any) (map[string]any, int, error) {
	for attempt := 0; ; attempt++ {
		data, err := callFeatureAPI(ctx, rctx, method, url, params, body)
		if err == nil || !errs.IsNetwork(err) || attempt >= len(featureRetryBackoff) || ctx.Err() != nil {
			return data, attempt + 1, err
		}
		select {
		case <-ctx.Done():
			return data, attempt + 1, err
		case <-time.After(featureRetryBackoff[attempt]):
		}
	}
}

// callFeatureAPI sends one request to an app features endpoint under ctx and
// classifies the answer the way CallAPITyped does, with one addition: a 404
// whose body is no API answer is reported as the HTTP 404 it was. The shared
// classifier reports any non-object JSON body as an invalid response without
// its status, and a gateway's plain-text "404 page not found" decodes as the
// JSON number 404, so without the status a missing route and a 200 answer
// with a null body would look the same.
func callFeatureAPI(ctx context.Context, rctx *common.RuntimeContext, method, url string, params map[string]any, body any) (map[string]any, error) {
	req := &larkcore.ApiReq{HttpMethod: method, ApiPath: url, QueryParams: larkcore.QueryParams{}}
	for k, v := range params {
		req.QueryParams.Set(k, fmt.Sprint(v))
	}
	if body != nil {
		req.Body = body
	}
	resp, err := rctx.DoAPIWithContext(ctx, req)
	if err != nil {
		if _, ok := errs.ProblemOf(err); ok {
			return nil, err
		}
		return nil, errs.WrapInternal(err)
	}
	data, err := rctx.ClassifyAPIResponse(resp)
	if err != nil && resp.StatusCode == http.StatusNotFound && isInvalidFeatureResponse(err) {
		return nil, featureHTTP404Error(resp, err)
	}
	return data, err
}

// isInvalidFeatureResponse reports a body that is no API envelope, such as a
// JSON null or a bare number.
func isInvalidFeatureResponse(err error) bool {
	p, ok := errs.ProblemOf(err)
	return ok && p.Category == errs.CategoryInternal && p.Subtype == errs.SubtypeInvalidResponse
}

// featureHTTP404Error reports a 404 without an API answer the way the shared
// classifier reports one whose body does not decode at all.
func featureHTTP404Error(resp *larkcore.ApiResp, cause error) error {
	e := errs.NewAPIError(errs.SubtypeNotFound, "HTTP %d: %s", resp.StatusCode, common.TruncateStr(strings.TrimSpace(string(resp.RawBody)), 500)).
		WithCode(resp.StatusCode).
		WithCause(cause)
	if p, ok := errs.ProblemOf(cause); ok && p.LogID != "" {
		e = e.WithLogID(p.LogID)
	}
	return e
}

func appFeaturesPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/features", apiBasePath, validate.EncodePathSegment(appID))
}

func appFeatureEnablePath(appID, feature string) string {
	return fmt.Sprintf("%s/apps/%s/features/%s/enable", apiBasePath, validate.EncodePathSegment(appID), validate.EncodePathSegment(feature))
}

func featureQueryCommand(appID, key string) string {
	return fmt.Sprintf("lark-cli apps +features --app-id %s --operation-ref %s", appID, key)
}

func featureEnableRetryCommand(appID, key string) string {
	return fmt.Sprintf("lark-cli apps +feature-enable --feature %s --app-id %s --idempotent-key %s", featurePG, appID, key)
}

// setFeatureRecoveryHint explains a failed enable request and what to do next.
// A server answer that is final for this app or this key gets its own subtype
// and wording and never points at the operation query; only outcomes that may
// still change (network failures, server errors, an unknown resource result)
// name the original key, the read-only query and the same-key resume.
// Authentication, authorization and local validation failures keep their own
// guidance: those are fixed by the caller, not by replaying the request.
func setFeatureRecoveryHint(err error, appID, key string) error {
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
	if explainFeatureRejection(p, appID, key) {
		return err
	}
	if isFeatureNonAPIAnswer(p) {
		// The gateway serves the features read but has no route for the
		// enable: no key can reach the API, so a same-key resume would only
		// repeat the 404.
		p, err = asFeatureServerAnswer(err)
		p.Subtype = errs.SubtypeFeatureNotAvailable
		p.Retryable = false
		p.Hint = "do not retry: the gateway has no route for enabling app features yet. Ask for the server to be upgraded, or enable the database for this app in Miaoda cloud development; report the log_id if app features should already be available"
		return err
	}
	query := featureQueryCommand(appID, key)
	switch {
	case featureStudioCode(p.Code) == featureCodeResultUnknown:
		p.Hint = fmt.Sprintf("the resource request outcome is unknown (idempotent_key=%s); check it with `%s`; do not start a new request with another key while it is unknown", key, query)
	case p.Category == errs.CategoryNetwork:
		p.Hint = fmt.Sprintf("the request may or may not have been applied (idempotent_key=%s); check it with `%s`; to resume the same request run `%s`", key, query, featureEnableRetryCommand(appID, key))
	default:
		p.Hint = fmt.Sprintf("the enable may have stopped partway (idempotent_key=%s); check it with `%s`; to resume the same request run `%s`", key, query, featureEnableRetryCommand(appID, key))
	}
	return err
}

// explainFeatureRejection rewrites a Studio 4xx answer to the enable request
// and reports whether it did. These answers are final for the request as sent:
// re-sending it, or asking for its operation, does not change them.
func explainFeatureRejection(p *errs.Problem, appID, key string) bool {
	code := featureStudioCode(p.Code)
	if code < 40000 || code >= 50000 {
		return false
	}
	features := fmt.Sprintf("lark-cli apps +features --app-id %s", appID)
	p.Retryable = false
	switch code {
	case featureCodeEnableUnsupported:
		p.Subtype = errs.SubtypeFailedPrecondition
		if strings.Contains(p.Message, featureReasonAppIdentityMissing) {
			p.Message = fmt.Sprintf("app %s has no resolvable tenant or owner, so the server cannot request database resources for it (server: %s)", appID, p.Message)
			p.Hint = "the platform has to repair this app's ownership data; report the log_id to the Miaoda platform. Retrying, with or without --idempotent-key, will not help"
			return true
		}
		p.Message = fmt.Sprintf("app %s does not accept enabling its database through lark-cli; this happens when it is not a new-contract app (server: %s)", appID, p.Message)
		p.Hint = "only new-contract apps can enable a database from lark-cli; enable the database for this app in Miaoda cloud development instead (open the app in Miaoda and ask for a database there). Retrying this command will not help"
	case featureCodeFeatureUnsupported:
		p.Subtype = errs.SubtypeFeatureNotAvailable
		p.Message = fmt.Sprintf("the server does not offer enabling this feature for app %s (server: %s)", appID, p.Message)
		p.Hint = fmt.Sprintf("nothing was changed; list the features the server reports with `%s`", features)
	case featureCodeInvalidArgument:
		p.Subtype = errs.SubtypeInvalidParameters
		p.Hint = "the server rejected the request parameters and changed nothing; check --app-id, and pass --idempotent-key only as the UUID printed by an earlier attempt"
	case featureCodeRevisionChanged:
		p.Subtype = errs.SubtypeConflict
		p.Hint = fmt.Sprintf("the app's configuration changed while the enable was running (idempotent_key=%s); nothing was overwritten and resources already requested are kept; review `%s` and decide again whether the feature is still needed", key, features)
	case featureCodeOperationUnavailable:
		p.Subtype = errs.SubtypeFailedPrecondition
		p.Hint = featureKeyNotKeptHint(appID, key)
	case featureCodeKeyConflict:
		p.Subtype = errs.SubtypeConflict
		switch {
		case strings.Contains(p.Message, featureReasonOperationInProgress):
			p.Hint = fmt.Sprintf("another enable is still running for app %s; do not send a new request. Check `%s` in a minute: enabled=true means it is done; if it stays false for several minutes, start a new attempt without --idempotent-key", appID, features)
		case strings.Contains(p.Message, featureReasonOperationUnavailable):
			p.Hint = featureKeyNotKeptHint(appID, key)
		case strings.Contains(p.Message, featureReasonOperationSuperseded):
			p.Hint = fmt.Sprintf("a newer enable request replaced idempotent_key=%s and decides the outcome; review `%s` instead of re-sending this one", key, features)
		default:
			p.Hint = featureConflictHint(appID, key)
		}
	case featureCodeRateLimited:
		// The limit is checked before anything else, so the limited request
		// did nothing. This answer is only reached when nothing else can be
		// kept under the key either — a first request, or a key the server
		// confirmed it does not keep (runFeatureEnable reads the key after a
		// replay or on a resume, see explainRateLimitedKeptKey) — so a resume
		// would find no operation and stop. A new attempt is the recovery, as
		// for any key the server does not keep.
		p.Subtype = errs.SubtypeRateLimit
		p.Retryable = true
		p.Hint = fmt.Sprintf("the enable API is rate limited and did not act on this request, so idempotent_key=%s has nothing to resume; wait about a minute, then run `lark-cli apps +feature-enable --feature %s --app-id %s` again without --idempotent-key", key, featurePG, appID)
	default:
		p.Hint = "the server rejected the enable request; the message says why, so fix that before trying again"
	}
	return true
}

// explainRateLimitedKeptKey answers a rate limit on a request whose key may
// already name an operation on the server: a network replay, whose earlier
// attempt may have reached the server, or a resume. The limited request did
// nothing, but the key is still the way to that operation, so the hint keeps
// it instead of asking for a new one.
func explainRateLimitedKeptKey(err error, appID, key string) error {
	p, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	p.Subtype = errs.SubtypeRateLimit
	p.Retryable = true
	p.Hint = fmt.Sprintf("the enable API is rate limited and did not act on this request, but idempotent_key=%s may already name an operation on the server; wait about a minute, check it with `%s`, then resume the same request with `%s`; do not start a new request with another key while that operation may exist", key, featureQueryCommand(appID, key), featureEnableRetryCommand(appID, key))
	return err
}

// isFeatureKeyNotKept reports the server's answer that key names no operation
// this caller can follow: 40915, or a 40913 led by operation_unavailable.
func isFeatureKeyNotKept(err error) bool {
	p, ok := errs.ProblemOf(err)
	if !ok {
		return false
	}
	switch featureStudioCode(p.Code) {
	case featureCodeOperationUnavailable:
		return true
	case featureCodeKeyConflict:
		return strings.Contains(p.Message, featureReasonOperationUnavailable)
	}
	return false
}

// featureAuthFailure reports whether err is an authentication failure and, if
// so, returns it as one: the CLI's own authentication errors as they are, and
// an HTTP 401 that reached the CLI without an API envelope re-typed as one.
// Spark answers its own failures inside an envelope, so a bare 401 comes from
// a proxy or gateway in front of it rejecting the credential, as the Miaoda
// sandbox proxy does for an invalid one.
func featureAuthFailure(err error) (error, bool) {
	p, ok := errs.ProblemOf(err)
	if !ok {
		return err, false
	}
	switch {
	case p.Category == errs.CategoryAuthentication:
		return err, true
	case p.Category == errs.CategoryAPI && p.Code == http.StatusUnauthorized:
		authErr := errs.NewAuthenticationError(errs.SubtypeTokenInvalid, "%s", p.Message).
			WithCode(p.Code).
			WithCause(err)
		if p.LogID != "" {
			authErr = authErr.WithLogID(p.LogID)
		}
		return authErr, true
	}
	return err, false
}

// withFeatureAuthRecovery handles an authentication failure, such as an
// expired or invalid user credential. No key can succeed until the user
// authorizes again, so the error is not retryable and carries the CLI's own
// re-authorization guidance instead of a same-key resume. A hint the producer
// already set is kept; the generic guidance is the form the error presenter
// completes with the command's scopes.
func withFeatureAuthRecovery(err error) error {
	p, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	p.Retryable = false
	if strings.TrimSpace(p.Hint) != "" {
		return err
	}
	return recovery.Attach(err, recovery.UserAuthorization())
}

// isFeatureAPIAbsentAnswer reports whether err is one of the answers a
// deployment without the app features API gives: the gateway's internal error
// (2200) when the route exists but nothing behind it serves the API, or a body
// that is no API answer at all when the gateway has no route for it.
func isFeatureAPIAbsentAnswer(err error) bool {
	p, ok := errs.ProblemOf(err)
	if !ok {
		return false
	}
	return p.Code == featureCodeGatewayInternal || isFeatureNonAPIAnswer(p)
}

// asFeatureServerAnswer returns err, and its problem, as an API error when the
// classifier typed a response that is no API answer as an internal
// invalid-response error: what the server or its gateway answered is not a CLI
// fault, and the exit code follows the error category. The problem is nil
// when err is not typed.
func asFeatureServerAnswer(err error) (*errs.Problem, error) {
	p, ok := errs.ProblemOf(err)
	if !ok {
		return nil, err
	}
	if p.Category != errs.CategoryInternal {
		return p, err
	}
	apiErr := errs.NewAPIError(p.Subtype, "%s", p.Message).WithCause(err)
	if p.LogID != "" {
		apiErr = apiErr.WithLogID(p.LogID)
	}
	return &apiErr.Problem, apiErr
}

// isFeatureNonAPIAnswer reports an HTTP 404 that did not come from the API.
// Spark answers its own failures, a missing app included, inside a JSON
// envelope; a gateway without the route answers a bare 404 (callFeatureAPI
// keeps that status even when the body happens to decode as a JSON number).
// Any other body that is no API answer, such as a 200 with a null body, is not
// evidence of a missing route: the request may have been applied.
func isFeatureNonAPIAnswer(p *errs.Problem) bool {
	return p.Category == errs.CategoryAPI && p.Subtype == errs.SubtypeNotFound && p.Code == http.StatusNotFound
}

// confirmFeatureAPIMissing re-checks an answer isFeatureAPIAbsentAnswer
// recognizes with one plain read of the features endpoint and reports whether
// that read was answered the same way. Any other answer, including a success,
// means the API is served and the first failure stays as it was reported.
func confirmFeatureAPIMissing(rctx *common.RuntimeContext, appID string, err error) bool {
	if !isFeatureAPIAbsentAnswer(err) {
		return false
	}
	_, probeErr := callFeatureAPI(rctx.Ctx(), rctx, "GET", appFeaturesPath(appID), nil, nil)
	return isFeatureAPIAbsentAnswer(probeErr)
}

// explainFeatureAPIMissing reports a confirmed missing app features API as
// final: retrying the same command cannot succeed until the server changes.
func explainFeatureAPIMissing(err error, appID string) error {
	p, err := asFeatureServerAnswer(err)
	if p == nil {
		return err
	}
	answer := "failed with the gateway's internal error (2200)"
	if p.Code != featureCodeGatewayInternal {
		answer = "got an HTTP 404 that is no API answer, as a gateway without the route answers,"
	}
	p.Subtype = errs.SubtypeFeatureNotAvailable
	p.Retryable = false
	p.Message = fmt.Sprintf("the server did not run the app features API for app %s: the call %s and a read-only re-check was answered the same way, which is how a server without app features answers (server: %s)", appID, answer, p.Message)
	p.Hint = "do not retry: this server does not offer app features yet. Ask for the server to be upgraded, or enable the database for this app in Miaoda cloud development; report the log_id if app features should already be available"
	return err
}

// featureConflictHint answers a 40913 whose reason is not known, which holds
// for every reason the server gives it.
func featureConflictHint(appID, key string) string {
	return fmt.Sprintf("idempotent_key=%s belongs to a different, superseded or still-running enable request; review `lark-cli apps +features --app-id %s`, and start a new attempt without --idempotent-key only if the feature is still not enabled", key, appID)
}

// featureKeyNotKeptHint answers the server's operation_unavailable: the key
// names no operation this caller can follow, so the same key can never
// succeed and only a new attempt can help.
func featureKeyNotKeptHint(appID, key string) string {
	return fmt.Sprintf("idempotent_key=%s is no longer usable for app %s (replaced by a later operation, owned by another caller, or never created); check the database state with `lark-cli apps +features --app-id %s`: if database is already enabled there is nothing to do; if it is still needed, run `lark-cli apps +feature-enable --feature %s --app-id %s` without --idempotent-key so a new key is generated; do not retry with the same key", key, appID, appID, featurePG, appID)
}

// explainFeatureKeyNotKept reports whether err is the server's 40915 answer
// and, if so, explains it for key. It is final: the subtype says the key's
// state, not a transient failure, and the same key is never retried.
func explainFeatureKeyNotKept(err error, appID, key string) bool {
	p, ok := errs.ProblemOf(err)
	if !ok || featureStudioCode(p.Code) != featureCodeOperationUnavailable {
		return false
	}
	p.Subtype = errs.SubtypeFailedPrecondition
	p.Retryable = false
	p.Hint = featureKeyNotKeptHint(appID, key)
	return true
}
