// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
)

// appsService 是 CLI 命令的 service 前缀（lark-cli apps ...）。
const appsService = "apps"

// apiBasePath is the registered OAPI prefix for the apps domain.
const apiBasePath = "/open-apis/spark/v1"

// appIDListHint is the shared recovery hint for commands whose most likely
// failure cause is a wrong/inaccessible --app-id. It points at +list to find
// the correct app id. The app_/cli_ format rule is taught in
// lark-apps SKILL.md ("app_id 获取"); the hint stays lean and does not repeat it.
const appIDListHint = "verify --app-id is correct and you have access to the app; list your apps with `lark-cli apps +list`"

// appNoDatabaseCode / appNoDatabaseLegacyCode are the Spark business codes seen
// when a db command runs against an app that has not initialized a database yet.
// The raw server message carries internal workspace terminology, so the CLI
// rewrites it into a user-facing explanation and attaches a recoverable
// cloud-development next step (see appNoDatabaseMessage / appNoDatabaseHint).
//
// Two codes, not one: the server renumbered this case from 500002759 to
// 400002465 when the domain moved its client-class errors into the 4xx band.
// Keying on a single literal made the recovery flow disappear silently on the
// day that shipped — no compile error, no failing unit test (they assert against
// the same constant), and the dry-run E2E never reaches a live server. Hence
// isAppNoDatabaseError matches code OR message; see that function.
const (
	appNoDatabaseCode       = 400002465 // current
	appNoDatabaseLegacyCode = 500002759 // pre-4xx renumber; kept so older servers still match
)

// appNoDatabaseMessage is the user-facing explanation for appNoDatabaseCode.
// It deliberately drops internal workspace / db-branch terms.
const appNoDatabaseMessage = "this app does not have a database yet"

// appNoDatabaseHint guides adding a database via Miaoda cloud development. It
// only uses existing commands with stable placeholder args, so a harness can
// execute it without matching natural-language error text. Adding a database is
// a cloud write: a failed read alone does not authorize it — confirm with the
// user before starting a +chat.
const appNoDatabaseHint = "ask the user whether to add a database through Miaoda cloud development; if confirmed, run `lark-cli apps +session-list --app-id <app_id>` and reuse an active session, or run `lark-cli apps +session-create --app-id <app_id>`; send the database requirement with `lark-cli apps +chat --app-id <app_id> --session-id <session_id> --message \"<database requirement>\"`, poll `lark-cli apps +session-get --app-id <app_id> --session-id <session_id>` until `latest_turn.status=completed`, then retry the original db command"

// appNoContainerCode is the Spark business code returned by the online
// observability endpoints (query_metrics_data / query_analytics_data) when the
// target app has no running container/deployment. It is an expected business
// state, not a system fault: an app that is not deployed and serving traffic
// produces no client_api_request metrics or analytics to query. The raw upstream
// message is "Container not exists", which reads like an infrastructure failure
// and misleads callers (including AI agents) into retrying a non-retryable,
// expected state — so the observability commands rewrite it into a user-facing
// explanation with a deploy-then-retry next step (see withObservabilityHint).
const appNoContainerCode = 400002655

// appNoContainerMessage is the user-facing explanation for appNoContainerCode.
// It replaces the infra-sounding "Container not exists" with the actual cause.
const appNoContainerMessage = "this app has no running container; online metrics and analytics are only produced after the app is deployed and serving traffic"

// appNoContainerHint guides diagnosing and (only with user authorization)
// deploying the app so observability data starts flowing. Deploying via
// +release-create is a "write" that takes the whole app live and can affect
// existing production traffic, so — like appNoDatabaseHint — the hint gates
// that go-live behind an explicit user confirmation and leads with a read-only
// status check; a failed metrics read alone does not authorize a release. It
// names existing commands with a stable placeholder arg so a harness can act on
// it without parsing natural-language error text.
const appNoContainerHint = "check the app's deployment status with `lark-cli apps +release-list --app-id <app_id> --status finished` (a newly created or undeployed app has no finished release, so it produces no metrics yet); if it is not deployed, ask the user whether to deploy — deploying takes the whole app live and can affect existing production traffic — and only if confirmed run `lark-cli apps +release-create --app-id <app_id>`, then retry once it is serving traffic"

// appNoContainerMessageMarkers are lowercase substrings of the raw "Container
// not exists" server message, used as a fallback when the business code is not
// the one the CLI knows. A bare numeric literal is not a stable signal — the
// no-database case (see isAppNoDatabaseError) was silently dropped when the
// server renumbered it, with nothing in CI to catch it — so detection is
// code-OR-message here too. "not exist" (no trailing s) matches both
// "Container not exists" and a possible "Container not exist".
var appNoContainerMessageMarkers = []string{
	"container not exist",
}

