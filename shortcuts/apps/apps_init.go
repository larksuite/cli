// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/charcheck"
	"github.com/larksuite/cli/shortcuts/common"
)

// defaultInitBranch is the fixed remote branch +init checks out after clone.
const defaultInitBranch = "sprint/default"

// Fixed init commit subjects. Constants — never interpolate user input. The
// empty-repo (`app init`) path splits the scaffolded tree into two commits;
// the non-empty (`app sync`) path stays a single commit.
const (
	commitMsgAppCode   = "chore: initialize app project code"
	commitMsgAppConfig = "chore: initialize miaoda app config"
	commitMsgUpgrade   = "chore: initialize miaoda app repository"
)

// scaffold kinds returned by runScaffold and consumed by commitAndPushIfDirty.
const (
	scaffoldKindInit    = "init"
	scaffoldKindUpgrade = "upgrade"
)

const (
	miaodaCLIPkg    = "@lark-apaas/miaoda-cli@latest"
	defaultTemplate = "nestjs-react-fullstack"
	metaRelPath     = ".spark/meta.json"
	steeringRelPath = ".agent/skills/steering"
	seedReadme      = "README.md"
)

// initRunner is the commandRunner used by +init. Package-level so unit tests
// can swap in a fakeCommandRunner. Production uses execCommandRunner.
var initRunner commandRunner = execCommandRunner{}

// AppsInit initializes a Miaoda app's code and local development environment.
var AppsInit = common.Shortcut{
	Service:     appsService,
	Command:     "+init",
	Description: "Initialize a Miaoda app's code and local development environment",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +init --app-id <app_id> --dir <dir>",
		"Example: lark-cli apps +init --app-id <app_id> --dir <dir> --dry-run",
	},
	// +init makes no direct lark API calls (it shells out to the
	// +git-credential-init subprocess, which enforces its own scopes), so it
	// declares no scopes of its own. Explicit []string{} (not nil) per the
	// convention enforced by TestAllShortcutsScopesNotNil.
	Scopes:    []string{},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		// NOTE: --app-id is intentionally NOT Required:true. The framework maps
		// Required:true to cobra's MarkFlagRequired, whose error is plain-text
		// exit-1 (root.go handleRootError case 4), bypassing the structured
		// envelope. The spec and the E2E assert exit-2 + a structured
		// {"ok":false,"error":{...}} envelope for missing --app-id, so the empty
		// check lives in Validate (typed validation error -> exit 2).
		{Name: "app-id", Desc: "Miaoda app ID"},
		{Name: "dir", Desc: "clone target directory; absolute or relative path (default ./<app-id>)"},
		{Name: "template", Desc: "code-init template for an empty repo; optional — if omitted, derived from the app's tech stack"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		template := resolveTemplate(rctx, appID)
		dry := common.NewDryRunAPI().
			Desc("Initialize Miaoda app code (credential-init, clone, checkout, npx code-init, optional commit/push)").
			Set("credential_init", fmt.Sprintf("apps +git-credential-init --app-id %s --format json", appID)).
			Set("checkout", "git checkout "+defaultInitBranch).
			Set("scaffold", fmt.Sprintf("empty repo: npx -y --prefer-online %s app init --template %s --app-id %s; non-empty: npx -y --prefer-online %s app sync + .spark/meta.json app_id patch + conditional skills sync --local", miaodaCLIPkg, template, appID, miaodaCLIPkg)).
			Set("commit_push", "conditional: git add -A + commit + push origin "+defaultInitBranch+" when the working tree has changes").
			Set("template", template).
			Set("env_pull", fmt.Sprintf("apps +env-pull --app-id %s --project-path <clone_path> --format json (after successful init)", appID))
		dir, err := resolveTargetPath(rctx, appID)
		if err != nil {
			dry.Set("dir_error", err.Error())
			dir = defaultCloneDir(appID)
		} else if isAlreadyInitialized(dir) {
			dry.Set("already_initialized", true)
		} else if e := ensureEmptyDir(dir); e != nil {
			dry.Set("dir_error", e.Error())
		}
		dry.Set("clone", fmt.Sprintf("git clone -- <repository_url-from-credential-init> %s", dir))
		dry.Set("clone_path", dir)
		return dry
	},
	Execute: appsInitExecute,
}

