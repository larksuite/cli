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

	"github.com/larksuite/cli/internal/charcheck"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// defaultInitBranch is the fixed remote branch +init checks out after clone.
const defaultInitBranch = "sprint/default"

// initCommitMessage is the fixed commit subject used when the post-init working
// tree has changes to push. Fixed constant — never interpolates user input.
const initCommitMessage = "chore: scaffold app via lark-cli apps +init"

const (
	miaodaCLIPkg    = "@lark-apaas/miaoda-cli@alpha"
	defaultTemplate = "nestjs-react-fullstack"
	metaRelPath     = ".spark/meta.json"
	steeringRelPath = ".agent/skills/steering"
	seedReadme      = "README.md"
)

// initRunner is the commandRunner used by +init. Package-level so unit tests
// can swap in a fakeCommandRunner. Production uses execCommandRunner.
var initRunner commandRunner = execCommandRunner{}

// AppsInit initializes a Miaoda app's local development repository.
var AppsInit = common.Shortcut{
	Service:     appsService,
	Command:     "+init",
	Description: "Initialize a Miaoda app's local development repository",
	Risk:        "write",
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
		// check lives in Validate (output.ErrValidation -> ExitValidation=2).
		{Name: "app-id", Desc: "Miaoda app ID"},
		{Name: "dir", Desc: "clone target directory; absolute or relative path (default ./<app-id>)"},
		{Name: "template", Desc: "scaffold template for an empty repo; optional — if omitted, derived from the app's tech stack"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return output.ErrValidation("--app-id is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		template := resolveTemplate(rctx, appID)
		dry := common.NewDryRunAPI().
			Desc("Initialize Miaoda app repository (credential-init, clone, checkout, npx scaffold, optional commit/push)").
			Set("credential_init", fmt.Sprintf("apps +git-credential-init --app-id %s --format json", appID)).
			Set("checkout", "git checkout "+defaultInitBranch).
			Set("scaffold", fmt.Sprintf("empty repo: npx %s app init --template %s --app-id %s; non-empty: npx %s app upgrade + .spark/meta.json app_id patch + conditional skills sync", miaodaCLIPkg, template, appID, miaodaCLIPkg)).
			Set("commit_push", "conditional: git add -A + commit + push origin "+defaultInitBranch+" when the working tree has changes").
			Set("template", template)
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
		return "", output.ErrValidation("--dir must not contain control characters")
	}
	if err := charcheck.RejectControlChars(raw, "--dir"); err != nil {
		return "", output.ErrValidation("%v", err)
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", output.ErrValidation("--dir cannot be resolved: %v", err)
	}
	return abs, nil
}

// ensureEmptyDir refuses to clone into an existing non-empty dir, a symlink, or
// a non-directory. A non-existent path is fine (git clone creates it). Uses
// Lstat so a symlinked target is rejected rather than followed.
func ensureEmptyDir(dir string) error {
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return output.ErrValidation("--dir cannot be read: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return output.ErrValidation("--dir must not be a symlink: %q", dir)
	}
	if !info.IsDir() {
		return output.ErrValidation("--dir exists and is not a directory: %q", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return output.ErrValidation("--dir cannot be read: %v", err)
	}
	if len(entries) > 0 {
		return output.ErrValidation("target directory %q already exists and is not empty", dir)
	}
	return nil
}

// isAlreadyInitialized reports whether dir is an already-initialized Miaoda app
// repo, detected by the presence of <dir>/.spark/meta.json (regardless of its
// app_id value). Used to short-circuit +init into a friendly no-op.
func isAlreadyInitialized(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, metaRelPath))
	return err == nil && !info.IsDir()
}

// ensureMetaAppID patches <dir>/.spark/meta.json to include app_id when the file
// exists but lacks (or has an empty) app_id. Other fields are preserved. When
// the file does not exist, this is a no-op (we never create it).
func ensureMetaAppID(dir, appID string) error {
	path := filepath.Join(dir, metaRelPath)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return output.Errorf(output.ExitAPI, "meta_write", "read %s failed: %v", metaRelPath, err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return output.Errorf(output.ExitAPI, "meta_write", "parse %s failed: %v", metaRelPath, err)
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
		return output.Errorf(output.ExitAPI, "meta_write", "marshal %s failed: %v", metaRelPath, err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return output.Errorf(output.ExitAPI, "meta_write", "write %s failed: %v", metaRelPath, err)
	}
	return nil
}

// hasSteeringSkills reports whether <dir>/.agent/skills/steering exists as a dir.
func hasSteeringSkills(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, steeringRelPath))
	return err == nil && info.IsDir()
}

