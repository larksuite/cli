// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package clie2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveBinaryPath(t *testing.T) {
	t.Run("request binary path wins", func(t *testing.T) {
		tmpDir := t.TempDir()
		reqBin := mustWriteExecutable(t, filepath.Join(tmpDir, "req-bin"))
		envBin := mustWriteExecutable(t, filepath.Join(tmpDir, "env-bin"))
		t.Setenv(EnvBinaryPath, envBin)

		got, err := ResolveBinaryPath(Request{BinaryPath: reqBin})
		require.NoError(t, err)
		assert.Equal(t, reqBin, got)
	})

	t.Run("uses env binary path", func(t *testing.T) {
		tmpDir := t.TempDir()
		envBin := mustWriteExecutable(t, filepath.Join(tmpDir, "env-bin"))
		t.Setenv(EnvBinaryPath, envBin)

		got, err := ResolveBinaryPath(Request{})
		require.NoError(t, err)
		assert.Equal(t, envBin, got)
	})

	t.Run("uses project root binary", func(t *testing.T) {
		tmpDir := t.TempDir()
		testsDir := filepath.Join(tmpDir, projectRootMarkerDir)
		require.NoError(t, os.MkdirAll(testsDir, 0o755))
		projectBin := mustWriteExecutable(t, filepath.Join(tmpDir, cliBinaryName))

		oldWD, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(testsDir))
		defer func() {
			require.NoError(t, os.Chdir(oldWD))
		}()

		t.Setenv(EnvBinaryPath, "")
		got, err := ResolveBinaryPath(Request{})
		require.NoError(t, err)
		assertSamePath(t, projectBin, got)
	})

	t.Run("rejects non-executable path", func(t *testing.T) {
		tmpDir := t.TempDir()
		file := filepath.Join(tmpDir, "not-exec")
		require.NoError(t, os.WriteFile(file, []byte("plain"), 0o644))

		_, err := ResolveBinaryPath(Request{BinaryPath: file})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not executable")
	})
}

