// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestCompileJQReportsErrorEarly(t *testing.T) {
	_, err := CompileJQ("invalid{{{")
	if err == nil {
		t.Fatal("expected compile error for invalid jq expression")
	}
	msg := err.Error()
	if !strings.Contains(msg, "compile") && !strings.Contains(msg, "parse") && !strings.Contains(msg, "invalid") {
		t.Errorf("error should mention compile/parse/invalid, got: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument || ve.Param != "--jq" {
		t.Errorf("subtype/param = %s/%q, want %s/%q", ve.Subtype, ve.Param, errs.SubtypeInvalidArgument, "--jq")
	}
	if errors.Unwrap(err) == nil {
		t.Error("compile error should preserve its cause")
	}
}

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

func TestApplyJQFilterReturnsNilOnNoOutput(t *testing.T) {
	code, err := CompileJQ(`select(.type == "match")`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := applyJQ(code, json.RawMessage(`{"type":"nomatch"}`))
	if err != nil {
		t.Fatalf("should not error on filter-out: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for filtered-out event, got %s", string(result))
	}
}

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
