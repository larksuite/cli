# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
LAYOUT_LINT_PATH = SCRIPT_DIR / "layout-lint.py"

spec = importlib.util.spec_from_file_location("layout_lint", LAYOUT_LINT_PATH)
layout_lint = importlib.util.module_from_spec(spec)
assert spec and spec.loader
spec.loader.exec_module(layout_lint)


class LayoutLintTest(unittest.TestCase):
    def test_lint_xml_detects_overlapping_text_boxes(self) -> None:
        result = layout_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <shape type="text" topLeftX="80" topLeftY="80" width="300" height="60">
                    <content textType="title"><p>Title</p></content>
                  </shape>
                  <shape type="text" topLeftX="100" topLeftY="110" width="300" height="80">
                    <content textType="body"><p>Body</p></content>
                  </shape>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["slides"][0]["issues"][0]["code"], "bbox_overlap")

    def test_lint_xml_detects_out_of_bounds_elements_and_text_height_risks(self) -> None:
        result = layout_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide xmlns="http://www.larkoffice.com/sml/2.0">
                <data>
                  <shape type="text" topLeftX="80" topLeftY="80" width="180" height="20">
                    <content textType="body" fontSize="18"><p>This paragraph is intentionally much longer than the box can safely contain.</p></content>
                  </shape>
                  <img src="tok" topLeftX="900" topLeftY="500" width="120" height="80"/>
                </data>
              </slide>
            </presentation>
            """
        )
        self.assertEqual(result["summary"]["error_count"], 1)
        self.assertEqual(result["summary"]["warning_count"], 1)
        self.assertTrue(any(issue["code"] == "out_of_bounds" for issue in result["slides"][0]["issues"]))
        self.assertTrue(any(issue["code"] == "text_height_risk" for issue in result["slides"][0]["issues"]))
