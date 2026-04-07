// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/spf13/cobra"
)

func buildSubscribeRootCmd(t *testing.T, f *cmdutil.Factory) *cobra.Command {
	t.Helper()

	rootCmd := &cobra.Command{Use: "lark-cli"}
	rootCmd.SilenceErrors = true
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		cmd.SilenceUsage = true
	}

	eventCmd := &cobra.Command{
		Use:   "event",
		Short: "event operations",
	}
	rootCmd.AddCommand(eventCmd)
	EventSubscribe.Mount(eventCmd, f)

	return rootCmd
}

func executeSubscribeRootCmd(t *testing.T, rootCmd *cobra.Command, args []string) error {
	t.Helper()
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

func TestE2E_EventSubscribeRoot_CatchAllProcessesLegacyDomain(t *testing.T) {
	factory, stdoutBuf, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID:     "cli_test_app",
		AppSecret: "cli_test_secret",
		Brand:     core.BrandFeishu,
	})

	origFactory := newWSClient
	newWSClient = func(_ *core.CliConfig, eventDispatcher *dispatcher.EventDispatcher, _ larkcore.Logger) wsClient {
		return fakeSubscribeClient{
			start: func(ctx context.Context) error {
				_, err := eventDispatcher.Do(ctx, []byte(`{"schema":"2.0","header":{"event_id":"evt_contact","event_type":"contact.user.created_v3"},"event":{"user":{"open_id":"ou_123"}}}`))
				return err
			},
		}
	}
	t.Cleanup(func() { newWSClient = origFactory })

	rootCmd := buildSubscribeRootCmd(t, factory)
	if err := executeSubscribeRootCmd(t, rootCmd, []string{"event", "+subscribe", "--as", "bot", "--force"}); err != nil {
		t.Fatalf("rootCmd.Execute() error = %v", err)
	}

	if got := stdoutBuf.String(); got == "" {
		t.Fatal("stdout is empty, want routed contact event record")
	}
	if !bytes.Contains(stdoutBuf.Bytes(), []byte(`"event_type":"contact.user.created_v3"`)) {
		t.Fatalf("stdout = %q, want contact.user.created_v3 record", stdoutBuf.String())
	}
	if !bytes.Contains(stdoutBuf.Bytes(), []byte(`"status":"handled"`)) {
		t.Fatalf("stdout = %q, want handled status", stdoutBuf.String())
	}
}

func TestE2E_EventSubscribeRoot_QuietSuppressesStatusLogs(t *testing.T) {
	factory, stdoutBuf, stderrBuf, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID:     "cli_test_app",
		AppSecret: "cli_test_secret",
		Brand:     core.BrandFeishu,
	})

	origFactory := newWSClient
	newWSClient = func(_ *core.CliConfig, eventDispatcher *dispatcher.EventDispatcher, _ larkcore.Logger) wsClient {
		return fakeSubscribeClient{
			start: func(ctx context.Context) error {
				_, err := eventDispatcher.Do(ctx, []byte(`{"schema":"2.0","header":{"event_id":"evt_im","event_type":"im.message.receive_v1"},"event":{"message":{"message_id":"om_123"}}}`))
				return err
			},
		}
	}
	t.Cleanup(func() { newWSClient = origFactory })

	rootCmd := buildSubscribeRootCmd(t, factory)
	if err := executeSubscribeRootCmd(t, rootCmd, []string{"event", "+subscribe", "--as", "bot", "--force", "--quiet"}); err != nil {
		t.Fatalf("rootCmd.Execute() error = %v", err)
	}

	if stderrBuf.Len() != 0 {
		t.Fatalf("stderr = %q, want quiet mode to suppress status logs", stderrBuf.String())
	}
	if !bytes.Contains(stdoutBuf.Bytes(), []byte(`"event_type":"im.message.receive_v1"`)) {
		t.Fatalf("stdout = %q, want im.message.receive_v1 record", stdoutBuf.String())
	}
}

func TestE2E_EventSubscribeRoot_QuietSuppressesInteractiveFallbackHint(t *testing.T) {
	factory, stdoutBuf, stderrBuf, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID:     "cli_test_app",
		AppSecret: "cli_test_secret",
		Brand:     core.BrandFeishu,
	})

	origFactory := newWSClient
	newWSClient = func(_ *core.CliConfig, eventDispatcher *dispatcher.EventDispatcher, _ larkcore.Logger) wsClient {
		return fakeSubscribeClient{
			start: func(ctx context.Context) error {
				_, err := eventDispatcher.Do(ctx, []byte(`{"schema":"2.0","header":{"event_id":"evt_interactive","event_type":"im.message.receive_v1"},"event":{"message":{"message_id":"om_interactive","message_type":"interactive","content":"{\"type\":\"template\"}"}}}`))
				return err
			},
		}
	}
	t.Cleanup(func() { newWSClient = origFactory })

	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = origStderr
	}()

	rootCmd := buildSubscribeRootCmd(t, factory)
	runErr := executeSubscribeRootCmd(t, rootCmd, []string{"event", "+subscribe", "--as", "bot", "--force", "--quiet", "--compact"})
	if err := w.Close(); err != nil {
		t.Fatalf("stderr close error = %v", err)
	}
	rawStderr, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("ReadAll(stderr) error = %v", readErr)
	}
	if runErr != nil {
		t.Fatalf("rootCmd.Execute() error = %v", runErr)
	}

	if stderrBuf.Len() != 0 {
		t.Fatalf("runtime stderr = %q, want quiet mode to suppress status logs", stderrBuf.String())
	}
	if strings.Contains(string(rawStderr), "interactive") || strings.Contains(string(rawStderr), "returning raw event data") {
		t.Fatalf("os.Stderr = %q, want quiet mode to suppress interactive fallback hint", string(rawStderr))
	}
	if !bytes.Contains(stdoutBuf.Bytes(), []byte(`"reason":"interactive_fallback"`)) {
		t.Fatalf("stdout = %q, want interactive_fallback reason", stdoutBuf.String())
	}
}