// isEmptyRepo reports whether the checked-out branch has no tracked files
// other than the backend's default seed README.md. `git ls-files` listing
// nothing — or only README.md — counts as empty (→ scaffold via `app init`).
func isEmptyRepo(ctx context.Context, dir string) (bool, error) {
	stdout, stderr, err := initRunner.Run(ctx, dir, "git", "ls-files")
	if err != nil {
		return false, output.Errorf(output.ExitAPI, "git_ls_files", "git ls-files failed: %s", gitErr(stderr, err))
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
// Empty repo -> `app init`; non-empty -> `app upgrade` + meta app_id patch +
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
		if _, stderr, err := initRunner.Run(ctx, dir, "npx", miaodaCLIPkg, "app", "init", "--template", template, "--app-id", appID); err != nil {
			return "", output.Errorf(output.ExitAPI, "npx_app_init", "npx app init failed: %s", gitErr(stderr, err))
		}
		return "init", nil
	}
	if _, stderr, err := initRunner.Run(ctx, dir, "npx", miaodaCLIPkg, "app", "upgrade"); err != nil {
		return "", output.Errorf(output.ExitAPI, "npx_app_upgrade", "npx app upgrade failed: %s", gitErr(stderr, err))
	}
	if err := ensureMetaAppID(dir, appID); err != nil {
		return "", err
	}
	if !hasSteeringSkills(dir) {
		if _, stderr, err := initRunner.Run(ctx, dir, "npx", miaodaCLIPkg, "skills", "sync"); err != nil {
			return "", output.Errorf(output.ExitAPI, "npx_skills_sync", "npx skills sync failed: %s", gitErr(stderr, err))
		}
	}
	return "upgrade", nil
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
		return "", output.Errorf(output.ExitInternal, "credential_init", "could not parse +git-credential-init output as JSON: %v", err)
	}
	if !env.OK {
		return "", output.Errorf(output.ExitInternal, "credential_init", "+git-credential-init reported failure")
	}
	if strings.TrimSpace(env.Data.RepositoryURL) == "" {
		return "", output.Errorf(output.ExitInternal, "credential_init", "+git-credential-init returned no repository_url")
	}
	return env.Data.RepositoryURL, nil
}

// validateRepoURLScheme rejects any repository_url that is not http(s):// to
// block git's dangerous transports (ext::, file://, ssh://) and option injection.
func validateRepoURLScheme(repoURL string) error {
	if strings.HasPrefix(repoURL, "http://") || strings.HasPrefix(repoURL, "https://") {
		return nil
	}
	return output.Errorf(output.ExitValidation, "validation",
		"repository_url from +git-credential-init must be http(s); refusing %q", redactURLCredentials(repoURL))
}

