#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Tests for inventory.py. Run: python3 inventory_test.py"""

from __future__ import annotations

import json
import os
import sys
import tempfile
import unittest
from pathlib import Path

import inventory


class InventoryHelpersTest(unittest.TestCase):
    def test_normalize_extensions_defaults(self):
        self.assertEqual(inventory.normalize_extensions(None), set(inventory.DEFAULT_EXTENSIONS))
        self.assertEqual(inventory.normalize_extensions(""), set(inventory.DEFAULT_EXTENSIONS))

    def test_normalize_extensions_adds_leading_dot(self):
        self.assertEqual(inventory.normalize_extensions("pdf, docx"), {".pdf", ".docx"})

    def test_normalize_extensions_rejects_empty_list(self):
        with self.assertRaises(ValueError):
            inventory.normalize_extensions(", ,")

    def test_risk_hint_flags_sensitive_filename(self):
        self.assertTrue(inventory.risk_hint("员工薪资明细.xlsx").startswith("possible_sensitive:"))
        self.assertTrue(inventory.risk_hint("api_key_backup.txt").startswith("possible_sensitive:"))

    def test_risk_hint_default_needs_review(self):
        self.assertEqual(inventory.risk_hint("产品说明.docx"), "needs_content_review")

    def test_parse_readiness_classes(self):
        self.assertEqual(inventory.parse_readiness(".pdf"), "text_extractable")
        self.assertEqual(inventory.parse_readiness(".png"), "ocr_or_visual_review")
        self.assertEqual(inventory.parse_readiness(".mov"), "manual_review")


