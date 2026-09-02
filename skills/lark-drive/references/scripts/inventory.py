#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Build a read-only inventory for an explicitly authorized local source path.

This script is the deterministic ingest step for the `knowledge_ingest`
workflow's INVENTORY state. It scans an authorized local file or directory and
produces a source ledger (inventory.csv / inventory.json) without modifying,
moving, or deleting any source file.

Design principle: the inventory is metadata only. It computes SHA-256 for exact
deduplication, flags possibly-sensitive files by filename, and classifies parse
readiness by extension. It never reads file *content* to make governance
decisions -- that is the agent's job in a later state. Incremental diffing (skip
files whose SHA-256 is unchanged since a prior run) is likewise done by the
agent comparing a fresh inventory.json against the prior one; this script always
performs a full scan and carries no baseline state.

Symlink safety: the authorized root must not itself be a symbolic link, and any
symlink encountered inside the tree is skipped (never resolved or read), so a
shortcut cannot redirect the scan outside the authorized scope.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
import os
import sys
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path


DEFAULT_EXTENSIONS = {
    ".csv", ".doc", ".docx", ".htm", ".html", ".jpeg", ".jpg", ".json",
    ".md", ".ods", ".odt", ".pdf", ".png", ".ppt", ".pptx", ".rtf",
    ".svg", ".tif", ".tiff", ".tsv", ".txt", ".xls", ".xlsx", ".yaml", ".yml",
}

SKIP_DIRS = {
    ".git", ".svn", "__pycache__", "node_modules", "dist", "build",
    ".cache", ".idea", ".vscode",
}

SENSITIVE_NAME_HINTS = {
    "身份证", "手机号", "银行卡", "花名册", "客户名单", "员工名单", "工资明细",
    "薪资明细", "绩效结果", "病历", "体检结果", "合同", "仲裁", "申诉", "奖惩",
    "password", "secret", "access_token", "api_key", "credential", "private_key",
}

TEXT_EXTRACTABLE = {
    ".csv", ".doc", ".docx", ".htm", ".html", ".json", ".md", ".ods", ".odt",
    ".pdf", ".ppt", ".pptx", ".rtf", ".tsv", ".txt", ".xls", ".xlsx", ".yaml", ".yml",
}

IMAGE_EXTENSIONS = {".jpeg", ".jpg", ".png", ".svg", ".tif", ".tiff"}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Inventory authorized source files without modifying them."
    )
    parser.add_argument("--root", required=True, help="Authorized file or directory to scan")
    parser.add_argument("--output-dir", required=True, help="Directory for inventory.csv/json")
    parser.add_argument(
        "--extensions",
        help="Comma-separated extensions; defaults to common knowledge-source formats",
    )
    parser.add_argument("--include-hidden", action="store_true", help="Include hidden files")
    return parser.parse_args()


def normalize_extensions(raw: str | None) -> set[str]:
    if not raw:
        return set(DEFAULT_EXTENSIONS)
    extensions = set()
    for value in raw.split(","):
        value = value.strip().lower()
        if value:
            extensions.add(value if value.startswith(".") else f".{value}")
    if not extensions:
        raise ValueError("--extensions did not contain a usable extension")
    return extensions


def hash_file(path: Path) -> str:
    digest = hashlib.sha256()
    flags = os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0)
    descriptor = os.open(path, flags)
    with os.fdopen(descriptor, "rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def iter_files(root: Path, include_hidden: bool, skipped_symlinks: list[str],
               skipped_nonregular: list[str], exclude_dir: Path | None = None):
    if root.is_file():
        yield root
        return
    for current, dirs, files in os.walk(root):
        dirs[:] = sorted(
            item for item in dirs
            if item not in SKIP_DIRS and (include_hidden or not item.startswith("."))
            and not _record_symlink(Path(current) / item, root, skipped_symlinks)
            and not _is_excluded(Path(current) / item, exclude_dir)
        )
        for name in sorted(files):
            if name.startswith("~$") or (not include_hidden and name.startswith(".")):
                continue
            path = Path(current) / name
            if _record_symlink(path, root, skipped_symlinks):
                continue
            # Skip non-regular files (FIFOs, devices, sockets): opening a FIFO
            # for reading blocks indefinitely in hash_file. is_file() follows no
            # symlink here because symlinks are already filtered above.
            if not path.is_file():
                skipped_nonregular.append(_relative_name(path, root))
                continue
            yield path


def _relative_name(path: Path, root: Path) -> str:
    """Best-effort path relative to root, falling back to the bare name."""
    try:
        return str(path.relative_to(root))
    except ValueError:
        return path.name


def _is_excluded(path: Path, exclude_dir: Path | None) -> bool:
    """Return True if path is the workflow output dir nested under the root.

    Keeps a re-run from ingesting its own inventory.csv/json ledger.
    """
    if exclude_dir is None:
        return False
    try:
        return path.resolve() == exclude_dir
    except OSError:
        return False


def _record_symlink(path: Path, root: Path, skipped_symlinks: list[str]) -> bool:
    """Return True for symlinks without resolving or reading their targets."""
    if not path.is_symlink():
        return False
    skipped_symlinks.append(_relative_name(path, root))
    return True


def risk_hint(name: str) -> str:
    lowered = name.lower()
    hits = sorted(hint for hint in SENSITIVE_NAME_HINTS if hint.lower() in lowered)
    return "possible_sensitive:" + "|".join(hits) if hits else "needs_content_review"


def parse_readiness(extension: str) -> str:
    if extension in TEXT_EXTRACTABLE:
        return "text_extractable"
    if extension in IMAGE_EXTENSIONS:
        return "ocr_or_visual_review"
    return "manual_review"


