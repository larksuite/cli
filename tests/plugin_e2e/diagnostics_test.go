// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package plugin_e2e

import (
	"testing"

	"github.com/tidwall/gjson"
)

// TestDiagnostics asserts the VERIFIED stdout shapes of the two policy/plugin
// diagnostic commands on a fork carrying the readonly Restrict rule:
//   - `config policy show`: source_name == the plugin name that installed the
//     active rule.
//   - `config plugins show`: {"plugins":[{"name","version","capabilities",...,
//     "hooks":{...}}],"total":N} with the readonly plugin present.
func TestDiagnostics(t *testing.T) {
	bin := buildFork(t, "readonly", readonlyPlugin)

	pol := run(t, bin, "config", "policy", "show")
	if pol.exit != 0 || !gjson.Valid(pol.stdout) {
		t.Fatalf("policy show exit=%d stdout=%s stderr=%s", pol.exit, pol.stdout, pol.stderr)
	}
	if src := gjson.Get(pol.stdout, "source_name").String(); src != "readonly" {
		t.Errorf("policy source_name=%q want readonly (stdout=%s)", src, pol.stdout)
	}

	plug := run(t, bin, "config", "plugins", "show")
	if plug.exit != 0 || !gjson.Valid(plug.stdout) {
		t.Fatalf("plugins show exit=%d stdout=%s", plug.exit, plug.stdout)
	}
	if total := gjson.Get(plug.stdout, "total").Int(); total < 1 {
		t.Errorf("plugins total=%d want >=1 (stdout=%s)", total, plug.stdout)
	}
	if name := gjson.Get(plug.stdout, "plugins.0.name").String(); name != "readonly" {
		t.Errorf("plugins.0.name=%q want readonly", name)
	}
}
