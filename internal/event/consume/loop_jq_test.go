// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// TestCompileJQReportsErrorEarly verifies that invalid jq expressions fail
// at CompileJQ time, not when the first event arrives. Before the fix,
// a bad expression would fail repeatedly on every event via applyJQ.
func TestCompileJQReportsErrorEarly(t *testing.T) {
	_, err := CompileJQ("invalid{{{")
	if err == nil {
		t.Fatal("expected compile error for invalid jq expression")
	}
	msg := err.Error()
	if !strings.Contains(msg, "compile") && !strings.Contains(msg, "parse") && !strings.Contains(msg, "invalid") {
		t.Errorf("error should mention compile/parse/invalid, got: %v", err)
	}
}

// TestCompileJQReturnsUsableCode verifies a valid expression compiles to
// a gojq.Code that applyJQ can invoke.
func TestCompileJQReturnsUsableCode(t *testing.T) {
	code, err := CompileJQ(".foo")
	if err != nil {
		t.Fatal(err)
	}
	if code == nil {
		t.Fatal("expected non-nil code")
	}

	input := json.RawMessage(`{"foo":"bar"}`)
	result, err := applyJQ(code, input)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != `"bar"` {
		t.Errorf("expected \"bar\", got %s", string(result))
	}
}

// TestApplyJQReusesCompiledCode confirms that applyJQ can be called many
// times without reallocating compilation state. This is the core perf goal:
// pre-fix, this test loop would have called gojq.Parse + Compile 10000 times.
func TestApplyJQReusesCompiledCode(t *testing.T) {
	code, err := CompileJQ(".foo")
	if err != nil {
		t.Fatal(err)
	}
	data := json.RawMessage(`{"foo":"bar"}`)
	for i := 0; i < 10000; i++ {
		result, err := applyJQ(code, data)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if string(result) != `"bar"` {
			t.Fatalf("iteration %d: unexpected result %s", i, string(result))
		}
	}
}

// TestApplyJQFilterReturnsNilOnNoOutput verifies that expressions producing
// no output (e.g., select that filters out) return (nil, nil), not error.
// The existing applyJQ had this semantic; the refactor must preserve it.
func TestApplyJQFilterReturnsNilOnNoOutput(t *testing.T) {
	code, err := CompileJQ(`select(.type == "match")`)
	if err != nil {
		t.Fatal(err)
	}
	// This input doesn't match the filter, so select produces no output.
	result, err := applyJQ(code, json.RawMessage(`{"type":"nomatch"}`))
	if err != nil {
		t.Fatalf("should not error on filter-out: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for filtered-out event, got %s", string(result))
	}
}

// TestApplyJQConcurrentSafe verifies that a single compiled *gojq.Code
// can be safely shared across many goroutines concurrently — the whole
// point of pre-compile is that workers share one Code instance without
// mutex overhead. A regression (e.g., gojq Code becoming internally
// stateful) would manifest as a -race failure or a wrong result.
func TestApplyJQConcurrentSafe(t *testing.T) {
	code, err := CompileJQ(".value")
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 32
	const iterationsPerGoroutine = 1000

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < iterationsPerGoroutine; i++ {
				// Different input per goroutine to catch any shared-buffer mistakes.
				input := json.RawMessage(fmt.Sprintf(`{"value":"goroutine-%d-iter-%d"}`, gid, i))
				result, err := applyJQ(code, input)
				if err != nil {
					errs <- fmt.Errorf("goroutine %d iter %d: %w", gid, i, err)
					return
				}
				expected := fmt.Sprintf(`"goroutine-%d-iter-%d"`, gid, i)
				if string(result) != expected {
					errs <- fmt.Errorf("goroutine %d iter %d: expected %s, got %s", gid, i, expected, string(result))
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}
