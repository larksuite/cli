// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !authsidecar

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/envvars"
)

func TestCheckNoAuthsidecarBuild_Unset(t *testing.T) {
	var stderr bytes.Buffer
	code := checkNoAuthsidecarBuild(func(string) string { return "" }, &stderr)
	if code != 0 {
		t.Errorf("exit code = %d, want 0 when AUTH_PROXY is unset", code)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr should be empty, got %q", stderr.String())
	}
}

// TestCheckNoAuthsidecarBuild_Set verifies that deploying a plain build into
// a sandbox that expects sidecar isolation fails loudly at startup instead
// of silently leaking credentials through the env provider path.
func TestCheckNoAuthsidecarBuild_Set(t *testing.T) {
	var stderr bytes.Buffer
	env := func(k string) string {
		if k == envvars.CliAuthProxy {
			return "http://127.0.0.1:16384"
		}
		return ""
	}
	code := checkNoAuthsidecarBuild(env, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code when AUTH_PROXY is set")
	}
	msg := stderr.String()
	for _, want := range []string{
		envvars.CliAuthProxy,
		"authsidecar", // build-tag name must appear so operators can act on it
		"rebuild",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("stderr message missing %q; got:\n%s", want, msg)
		}
	}
}