func appsInitExecute(ctx context.Context, rctx *common.RuntimeContext) error {
	appID := strings.TrimSpace(rctx.Str("app-id"))

	dir, err := resolveTargetPath(rctx, appID)
	if err != nil {
		return err
	}

	// Already-initialized short-circuit: a dir containing .spark/meta.json is an
	// initialized Miaoda app repo -> friendly no-op, no clone/scaffold.
	if isAlreadyInitialized(dir) {
		initLogf(rctx, "Already initialized at %s — nothing to do", dir)
		out := map[string]interface{}{
			"app_id":     appID,
			"clone_path": dir,
			"scaffold":   "already_initialized",
			"committed":  false,
			"pushed":     false,
			"message":    "Repository already initialized. You can start developing.",
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✓ Already initialized at %s\n", dir)
			fmt.Fprintln(w, "仓库已初始化完成，可以开始开发了。")
		})
		return nil
	}

	if _, err := exec.LookPath("git"); err != nil {
		return output.ErrWithHint(output.ExitInternal, "dependency",
			"git executable not found on PATH", "install git and ensure it is on your PATH")
	}
	if _, err := exec.LookPath("npx"); err != nil {
		return output.ErrWithHint(output.ExitInternal, "dependency",
			"npx executable not found on PATH", "install Node.js (which provides npx) and ensure it is on your PATH")
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
		return output.Errorf(output.ExitAPI, "git_clone", "git clone failed: %s", gitErr(stderr, err))
	}
	initLogf(rctx, "Checking out %s...", defaultInitBranch)
	if _, stderr, err := initRunner.Run(ctx, dir, "git", "checkout", defaultInitBranch); err != nil {
		return output.Errorf(output.ExitAPI, "git_checkout", "git checkout %s failed: %s", defaultInitBranch, gitErr(stderr, err))
	}

	initLogf(rctx, "Scaffolding (running miaoda-cli)...")
	scaffold, err := runScaffold(ctx, dir, appID, resolveTemplate(rctx, appID))
	if err != nil {
		return err
	}

	committed, pushed, err := commitAndPushIfDirty(ctx, dir)
	if err != nil {
		return err
	}
	if pushed {
		initLogf(rctx, "Committed and pushed to %s", defaultInitBranch)
	} else {
		initLogf(rctx, "Working tree clean — skipped commit/push")
	}

	out := map[string]interface{}{
		"app_id":         appID,
		"repository_url": redactURLCredentials(repoURL),
		"branch":         defaultInitBranch,
		"clone_path":     dir,
		"scaffold":       scaffold,
		"committed":      committed,
		"pushed":         pushed,
		"message":        "Repository initialized. You can start developing.",
	}
	rctx.OutFormat(out, nil, func(w io.Writer) {
		fmt.Fprintf(w, "✓ Repository initialized at %s\n", dir)
		fmt.Fprintf(w, "  branch: %s\n  scaffold: %s\n", defaultInitBranch, scaffold)
		fmt.Fprintln(w, "仓库已初始化完成，可以开始开发了。")
	})
	return nil
}

// issueCredentials runs `<self> apps +git-credential-init --app-id <id> --format json`
// and returns the repo_url it reports. Forwards --as when set.
func issueCredentials(ctx context.Context, rctx *common.RuntimeContext, appID string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", output.Errorf(output.ExitInternal, "internal", "cannot locate lark-cli executable: %v", err)
	}
	args := []string{"apps", "+git-credential-init", "--app-id", appID, "--format", "json"}
	if as := strings.TrimSpace(rctx.Str("as")); as != "" {
		args = append(args, "--as", as)
	}
	stdout, stderr, err := initRunner.Run(ctx, "", self, args...)
	if err != nil {
		return "", output.ErrWithHint(output.ExitAPI, "credential_init",
			fmt.Sprintf("apps +git-credential-init failed: %s", gitErr(stderr, err)),
			"ensure apps +git-credential-init is available and you are logged in")
	}
	return parseRepoURLFromEnvelope(stdout)
}

// commitAndPushIfDirty commits and pushes only when the working tree has
// changes; a clean tree is a no-op (returns false,false).
func commitAndPushIfDirty(ctx context.Context, dir string) (committed, pushed bool, err error) {
	status, stderr, runErr := initRunner.Run(ctx, dir, "git", "status", "--porcelain")
	if runErr != nil {
		return false, false, output.Errorf(output.ExitAPI, "git_status", "git status failed: %s", gitErr(stderr, runErr))
	}
	if strings.TrimSpace(status) == "" {
		return false, false, nil
	}
	if _, se, e := initRunner.Run(ctx, dir, "git", "add", "-A"); e != nil {
		return false, false, output.Errorf(output.ExitAPI, "git_add", "git add failed: %s", gitErr(se, e))
	}
	// --no-verify skips the scaffold repo's pre-commit / commit-msg hooks, which
	// the miaoda template may carry and which would otherwise block or prompt on
	// this automated init commit. Local hooks only — signing/remote checks are
	// unaffected.
	if _, se, e := initRunner.Run(ctx, dir, "git", "commit", "--no-verify", "-m", initCommitMessage); e != nil {
		return false, false, output.Errorf(output.ExitAPI, "git_commit", "git commit failed: %s", gitErr(se, e))
	}
	if _, se, e := initRunner.Run(ctx, dir, "git", "push", "origin", defaultInitBranch); e != nil {
		return true, false, output.Errorf(output.ExitAPI, "git_push", "git push failed: %s", gitErr(se, e))
	}
	return true, true, nil
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
