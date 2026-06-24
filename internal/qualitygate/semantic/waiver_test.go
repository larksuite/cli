// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"testing"
	"time"
)

func TestLoadWaivers(t *testing.T) {
	now := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	repo := t.TempDir()
	w, diags, err := LoadWaivers(repo, now)
	if err != nil {
		t.Fatalf("missing waivers should be empty, got %v", err)
	}
	if len(w.Items) != 0 || len(diags) != 0 {
		t.Fatalf("missing waivers = %#v %#v, want empty", w, diags)
	}

	writeSemanticFile(t, repo, "waivers.txt", "# waiver_id\tcategory\tfact_kind\tsource_file\tline\tcommand_path\towner\treason\tadded_at\texpires_at\n"+
		"wiki-move-202606\tskill_quality\tskill\tskills/lark-wiki/SKILL.md\t30\t\twiki-owner\tmigration\t2026-06-08\t2026-07-15\n"+
		"wiki-move-202606\tskill_quality\tskill\tskills/lark-wiki/references/move.md\t12\t\twiki-owner\tmigration\t2026-06-08\t2026-07-15\n"+
		"public-doc-202606\tpublic_content_leakage\tpublic_content\tdocs/public.md\t4\t\tsecurity-owner\treviewed false positive\t2026-06-08\t2026-07-15\n")
	w, diags, err = LoadWaivers(repo, now)
	if err != nil {
		t.Fatalf("LoadWaivers() error = %v", err)
	}
	if len(diags) != 0 || len(w.Items) != 3 {
		t.Fatalf("LoadWaivers() = %#v %#v", w, diags)
	}

	for name, body := range map[string]string{
		"bad columns":                     "one\ttoo-few\n",
		"bad id":                          "BAD\terror_hint\terror\tcmd/root.go\t1\t\to\tr\t2026-06-08\t2026-07-15\n",
		"bad fact kind":                   "id1\terror_hint\tskill_quality\tcmd/root.go\t1\t\to\tr\t2026-06-08\t2026-07-15\n",
		"missing owner":                   "id1\terror_hint\terror\tcmd/root.go\t1\t\t\tr\t2026-06-08\t2026-07-15\n",
		"missing line":                    "id1\terror_hint\terror\tcmd/root.go\t\t\to\tr\t2026-06-08\t2026-07-15\n",
		"missing command":                 "id1\tdefault_output\toutput\t\t\t\to\tr\t2026-06-08\t2026-07-15\n",
		"public content missing line":     "id1\tpublic_content_leakage\tpublic_content\tdocs/public.md\t\t\to\tr\t2026-06-08\t2026-07-15\n",
		"public content command selector": "id1\tpublic_content_leakage\tpublic_content\t\t\tcmd/foo\to\tr\t2026-06-08\t2026-07-15\n",
		"bad source path":                 "id1\terror_hint\terror\t../cmd/root.go\t1\t\to\tr\t2026-06-08\t2026-07-15\n",
		"bad date format":                 "id1\terror_hint\terror\tcmd/root.go\t1\t\to\tr\t20260608\t2026-07-15\n",
	} {
		t.Run(name, func(t *testing.T) {
			writeSemanticFile(t, repo, "waivers.txt", body)
			if _, _, err := LoadWaivers(repo, now); err == nil {
				t.Fatalf("LoadWaivers accepted %s", name)
			}
		})
	}
}

func TestLoadWaiversExpiresRows(t *testing.T) {
	repo := t.TempDir()
	writeSemanticFile(t, repo, "waivers.txt", "id1\terror_hint\terror\tcmd/root.go\t10\t\to\tr\t2026-01-01\t2026-06-08\n")
	w, diags, err := LoadWaivers(repo, time.Date(2026, 6, 8, 23, 59, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("LoadWaivers() error = %v", err)
	}
	if len(w.Items) != 1 || len(diags) != 0 {
		t.Fatalf("waiver should remain active through expires_at date: %#v %#v", w, diags)
	}

	w, diags, err = LoadWaivers(repo, time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("LoadWaivers() error = %v", err)
	}
	if len(w.Items) != 0 {
		t.Fatalf("expired waiver should not be active: %#v", w)
	}
	if len(diags) != 1 || diags[0].Rule != "semantic_waiver_expired" {
		t.Fatalf("expired diagnostics = %#v", diags)
	}
}
