// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"path/filepath"
	"regexp"
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

// Hints chosen by commonAppsFailureHint for failures whose cause is the app or
// the platform rather than the request, so the per-command request-fix hint
// would send the caller after the wrong thing.
const (
	appUnavailableHint  = "the app cannot be resolved because it was deleted or is not available to the current tenant; use `lark-cli apps +list` to verify the app_id, then stop polling or operating on this app"
	appBannedHint       = "this app is banned, so operations cannot continue; stop retrying and contact the app owner or enterprise administrator"
	dbConnectionHint    = "the platform database connection is unavailable; do not create a dev database or change the app_id because of this error. Wait before trying once more; if it continues, report the request identifier"
	neutralServerHint   = "the request failed, and the CLI has no specific recovery guidance for this response. Follow any explicit cause in the server message; do not change arguments or retry solely because of the error code. If a write may have taken effect, confirm its outcome before trying again. If the cause remains unclear, report the request identifier"
	internalFailureHint = "the service returned an internal failure. If this command changes data, first confirm whether the change took effect; if it did, do not run the command again. Otherwise wait before trying once more, and if the outcome is unclear or the failure continues, stop and report the request identifier"
	appNotFoundHint     = "the app identifier cannot be identified; use `lark-cli apps +list` to verify the app_id without inferring any unavailable state"
)

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

