// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

func TestRequireConfirmation_TypedShape(t *testing.T) {
	err := RequireConfirmation("drive +delete")
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	var cre *errs.ConfirmationRequiredError
	if !errors.As(err, &cre) {
		t.Fatalf("expected *errs.ConfirmationRequiredError, got %T", err)
	}
	if cre.Category != errs.CategoryConfirmation {
		t.Errorf("Category = %q, want %q", cre.Category, errs.CategoryConfirmation)
	}
	if cre.Subtype != errs.SubtypeConfirmationRequired {
		t.Errorf("Subtype = %q, want %q", cre.Subtype, errs.SubtypeConfirmationRequired)
	}
	if got := output.ExitCodeOf(err); got != output.ExitConfirmationRequired {
		t.Errorf("ExitCodeOf = %d, want %d", got, output.ExitConfirmationRequired)
	}
	if !strings.Contains(cre.Message, "drive +delete") || !strings.Contains(cre.Message, "requires confirmation") {
		t.Errorf("Message = %q, want it to mention action and 'requires confirmation'", cre.Message)
	}
	// The hint may additionally carry a re-run line composed from the live
	// os.Args (environment-dependent under `go test`), but the add-yes
	// contract always leads.
	if !strings.HasPrefix(cre.Hint, "add --yes to confirm") {
		t.Errorf("Hint = %q, want prefix 'add --yes to confirm'", cre.Hint)
	}
	if cre.Risk != errs.RiskHighRiskWrite {
		t.Errorf("Risk = %q, want %q", cre.Risk, errs.RiskHighRiskWrite)
	}
	if cre.Action != "drive +delete" {
		t.Errorf("Action = %q, want drive +delete", cre.Action)
	}
}

func TestRequireConfirmation_JSONShape(t *testing.T) {
	err := RequireConfirmation("mail +send")
	var cre *errs.ConfirmationRequiredError
	if !errors.As(err, &cre) {
		t.Fatalf("expected *errs.ConfirmationRequiredError, got %T", err)
	}
	raw, mErr := json.Marshal(cre)
	if mErr != nil {
		t.Fatalf("marshal: %v", mErr)
	}
	var back map[string]interface{}
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// No fix_command field leaks into the envelope: the retry line lives in
	// the free-text hint only; the typed protocol stays action-only.
	if _, has := back["fix_command"]; has {
		t.Errorf("unexpected fix_command present in JSON: %s", raw)
	}

	if back["risk"] != "high-risk-write" {
		t.Errorf("risk in JSON = %v", back["risk"])
	}
	if back["action"] != "mail +send" {
		t.Errorf("action in JSON = %v", back["action"])
	}
	// Action-only protocol: no UpgradedBy / fix_command / upgraded_by leak.
	if _, has := back["upgraded_by"]; has {
		t.Errorf("unexpected upgraded_by present in JSON: %s", raw)
	}
}

// TestRequireConfirmationFor pins the composed hint end to end for a fixed
// argv — the retry line must ride behind the add-yes contract — and the plain
// fallback when the argv suppresses the line.
func TestRequireConfirmationFor(t *testing.T) {
	t.Run("composed hint carries the full retry line", func(t *testing.T) {
		err := requireConfirmationFor("sheets +cells-clear", []string{
			"/usr/local/bin/lark-cli", "sheets", "+cells-clear", "--range", "A1:B2",
		})
		var cre *errs.ConfirmationRequiredError
		if !errors.As(err, &cre) {
			t.Fatalf("expected *errs.ConfirmationRequiredError, got %T", err)
		}
		want := "add --yes to confirm; re-run: lark-cli sheets +cells-clear --range A1:B2 --yes"
		if cre.Hint != want {
			t.Errorf("Hint = %q, want %q", cre.Hint, want)
		}
	})

	t.Run("stdin argv falls back to the plain hint", func(t *testing.T) {
		err := requireConfirmationFor("sheets +batch-update", []string{
			"lark-cli", "sheets", "+batch-update", "--operations", "-",
		})
		var cre *errs.ConfirmationRequiredError
		if !errors.As(err, &cre) {
			t.Fatalf("expected *errs.ConfirmationRequiredError, got %T", err)
		}
		if cre.Hint != "add --yes to confirm" {
			t.Errorf("Hint = %q, want the plain add-yes hint", cre.Hint)
		}
	})
}

