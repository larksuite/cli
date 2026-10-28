#!/usr/bin/env python3
"""Check solid conditional fills without recalculating or modifying an xlsx.

Uses only the Python standard library. Exit 0 means no matching compatibility
issue, 3 means foreground-only solid conditional fills, and 2 means invalid or
unsupported input. Transitional and Strict worksheet rules are supported;
x14 extension rules require manual verification and never return success.
This check does not evaluate rule formulas or prove final visual correctness.
"""
from __future__ import annotations

import argparse
import json
import posixpath
import xml.etree.ElementTree as ET
import zipfile


CONDITIONAL_FILL_ADVICE = (
    "条件格式的实心填充仅设置 fgColor、未设置 bgColor，存在 Excel / 在线表格 / 预览显示不一致的兼容风险。"
    "若目标是实心背景色，创建规则时用 PatternFill('solid', fgColor='FF66BB6A', bgColor='FF66BB6A')"
    "（颜色替换为目标色），重新保存后回读规则样式并检查最终显示。"
)
MAIN_NS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
DOC_REL_NS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
PACKAGE_REL_NS = "http://schemas.openxmlformats.org/package/2006/relationships"
STRICT_NS = "http://purl.oclc.org/ooxml/spreadsheetml/main"
STRICT_REL_NS = "http://purl.oclc.org/ooxml/officeDocument/relationships"
X14_NS = "http://schemas.microsoft.com/office/spreadsheetml/2009/9/main"
X14_CONDITIONAL_TAGS = {"{" + X14_NS + "}" + name for name in
                        ("conditionalFormattings", "conditionalFormatting", "cfRule")}


def _foreground_only(fill):
    if fill is None:
        return None
    namespace = fill.tag.rpartition("}")[0] + "}" if fill.tag.startswith("{") else ""
    pattern = fill.find(namespace + "patternFill")
    if pattern is None or pattern.get("patternType") != "solid":
        return None
    foreground = pattern.find(namespace + "fgColor")
    background = pattern.find(namespace + "bgColor")
    # openpyxl omits its default 00000000 color when serializing a fill.
    if foreground is None or dict(foreground.attrib) == {"rgb": "00000000"}:
        return None
    if background is None or dict(background.attrib) == {"rgb": "00000000"}:
        return dict(foreground.attrib)
    return None


def _part_name(target: str) -> str:
    return (posixpath.normpath(target.lstrip("/")) if target.startswith("/")
            else posixpath.normpath(posixpath.join("xl", target)))


def verify(path: str) -> dict:
    hits = []
    with zipfile.ZipFile(path) as archive:
        workbook = ET.fromstring(archive.read("xl/workbook.xml"))
        namespace = workbook.tag.partition("}")[0].lstrip("{")
        doc_rel_ns = {MAIN_NS: DOC_REL_NS, STRICT_NS: STRICT_REL_NS}.get(namespace)
        ns = "{" + namespace + "}"
        if doc_rel_ns is None or workbook.tag != ns + "workbook":
            raise ValueError("unsupported or invalid workbook root/namespace")
        sheets = workbook.findall(ns + "sheets/" + ns + "sheet")
        if not sheets:
            raise ValueError("workbook contains no sheet definitions")
        relationships = ET.fromstring(archive.read("xl/_rels/workbook.xml.rels"))
        if relationships.tag != "{" + PACKAGE_REL_NS + "}Relationships":
            raise ValueError("invalid workbook relationships root")
        targets = {
            rel.attrib["Id"]: rel
            for rel in relationships.findall("{" + PACKAGE_REL_NS + "}Relationship")
        }
        dxfs = []
        style_relationships = [rel for rel in targets.values() if rel.get("Type") == doc_rel_ns + "/styles"]
        if len(style_relationships) > 1:
            raise ValueError("workbook has multiple styles relationships")
        if style_relationships:
            rel = style_relationships[0]
            if rel.get("TargetMode") == "External":
                raise ValueError("external styles relationship is not supported")
            styles = ET.fromstring(archive.read(_part_name(rel.attrib["Target"])))
            if styles.tag != ns + "styleSheet":
                raise ValueError("unsupported or invalid styles root/namespace")
            formats = styles.find(ns + "dxfs")
            if formats is not None:
                dxfs = list(formats)
        for sheet in sheets:
            rel = targets[sheet.attrib["{" + doc_rel_ns + "}id"]]
            if rel.get("TargetMode") == "External":
                raise ValueError("external sheet relationship is not supported")
            if rel.get("Type") == doc_rel_ns + "/chartsheet":
                continue
            if rel.get("Type") != doc_rel_ns + "/worksheet":
                raise ValueError("unsupported sheet relationship type; manual verification required")
            worksheet = ET.fromstring(archive.read(_part_name(rel.attrib["Target"])))
            if worksheet.tag != ns + "worksheet":
                raise ValueError("unsupported or invalid worksheet root/namespace")
            if any(element.tag in X14_CONDITIONAL_TAGS for element in worksheet.iter()):
                raise ValueError("x14 conditional formatting is not supported; manual verification required")
            for conditional in worksheet.findall(ns + "conditionalFormatting"):
                for rule in conditional.findall(ns + "cfRule"):
                    if rule.get("dxfId") is None:
                        continue
                    dxf_id = int(rule.attrib["dxfId"])
                    if not 0 <= dxf_id < len(dxfs):
                        raise ValueError("conditional format references an invalid dxfId")
                    foreground = _foreground_only(dxfs[dxf_id].find(ns + "fill"))
                    if foreground is not None:
                        hits.append({
                            "sheet": sheet.attrib["name"], "range": conditional.attrib["sqref"],
                            "priority": int(rule.attrib["priority"]), "dxf_id": dxf_id,
                            "fgColor": foreground,
                        })
    violations = [{
        "kind": "conditional_format_fill_compatibility",
        "location": hit["sheet"] + "!" + hit["range"],
        "message": CONDITIONAL_FILL_ADVICE, "actual": hit,
    } for hit in hits]
    return {"status": "violations_found" if hits else "success", "violations": violations}


def check_file(path: str) -> dict:
    """Return an explicit error when raw formatting cannot be verified."""
    try:
        return verify(path)
    except (OSError, KeyError, ValueError, ET.ParseError, zipfile.BadZipFile) as exc:
        return {"status": "error", "error_type": type(exc).__name__, "error": str(exc)}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("xlsx")
    args = parser.parse_args()
    result = check_file(args.xlsx)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    if result["status"] == "error":
        return 2
    return 3 if result["violations"] else 0


if __name__ == "__main__":
    raise SystemExit(main())
