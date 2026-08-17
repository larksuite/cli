// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalWrapperCommandSurface(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "business-cli")
	build := exec.Command("go", "build", "-o", binary, "./testdata/wrapper")
	build.Dir = packageDir
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build wrapper: %v\n%s", err, output)
	}

	configDir := t.TempDir()
	baseEnv := withoutEnvironment(os.Environ(),
		"LARKSUITE_CLI_CONFIG_DIR", "LARKSUITE_CLI_APP_ID", "LARKSUITE_CLI_APP_SECRET",
		"LARKSUITE_CLI_USER_ACCESS_TOKEN", "LARKSUITE_CLI_PROFILE",
	)
	run := func(args ...string) string {
		t.Helper()
		process := exec.Command(binary, args...)
		process.Env = append(baseEnv,
			"LARKSUITE_CLI_CONFIG_DIR="+configDir,
			"LARKSUITE_CLI_APP_ID=wrapper_test_app",
			"LARKSUITE_CLI_APP_SECRET=wrapper_test_secret",
			"LARKSUITE_CLI_USER_ACCESS_TOKEN=expired_wrapper_token",
			"LARKSUITE_CLI_NO_UPDATE_NOTIFIER=1",
			"LARKSUITE_CLI_NO_SKILLS_NOTIFIER=1",
		)
		output, err := process.CombinedOutput()
		if err != nil {
			t.Fatalf("wrapper %v: %v\n%s", args, err, output)
		}
		return string(output)
	}

	dryRun := run("im", "+wrapper-read", "--id", "chat_1", "--as", "user", "--dry-run")
	if !strings.Contains(dryRun, `"dry_run": true`) || !strings.Contains(dryRun, "/open-apis/im/v1/chats/chat_1") {
		t.Fatalf("wrapper dry-run = %s", dryRun)
	}
	backupDryRun := run("drive", "+wrapper-backup", "--file-token", "file_1", "--output", "reports/file.bin", "--as", "user", "--dry-run")
	if !strings.Contains(backupDryRun, "/open-apis/drive/v1/files/file_1/download_url") || !strings.Contains(backupDryRun, `"files"`) ||
		!strings.Contains(backupDryRun, `"name": "reports/file.bin"`) || !strings.Contains(backupDryRun, `"if_exists": "fail"`) {
		t.Fatalf("wrapper backup dry-run = %s", backupDryRun)
	}
	// A value that is unchanged by escaping cannot tell whether the wrapper
	// wrapped it at all, so send a separator: dropping PathSegment retargets
	// the request at a sibling resource, and ValidateRequestView would not
	// catch it because the concatenated path is still canonical.
	escaped := run("im", "+wrapper-read", "--id", "chat/1", "--as", "user", "--dry-run")
	if !strings.Contains(escaped, "/open-apis/im/v1/chats/chat%2F1") {
		t.Fatalf("wrapper did not escape the path segment: %s", escaped)
	}
	if strings.Contains(escaped, "/open-apis/im/v1/chats/chat/1") {
		t.Fatalf("wrapper leaked an unescaped separator into the path: %s", escaped)
	}
	schema := run("schema", "im", "+wrapper-read")
	if !strings.Contains(schema, `"name": "im +wrapper-read"`) || !strings.Contains(schema, `"outputSchema"`) {
		t.Fatalf("wrapper schema = %s", schema)
	}
	completion := run("__complete", "im", "+wrap")
	if !strings.Contains(completion, "+wrapper-read") {
		t.Fatalf("wrapper completion = %s", completion)
	}
	skills := run("skills", "list")
	if !strings.Contains(skills, "lark-doc") {
		t.Fatalf("wrapper skills = %s", skills)
	}
}

func withoutEnvironment(environment []string, names ...string) []string {
	blocked := make(map[string]struct{}, len(names))
	for _, name := range names {
		blocked[name] = struct{}{}
	}
	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if _, ok := blocked[name]; !ok {
			result = append(result, entry)
		}
	}
	return result
}