func TestBuildArgs(t *testing.T) {
	t.Run("encodes json payloads", func(t *testing.T) {
		args, err := BuildArgs(Request{
			Args:   []string{"task", "+create"},
			Params: map[string]any{"task_guid": "abc"},
			Data:   map[string]any{"summary": "hello"},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{
			"task", "+create",
			"--params", `{"task_guid":"abc"}`,
			"--data", `{"summary":"hello"}`,
		}, args)
	})

	t.Run("adds default-as and format when set", func(t *testing.T) {
		args, err := BuildArgs(Request{
			Args:      []string{"task", "+update"},
			DefaultAs: "user",
			Format:    "pretty",
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"task", "+update", "--as", "user", "--format", "pretty"}, args)
	})

	t.Run("requires args", func(t *testing.T) {
		_, err := BuildArgs(Request{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "args are required")
	})
}

func TestSkipWithoutUserToken(t *testing.T) {
	t.Run("returns immediately when env user access token exists", func(t *testing.T) {
		t.Setenv("LARKSUITE_CLI_USER_ACCESS_TOKEN", "uat-from-env")

		ran := false
		ok := t.Run("inner", func(t *testing.T) {
			SkipWithoutUserToken(t)
			ran = true
		})
		require.True(t, ok)
		assert.True(t, ran)
	})

	t.Run("accepts verified local auth status", func(t *testing.T) {
		fake := newFakeCLI(t)
		t.Setenv("LARKSUITE_CLI_USER_ACCESS_TOKEN", "")
		t.Setenv(EnvBinaryPath, fake.BinaryPath)
		t.Setenv("FAKE_AUTH_STATUS_STDOUT", `{"identity":"user","verified":true}`)
		t.Setenv("FAKE_AUTH_STATUS_EXIT_CODE", "0")

		ran := false
		ok := t.Run("inner", func(t *testing.T) {
			SkipWithoutUserToken(t)
			ran = true
		})
		require.True(t, ok)
		assert.True(t, ran)
	})

	t.Run("skips when local auth is not user", func(t *testing.T) {
		fake := newFakeCLI(t)
		t.Setenv("LARKSUITE_CLI_USER_ACCESS_TOKEN", "")
		t.Setenv(EnvBinaryPath, fake.BinaryPath)
		t.Setenv("FAKE_AUTH_STATUS_STDOUT", `{"identity":"bot","verified":false}`)
		t.Setenv("FAKE_AUTH_STATUS_EXIT_CODE", "0")

		ran := false
		ok := t.Run("inner", func(t *testing.T) {
			SkipWithoutUserToken(t)
			ran = true
		})
		require.True(t, ok)
		assert.False(t, ran)
	})
}

func TestRunCmd(t *testing.T) {
	t.Run("returns stdout json on success", func(t *testing.T) {
		fake := newFakeCLI(t)
		result, err := RunCmd(context.Background(), Request{
			BinaryPath: fake.BinaryPath,
			Args:       []string{"--stdout-json", `{"ok":true}`},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		outMap, ok := result.StdoutJSON(t).(map[string]any)
		require.True(t, ok)
		assert.Equal(t, true, outMap["ok"])
	})

	t.Run("captures stderr and exit code on failure", func(t *testing.T) {
		fake := newFakeCLI(t)
		result, err := RunCmd(context.Background(), Request{
			BinaryPath: fake.BinaryPath,
			Args:       []string{"--stderr-json", `{"ok":false}`, "--exit", "3"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 3)
		assert.Error(t, result.RunErr)

		errMap, ok := result.StderrJSON(t).(map[string]any)
		require.True(t, ok)
		assert.Equal(t, false, errMap["ok"])
	})

	t.Run("passes explicit default-as as flag", func(t *testing.T) {
		fake := newFakeCLI(t)
		result, err := RunCmd(context.Background(), Request{
			BinaryPath: fake.BinaryPath,
			Args:       []string{"emit-arg", "--as"},
			DefaultAs:  "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "user", strings.TrimSpace(result.Stdout))
	})

	t.Run("asserts stdout code payloads", func(t *testing.T) {
		fake := newFakeCLI(t)
		result, err := RunCmd(context.Background(), Request{
			BinaryPath: fake.BinaryPath,
			Args:       []string{"--stdout-json", `{"code":0,"data":{"id":"x"}}`},
			Format:     "json",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})

	t.Run("passes stdin to process", func(t *testing.T) {
		fake := newFakeCLI(t)
		result, err := RunCmd(context.Background(), Request{
			BinaryPath: fake.BinaryPath,
			Args:       []string{"emit-stdin"},
			Stdin:      []byte("hello from stdin\n"),
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "hello from stdin\n", result.Stdout)
	})
}

type fakeCLI struct {
	BinaryPath string
}

func newFakeCLI(t *testing.T) fakeCLI {
	t.Helper()

	tmpDir := t.TempDir()

	script := `#!/bin/sh
if [ "$1" = "auth" ] && [ "$2" = "status" ] && [ "$3" = "--verify" ]; then
  if [ -n "$FAKE_AUTH_STATUS_STDOUT" ]; then
    echo "$FAKE_AUTH_STATUS_STDOUT"
  fi
  exit "${FAKE_AUTH_STATUS_EXIT_CODE:-0}"
fi

if [ "$1" = "emit-arg" ]; then
  key="$2"
  shift 2
  while [ "$#" -gt 1 ]; do
    if [ "$1" = "$key" ]; then
      echo "$2"
      exit 0
    fi
    shift
  done
  exit 1
fi

if [ "$1" = "emit-stdin" ]; then
  cat
  exit 0
fi

exit_code=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --stdout-json)
      echo "$2"
      shift 2
      ;;
    --stderr-json)
      echo "$2" >&2
      shift 2
      ;;
    --exit)
      exit_code="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
exit "$exit_code"
`

	binaryPath := filepath.Join(tmpDir, "fake-"+cliBinaryName)
	require.NoError(t, os.WriteFile(binaryPath, []byte(script), 0o755))

	return fakeCLI{
		BinaryPath: binaryPath,
	}
}

func assertSamePath(t *testing.T, want string, got string) {
	t.Helper()
	gotReal, err := filepath.EvalSymlinks(got)
	require.NoError(t, err)
	wantReal, err := filepath.EvalSymlinks(want)
	require.NoError(t, err)
	assert.Equal(t, wantReal, gotReal)
}

func mustWriteExecutable(t *testing.T, path string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	absPath, err := filepath.Abs(path)
	require.NoError(t, err)
	return absPath
}