def empty_row(relative: str, title: str, extension: str, error: str) -> dict:
    return {
        "source_id": "", "source_type": "local_file", "source_location": relative,
        "title": title, "extension": extension, "size_bytes": "", "modified_at": "",
        "sha256": "", "duplicate_group": "", "duplicate_count": "",
        "parse_readiness": "failed", "risk_hint": "unknown", "version": "待确认",
        "publisher": "待确认", "business_owner": "待指定", "topic": "待分类",
        "scope": "待确认", "audience": "待确认", "sensitivity": "待人工审核",
        "conflict_status": "unknown", "target_node": "待映射",
        "proposed_action": "manual_review", "review_status": "blocked", "error": error,
    }


def build_inventory(
    root: Path,
    extensions: set[str],
    include_hidden: bool,
    skipped_symlinks: list[str],
    skipped_nonregular: list[str],
    exclude_dir: Path | None = None,
) -> list[dict]:
    rows = []
    for path in iter_files(root, include_hidden, skipped_symlinks,
                           skipped_nonregular, exclude_dir):
        extension = path.suffix.lower()
        if extension not in extensions:
            continue
        relative = path.name if root.is_file() else str(path.relative_to(root))
        try:
            stat = path.stat()
            digest = hash_file(path)
            rows.append({
                "source_id": digest,
                "source_type": "local_file",
                "source_location": relative,
                "title": path.name,
                "extension": extension,
                "size_bytes": stat.st_size,
                "modified_at": datetime.fromtimestamp(stat.st_mtime, tz=timezone.utc).isoformat(),
                "sha256": digest,
                "duplicate_group": "",
                "duplicate_count": 1,
                "parse_readiness": parse_readiness(extension),
                "risk_hint": risk_hint(path.name),
                "version": "待确认",
                "publisher": "待确认",
                "business_owner": "待指定",
                "topic": "待分类",
                "scope": "待确认",
                "audience": "待确认",
                "sensitivity": "待人工审核",
                "conflict_status": "none",
                "target_node": "待映射",
                "proposed_action": "review",
                "review_status": "pending",
                "error": "",
            })
        except (OSError, PermissionError) as exc:
            rows.append(empty_row(relative, path.name, extension, str(exc)))

    counts = Counter(row["sha256"] for row in rows if row["sha256"])
    groups = {
        digest: f"exact-{index:04d}"
        for index, (digest, count) in enumerate(
            ((digest, count) for digest, count in sorted(counts.items()) if count > 1),
            start=1,
        )
    }
    for row in rows:
        digest = row["sha256"]
        if digest:
            row["duplicate_count"] = counts[digest]
            row["duplicate_group"] = groups.get(digest, "")
            if counts[digest] > 1:
                row["proposed_action"] = "deduplicate_review"
    return rows


def write_outputs(
    rows: list[dict],
    root: Path,
    output_dir: Path,
    skipped_symlinks: list[str],
    skipped_nonregular: list[str],
) -> None:
    output_dir.mkdir(parents=True, exist_ok=True)
    fields = list(rows[0]) if rows else list(empty_row("", "", "", ""))
    with (output_dir / "inventory.csv").open("w", encoding="utf-8-sig", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=fields)
        writer.writeheader()
        writer.writerows(rows)

    payload = {
        "schema_version": "1.0",
        "generated_at": datetime.now(tz=timezone.utc).isoformat(),
        "authorized_root": str(root.resolve()),
        "source_files_modified": False,
        "skipped_symlinks": sorted(skipped_symlinks),
        "skipped_nonregular": sorted(skipped_nonregular),
        "summary": {
            "files": len(rows),
            "skipped_symlinks": len(skipped_symlinks),
            "skipped_nonregular": len(skipped_nonregular),
            "failed": sum(row["parse_readiness"] == "failed" for row in rows),
            "exact_duplicate_groups": len({row["duplicate_group"] for row in rows if row["duplicate_group"]}),
            "possible_sensitive_by_filename": sum(
                row["risk_hint"].startswith("possible_sensitive:") for row in rows
            ),
        },
        "items": rows,
    }
    (output_dir / "inventory.json").write_text(
        json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8"
    )


def main() -> int:
    args = parse_args()
    root = Path(args.root).expanduser()
    if root.is_symlink():
        print(
            f"error: authorized root must not be a symbolic link: {root}",
            file=sys.stderr,
        )
        return 2
    if not root.exists() or not (root.is_file() or root.is_dir()):
        print(f"error: authorized root is not a readable file or directory: {root}", file=sys.stderr)
        return 2
    try:
        output_dir = Path(args.output_dir).expanduser()
        # If the output dir is nested under the scanned root, exclude it so a
        # re-run does not ingest its own inventory ledger (and the JSON's
        # changing timestamp does not read as a new file every run).
        try:
            exclude_dir = output_dir.resolve()
        except OSError:
            exclude_dir = None
        skipped_symlinks: list[str] = []
        skipped_nonregular: list[str] = []
        rows = build_inventory(
            root,
            normalize_extensions(args.extensions),
            args.include_hidden,
            skipped_symlinks,
            skipped_nonregular,
            exclude_dir,
        )
        write_outputs(rows, root, output_dir, skipped_symlinks, skipped_nonregular)
    except (OSError, ValueError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    print(json.dumps({
        "ok": True,
        "files": len(rows),
        "exact_duplicate_groups": len({row["duplicate_group"] for row in rows if row["duplicate_group"]}),
        "skipped_symlinks": len(skipped_symlinks),
        "skipped_nonregular": len(skipped_nonregular),
        "output_dir": str(output_dir.resolve()),
        "source_files_modified": False,
    }, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