// appNoDatabaseHint guides adding a database. Apps whose +features lists database
// enable it with +feature-enable, whose next_steps say how the database takes
// effect where the command runs; an already enabled database only has to take
// effect; apps that cannot (database not listed, or not a new-contract app) keep the
// Miaoda cloud development flow. It only uses
// existing commands with stable placeholder args, so a harness can execute it
// without matching natural-language error text. Adding a database is a cloud
// write: a failed read alone does not authorize it — confirm with the user
// before enabling it or starting a +chat.
const appNoDatabaseHint = "ask the user whether to add a database; if confirmed, run `lark-cli apps +features --app-id <app_id>`: when it lists database with enabled=false, run `lark-cli apps +feature-enable --feature database --app-id <app_id>`, follow the next_steps it prints and retry the original db command; when database is already enabled=true, the database only has to take effect: locally run `lark-cli apps +env-pull --app-id <app_id>` and restart the local dev process, in a Miaoda sandbox wait a moment for the platform, then retry; when database is not listed, +features fails, or +feature-enable reports the app is not a new-contract app, add it through Miaoda cloud development: run `lark-cli apps +session-list --app-id <app_id>` and reuse an active session, or run `lark-cli apps +session-create --app-id <app_id>`; send the database requirement with `lark-cli apps +chat --app-id <app_id> --session-id <session_id> --message \"<database requirement>\"`, poll `lark-cli apps +session-get --app-id <app_id> --session-id <session_id>` until `latest_turn.status=completed`, then retry the original db command"

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
const appNoContainerHint = "check the app's deployment status with `lark-cli apps +release-list --app-id <app_id> --status finished` (a newly created or undeployed app has no finished release, so it produces no metrics yet); if it is not deployed, inspect its type with `lark-cli apps +get --app-id <app_id> --jq '.data.app.app_type'`, then ask the user whether to deploy — deploying takes the whole app live and can affect existing production traffic — and only if confirmed run `lark-cli apps +release-create --help` and follow the matching app-type example; retry once the app is serving traffic"

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
	if p, ok := errs.ProblemOf(err); ok {
		// A gateway throttle was not evaluated by the observability service.
		// Classify it before inspecting message text that can coincidentally
		// resemble the service's no-container business error.
		if p.Subtype == errs.SubtypeRateLimit {
			return withAppsHint(err, appIDListHint)
		}
		if isAppNoContainerError(p) {
			p.Message = appNoContainerMessage
			p.Hint = appNoContainerHint
			return err
		}
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
//
// Second special case: db failures whose numeric code is a shared
// responsibility bucket and whose real reason is a `k_dl_` subcode on the message
// (see applyDBSubcode). Checked here, at the chokepoint, because the collapse is a
// property of the db OpenAPI rather than of any one command — audit, env-create
// and their neighbours all receive the same code for unrelated reasons. Matching
// requires an exact known subcode, so commands that never see one are unaffected.
//
// Third: failures caused by the app or the platform rather than the request —
// an app the gateway cannot resolve, a banned app, a database connection failure,
// an unclassified server error — get a shared hint before the per-command one
// (see commonAppsFailureHint), because every per-command hint asks the caller to
// fix the request.
func withAppsHint(err error, hint string) error {
	if err == nil {
		return nil
	}
	// p points at the embedded Problem, so the mutation is reflected in err.
	if p, ok := errs.ProblemOf(err); ok {
		// Classification beats message heuristics. A gateway throttle was not
		// evaluated by the database service, so text that resembles a database
		// state marker must not reclassify it or replace backoff guidance.
		if p.Subtype == errs.SubtypeRateLimit {
			if strings.TrimSpace(p.Hint) == "" {
				p.Hint = appRateLimitHint
			}
			return err
		}
		if isAppNoDatabaseError(p) {
			p.Message = appNoDatabaseMessage
			p.Hint = appNoDatabaseHint
			return err
		}
		// Before the generic hint: the subcode identifies one scenario, so its
		// guidance always beats a per-command fallback written for the whole family.
		if applyDBSubcode(p) {
			return err
		}
		if strings.TrimSpace(p.Hint) != "" {
			return err
		}
		// Before the per-command hint: these failures are about the app or the
		// platform, so a hint written to fix the request cannot explain them.
		if hint := commonAppsFailureHint(p); hint != "" {
			p.Hint = hint
			return err
		}
		if hintExplainsFailure(p) {
			p.Hint = hint
		}
		return err
	}
	return err
}

// appRateLimitHint answers a throttled apps call (api/rate_limit — the OpenAPI
// gateway's 99991400). The server did not judge the request, so the arguments are
// not what to change; retrying at once only adds to the traffic that tripped the
// limit. The backoff wording matches the stream client's rate-limit hint
// (internal/client), so one condition reads the same on every path.
const appRateLimitHint = "the request was rate limited, not rejected for its arguments, so do not change them; stop immediate retries, then rerun the same command later with exponential backoff and jitter, and send fewer calls in parallel"

// commonAppsFailureHint picks the hint for failures that mean the same thing on
// every apps command, and returns "" when the per-command hint should apply.
//
// Only api/unknown failures qualify (plus one registered code below). A code the
// classifier already knows keeps its subtype, and whether the per-command hint
// applies to it is still decided by hintExplainsFailure. Messages are matched
// whole, ignoring case and one trailing period, against wording observed on the
// wire; a longer or different message is not guessed at.
//
// Order matters: the app-level and connection signatures name the cause, so they
// come before the code-range fallback that only says the server failed.
//   - 2320008 is the gateway's answer for an app_id it cannot resolve — the app
//     was deleted or belongs to another tenant. It reaches every app-scoped
//     command, and no flag change makes it succeed.
//   - 500232002 is a generic internal code; only the ban wording identifies the
//     app state, so other messages under it stay neutral.
//   - 500002776 collapses every database failure; only a connection failure
//     (dbConnectionFailurePatterns) says the request was never executed.
//   - 50001 "upstream internal error" and 500000049 with the generic
//     "something went wrong" wording are internal failures. On a command that
//     changes data the change may have partly happened, so the caller confirms
//     the outcome first; the selector cannot tell reads from writes, so the hint
//     makes that step conditional.
//   - Any other 9-digit 5xx code, and 50001 with other wording, only says the
//     server failed: the neutral hint keeps the server's message in charge and
//     neither blames nor clears the request.
//   - 400002484 is registered as validation/invalid_argument ("app id does not
//     identify an app"); the command hints for it ask about tables or
//     environments, so it gets the app-identifier hint instead.
func commonAppsFailureHint(p *errs.Problem) string {
	if p == nil {
		return ""
	}
	if p.Category == errs.CategoryValidation && p.Subtype == errs.SubtypeInvalidArgument && p.Code == 400002484 {
		return appNotFoundHint
	}
	if p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeUnknown {
		return ""
	}
	message := normalizedExactMessage(p.Message)
	switch {
	case p.Code == 2320008 && message == "app tenant not found":
		return appUnavailableHint
	case p.Code == 500232002 && isAppBannedMessage(message):
		return appBannedHint
	case p.Code == 500002776 && isDatabaseConnectionMessage(p.Message):
		return dbConnectionHint
	case p.Code == 50001 && message == "upstream internal error":
		return internalFailureHint
	case p.Code == 500000049 && isGenericInternalFailureMessage(message):
		return internalFailureHint
	case (p.Code >= 500000000 && p.Code < 600000000) || p.Code == 50001:
		return neutralServerHint
	}
	return ""
}

// normalizedExactMessage prepares a server message for whole-message matching:
// surrounding space and one trailing period are dropped and case is folded.
func normalizedExactMessage(message string) string {
	normalized := strings.ToLower(strings.TrimSpace(message))
	return strings.TrimSuffix(normalized, ".")
}

// isAppBannedMessage matches the ban wording only as the whole message, so a
// failure that merely quotes the phrase (a SQL error echoing user input) is not
// read as a ban.
func isAppBannedMessage(message string) bool {
	switch message {
	case "app is banned", "app is banned for miaoda", "[getapp] app is banned for miaoda":
		return true
	default:
		return false
	}
}

// isGenericInternalFailureMessage matches the generic internal-failure wording
// the release endpoints send in both locales.
func isGenericInternalFailureMessage(message string) bool {
	switch message {
	case "something went wrong. please try again later", "出错了，请稍后重试":
		return true
	default:
		return false
	}
}

// dbConnectionFailureSubcodes are the k_dl_ prefixes observed in front of a
// database connection failure. k_dl_000001 is a generic wrapper that also
// carries ordinary SQL errors, so the text after the prefix decides.
var dbConnectionFailureSubcodes = map[string]bool{
	"k_dl_000001":  true,
	"k_dl_000002":  true,
	"k_dl_1300002": true,
	"k_dl_1600052": true,
}

// dbConnectionFailurePatterns match the whole lower-cased text after the subcode,
// in the forms observed on the wire. The variable parts are the database user,
// the pool name, the timeout, and the connection target and stage reported by the
// driver; anything else — added context, another SQLSTATE, a quoted statement —
// does not match and falls back to the neutral server hint.
var dbConnectionFailurePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^(code:k_dl_1600052,msg:)?get db connection failed$`),
	regexp.MustCompile(`^error: remaining connection slots are reserved for roles with the superuser attribute \(sqlstate xx000\)$`),
	regexp.MustCompile(`^error: sorry, too many clients already \(sqlstate xx000\)$`),
	regexp.MustCompile(`^error: password authentication failed for user '[^']+' \(sqlstate xx000\)$`),
	regexp.MustCompile(`^error: hikaripool-\S+ - connection is not available, request timed out after \d+ms\. \(sqlstate [0-9a-z]{5}\)$`),
	regexp.MustCompile("^failed to connect to `user=[^`\\s]+ database=[^`\\s]+`: \\S+ \\(\\S+\\): (dial error|tls error|server error|failed sasl auth|failed to receive message): .+$"),
}