// defaultCloneDir returns the default clone target (./<app-id>) for an app ID.
func defaultCloneDir(appID string) string {
	return filepath.Join(".", appID)
}

// resolveTemplate returns the scaffold template for an empty-repo `app init`.
// An explicit --template wins. When omitted, it should be derived from the
// app's tech stack.
// TODO(apps-init): look up the app by appID via the apps API (e.g. `apps +list`
// or a get-app endpoint), read its tech stack, and map tech-stack -> template
// through a (future) enum. Until that lands, fall back to defaultTemplate.
func resolveTemplate(rctx *common.RuntimeContext, appID string) string {
	if t := strings.TrimSpace(rctx.Str("template")); t != "" {
		return t
	}
	// TODO(apps-init): derive from app tech stack (apps API + enum mapping).
	return defaultTemplate
}

// initLogf writes a one-line progress message to stderr. stdout stays reserved
// for the structured JSON envelope, so progress never pollutes it. Callers must
// never pass a raw repository_url (it may embed a token) — pass step names,
// clone_path, branch, or scaffold kind, and route any URL through
// redactURLCredentials first.
func initLogf(rctx *common.RuntimeContext, format string, args ...interface{}) {
	fmt.Fprintf(rctx.IO().ErrOut, "→ "+format+"\n", args...)
}

// resolveTargetPath computes the absolute clone target from --dir (or the
// ./<app-id> default). Unlike the prior SafeInputPath approach it does NOT
// confine to cwd — the clone destination is user-chosen (the skill prompts for
// it). It rejects empty input and control characters; symlink/no-clobber
// guarding happens in ensureEmptyDir.
func resolveTargetPath(rctx *common.RuntimeContext, appID string) (string, error) {
	raw := strings.TrimSpace(rctx.Str("dir"))
	if raw == "" {
		raw = defaultCloneDir(appID)
	}
	// Reject ALL control characters (incl. tab/newline — a newline in an echoed
	// path is a log-injection vector); charcheck additionally rejects dangerous
	// Unicode (bidi overrides, zero-width) that IsControl does not.
	if strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return "", appsValidationParamError("--dir", "--dir must not contain control characters")
	}
	if err := charcheck.RejectControlChars(raw, "--dir"); err != nil {
		return "", appsValidationParamError("--dir", "%v", err).WithCause(err)
	}
	abs, err := filepath.Abs(raw) //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard rule shortcuts-no-vfs); raw is control-char-validated above, and FileIO.ResolvePath cannot resolve a clone target (it rejects absolute paths).
	if err != nil {
		return "", appsValidationParamError("--dir", "--dir cannot be resolved: %v", err)
	}
	return abs, nil
}

// ensureEmptyDir refuses to clone into an existing non-empty dir, a symlink, or
// a non-directory. A non-existent path is fine (git clone creates it). Uses
// Lstat so a symlinked target is rejected rather than followed.
func ensureEmptyDir(dir string) error {
	info, err := os.Lstat(dir) //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard rule shortcuts-no-vfs); dir is the validated clone target, and lstat is required to reject a symlink (FileIO has no Lstat; its Stat follows symlinks).
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return appsValidationParamError("--dir", "--dir cannot be read: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return appsValidationParamError("--dir", "--dir must not be a symlink: %q", dir)
	}
	if !info.IsDir() {
		return appsValidationParamError("--dir", "--dir exists and is not a directory: %q", dir)
	}
	entries, err := os.ReadDir(dir) //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard rule shortcuts-no-vfs); dir is the validated clone target, and FileIO has no ReadDir.
	if err != nil {
		return appsValidationParamError("--dir", "--dir cannot be read: %v", err)
	}
	if len(entries) > 0 {
		return appsValidationParamError("--dir", "target directory %q already exists and is not empty", dir)
	}
	return nil
}