class InventoryScanTest(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.root = Path(self._tmp.name)

    def tearDown(self):
        self._tmp.cleanup()

    def _write(self, name: str, content: bytes) -> Path:
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(content)
        return path

    def _build(self, **kwargs):
        skipped: list[str] = []
        skipped_nonregular: list[str] = []
        rows = inventory.build_inventory(
            self.root,
            inventory.normalize_extensions(kwargs.get("extensions")),
            kwargs.get("include_hidden", False),
            skipped,
            skipped_nonregular,
            kwargs.get("exclude_dir"),
        )
        return rows, skipped, skipped_nonregular

    def test_exact_duplicates_grouped(self):
        self._write("a.txt", b"same content")
        self._write("sub/b.txt", b"same content")
        self._write("c.txt", b"different")
        rows, _, _ = self._build()
        dup_rows = [r for r in rows if r["duplicate_group"]]
        self.assertEqual(len(dup_rows), 2)
        self.assertEqual({r["duplicate_group"] for r in dup_rows}, {"exact-0001"})
        for r in dup_rows:
            self.assertEqual(r["proposed_action"], "deduplicate_review")
            self.assertEqual(r["duplicate_count"], 2)
        unique = [r for r in rows if not r["duplicate_group"]]
        self.assertEqual(len(unique), 1)
        self.assertEqual(unique[0]["proposed_action"], "review")

    def test_sensitive_filename_flagged(self):
        self._write("薪资明细.xlsx", b"x")
        rows, _, _ = self._build()
        self.assertTrue(rows[0]["risk_hint"].startswith("possible_sensitive:"))

    def test_extension_filter_excludes_others(self):
        self._write("keep.pdf", b"x")
        self._write("drop.exe", b"x")
        rows, _, _ = self._build(extensions="pdf")
        self.assertEqual([r["title"] for r in rows], ["keep.pdf"])

    def test_skip_dirs_ignored(self):
        self._write("node_modules/pkg.json", b"{}")
        self._write("real.json", b"{}")
        rows, _, _ = self._build()
        self.assertEqual([r["title"] for r in rows], ["real.json"])

    def test_hidden_files_excluded_by_default(self):
        self._write(".secret.txt", b"x")
        self._write("visible.txt", b"x")
        rows, _, _ = self._build()
        self.assertEqual([r["title"] for r in rows], ["visible.txt"])

    def test_office_lock_files_excluded(self):
        self._write("~$draft.docx", b"x")
        self._write("draft.docx", b"x")
        rows, _, _ = self._build()
        self.assertEqual([r["title"] for r in rows], ["draft.docx"])

    @unittest.skipUnless(hasattr(os, "symlink"), "symlink not supported")
    def test_symlink_file_skipped(self):
        target = self._write("target.txt", b"x")
        try:
            os.symlink(target, self.root / "link.txt")
        except (OSError, NotImplementedError):
            self.skipTest("symlink creation not permitted")
        rows, skipped, _ = self._build()
        self.assertEqual([r["title"] for r in rows], ["target.txt"])
        self.assertIn("link.txt", skipped)

    def test_empty_dir_produces_no_rows(self):
        rows, skipped, _ = self._build()
        self.assertEqual(rows, [])
        self.assertEqual(skipped, [])

    @unittest.skipUnless(hasattr(os, "mkfifo"), "mkfifo not supported")
    def test_fifo_skipped_not_hashed(self):
        # A FIFO with an allowed suffix must be skipped, not opened (which would
        # block hash_file indefinitely).
        try:
            os.mkfifo(self.root / "pipe.txt")
        except (OSError, NotImplementedError, AttributeError):
            self.skipTest("mkfifo not permitted")
        self._write("real.txt", b"x")
        rows, _, nonregular = self._build()
        self.assertEqual([r["title"] for r in rows], ["real.txt"])
        self.assertIn("pipe.txt", nonregular)

    def test_nested_output_dir_excluded(self):
        # The workflow ledger under the scanned root must not ingest itself.
        self._write("doc.txt", b"x")
        self._write("inventory/inventory.csv", b"source_id,\n")
        self._write("inventory/inventory.json", b"{}")
        exclude = (self.root / "inventory").resolve()
        rows, _, _ = self._build(exclude_dir=exclude)
        self.assertEqual([r["title"] for r in rows], ["doc.txt"])


class InventoryOutputTest(unittest.TestCase):
    def test_write_outputs_empty_dir_has_header(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "src"
            root.mkdir()
            out = Path(tmp) / "out"
            inventory.write_outputs([], root, out, [], [])
            csv_text = (out / "inventory.csv").read_text(encoding="utf-8-sig")
            self.assertTrue(csv_text.startswith("source_id,"))
            payload = json.loads((out / "inventory.json").read_text(encoding="utf-8"))
            self.assertEqual(payload["summary"]["files"], 0)
            self.assertFalse(payload["source_files_modified"])

    def test_write_outputs_records_summary(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "src"
            root.mkdir()
            (root / "a.txt").write_bytes(b"dup")
            (root / "b.txt").write_bytes(b"dup")
            (root / "薪资明细.xlsx").write_bytes(b"x")
            out = Path(tmp) / "out"
            skipped: list[str] = []
            skipped_nonregular: list[str] = []
            rows = inventory.build_inventory(
                root, set(inventory.DEFAULT_EXTENSIONS), False, skipped, skipped_nonregular
            )
            inventory.write_outputs(rows, root, out, skipped, skipped_nonregular)
            payload = json.loads((out / "inventory.json").read_text(encoding="utf-8"))
            self.assertEqual(payload["summary"]["files"], 3)
            self.assertEqual(payload["summary"]["exact_duplicate_groups"], 1)
            self.assertEqual(payload["summary"]["possible_sensitive_by_filename"], 1)


class InventoryMainTest(unittest.TestCase):
    def _run_main(self, argv):
        old = sys.argv
        sys.argv = ["inventory.py"] + argv
        try:
            return inventory.main()
        finally:
            sys.argv = old

    def test_output_dir_equal_to_root_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            (Path(tmp) / "doc.txt").write_bytes(b"x")
            code = self._run_main(["--root", tmp, "--output-dir", tmp])
            self.assertEqual(code, 2)
            # No ledger should have been written into the scanned root.
            self.assertFalse((Path(tmp) / "inventory.json").exists())

    def test_output_dir_outside_root_ok(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "src"
            root.mkdir()
            (root / "doc.txt").write_bytes(b"x")
            out = Path(tmp) / "out"
            code = self._run_main(["--root", str(root), "--output-dir", str(out)])
            self.assertEqual(code, 0)
            self.assertTrue((out / "inventory.json").exists())

    def test_nested_output_dir_not_self_ingested(self):
        with tempfile.TemporaryDirectory() as tmp:
            (Path(tmp) / "doc.txt").write_bytes(b"x")
            out = Path(tmp) / "inventory"
            # First run writes the ledger nested under root.
            self.assertEqual(self._run_main(["--root", tmp, "--output-dir", str(out)]), 0)
            # Second run must not ingest the ledger it just wrote.
            self.assertEqual(self._run_main(["--root", tmp, "--output-dir", str(out)]), 0)
            payload = json.loads((out / "inventory.json").read_text(encoding="utf-8"))
            titles = [item["title"] for item in payload["items"]]
            self.assertEqual(titles, ["doc.txt"])


if __name__ == "__main__":
    unittest.main()
