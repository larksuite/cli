// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package workflowrun

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

// Exercise abrupt process termination, without deferred closes/manifest writes.
// Process.Kill uses the OS hard termination primitive (SIGKILL on Linux).
func TestHardKillHelper(t *testing.T) {
	root := os.Getenv("WORKFLOW_MOCK_KILL_ROOT")
	if root == "" {
		return
	}
	s, e := OpenStore(root, false)
	if e != nil {
		t.Fatal(e)
	}
	m := NewManifest(params())
	s.Hook = func(stage string, b Batch) error {
		if stage == os.Getenv("WORKFLOW_MOCK_KILL_STAGE") && b.Page.Plane == Runs && b.Query.Window.Start == day {
			p, e := os.FindProcess(os.Getpid())
			if e != nil {
				return e
			}
			return p.Kill()
		}
		return nil
	}
	if e = engine(t, fixture(t), 3*day).Export(context.Background(), s, m, 0); e != nil {
		t.Fatal(e)
	}
	t.Fatal("hard kill did not happen")
}
func TestHardKillResume(t *testing.T) {
	for _, stage := range []string{"data_durable", "checkpoint_durable"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestHardKillHelper$")
			cmd.Env = append(os.Environ(), "WORKFLOW_MOCK_KILL_ROOT="+root, "WORKFLOW_MOCK_KILL_STAGE="+stage)
			if e := cmd.Run(); e == nil {
				t.Fatal("helper did not terminate")
			}
			s, e := OpenStore(root, true)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			m, e := s.ReadManifest()
			if e != nil {
				t.Fatal(e)
			}
			if e = s.Recover(m); e != nil {
				t.Fatal(e)
			}
			if e = engine(t, fixture(t), 4*day).Export(context.Background(), s, m, 0); e != nil {
				t.Fatal(e)
			}
			assertComplete(t, m)
			assertIDs(t, s, Runs, []string{"E1", "E2"})
			assertIDs(t, s, Buttons, []string{"a", "b", "c", "d"})
		})
	}
}