// isAlreadyInitialized reports whether dir is an already-initialized Miaoda app
// repo, detected by the presence of <dir>/.spark/meta.json (regardless of its
// app_id value). Used to short-circuit +init into a friendly no-op.
func isAlreadyInitialized(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, metaRelPath)) //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard rule shortcuts-no-vfs); path is under the validated clone dir, and FileIO.Stat rejects absolute paths.
	return err == nil && !info.IsDir()
}

// ensureMetaAppID patches <dir>/.spark/meta.json to include app_id when the file
// exists but lacks (or has an empty) app_id. Other fields are preserved. When
// the file does not exist, this is a no-op (we never create it).
func ensureMetaAppID(dir, appID string) error {
	path := filepath.Join(dir, metaRelPath)
	b, err := os.ReadFile(path) //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard rule shortcuts-no-vfs); path is under the validated clone dir, and FileIO.Open rejects absolute paths.
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return appsFileIOError(err, "read %s failed: %v", metaRelPath, err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return appsFileIOError(err, "parse %s failed: %v", metaRelPath, err)
	}
	if cur, _ := m["app_id"].(string); strings.TrimSpace(cur) != "" {
		return nil
	}
	if m == nil {
		m = map[string]interface{}{}
	}
	m["app_id"] = appID
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return appsFileIOError(err, "marshal %s failed: %v", metaRelPath, err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil { //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard rule shortcuts-no-vfs); path is under the validated clone dir, and FileIO.Save rejects absolute paths.
		return appsFileIOError(err, "write %s failed: %v", metaRelPath, err)
	}
	return nil
}

// hasSteeringSkills reports whether <dir>/.agent/skills/steering exists as a dir.
func hasSteeringSkills(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, steeringRelPath)) //nolint:forbidigo // shortcuts cannot import internal/vfs (depguard rule shortcuts-no-vfs); path is under the validated clone dir, and FileIO.Stat rejects absolute paths.
	return err == nil && info.IsDir()
}

// isEmptyRepo reports whether the checked-out branch has no tracked files
// other than the backend's default seed README.md. `git ls-files` listing
// nothing — or only README.md — counts as empty (→ scaffold via `app init`).
func isEmptyRepo(ctx context.Context, dir string) (bool, error) {
	stdout, stderr, err := initRunner.Run(ctx, dir, "git", "ls-files")
	if err != nil {
		return false, appsExternalToolError(err, "git ls-files failed: %s", gitErr(stderr, err))
	}
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		f := strings.TrimSpace(line)
		// Match the seed exactly (case- and path-sensitive): only a root-level
		// "README.md" is the backend's default seed. A docs/README.md or readme.md
		// is treated as real content (→ non-empty), which is the safe direction
		// (skip scaffolding rather than risk overwriting). Extend this allow-list
		// here if the backend's seed set grows.
		if f == "" || f == seedReadme {
			continue
		}
		return false, nil // a non-README tracked file → non-empty repo
	}
	return true, nil
}

// runScaffold runs the npx scaffolding step inside the cloned repo (cwd=dir).
// Empty repo -> `app init`; non-empty -> `app sync` + meta app_id patch +
// conditional `skills sync`. Returns "init" or "upgrade".
func runScaffold(ctx context.Context, dir, appID, template string) (string, error) {
	empty, err := isEmptyRepo(ctx, dir)
	if err != nil {
		return "", err
	}
	if empty {
		// isEmptyRepo treats a repo with no tracked files — or only the backend's
		// seed README.md — as empty. If other seed files (e.g. .gitignore) can
		// appear, extend isEmptyRepo's allow-list accordingly.
		if _, stderr, err := initRunner.Run(ctx, dir, "npx", "-y", "--prefer-online", miaodaCLIPkg, "app", "init", "--template", template, "--app-id", appID); err != nil {
			return "", appsExternalToolError(err, "npx app init failed: %s", gitErr(stderr, err))
		}
		return scaffoldKindInit, nil
	}
	if _, stderr, err := initRunner.Run(ctx, dir, "npx", "-y", "--prefer-online", miaodaCLIPkg, "app", "sync"); err != nil {
		return "", appsExternalToolError(err, "npx app sync failed: %s", gitErr(stderr, err))
	}
	if err := ensureMetaAppID(dir, appID); err != nil {
		return "", err
	}
	if !hasSteeringSkills(dir) {
		if _, stderr, err := initRunner.Run(ctx, dir, "npx", "-y", "--prefer-online", miaodaCLIPkg, "skills", "sync", "--local"); err != nil {
			return "", appsExternalToolError(err, "npx skills sync failed: %s", gitErr(stderr, err))
		}
	}
	return scaffoldKindUpgrade, nil
}

