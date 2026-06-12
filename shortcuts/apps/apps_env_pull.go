// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// envKeyPattern matches valid environment variable names: [A-Za-z_][A-Za-z0-9_]*
var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type envPullDatabaseInfo struct {
	Detected      bool
	ExpiresAtRaw  string
	ExpiresAtText string
}

// AppsEnvPull pulls startup env vars for an app into the local .env.local file.
var AppsEnvPull = common.Shortcut{
	Service:     appsService,
	Command:     "+env-pull",
	Description: "Pull app startup env vars into the local project .env.local",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +env-pull --app-id <app_id>",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID"},
		{Name: "project-path", Desc: "local project root path (defaults to current directory)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		_, envFile, err := resolveEnvPullTarget(strings.TrimSpace(rctx.Str("project-path")))
		if err != nil {
			return appsValidationParamError("--project-path", "--project-path: %v", err).WithCause(err)
		}
		if err := checkEnvPullTarget(envFile); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		projectPath, envFile, _ := resolveEnvPullTarget(strings.TrimSpace(rctx.Str("project-path")))
		appID := strings.TrimSpace(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			POST(fmt.Sprintf("%s/apps/%s/env_vars", apiBasePath, validate.EncodePathSegment(appID))).
			Desc("Pull app startup env vars into the local .env.local file").
			Set("project_path", projectPath).
			Set("env_file", envFile)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		_, envFile, err := resolveEnvPullTarget(strings.TrimSpace(rctx.Str("project-path")))
		if err != nil {
			return appsValidationParamError("--project-path", "--project-path: %v", err).WithCause(err)
		}
		if err := checkEnvPullTarget(envFile); err != nil {
			return err
		}
		if err := rctx.EnsureScopes([]string{"spark:app:read"}); err != nil {
			return err
		}

		path := fmt.Sprintf("%s/apps/%s/env_vars", apiBasePath, validate.EncodePathSegment(appID))
		data, err := rctx.CallAPITyped("POST", path, nil, nil)
		if err != nil {
			return withAppsHint(err, "verify --app-id is correct and you have access to the app; list your apps with `lark-cli apps +list`")
		}

		envVars, databaseInfo, skippedKeys, err := extractEnvPullVars(data)
		if err != nil {
			return err
		}
		if envVars == nil {
			envVars = map[string]string{}
		}
		envVars["FORCE_DB_BRANCH"] = "dev"
		original, err := readEnvPullFile(envFile)
		if err != nil {
			return err
		}
		merged, updated, created := mergeEnvPullFileContent(original, envVars)
		if err := ensureEnvPullParentDir(envFile); err != nil {
			return err
		}
		if err := validate.AtomicWrite(envFile, []byte(merged), 0o600); err != nil {
			return &errs.InternalError{Problem: errs.Problem{Category: errs.CategoryInternal, Subtype: errs.SubtypeUnknown, Message: fmt.Sprintf("cannot write %s: %v", envFile, err)}, Cause: err}
		}

		result := buildEnvPullSuccessData(appID, envFile, databaseInfo)
		rctx.OutFormat(result, nil, func(w io.Writer) {
			writeEnvPullPretty(w, appID, envFile, databaseInfo, skippedKeys)
		})
		_ = updated
		_ = created
		return nil
	},
}

func resolveEnvPullTarget(projectPath string) (string, string, error) {
	if strings.TrimSpace(projectPath) == "" {
		cwd, err := os.Getwd() //nolint:forbidigo // shortcuts cannot import internal/vfs; cwd lookup is local-only and bounded.
		if err != nil {
			return "", "", errs.NewInternalError(errs.SubtypeUnknown, "cannot determine working directory: %v", err).WithCause(err)
		}
		projectPath = cwd
	}
	if err := validate.RejectControlChars(projectPath, "--project-path"); err != nil {
		return "", "", err
	}
	projectPath = filepath.Clean(projectPath)
	return projectPath, filepath.Join(projectPath, ".env.local"), nil
}

func checkEnvPullTarget(envFile string) error {
	info, err := os.Lstat(envFile) //nolint:forbidigo // shortcuts cannot import internal/vfs; direct lstat is needed to reject symlinks before write.
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return appsValidationParamError("--project-path", "cannot inspect %s: %v", envFile, err).WithCause(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return appsValidationParamError("--project-path", "target %s must be a regular file, not a symlink", envFile)
	}
	if !info.Mode().IsRegular() {
		return appsValidationParamError("--project-path", "target %s must be a regular file", envFile)
	}
	return nil
}

func extractEnvPullVars(data map[string]interface{}) (map[string]string, envPullDatabaseInfo, []string, error) {
	raw := data["env_vars"]
	if raw == nil {
		if nested, ok := data["data"].(map[string]interface{}); ok {
			raw = nested["env_vars"]
		}
	}
	if raw == nil {
		return nil, envPullDatabaseInfo{}, nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "response field env_vars must be an object or array of key/value entries")
	}

	var skippedKeys []string
	switch typed := raw.(type) {
	case map[string]interface{}:
		out := make(map[string]string, len(typed))
		for key, value := range typed {
			if !envKeyPattern.MatchString(key) {
				skippedKeys = append(skippedKeys, key)
				continue
			}
			s, ok := value.(string)
			if !ok {
				continue
			}
			out[key] = s
		}
		return out, envPullDatabaseInfo{Detected: hasEnvPullDatabase(out)}, skippedKeys, nil
	case []interface{}:
		out := make(map[string]string, len(typed))
		info := envPullDatabaseInfo{}
		for _, item := range typed {
			entry, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			key, ok := entry["key"].(string)
			if !ok || strings.TrimSpace(key) == "" {
				continue
			}
			if !envKeyPattern.MatchString(key) {
				skippedKeys = append(skippedKeys, key)
				continue
			}
			value, ok := entry["value"].(string)
			if !ok {
				continue
			}
			out[key] = value
			if key == "SUDA_DATABASE_URL" {
				info.Detected = true
				info.ExpiresAtRaw, info.ExpiresAtText = extractEnvPullDatabaseExpiry(entry["extras"])
			}
		}
		return out, info, skippedKeys, nil
	default:
		return nil, envPullDatabaseInfo{}, nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "response field env_vars must be an object or array of key/value entries")
	}
}

