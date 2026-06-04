// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package ratelimit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/output"
)

func TestCrossProcessWindowIsShared(t *testing.T) {
	if os.Getenv("MAIL_RATELIMIT_HELPER") == "1" {
		runProcessHelper()
		return
	}

	dir := t.TempDir()
	const workers = 5
	type helper struct {
		cmd    *exec.Cmd
		stdout bytes.Buffer
		stderr bytes.Buffer
	}
	helpers := make([]*helper, 0, workers)
	for i := 0; i < workers; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=TestCrossProcessWindowIsShared")
		h := &helper{cmd: cmd}
		cmd.Env = append(os.Environ(),
			"MAIL_RATELIMIT_HELPER=1",
			"MAIL_RATELIMIT_STATE_DIR="+dir,
		)
		cmd.Stdout = &h.stdout
		cmd.Stderr = &h.stderr
		if err := cmd.Start(); err != nil {
			t.Fatalf("start helper %d: %v", i, err)
		}
		helpers = append(helpers, h)
	}
	allowed := 0
	limited := 0
	for i, h := range helpers {
		err := h.cmd.Wait()
		out := h.stdout.String()
		if err != nil {
			t.Fatalf("helper %d failed: %v\nstdout=%s\nstderr=%s", i, err, out, h.stderr.String())
		}
		switch strings.TrimSpace(out) {
		case "allowed":
			allowed++
		case "limited":
			limited++
		default:
			t.Fatalf("unexpected helper output %q", out)
		}
	}
	if allowed > 2 {
		t.Fatalf("allowed helpers = %d, want <= 2", allowed)
	}
	if limited == 0 {
		t.Fatal("expected at least one helper to be locally rate limited")
	}
}

func runProcessHelper() {
	dir := os.Getenv("MAIL_RATELIMIT_STATE_DIR")
	if dir == "" {
		fmt.Fprint(os.Stderr, "missing MAIL_RATELIMIT_STATE_DIR")
		os.Exit(2)
	}
	limiter := NewLimiterForDir(dir, []Rule{testRule()}, func() time.Time { return time.Unix(100, 0) })
	err := limiter.Allow(context.Background(), testRequest())
	if err == nil {
		fmt.Print("allowed")
		os.Exit(0)
	}
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) && exitErr.Detail != nil && exitErr.Detail.Type == "rate_limit" {
		fmt.Print("limited")
		os.Exit(0)
	}
	fmt.Fprintf(os.Stderr, "unexpected helper err: %v", err)
	os.Exit(2)
}