// parseRepoURLFromEnvelope extracts data.repository_url from a lark-cli JSON
// envelope ({"ok":true,"data":{"repository_url":"..."}}). The field name
// matches the contract emitted by `apps +git-credential-init`.
func parseRepoURLFromEnvelope(stdout string) (string, error) {
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			RepositoryURL string `json:"repository_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		return "", appsSubprocessEnvelopeError("could not parse +git-credential-init output as JSON: %v", err)
	}
	if !env.OK {
		return "", appsSubprocessEnvelopeError("+git-credential-init reported failure")
	}
	if strings.TrimSpace(env.Data.RepositoryURL) == "" {
		return "", appsSubprocessEnvelopeError("+git-credential-init returned no repository_url")
	}
	return env.Data.RepositoryURL, nil
}

// parseEnvFileFromEnvelope extracts data.env_file from a `+env-pull` success
// envelope ({"ok":true,"data":{"env_file":"..."}}) on stdout.
func parseEnvFileFromEnvelope(stdout string) (string, error) {
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			EnvFile string `json:"env_file"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		return "", appsSubprocessEnvelopeError("could not parse +env-pull output as JSON: %v", err)
	}
	if !env.OK {
		return "", appsSubprocessEnvelopeError("+env-pull reported failure")
	}
	if strings.TrimSpace(env.Data.EnvFile) == "" {
		return "", appsSubprocessEnvelopeError("+env-pull returned no env_file")
	}
	return env.Data.EnvFile, nil
}

// parseEnvPullErrorEnvelope extracts a single-line reason from a `+env-pull`
// error envelope ({"ok":false,"error":{"type":...,"message":...}}) on stderr.
// Returns "" when stderr is not a parseable error envelope (caller falls back).
func parseEnvPullErrorEnvelope(stderr string) string {
	var env struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); err != nil {
		return ""
	}
	msg := strings.TrimSpace(env.Error.Message)
	if msg == "" {
		return ""
	}
	if t := strings.TrimSpace(env.Error.Type); t != "" {
		return t + ": " + msg
	}
	return msg
}

// validateRepoURLScheme rejects any repository_url that is not http(s):// to
// block git's dangerous transports (ext::, file://, ssh://) and option injection.
func validateRepoURLScheme(repoURL string) error {
	if strings.HasPrefix(repoURL, "http://") || strings.HasPrefix(repoURL, "https://") {
		return nil
	}
	// The URL comes from the +git-credential-init subprocess response, not user
	// input, so a non-http(s) scheme is a broken upstream contract.
	return appsSubprocessEnvelopeError(
		"repository_url from +git-credential-init must be http(s); refusing %q", redactURLCredentials(repoURL))
}

