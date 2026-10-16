// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Platform-managed env keys that control the app database. The server is the
// authority for them; user variables and other platform keys (e.g. the Redis
// MIAODA_CACHE_URL) are merged as before and never cleared here.
const (
	envKeyDisableDatapass = "FORCE_FRAMEWORK_DISABLE_DATAPASS"
	envKeyTempDatabaseURL = "SUDA_DATABASE_URL"
	envKeyDatabaseURL     = "FORCE_DB_CONNECT_URL"
	envKeyDBBranch        = "FORCE_DB_BRANCH"
)

// envPullDevBranch is the database branch local development always uses.
const envPullDevBranch = "dev"

var envPullPGConnectionKeys = []string{envKeyTempDatabaseURL, envKeyDatabaseURL}

// envPullManagedKeys are the database keys whose stale values must not win
// over this pull, so they are also checked against the current shell.
var envPullManagedKeys = []string{envKeyDisableDatapass, envKeyTempDatabaseURL, envKeyDatabaseURL, envKeyDBBranch}

// envPullDBExport is what the pulled environment says about the local
// database integration. It only describes the exported keys: it neither tells
// whether the app's database feature is enabled (legacy USER/TCC variables can carry
// the same names) nor proves the database is reachable.
type envPullDBExport string

const (
	// envPullDBDisabled: the server explicitly disabled the local database
	// integration and exported both connection keys as empty values.
	envPullDBDisabled envPullDBExport = "disabled"
	// envPullDBConnectionExported: the server exported a database connection
	// for the dev branch.
	envPullDBConnectionExported envPullDBExport = "connection_exported"
)

// applyEnvPullDBKeys sets the platform database keys in envVars to their final
// local values. The env response carries no contract marker, so only two
// explicit answers are recognized:
//   - FORCE_FRAMEWORK_DISABLE_DATAPASS="true" with SUDA_DATABASE_URL and
//     FORCE_DB_CONNECT_URL both present and empty: the connection keys and
//     FORCE_DB_BRANCH are written as empty strings, clearing stale local
//     connections;
//   - "false" with a non-empty SUDA_DATABASE_URL: a connection was exported
//     and local development uses the dev branch.
//
// Any other shape keeps the established behavior of pinning FORCE_DB_BRANCH to
// dev and returns "": a missing key is not an explicit clear, so local values
// the server did not send are left alone, and a server that has no dev
// database for the app answers with an error instead.
func applyEnvPullDBKeys(envVars map[string]string) envPullDBExport {
	switch envVars[envKeyDisableDatapass] {
	case "true":
		if envPullConnectionsExplicitlyEmpty(envVars) {
			for _, key := range envPullPGConnectionKeys {
				envVars[key] = ""
			}
			envVars[envKeyDBBranch] = ""
			return envPullDBDisabled
		}
	case "false":
		if strings.TrimSpace(envVars[envKeyTempDatabaseURL]) != "" {
			envVars[envKeyDBBranch] = envPullDevBranch
			return envPullDBConnectionExported
		}
	}
	envVars[envKeyDBBranch] = envPullDevBranch
	return ""
}

// envPullConnectionsExplicitlyEmpty reports whether the server sent every
// connection key and left all of them empty.
func envPullConnectionsExplicitlyEmpty(envVars map[string]string) bool {
	for _, key := range envPullPGConnectionKeys {
		value, sent := envVars[key]
		if !sent || strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

// envPullShellConflicts lists managed database keys exported by the current
// shell with a value different from the one just written. dotenv loading lets
// the shell win, so a dev process started from this shell would keep them.
// Only key names are returned; values may be credentials.
func envPullShellConflicts(envVars map[string]string) []string {
	var conflicts []string
	for _, key := range envPullManagedKeys {
		want, managed := envVars[key]
		if !managed {
			continue
		}
		if got, set := os.LookupEnv(key); set && got != want {
			conflicts = append(conflicts, key)
		}
	}
	sort.Strings(conflicts)
	return conflicts
}

func envPullShellConflictMessage(conflicts []string) string {
	return fmt.Sprintf("the current shell exports %s with values different from .env.local; a dev process started from this shell keeps the old values — run `unset %s` or open a new shell, then stop and restart the local dev process",
		strings.Join(conflicts, ", "), strings.Join(conflicts, " "))
}

func addEnvPullDBData(result map[string]any, conflicts []string) {
	// Writing the file never changes an already running process.
	result["restart_required"] = true
	if len(conflicts) > 0 {
		result["shell_conflicts"] = conflicts
	}
}

func writeEnvPullDBPretty(w io.Writer, export envPullDBExport) {
	switch export {
	case envPullDBDisabled:
		fmt.Fprintln(w, "Local database integration: disabled by the pulled environment (database connection settings cleared in .env.local)")
	case envPullDBConnectionExported:
		fmt.Fprintf(w, "Local database integration: connection exported (branch %s); this does not prove the database is reachable\n", envPullDevBranch)
	}
	fmt.Fprintln(w, "Restart the whole local dev process so it loads the new settings; a running process keeps its old environment. Then verify one real database read/write.")
}