func readEnvPullFile(envFile string) (string, error) {
	data, err := os.ReadFile(envFile) //nolint:forbidigo // shortcuts cannot import internal/vfs; validated local file read for a single env file.
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", &errs.InternalError{Problem: errs.Problem{Category: errs.CategoryInternal, Subtype: errs.SubtypeUnknown, Message: fmt.Sprintf("cannot read %s: %v", envFile, err)}, Cause: err}
	}
	return string(data), nil
}

func ensureEnvPullParentDir(envFile string) error {
	dir := filepath.Dir(envFile)
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:forbidigo // shortcuts cannot import internal/vfs; local mkdir for target env parent dir.
		return &errs.InternalError{Problem: errs.Problem{Category: errs.CategoryInternal, Subtype: errs.SubtypeUnknown, Message: fmt.Sprintf("cannot create %s: %v", dir, err)}, Cause: err}
	}
	return nil
}

func mergeEnvPullFileContent(original string, envVars map[string]string) (string, []string, []string) {
	if len(envVars) == 0 {
		if original == "" {
			return "", nil, nil
		}
		return ensureTrailingNewline(original), nil, nil
	}

	normalized := strings.ReplaceAll(original, "\r\n", "\n")
	lines := []string{}
	if normalized != "" {
		lines = strings.Split(normalized, "\n")
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}

	used := make(map[string]bool, len(envVars))
	updated := make([]string, 0, len(envVars))
	for i, line := range lines {
		key, ok := parseEnvPullAssignmentLine(line)
		if !ok {
			continue
		}
		value, exists := envVars[key]
		if !exists {
			continue
		}
		lines[i] = formatEnvPullAssignment(key, value)
		updated = append(updated, key)
		used[key] = true
	}

	created := make([]string, 0, len(envVars))
	pending := make([]string, 0, len(envVars))
	for key := range envVars {
		if used[key] {
			continue
		}
		pending = append(pending, key)
	}
	sort.Strings(pending)
	for _, key := range pending {
		lines = append(lines, formatEnvPullAssignment(key, envVars[key]))
		created = append(created, key)
	}

	sort.Strings(updated)
	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	return content, updated, created
}

func parseEnvPullAssignmentLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	if strings.HasPrefix(trimmed, "export ") || strings.HasPrefix(trimmed, "export\t") {
		remainder := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(trimmed, "export "), "export\t"))
		if remainder == "" || strings.HasPrefix(remainder, "=") {
			return "", false
		}
		trimmed = remainder
	}
	idx := strings.Index(trimmed, "=")
	if idx <= 0 {
		return "", false
	}
	key := strings.TrimSpace(trimmed[:idx])
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", false
	}
	return key, true
}

func formatEnvPullAssignment(key, value string) string {
	return fmt.Sprintf("%s=%s", key, strconv.Quote(value))
}

func buildEnvPullSuccessData(appID, envFile string, databaseInfo envPullDatabaseInfo) map[string]interface{} {
	result := map[string]interface{}{
		"app_id":   appID,
		"env_file": envFile,
	}
	if databaseInfo.ExpiresAtRaw != "" {
		result["database_url_expires_at"] = databaseInfo.ExpiresAtRaw
	}
	return result
}

func hasEnvPullDatabase(envVars map[string]string) bool {
	_, ok := envVars["SUDA_DATABASE_URL"]
	return ok
}

func extractEnvPullDatabaseExpiry(rawExtras interface{}) (string, string) {
	extras, ok := rawExtras.([]interface{})
	if !ok {
		return "", ""
	}
	for _, raw := range extras {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		key, _ := entry["key"].(string)
		if key != "expiresAt" {
			continue
		}
		switch value := entry["value"].(type) {
		case string:
			rawValue := strings.TrimSpace(value)
			ts, err := strconv.ParseInt(rawValue, 10, 64)
			if err != nil {
				return "", ""
			}
			return rawValue, time.Unix(ts, 0).Local().Format("2006-01-02 15:04:05 MST")
		case float64:
			ts := int64(value)
			rawValue := strconv.FormatInt(ts, 10)
			return rawValue, time.Unix(ts, 0).Local().Format("2006-01-02 15:04:05 MST")
		}
	}
	return "", ""
}

func writeEnvPullPretty(w io.Writer, appID, envFile string, databaseInfo envPullDatabaseInfo, skippedKeys []string) {
	fmt.Fprintf(w, "✓ App detected: %s\n", appID)
	if databaseInfo.Detected {
		fmt.Fprintln(w, "✓ Development database detected")
	}
	fmt.Fprintf(w, "✓ Local environment written to %s\n", envFile)
	if databaseInfo.ExpiresAtText != "" {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "DATABASE_URL is valid until %s.\n", databaseInfo.ExpiresAtText)
	}
	if len(skippedKeys) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "⚠ Skipped %d invalid key(s): %s (key names must match [A-Za-z_][A-Za-z0-9_]*)\n", len(skippedKeys), strings.Join(skippedKeys, ", "))
	}
	fmt.Fprintf(w, "Run `lark-cli apps +env-pull --app-id <app_id>` again to refresh it.\n")
}

func ensureTrailingNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}