func appsInitExecute(ctx context.Context, rctx *common.RuntimeContext) error {
	appID := strings.TrimSpace(rctx.Str("app-id"))

	dir, err := resolveTargetPath(rctx, appID)
	if err != nil {
		return err
	}

	// Already-initialized short-circuit: a dir containing .spark/meta.json is an
	// initialized Miaoda app repo -> skip clone/scaffold/commit, but still refresh
	// the local env so a re-run picks up the latest startup env vars.
	if isAlreadyInitialized(dir) {
		initLogf(rctx, "Already initialized at %s — refreshing local environment", dir)
		out := map[string]interface{}{
			"app_id":     appID,
			"clone_path": dir,
			"scaffold":   "already_initialized",
			"committed":  false,
			"pushed":     false,
		}
		initLogf(rctx, "Pulling local environment variables...")
		envFile, envPullErr := pullEnv(ctx, rctx, appID, dir)
		envPulled := envPullErr == ""
		out["env_pulled"] = envPulled
		if envPulled {
			initLogf(rctx, "Local environment written to %s", envFile)
			out["env_file"] = envFile
			out["message"] = "Repository already initialized. Local env refreshed — you can start developing."
		} else {
			initLogf(rctx, "Could not pull local env vars: %s", envPullErr)
			out["env_pull_error"] = envPullErr
			out["message"] = fmt.Sprintf("Repository already initialized. Could not pull local env vars automatically — run `lark-cli apps +env-pull --app-id %s` to retry.", appID)
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✓ Already initialized at %s\n", dir)
			if envPulled {
				fmt.Fprintf(w, "✓ Local environment written to %s\n", envFile)
			} else {
				fmt.Fprintf(w, "⚠ Could not pull local env vars: %s\n", envPullErr)
				fmt.Fprintf(w, "  run `lark-cli apps +env-pull --app-id %s` to retry\n", appID)
			}
			fmt.Fprintln(w, "仓库已初始化完成，可以开始开发了。")
		})
		return nil
	}

	if _, err := exec.LookPath("git"); err != nil {
		return appsFailedPreconditionError("git executable not found on PATH").
			WithHint("install git and ensure it is on your PATH")
	}
	if _, err := exec.LookPath("npx"); err != nil {
		return appsFailedPreconditionError("npx executable not found on PATH").
			WithHint("install Node.js (which provides npx) and ensure it is on your PATH")
	}

	if err := ensureEmptyDir(dir); err != nil {
		return err
	}

	initLogf(rctx, "Issuing repository credentials for %s...", appID)
	repoURL, err := issueCredentials(ctx, rctx, appID)
	if err != nil {
		return err
	}
	if err := validateRepoURLScheme(repoURL); err != nil {
		return err
	}

	initLogf(rctx, "Cloning into %s...", dir)
	if _, stderr, err := initRunner.Run(ctx, "", "git", "clone", "--", repoURL, dir); err != nil {
		return appsExternalToolError(err, "git clone failed: %s", gitErr(stderr, err))
	}
	initLogf(rctx, "Checking out %s...", defaultInitBranch)
	if _, stderr, err := initRunner.Run(ctx, dir, "git", "checkout", defaultInitBranch); err != nil {
		return appsExternalToolError(err, "git checkout %s failed: %s", defaultInitBranch, gitErr(stderr, err))
	}

	initLogf(rctx, "Initializing app code (running miaoda-cli)...")
	scaffold, err := runScaffold(ctx, dir, appID, resolveTemplate(rctx, appID))
	if err != nil {
		return err
	}

	committed, pushed, err := commitAndPushIfDirty(ctx, dir, scaffold)
	if err != nil {
		return err
	}
	if pushed {
		initLogf(rctx, "Committed and pushed to %s", defaultInitBranch)
	} else {
		initLogf(rctx, "Working tree clean — skipped commit/push")
	}

	initLogf(rctx, "Pulling local environment variables...")
	envFile, envPullErr := pullEnv(ctx, rctx, appID, dir)
	envPulled := envPullErr == ""
	if envPulled {
		initLogf(rctx, "Local environment written to %s", envFile)
	} else {
		initLogf(rctx, "Could not pull local env vars: %s", envPullErr)
	}

	out := map[string]interface{}{
		"app_id":         appID,
		"repository_url": redactURLCredentials(repoURL),
		"branch":         defaultInitBranch,
		"clone_path":     dir,
		"scaffold":       scaffold,
		"committed":      committed,
		"pushed":         pushed,
		"env_pulled":     envPulled,
		"message":        "Repository initialized. You can start developing.",
	}
	if envPulled {
		out["env_file"] = envFile
	} else {
		out["env_pull_error"] = envPullErr
		out["message"] = fmt.Sprintf("Repository initialized. Could not pull local env vars automatically — run `lark-cli apps +env-pull --app-id %s` to retry.", appID)
	}
	rctx.OutFormat(out, nil, func(w io.Writer) {
		fmt.Fprintf(w, "✓ Repository initialized at %s\n", dir)
		fmt.Fprintf(w, "  branch: %s\n  scaffold: %s\n", defaultInitBranch, scaffold)
		if envPulled {
			fmt.Fprintf(w, "✓ Local environment written to %s\n", envFile)
		} else {
			fmt.Fprintf(w, "⚠ Could not pull local env vars: %s\n", envPullErr)
			fmt.Fprintf(w, "  run `lark-cli apps +env-pull --app-id %s` to retry\n", appID)
		}
		fmt.Fprintln(w, "仓库已初始化完成，可以开始开发了。")
	})
	return nil
}