func TestE2E_EventSubscribeRoot_RouteAndJSONWriteCompactFile(t *testing.T) {
	factory, stdoutBuf, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID:     "cli_test_app",
		AppSecret: "cli_test_secret",
		Brand:     core.BrandFeishu,
	})

	tmpDir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()

	origFactory := newWSClient
	newWSClient = func(_ *core.CliConfig, eventDispatcher *dispatcher.EventDispatcher, _ larkcore.Logger) wsClient {
		return fakeSubscribeClient{
			start: func(ctx context.Context) error {
				_, err := eventDispatcher.Do(ctx, []byte(`{"schema":"2.0","header":{"event_id":"evt_im","event_type":"im.message.receive_v1"},"event":{"message":{"message_id":"om_123"}}}`))
				return err
			},
		}
	}
	t.Cleanup(func() { newWSClient = origFactory })

	rootCmd := buildSubscribeRootCmd(t, factory)
	if err := executeSubscribeRootCmd(t, rootCmd, []string{
		"event", "+subscribe",
		"--as", "bot",
		"--force",
		"--json",
		"--route", `^im\.message=dir:./im`,
	}); err != nil {
		t.Fatalf("rootCmd.Execute() error = %v", err)
	}

	if stdoutBuf.Len() != 0 {
		t.Fatalf("stdout = %q, want routed file output only", stdoutBuf.String())
	}

	routeDir := filepath.Join(tmpDir, "im")
	entries, err := os.ReadDir(routeDir)
	if err != nil {
		t.Fatalf("ReadDir(routeDir) error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}

	body, err := os.ReadFile(filepath.Join(routeDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if bytes.Contains(body, []byte("\n  ")) {
		t.Fatalf("routed file unexpectedly used pretty JSON: %q", string(body))
	}

	var record map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(body), &record); err != nil {
		t.Fatalf("unmarshal routed record: %v", err)
	}
	if got, want := record["event_type"], "im.message.receive_v1"; got != want {
		t.Fatalf("event_type = %v, want %v", got, want)
	}
	if got, want := record["status"], string(HandlerStatusHandled); got != want {
		t.Fatalf("status = %v, want %v", got, want)
	}
}

func TestE2E_EventSubscribeRoot_RouteAndCompactWriteCompactFile(t *testing.T) {
	factory, stdoutBuf, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID:     "cli_test_app",
		AppSecret: "cli_test_secret",
		Brand:     core.BrandFeishu,
	})

	tmpDir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()

	origFactory := newWSClient
	newWSClient = func(_ *core.CliConfig, eventDispatcher *dispatcher.EventDispatcher, _ larkcore.Logger) wsClient {
		return fakeSubscribeClient{
			start: func(ctx context.Context) error {
				_, err := eventDispatcher.Do(ctx, []byte(`{"schema":"2.0","header":{"event_id":"evt_im","event_type":"im.message.receive_v1"},"event":{"message":{"message_id":"om_123","message_type":"text","content":"{\"text\":\"hello\"}"}}}`))
				return err
			},
		}
	}
	t.Cleanup(func() { newWSClient = origFactory })

	rootCmd := buildSubscribeRootCmd(t, factory)
	if err := executeSubscribeRootCmd(t, rootCmd, []string{
		"event", "+subscribe",
		"--as", "bot",
		"--force",
		"--compact",
		"--route", `^im\.message=dir:./im`,
	}); err != nil {
		t.Fatalf("rootCmd.Execute() error = %v", err)
	}

	if stdoutBuf.Len() != 0 {
		t.Fatalf("stdout = %q, want routed file output only", stdoutBuf.String())
	}

	routeDir := filepath.Join(tmpDir, "im")
	entries, err := os.ReadDir(routeDir)
	if err != nil {
		t.Fatalf("ReadDir(routeDir) error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}

	body, err := os.ReadFile(filepath.Join(routeDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var record map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(body), &record); err != nil {
		t.Fatalf("unmarshal routed record: %v", err)
	}
	if got, want := record["event_type"], "im.message.receive_v1"; got != want {
		t.Fatalf("event_type = %v, want %v", got, want)
	}
	if got, want := record["message_id"], "om_123"; got != want {
		t.Fatalf("message_id = %v, want %v", got, want)
	}
	if got, want := record["content"], "hello"; got != want {
		t.Fatalf("content = %v, want %v", got, want)
	}
}
