# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
from __future__ import annotations

import unittest

import xml_layout_density_lint


class XmlLayoutDensityLintTest(unittest.TestCase):
    def test_lint_xml_warns_for_blank_slide(self) -> None:
        result = xml_layout_density_lint.lint_xml(
            """
            <presentation xmlns="http://www.larkoffice.com/sml/2.0" width="960" height="540">
              <slide id="content-slide">
                <data>
                  <shape id="title" type="text" topLeftX="60" topLeftY="60" width="400" height="50">
                    <content fontSize="28"><p>Investment report</p></content>
                  </shape>
                </data>
              </slide>
              <slide id="blank-slide">
                <style><fill><fillColor color="rgba(255, 255, 255, 1)"/></fill></style>
                <data/>
                <note><content/></note>
              </slide>
            </presentation>
            """
        )

        self.assertEqual(result["summary"], {"slide_count": 2, "warning_count": 1, "error_count": 0})
        self.assertEqual(result["slides"][0]["issues"], [])
        self.assertEqual(result["slides"][1]["element_count"], 0)
        self.assertEqual(
            result["slides"][1]["issues"],
            [
                {
                    "level": "warning",
                    "code": "blank_slide",
                    "schema_version": "1.0",
                    "target": {"slide_number": 2},
                    "rule": {
                        "name": "slide_has_visible_content",
                        "comparison": "visible_element_count == 0",
                    },
                    "measurement": {"visible_element_count": 0},
                    "elements": [],
                }
            ],
        )

    def test_lint_xml_warns_when_large_container_is_mostly_empty(self) -> None:
        result = xml_layout_density_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="trend-card" type="rect" topLeftX="500" topLeftY="135" width="410" height="370"/>
                <shape id="trend-title" type="text" topLeftX="515" topLeftY="147" width="380" height="28">
                  <content fontSize="15"><p>Core trends</p></content>
                </shape>
                <shape id="trend-copy" type="text" topLeftX="515" topLeftY="177" width="380" height="315">
                  <content fontSize="12"><p>First point</p><p>Second point</p><p>Third point</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        issue = result["slides"][0]["issues"][0]
        self.assertEqual(issue["code"], "sparse_container_content")
        self.assertEqual(issue["target"]["container_id"], "trend-card")
        self.assertEqual(issue["target"], {
            "slide_number": 1,
            "container_id": "trend-card",
            "container_type": "rect",
            "bbox": {"x": 500, "y": 135, "width": 410, "height": 370},
        })
        self.assertLess(issue["measurement"]["content_coverage_ratio"], 0.15)
        self.assertEqual(issue["rule"], {
            "name": "large_container_visible_content_coverage",
            "threshold": 0.15,
            "comparison": "content_coverage_ratio < threshold",
        })
        self.assertEqual(issue["measurement"]["container_area"], 151700)
        self.assertEqual(issue["measurement"]["content_coverage_ratio"], 0.032)
        self.assertEqual(issue["elements"], ["trend-card", "trend-title", "trend-copy"])
        self.assertEqual(set(issue), {"level", "code", "schema_version", "target", "rule", "measurement", "elements"})

    def test_lint_xml_counts_rect_own_content_as_visible_content(self) -> None:
        result = xml_layout_density_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="load-card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184">
                  <content fontSize="18">
                    <p>被吊物</p>
                    <p><span fontSize="36">32.0 t</span></p>
                    <p>钢结构模块</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["slides"][0]["issues"], [])

    def test_lint_xml_reports_nonzero_coverage_for_rect_own_content_reproduction(self) -> None:
        result = xml_layout_density_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="load-card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184">
                  <content fontSize="18">
                    <p>被吊物</p>
                    <p>32.0 t</p>
                    <p>钢结构模块</p>
                  </content>
                </shape>
              </data>
            </slide>
            """
        )

        issue = result["slides"][0]["issues"][0]
        self.assertGreater(issue["measurement"]["visible_content_area"], 0)
        self.assertEqual(issue["measurement"]["content_element_count"], 1)
        self.assertGreater(issue["measurement"]["content_coverage_ratio"], 0)

    def test_lint_xml_still_warns_for_sparse_rect_own_content(self) -> None:
        result = xml_layout_density_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="sparse-card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184">
                  <content fontSize="12"><p>A</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        issue = result["slides"][0]["issues"][0]
        self.assertEqual(issue["target"]["container_id"], "sparse-card")
        self.assertGreater(issue["measurement"]["visible_content_area"], 0)
        self.assertEqual(issue["measurement"]["content_element_count"], 1)
        self.assertEqual(issue["elements"], ["sparse-card"])

    def test_lint_xml_unions_rect_own_content_with_child_content(self) -> None:
        self_only = xml_layout_density_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184">
                  <content fontSize="12"><p>A</p></content>
                </shape>
              </data>
            </slide>
            """
        )
        with_overlapping_child = xml_layout_density_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184">
                  <content fontSize="12"><p>A</p></content>
                </shape>
                <shape id="child" type="text" topLeftX="60" topLeftY="140" width="220" height="184">
                  <content fontSize="12"><p>A</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self_issue = self_only["slides"][0]["issues"][0]
        mixed_issue = with_overlapping_child["slides"][0]["issues"][0]
        self.assertEqual(
            mixed_issue["measurement"]["visible_content_area"],
            self_issue["measurement"]["visible_content_area"],
        )
        self.assertEqual(mixed_issue["measurement"]["content_element_count"], 2)

    def test_extract_density_elements_reads_nested_font_size_from_rect_content(self) -> None:
        elements = xml_layout_density_lint.extract_density_elements(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184">
                  <content fontSize="12"><p><span fontSize="36">32.0 t</span></p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(elements[0]["fontSize"], 36)

    def test_extract_density_elements_does_not_attach_following_text_to_self_closing_rect(self) -> None:
        elements = xml_layout_density_lint.extract_density_elements(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="60" topLeftY="140" width="220" height="184"/>
                <shape id="title" type="text" topLeftX="80" topLeftY="160" width="180" height="30">
                  <content fontSize="18"><p>Following title</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(elements[0]["text"], "")
        self.assertEqual(elements[1]["text"], "Following title")

    def test_lint_xml_allows_container_with_large_visual_child(self) -> None:
        result = xml_layout_density_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="chart-card" type="rect" topLeftX="500" topLeftY="135" width="410" height="300"/>
                <chart id="chart" topLeftX="525" topLeftY="170" width="350" height="220"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_warns_for_small_empty_visual_placeholder_cards(self) -> None:
        result = xml_layout_density_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="letter-placeholder" type="rect" topLeftX="520" topLeftY="180" width="200" height="200"/>
                <shape id="letter" type="text" topLeftX="540" topLeftY="250" width="160" height="70">
                  <content fontSize="46"><p>Z</p></content>
                </shape>
                <shape id="empty-placeholder" type="rect" topLeftX="744" topLeftY="180" width="144" height="200"/>
              </data>
            </slide>
            """
        )

        issues = result["slides"][0]["issues"]
        self.assertEqual(
            [issue["target"]["container_id"] for issue in issues],
            ["letter-placeholder", "empty-placeholder"],
        )
        self.assertEqual(issues[1]["measurement"]["content_element_count"], 0)

    def test_lint_xml_applies_global_threshold_to_normal_text_card(self) -> None:
        result = xml_layout_density_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="70" topLeftY="184" width="260" height="288"/>
                <shape id="title" type="text" topLeftX="90" topLeftY="215" width="220" height="30">
                  <content fontSize="18"><p>梦境与现实</p></content>
                </shape>
                <shape id="copy" type="text" topLeftX="90" topLeftY="330" width="220" height="70">
                  <content fontSize="13"><p>边界溶解，逻辑失效。观众被拽入潜意识的迷宫。</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        issue = result["slides"][0]["issues"][0]
        self.assertEqual(issue["target"]["container_id"], "card")
        self.assertEqual(issue["rule"]["threshold"], 0.15)

    def test_lint_xml_allows_image_overlay_rect(self) -> None:
        result = xml_layout_density_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <img id="hero" topLeftX="560" topLeftY="0" width="400" height="540"/>
                <shape id="tint" type="rect" topLeftX="560" topLeftY="0" width="400" height="540"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_allows_edge_spanning_layout_panel_and_nested_decoration(self) -> None:
        result = xml_layout_density_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="panel" type="rect" topLeftX="600" topLeftY="0" width="360" height="540"/>
                <shape id="decoration" type="rect" topLeftX="660" topLeftY="150" width="240" height="240"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_counts_icons_as_visible_content(self) -> None:
        result = xml_layout_density_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="80" topLeftY="140" width="320" height="240"/>
                <icon id="visual" iconType="shield" topLeftX="100" topLeftY="160" width="180" height="180"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["summary"]["warning_count"], 0)

    def test_lint_xml_warns_when_coverage_is_below_global_threshold(self) -> None:
        result = xml_layout_density_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="80" topLeftY="140" width="200" height="200"/>
                <icon id="visual" iconType="shield" topLeftX="100" topLeftY="160" width="70" height="70"/>
              </data>
            </slide>
            """
        )

        issue = result["slides"][0]["issues"][0]
        self.assertEqual(issue["target"]["container_id"], "card")
        self.assertEqual(issue["measurement"]["content_coverage_ratio"], 0.122)
        self.assertEqual(issue["rule"]["threshold"], 0.15)

    def test_lint_xml_allows_quarter_coverage_under_lower_threshold(self) -> None:
        result = xml_layout_density_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="card" type="rect" topLeftX="80" topLeftY="140" width="200" height="200"/>
                <icon id="visual" iconType="shield" topLeftX="100" topLeftY="160" width="100" height="100"/>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["slides"][0]["issues"], [])

    def test_lint_xml_allows_large_metric_card_above_lower_threshold(self) -> None:
        result = xml_layout_density_lint.lint_xml(
            """
            <slide xmlns="http://www.larkoffice.com/sml/2.0">
              <data>
                <shape id="metric-card" type="rect" topLeftX="80" topLeftY="140" width="360" height="300"/>
                <shape id="metric" type="text" topLeftX="104" topLeftY="190" width="340" height="90">
                  <content fontSize="12.4"><p><strong><span fontSize="62">400</span></strong>+ 项</p></content>
                </shape>
              </data>
            </slide>
            """
        )

        self.assertEqual(result["slides"][0]["issues"], [])


if __name__ == "__main__":
    unittest.main()