// pullEnv runs `<self> apps +env-pull --app-id <appID> --project-path <dir>
// --format json`, forwarding --as when set. Returns (envFile, "") on success or
// ("", reason) on failure. Non-fatal by contract: the caller logs a warning and
// continues. The success envelope is read from stdout, the error envelope from
// stderr (lark-cli writes structured errors to stderr; see cmd/root.go
// handleRootError). The reason is always redacted.
func pullEnv(ctx context.Context, rctx *common.RuntimeContext, appID, dir string) (envFile, reason string) {
	self, err := os.Executable()
	if err != nil {
		return "", redactURLCredentials(fmt.Sprintf("cannot locate lark-cli executable: %v", err))
	}
	args := []string{"apps", "+env-pull", "--app-id", appID, "--project-path", dir, "--format", "json"}
	if as := strings.TrimSpace(rctx.Str("as")); as != "" {
		args = append(args, "--as", as)
	}
	stdout, stderr, runErr := initRunner.Run(ctx, "", self, args...)
	if runErr != nil {
		r := parseEnvPullErrorEnvelope(stderr)
		if r == "" {
			r = gitErr(stderr, runErr)
		}
		return "", redactURLCredentials(r)
	}
	envFile, perr := parseEnvFileFromEnvelope(stdout)
	if perr != nil {
		return "", redactURLCredentials(perr.Error())
	}
	return envFile, ""
}

// issueCredentials runs `<self> apps +git-credential-init --app-id <id> --format json`
// and returns the repo_url it reports. Forwards --as when set.
func issueCredentials(ctx context.Context, rctx *common.RuntimeContext, appID string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", errs.NewInternalError(errs.SubtypeUnknown, "cannot locate lark-cli executable: %v", err).WithCause(err)
	}
	args := []string{"apps", "+git-credential-init", "--app-id", appID, "--format", "json"}
	if as := strings.TrimSpace(rctx.Str("as")); as != "" {
		args = append(args, "--as", as)
	}
	stdout, stderr, err := initRunner.Run(ctx, "", self, args...)
	if err != nil {
		return "", appsExternalToolError(err, "apps +git-credential-init failed: %s", gitErr(stderr, err)).
			WithHint("ensure apps +git-credential-init is available and you are logged in").
			WithCause(err)
	}
	return parseRepoURLFromEnvelope(stdout)
}

// commitAndPushIfDirty commits and pushes only when the working tree has
// changes; a clean tree is a no-op (returns false,false). For the empty-repo
// init path (scaffoldKind == "init") it splits the scaffolded tree into two
// commits — app project code, then Miaoda config (.spark/.agent) — skipping
// either commit when that group has no changes (no empty commits). Other paths
// commit once. Push is a single `git push origin <branch>` for all commits.
func commitAndPushIfDirty(ctx context.Context, dir, scaffoldKind string) (committed, pushed bool, err error) {
	status, stderr, runErr := initRunner.Run(ctx, dir, "git", "status", "--porcelain")
	if runErr != nil {
		return false, false, appsExternalToolError(runErr, "git status failed: %s", gitErr(stderr, runErr))
	}
	if strings.TrimSpace(status) == "" {
		return false, false, nil
	}

	if scaffoldKind == scaffoldKindInit {
		// Stage each group by its exact porcelain paths (never gitignored files),
		// so neither `git add` errors on an ignored path like .agent.
		appPaths, configPaths := classifyPorcelain(status)
		if len(appPaths) > 0 {
			if e := stageAndCommit(ctx, dir, commitMsgAppCode, appPaths...); e != nil {
				return committed, false, e
			}
			committed = true
		}
		if len(configPaths) > 0 {
			if e := stageAndCommit(ctx, dir, commitMsgAppConfig, configPaths...); e != nil {
				return committed, false, e
			}
			committed = true
		}
	} else {
		if e := stageAndCommit(ctx, dir, commitMsgUpgrade, "."); e != nil {
			return false, false, e
		}
		committed = true
	}

	if !committed {
		return false, false, nil
	}

	if _, se, e := initRunner.Run(ctx, dir, "git", "push", "origin", defaultInitBranch); e != nil {
		return true, false, withAppsHint(
			appsExternalToolError(e, "git push failed: %s", gitErr(se, e)),
			"the push was rejected — the git output is in the message above; if it is a non-fast-forward (remote has new commits), sync the remote and retry; if it is an auth failure, make sure `lark-cli apps +git-credential-init` has succeeded")
	}
	return true, true, nil
}