// TestRetryCommandWithYes pins the retry-line contract: shell-safe quoting,
// basename argv[0], and the omission guards (stdin args, credential-bearing
// flags, oversized commands).
func TestRetryCommandWithYes(t *testing.T) {
	t.Run("quotes what needs quoting and appends --yes", func(t *testing.T) {
		got := retryCommandWithYes([]string{
			"/usr/local/bin/lark-cli", "sheets", "+cells-clear",
			"--url", "https://x.feishu.cn/sheets/tok",
			"--range", "A$1:B$2", "--sheet-id", "sh1",
		})
		want := `lark-cli sheets +cells-clear --url https://x.feishu.cn/sheets/tok --range 'A$1:B$2' --sheet-id sh1 --yes`
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("free-form values omit the retry line", func(t *testing.T) {
		// Values shaped like content rather than locators (whitespace, quotes,
		// braces, non-ASCII) can carry passwords, PII or business data — the
		// line is suppressed wholesale, not redacted.
		for _, args := range [][]string{
			{"lark-cli", "apps", "+db-execute", "--sql", "UPDATE users SET pw='x'"},
			{"lark-cli", "base", "+form-submit", "--json", `{"phone":"13800138000"}`},
			{"lark-cli", "sheets", "+cells-clear", "--sheet-name", "第 1 班"},
			{"lark-cli", "x", "--title", "it's"},
			{"lark-cli", "x", "--note=" + strings.Repeat("a", 200)},
		} {
			if got := retryCommandWithYes(args); got != "" {
				t.Errorf("argv %v must not render a retry line, got %q", args, got)
			}
		}
	})

	t.Run("stdin arg omits the retry line", func(t *testing.T) {
		if got := retryCommandWithYes([]string{"lark-cli", "sheets", "+batch-update", "--operations", "-"}); got != "" {
			t.Errorf("stdin invocation must not render a retry line, got %q", got)
		}
	})

	t.Run("bundled stdin flag omits the retry line", func(t *testing.T) {
		// --flag=- reads stdin the same as --flag -; both must suppress the line.
		if got := retryCommandWithYes([]string{"lark-cli", "sheets", "+cells-set", "--cells=-"}); got != "" {
			t.Errorf("--flag=- stdin invocation must not render a retry line, got %q", got)
		}
	})

	t.Run("credential-bearing flag omits the retry line", func(t *testing.T) {
		for _, args := range [][]string{
			{"lark-cli", "im", "+send", "--app-secret", "s3cr3t"},
			{"lark-cli", "im", "+send", "--user-access-token=u-abc"},
			{"lark-cli", "im", "+send", "--api_key", "k"},
		} {
			if got := retryCommandWithYes(args); got != "" {
				t.Errorf("argv %v must not render a retry line, got %q", args, got)
			}
		}
	})

	t.Run("positional value mentioning token still renders", func(t *testing.T) {
		got := retryCommandWithYes([]string{"lark-cli", "drive", "+delete", "--file-path", "token.json"})
		if got != "lark-cli drive +delete --file-path token.json --yes" {
			t.Errorf("non-flag token-ish value must not suppress the line, got %q", got)
		}
	})

	t.Run("oversized command omits the retry line", func(t *testing.T) {
		if got := retryCommandWithYes([]string{"lark-cli", "x", "--operations", strings.Repeat("a", 400)}); got != "" {
			t.Errorf("oversized invocation must not render a retry line, got %q", got)
		}
	})
}
