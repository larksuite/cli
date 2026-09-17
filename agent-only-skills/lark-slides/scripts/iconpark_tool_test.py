# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
from __future__ import annotations

import json
import subprocess
import sys
import unittest
from pathlib import Path

import iconpark_tool


SCRIPT_PATH = Path(__file__).resolve().with_name("iconpark_tool.py")


class IconParkToolTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.index_data = iconpark_tool.load_index()

    def test_search_icons_finds_growth_trend(self) -> None:
        results = iconpark_tool.search_icons(self.index_data, {"query": "增长趋势", "limit": 5})
        self.assertTrue(results)
        self.assertTrue(
            any(entry["iconType"] == "iconpark/Charts/positive-dynamics.svg" for entry in results)
        )

    def test_search_icons_supports_english_query(self) -> None:
        results = iconpark_tool.search_icons(self.index_data, {"query": "security protect", "limit": 3})
        self.assertTrue(results)
        self.assertEqual(results[0]["iconType"], "iconpark/Safe/protect.svg")

    def test_search_icons_supports_category_filter(self) -> None:
        results = iconpark_tool.search_icons(
            self.index_data,
            {"query": "data", "category": "Charts", "limit": 10},
        )
        self.assertTrue(results)
        self.assertTrue(all(entry["category"] == "Charts" for entry in results))

    def test_search_icons_does_not_expand_ai_inside_words(self) -> None:
        mail_results = iconpark_tool.search_icons(self.index_data, {"query": "mail", "limit": 5})
        self.assertEqual(mail_results[0]["iconType"], "iconpark/Office/envelope-one.svg")
        self.assertNotEqual(mail_results[0]["iconType"], "iconpark/Others/magic.svg")

        fail_results = iconpark_tool.search_icons(self.index_data, {"query": "fail", "limit": 5})
        self.assertNotEqual(fail_results[0]["iconType"], "iconpark/Others/magic.svg")

    def test_search_icons_supports_template_icon_queries(self) -> None:
        cases = [
            ("arrow", "iconpark/Arrows/arrow-right.svg"),
            ("right", "iconpark/Arrows/right.svg"),
            ("PPT", "iconpark/Music/ppt.svg"),
            ("table", "iconpark/Office/table.svg"),
            ("会议", "iconpark/Office/schedule.svg"),
            ("飞书", "iconpark/Brand/bydesign.svg"),
        ]
        for query, icon_type in cases:
            with self.subTest(query=query):
                results = iconpark_tool.search_icons(self.index_data, {"query": query, "limit": 10})
                self.assertTrue(
                    any(entry["iconType"] == icon_type for entry in results),
                    f"{icon_type} not found in {results}",
                )

    def test_search_icons_defaults_to_wider_candidate_set(self) -> None:
        results = iconpark_tool.search_icons(self.index_data, {"query": "data"})
        self.assertEqual(len(results), 8)

    def test_search_icons_boosts_common_slide_terms(self) -> None:
        results = iconpark_tool.search_icons(self.index_data, {"query": "会议", "limit": 3})
        self.assertTrue(
            any(entry["iconType"] == "iconpark/Office/schedule.svg" for entry in results),
            f"iconpark/Office/schedule.svg not found in {results}",
        )

    def test_search_icons_keeps_high_value_top_results(self) -> None:
        cases = [
            ("安全", "iconpark/Safe/protect.svg"),
            ("邮件", "iconpark/Office/mail-open.svg"),
            ("会议", "iconpark/Office/schedule.svg"),
            ("增长趋势", "iconpark/Charts/chart-line.svg"),
            ("飞书", "iconpark/Brand/bydesign.svg"),
        ]
        for query, icon_type in cases:
            with self.subTest(query=query):
                results = iconpark_tool.search_icons(self.index_data, {"query": query, "limit": 3})
                self.assertTrue(results)
                self.assertEqual(results[0]["iconType"], icon_type)

    def test_search_icons_requires_query(self) -> None:
        with self.assertRaises(iconpark_tool.IconParkToolError):
            iconpark_tool.search_icons(self.index_data, {"limit": 5})

    def test_search_icons_rejects_invalid_limit(self) -> None:
        with self.assertRaises(iconpark_tool.IconParkToolError):
            iconpark_tool.search_icons(self.index_data, {"query": "data", "limit": "abc"})

    def test_resolve_icon_accepts_name_and_icon_type(self) -> None:
        by_name = iconpark_tool.resolve_icon(self.index_data, "chart-line")
        by_type = iconpark_tool.resolve_icon(self.index_data, "iconpark/Charts/chart-line.svg")
        self.assertEqual(by_name["iconType"], "iconpark/Charts/chart-line.svg")
        self.assertEqual(by_name, by_type)

    def test_resolve_icon_accepts_template_icon_type(self) -> None:
        result = iconpark_tool.resolve_icon(self.index_data, "iconpark/Arrows/arrow-right.svg")
        self.assertEqual(result["iconType"], "iconpark/Arrows/arrow-right.svg")

    def test_resolve_icon_rejects_unknown_name(self) -> None:
        with self.assertRaises(iconpark_tool.IconParkToolError):
            iconpark_tool.resolve_icon(self.index_data, "not-a-real-icon")

    def test_list_categories_counts_index(self) -> None:
        categories = iconpark_tool.list_categories(self.index_data)
        self.assertTrue(any(entry["category"] == "Charts" and entry["count"] > 0 for entry in categories))


class IconParkToolCLITest(unittest.TestCase):
    def run_tool(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(SCRIPT_PATH), *args],
            capture_output=True,
            check=False,
            text=True,
        )

    def test_cli_search_writes_json_to_stdout(self) -> None:
        result = self.run_tool("search", "--query", "增长趋势", "--limit", "5")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stderr, "")

        output = json.loads(result.stdout)
        self.assertTrue(output)
        self.assertTrue(
            any(entry["iconType"] == "iconpark/Charts/positive-dynamics.svg" for entry in output)
        )

    def test_cli_resolve_writes_json_to_stdout(self) -> None:
        result = self.run_tool("resolve", "--name", "chart-line")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stderr, "")

        output = json.loads(result.stdout)
        self.assertEqual(output["iconType"], "iconpark/Charts/chart-line.svg")

    def test_cli_list_categories_writes_json_to_stdout(self) -> None:
        result = self.run_tool("list-categories")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stderr, "")

        output = json.loads(result.stdout)
        self.assertTrue(any(entry["category"] == "Charts" and entry["count"] > 0 for entry in output))

    def test_cli_help_writes_usage_to_stderr(self) -> None:
        result = self.run_tool("--help")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "")
        self.assertIn("Usage:", result.stderr)
        self.assertIn("python3 iconpark_tool.py search", result.stderr)

    def test_cli_invalid_argument_writes_error_to_stderr(self) -> None:
        result = self.run_tool("search", "增长趋势")
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("iconpark-tool error: unexpected argument: 增长趋势", result.stderr)

    def test_cli_unknown_command_writes_usage_and_error_to_stderr(self) -> None:
        result = self.run_tool("unknown")
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("Usage:", result.stderr)
        self.assertIn("iconpark-tool error: unknown command: unknown", result.stderr)


if __name__ == "__main__":
    unittest.main()