// stageAndCommit stages the given pathspecs (`git add -A -- <pathspecs>`) and
// makes one `git commit --no-verify -m message`. --no-verify skips the scaffold
// repo's local pre-commit / commit-msg hooks (local only; the later push is not
// --no-verify). Callers gate this on classifyPorcelain so the group is non-empty
// and the commit never hits "nothing to commit".
func stageAndCommit(ctx context.Context, dir, message string, pathspecs ...string) error {
	addArgs := append([]string{"add", "-A", "--"}, pathspecs...)
	if _, se, e := initRunner.Run(ctx, dir, "git", addArgs...); e != nil {
		return appsExternalToolError(e, "git add failed: %s", gitErr(se, e))
	}
	if _, se, e := initRunner.Run(ctx, dir, "git", "commit", "--no-verify", "-m", message); e != nil {
		return appsExternalToolError(e, "git commit failed: %s", gitErr(se, e))
	}
	return nil
}

// classifyPorcelain parses `git status --porcelain` output and partitions the
// changed paths into the "app code" group (anything outside .spark/ and .agent/)
// and the "Miaoda config" group (.spark/ and .agent/). It returns the exact
// porcelain paths so callers can stage them verbatim: porcelain never lists
// gitignored files, so `git add -- <these paths>` never trips git's ignored-path
// error. (Naming an ignored dir explicitly — or combining a "." pathspec with
// :(exclude) magic — DOES error when a scaffold template gitignores e.g. .agent,
// which is why we stage exact paths instead of pathspecs.)
func classifyPorcelain(status string) (appPaths, configPaths []string) {
	for _, line := range strings.Split(status, "\n") {
		p := porcelainPath(line)
		if p == "" {
			continue
		}
		if isConfigPath(p) {
			configPaths = append(configPaths, p)
		} else {
			appPaths = append(appPaths, p)
		}
	}
	return appPaths, configPaths
}

// porcelainPath extracts the path from a `git status --porcelain` v1 line.
// Format is "XY <path>" (2 status chars + space); rename/copy lines are
// "XY <orig> -> <dest>" (dest is what matters). Quoted paths are unquoted.
func porcelainPath(line string) string {
	if len(line) < 4 {
		return ""
	}
	p := line[3:]
	if i := strings.Index(p, " -> "); i >= 0 {
		p = p[i+len(" -> "):]
	}
	p = strings.TrimSpace(p)
	p = strings.Trim(p, `"`)
	return p
}

// isConfigPath reports whether p is the Miaoda app-config group: the .spark or
// .agent directory itself, or anything under them. ".sparkrc" is NOT config.
func isConfigPath(p string) bool {
	return p == ".spark" || p == ".agent" ||
		strings.HasPrefix(p, ".spark/") || strings.HasPrefix(p, ".agent/")
}

// gitErr builds a redacted, single-line error detail from stderr (falling back
// to the exec error). Always redacts embedded credentials.
func gitErr(stderr string, err error) string {
	s := strings.TrimSpace(stderr)
	if s == "" && err != nil {
		s = err.Error()
	}
	return redactURLCredentials(s)
}