// isDatabaseConnectionMessage reports whether a database failure message says
// the platform could not connect to the app database, i.e. the request itself
// was never executed.
func isDatabaseConnectionMessage(message string) bool {
	subcode, remainder, ok := subcodeOf(strings.TrimSpace(message))
	if !ok || !dbConnectionFailureSubcodes[subcode] {
		return false
	}
	remainder = normalizedExactMessage(remainder)
	for _, pattern := range dbConnectionFailurePatterns {
		if pattern.MatchString(remainder) {
			return true
		}
	}
	return false
}

// hintExplainsFailure reports whether the caller's hint can plausibly explain this
// failure. Every hint passed to withAppsHint describes something to fix about the
// REQUEST — "verify --app-id", "verify table/column names with +db-table-get", "if
// the release_id is unknown, list releases". Those sentences are only true if the
// request is what failed.
//
// SubtypeFailedPrecondition says the opposite by definition: "the request is valid
// but the system/resource state is not in the state required to execute; caller
// must change state (not retry)". Attaching request-shaped advice to it is wrong by
// construction — that is how 221800 "miaoda UAT not activated", a tenant that never
// enabled Miaoda, came back advised to "verify table/column names ... target the dev
// database with --environment dev": advice that cannot help and sends an agent
// looping over table lookups.
//
// Blast radius is two Spark codes, because that is all this subtype covers here:
//   - 221800 — the case above; now withheld.
//   - 400002655 "no running container" — only when it reaches a non-observability
//     command; the observability pair rewrites it first (withObservabilityHint), and
//     "verify --app-id" was never the fix for an undeployed app anyway.
//
// The other precondition codes never reach this line: 400002465 / 500002759
// (no database) are intercepted by the isAppNoDatabaseError branch above, and
// 400002479 (db-sync task state) is served by withDBSyncHint, which does not
// delegate here.
//
// SubtypeRateLimit is withheld too: a throttled call was never evaluated, so no
// sentence about the request can explain it. The OpenAPI gateway's 99991400
// reached every apps command that passes a request hint this way — +db-execute
// was told to check table/column names and switch to --environment dev,
// +trace-get to verify --app-id — and an agent following that sent more calls into
// the same limit. The failing cases that
// proved it are TestAppsDBExecute_RateLimitHintIsBackoffNotRequestFix (through the
// command) and TestAppsHints_RateLimitDoesNotInheritRequestHint (through every
// hint helper). Unlike the precondition class the hint is not left empty:
// withAppsHint fills it with appRateLimitHint, and like every other hint only when
// it is empty, so a rate limit that arrives with its own wording keeps it
// (TestWithAppsHint_RateLimitKeepsUpstreamHint).
//
// Deliberately these two subtypes. Gating on Category instead (deny authentication /
// authorization / network / ...) contradicts contracts this package asserts on
// purpose: an authentication failure on +role-list (99991663) is expected to keep
// the app-access hint, and a 503 on credential issuance the developer-access hint.
// Those hints are broad enough to survive a caller-standing failure; only the
// precondition and rate-limit classes are guaranteed to be misdescribed. Widen only
// with a failing case proving the hint is wrong for the new class.
func hintExplainsFailure(p *errs.Problem) bool {
	switch p.Subtype {
	case errs.SubtypeFailedPrecondition, errs.SubtypeRateLimit:
		return false
	}
	return true
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