// isAppNoContainerError reports whether a typed observability failure is "this
// app has no running container", matching on business code OR raw server
// message for the same code-vs-message stability trade-off documented on
// isAppNoDatabaseError. Only consulted from the observability helper
// (withObservabilityHint), so the message marker is scoped to observability
// context and cannot hijack unrelated apps failures.
func isAppNoContainerError(p *errs.Problem) bool {
	if p == nil {
		return false
	}
	if p.Code == appNoContainerCode {
		return true
	}
	message := strings.ToLower(p.Message)
	for _, marker := range appNoContainerMessageMarkers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// withObservabilityHint wraps an online observability (metric/analytics) API
// failure. It first rewrites the "app has no running container" business state
// into a user-facing explanation, since the raw "Container not exists" upstream
// message reads like an infra fault and drives non-retryable retries; any other
// failure falls through to the shared app-id recovery hint (which still applies
// its own no-database override). Scoped to the two observability commands rather
// than the global withAppsHint chokepoint because 400002655 is not known to be
// observability-exclusive, and the message-marker fallback must not reach
// unrelated commands. err==nil and untyped errors pass through unchanged.
func withObservabilityHint(err error) error {
	if err == nil {
		return nil
	}
	if p, ok := errs.ProblemOf(err); ok && isAppNoContainerError(p) {
		p.Message = appNoContainerMessage
		p.Hint = appNoContainerHint
		return err
	}
	return withAppsHint(err, appIDListHint)
}

// withAppsHint attaches an actionable next-step hint to a typed failure,
// preserving its original classification (subtype/code/log_id). A hint already
// present on the error is kept (the upstream wording wins); only an empty hint
// is filled in. Mirrors drive.appendDriveExportRecoveryHint. err==nil and
// untyped errors pass through unchanged.
//
// Special-case the "app has no database yet" failure (see isAppNoDatabaseError):
// rewrite the message to a user-facing explanation and force the
// cloud-development recovery hint, since the raw upstream message uses internal
// terms and any generic hint would be less actionable. That failure is only
// produced by db endpoints, so the override is safe to check for every apps
// command that funnels through here.
func withAppsHint(err error, hint string) error {
	if err == nil {
		return nil
	}
	// p points at the embedded Problem, so the mutation is reflected in err.
	if p, ok := errs.ProblemOf(err); ok {
		if isAppNoDatabaseError(p) {
			p.Message = appNoDatabaseMessage
			p.Hint = appNoDatabaseHint
			return err
		}
		if strings.TrimSpace(p.Hint) == "" {
			p.Hint = hint
		}
		return err
	}
	return err
}

// appNoDatabaseMessageMarkers are lowercase substrings of the raw server message
// for the no-database failure, used as a fallback when the business code is not
// one the CLI knows. They quote the server's internal vocabulary — "db branch",
// "workspace id ... app id" — which is exactly why the message gets rewritten for
// users.
//
// Deliberately narrow. A looser marker such as "workspace" alone would swallow
// neighbouring db failures that need their own hint; "no db branch" in particular
// must not also match env-pull's "invalid db branch" case
// (isEnvPullDevDBNotInitializedError). Widen only with a test proving the
// neighbours still pass through.
var appNoDatabaseMessageMarkers = []string{
	"get workspace id failed by app id",
	"no db branch",
}

// isAppNoDatabaseError reports whether a typed failure is "this app has no
// database yet", matching on business code OR raw server message.
//
// Why both: the code is the precise signal but not a stable one — the server has
// already renumbered this case once (500002759 → 400002465), and a code-only
// check fails open, silently dropping the recovery flow with nothing in CI to
// catch it. The message is the reverse trade: it survives renumbering but breaks
// on rewording or localization. Requiring either to match means one channel
// changing degrades nothing, and only a simultaneous change of both regresses.
func isAppNoDatabaseError(p *errs.Problem) bool {
	if p == nil {
		return false
	}
	if p.Code == appNoDatabaseCode || p.Code == appNoDatabaseLegacyCode {
		return true
	}
	message := strings.ToLower(p.Message)
	for _, marker := range appNoDatabaseMessageMarkers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// validateRealAppID checks that --app-id is a real app ID (app_ prefix).
// meta_token values are rejected with a hint to resolve via +get first.
func validateRealAppID(appID string) error {
	if !strings.HasPrefix(appID, "app_") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			`--app-id must be an app_id starting with "app_".`,
		).WithParam("--app-id").WithHint(
			`If you have a meta_token or a /page/<token>/ link, first resolve it:
lark-cli apps +get --app-id <meta_token> -q '.data.app.app_id'
Then retry this command with the returned app_id.`,
		)
	}
	return nil
}

// rejectOutputTraversal is a defense-in-depth pre-check on a user-supplied
// --output path. The authoritative guard is the local FileIO layer
// (validate.SafeOutputPath sandboxes every write to the cwd, resolving .. and
// symlinks), so traversal is already blocked at write time; this gives an
// earlier, clearer validation error and pins the contract in the command layer.
// Empty (use server-derived default) passes through. Absolute paths and any
// ".." path component are rejected.
func rejectOutputTraversal(output string) error {
	o := strings.TrimSpace(output)
	if o == "" {
		return nil
	}
	if filepath.IsAbs(o) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--output must be a relative path within the current directory, got %q", o).WithParam("--output")
	}
	for _, seg := range strings.Split(filepath.Clean(o), string(filepath.Separator)) {
		if seg == ".." {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--output must not contain .. path traversal, got %q", o).WithParam("--output")
		}
	}
	return nil
}
