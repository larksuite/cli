// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package version

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
)

func TestVersionJSONReportsCompiledEdition(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, out, _, _ := cmdutil.TestFactory(t, nil)
	cmd := NewCmdVersion(f)
	cmd.SetArgs([]string{"--json"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Version      string   `json:"version"`
		Edition      string   `json:"edition"`
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != build.Version || got.Edition != build.Edition || !reflect.DeepEqual(got.Capabilities, build.Capabilities()) {
		t.Fatalf("version output = %#v, want version=%q edition=%q", got, build.Version, build.Edition)
	}
}

func TestVersionVisibilityPreservesStandardHelpSurface(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	cmd := NewCmdVersion(f)
	wantHidden := build.Edition == "standard"
	if cmd.Hidden != wantHidden {
		t.Fatalf("version command hidden = %v, want %v for %s", cmd.Hidden, wantHidden, build.Edition)
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestVersionTextWriteFailureIsTyped(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	writeErr := errors.New("write failed")
	f.IOStreams.Out = failingWriter{err: writeErr}

	cmd := NewCmdVersion(f)
	err := cmd.ExecuteContext(context.Background())
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeSDKError {
		t.Fatalf("error = %#v, want internal/sdk_error", err)
	}
	if !errors.Is(err, writeErr) {
		t.Fatalf("error does not preserve write failure: %v", err)
	}
}
