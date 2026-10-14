#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lark_common.sh
. "$SCRIPT_DIR/lark_common.sh"

DATE=""
START=""
END=""
OUT_DIR=""
PROJECT_ROOTS=()
IMPORTANT_PROJECTS=(AIEXCEL Swimlane Wardrobe)
MAX_FILES_PER_PROJECT=40

while [[ $# -gt 0 ]]; do
  case "$1" in
    --date|-Date) DATE=$2; shift 2 ;;
    --start|-Start) START=$2; shift 2 ;;
    --end|-End) END=$2; shift 2 ;;
    --out-dir|-OutDir) OUT_DIR=$2; shift 2 ;;
    --project-root|--project-roots|-ProjectRoots) PROJECT_ROOTS+=("$2"); shift 2 ;;
    --important-project|-ImportantProjects) IMPORTANT_PROJECTS+=("$2"); shift 2 ;;
    --max-files-per-project|-MaxFilesPerProject) MAX_FILES_PER_PROJECT=$2; shift 2 ;;
    *) die "Unknown argument: $1" ;;
  esac
done

[[ -n "$DATE" && -n "$START" && -n "$END" && -n "$OUT_DIR" ]] || die "Required: --date --start --end --out-dir"
need_cmd jq
need_cmd python3

mkdir -p "$OUT_DIR"
[[ ${#PROJECT_ROOTS[@]} -eq 0 ]] && PROJECT_ROOTS=("$PWD")
CODEX_HOME_DIR="${CODEX_HOME:-$HOME/.codex}"
CLAUDE_HOME_DIR="$HOME/.claude"
JSON_PATH="$OUT_DIR/agent_evidence_$DATE.json"
MD_PATH="$OUT_DIR/agent_session_evidence_$DATE.md"

python3 - "$DATE" "$START" "$END" "$OUT_DIR" "$CODEX_HOME_DIR" "$CLAUDE_HOME_DIR" "$MAX_FILES_PER_PROJECT" "$JSON_PATH" "$MD_PATH" "${PROJECT_ROOTS[@]}" -- "${IMPORTANT_PROJECTS[@]}" <<'PY'
from __future__ import annotations
import json, os, re, sys
from datetime import datetime
from pathlib import Path

date, start, end, out_dir, codex_home, claude_home, max_files, json_path, md_path = sys.argv[1:10]
sep = sys.argv.index("--")
project_roots = sys.argv[10:sep]
important_projects = set(sys.argv[sep+1:])
max_files = int(max_files)

def parse_dt(s):
    if s.endswith("Z"):
        s = s[:-1] + "+00:00"
    return datetime.fromisoformat(s).astimezone()

start_dt = parse_dt(start)
end_dt = parse_dt(end)

def in_range_ts(ts):
    try:
        return start_dt.timestamp() <= ts <= end_dt.timestamp()
    except Exception:
        return False

def in_range_path(path: Path):
    try:
        return in_range_ts(path.stat().st_mtime)
    except Exception:
        return False

def read_json_line(line):
    try:
        return json.loads(line)
    except Exception:
        return None

def limit_text(text, max_len=240):
    if not text:
        return None
    clean = re.sub(r"\s+", " ", str(text)).strip()
    return clean if len(clean) <= max_len else clean[:max_len] + "..."

def payload_text(payload):
    if not isinstance(payload, dict):
        return None
    if payload.get("message"):
        return str(payload["message"])
    content = payload.get("content")
    if isinstance(content, list):
        parts = []
        for item in content:
            if isinstance(item, dict) and item.get("text"):
                parts.append(str(item["text"]))
        return " ".join(parts) if parts else None
    return None

def read_codex_session(path: Path):
    meta = {}
    users, assistants, commands, mentioned = [], [], [], []
    try:
        with path.open("r", encoding="utf-8", errors="ignore") as fh:
            for line in fh:
                j = read_json_line(line)
                if not j:
                    continue
                if j.get("type") == "session_meta":
                    meta = j.get("payload") or {}
                    continue
                if j.get("type") != "response_item":
                    continue
                payload = j.get("payload") or {}
                if payload.get("type") == "function_call":
                    cmd = payload.get("name") or ""
                    if payload.get("arguments"):
                        arg = limit_text(payload.get("arguments"), 180)
                        cmd = f"{cmd} {arg}" if arg else cmd
                    if cmd and len(commands) < 20:
                        commands.append(cmd)
                    continue
                if payload.get("type") != "message":
                    continue
                text = limit_text(payload_text(payload), 260)
                if not text:
                    continue
                mentioned += re.findall(r"[A-Za-z]:\\[^`\"')\]\s]+|/Users/[^`\"')\]\s]+", text)[:20]
                if payload.get("role") == "user" and "AGENTS.md instructions" not in text and len(users) < 8:
                    users.append(text)
                elif payload.get("role") == "assistant" and len(assistants) < 16:
                    assistants.append(text)
    except Exception:
        pass
    return {
        "path": str(path),
        "cwd": meta.get("cwd"),
        "session_id": meta.get("id") or path.stem,
        "user_messages": users,
        "assistant_messages": assistants[-6:],
        "commands": commands,
        "mentioned_paths": sorted(set(mentioned))[:20],
    }

codex_sessions = []
session_index = Path(codex_home) / "session_index.jsonl"
if session_index.exists():
    for line in session_index.read_text(encoding="utf-8", errors="ignore").splitlines():
        j = read_json_line(line)
        if not j or not j.get("updated_at"):
            continue
        try:
            updated = parse_dt(j["updated_at"])
        except Exception:
            continue
        if start_dt <= updated <= end_dt:
            codex_sessions.append({
                "source": "codex_index",
                "thread_name": j.get("thread_name"),
                "id": j.get("id"),
                "updated_at": updated.strftime("%Y-%m-%d %H:%M:%S"),
            })

day_parts = date.split("-")
codex_day = Path(codex_home) / "sessions" / day_parts[0] / day_parts[1] / day_parts[2]
for base in [codex_day, Path(codex_home) / "archived_sessions"]:
    if not base.exists():
        continue
    for path in sorted(base.glob("*.jsonl")):
        if not in_range_path(path):
            continue
        summary = read_codex_session(path)
        codex_sessions.append({
            "source": "codex_session_file",
            "thread_name": None,
            "id": summary.get("session_id") or path.stem,
            "path": str(path),
            "cwd": summary.get("cwd"),
            "updated_at": datetime.fromtimestamp(path.stat().st_mtime).strftime("%Y-%m-%d %H:%M:%S"),
            "user_messages": summary["user_messages"],
            "assistant_messages": summary["assistant_messages"],
            "commands": summary["commands"],
            "mentioned_paths": summary["mentioned_paths"],
        })

claude_sessions = []
claude_projects = Path(claude_home) / "projects"
if claude_projects.exists():
    count = 0
    for path in claude_projects.rglob("*.jsonl"):
        if count >= 200 or not in_range_path(path):
            continue
        cwd = branch = None
        try:
            with path.open("r", encoding="utf-8", errors="ignore") as fh:
                for _, line in zip(range(12), fh):
                    j = read_json_line(line)
                    if isinstance(j, dict):
                        cwd = cwd or j.get("cwd")
                        branch = branch or j.get("gitBranch")
        except Exception:
            pass
        claude_sessions.append({
            "source": "claude_project",
            "path": str(path),
            "cwd": cwd,
            "git_branch": branch,
            "updated_at": datetime.fromtimestamp(path.stat().st_mtime).strftime("%Y-%m-%d %H:%M:%S"),
        })
        count += 1

def skip_path(path: Path):
    s = str(path)
    return any(part in s for part in ["/node_modules/", "/.git/", "/dist/", "/cache/", "/__pycache__/"])

project_candidates = []
for root_str in project_roots:
    root = Path(root_str).expanduser()
    if not root.exists():
        continue
    children = [root] if root.is_file() else list(root.iterdir())
    for project in children:
        if not project.is_dir():
            continue
        is_important = project.name in important_projects
        recent_project = in_range_path(project)
        recent_files = []
        if recent_project or is_important:
            try:
                files = [p for p in project.rglob("*") if p.is_file() and not skip_path(p) and in_range_path(p)]
                files.sort(key=lambda p: p.stat().st_mtime, reverse=True)
                for p in files[:max_files]:
                    recent_files.append({"path": str(p), "last_write_time": datetime.fromtimestamp(p.stat().st_mtime).strftime("%Y-%m-%d %H:%M:%S")})
            except Exception:
                pass
        if recent_files:
            status = "has_today_files"
        elif recent_project:
            status = "project_timestamp_only"
        elif is_important:
            status = "important_project_no_today_evidence"
        else:
            continue
        project_candidates.append({
            "name": project.name,
            "path": str(project),
            "last_write_time": datetime.fromtimestamp(project.stat().st_mtime).strftime("%Y-%m-%d %H:%M:%S"),
            "status": status,
            "recent_files": recent_files,
        })

evidence = {
    "date": date,
    "start": start,
    "end": end,
    "generated_at": datetime.now().strftime("%Y-%m-%d %H:%M:%S %z"),
    "codex_home": codex_home,
    "claude_home": claude_home,
    "codex_sessions": codex_sessions,
    "claude_sessions": claude_sessions,
    "project_candidates": project_candidates,
}
Path(json_path).write_text(json.dumps(evidence, ensure_ascii=False, indent=2), encoding="utf-8")

tick = "`"
lines = [f"## 本地 Agent 证据摘要（{date}）", "", "### 候选本地项目", "", "| 项目 | 路径 | 状态 | 当天文件数 | 说明 |", "| --- | --- | --- | ---: | --- |"]
if project_candidates:
    for p in project_candidates:
        reason = {
            "has_today_files": "有当天修改文件，必须由 Agent 判断是否纳入日报。",
            "project_timestamp_only": "目录时间有变化，但未定位到可用文件证据，默认待确认。",
            "important_project_no_today_evidence": "历史重点项目，本次未发现当天文件证据，默认不纳入。",
        }.get(p["status"], "无当天证据。")
        lines.append(f"| {p['name']} | {tick}{p['path']}{tick} | {p['status']} | {len(p['recent_files'])} | {reason} |")
else:
    lines.append("| 无 | - | no_candidates | 0 | 未发现当天本地项目证据。 |")
lines += ["", "### 纳入日报的本地工作包", "", "> 由 Agent 根据候选项目、会话摘要和实际产物判断。脚本只提供证据，不直接写最终结论。", "", "### 未纳入原因", "", "| 项目/会话 | 原因 |", "| --- | --- |"]
for p in project_candidates:
    if p["status"] == "important_project_no_today_evidence":
        lines.append(f"| {p['name']} | 历史重点项目，但本次未发现当天文件证据；除非会话或用户补充证明当天有产出，否则不纳入。 |")
lines += ["", "### Codex 会话候选", "", "| 来源 | 线程/文件 | 更新时间 |", "| --- | --- | --- |"]
for s in codex_sessions[:80]:
    name = s.get("thread_name") or s.get("path") or s.get("id")
    lines.append(f"| {s.get('source')} | {tick}{name}{tick} | {s.get('updated_at')} |")
if not codex_sessions:
    lines.append("| 无 | - | - |")
lines += ["", "### Codex 会话摘要", ""]
for s in [x for x in codex_sessions if x.get("path")][:40]:
    title = s.get("thread_name") or s.get("cwd") or s.get("path")
    lines += [f"#### {title}", "", f"- 会话文件：{tick}{s.get('path')}{tick}"]
    if s.get("cwd"):
        lines.append(f"- 工作目录：{tick}{s.get('cwd')}{tick}")
    if s.get("user_messages"):
        lines.append("- 用户请求：" + " / ".join(s["user_messages"][:3]))
    if s.get("assistant_messages"):
        lines.append("- 处理摘要：" + " / ".join(s["assistant_messages"][-3:]))
    if s.get("mentioned_paths"):
        lines.append("- 提到路径：" + "；".join(s["mentioned_paths"][:8]))
    lines.append("")
lines += ["", "### Claude Code 会话候选", "", "| 路径 | cwd | 分支 | 更新时间 |", "| --- | --- | --- | --- |"]
for s in claude_sessions[:80]:
    lines.append(f"| {tick}{s.get('path')}{tick} | {tick}{s.get('cwd')}{tick} | {tick}{s.get('git_branch')}{tick} | {s.get('updated_at')} |")
if not claude_sessions:
    lines.append("| 无 | - | - | - |")
Path(md_path).write_text("\n".join(lines), encoding="utf-8")
print(json_path)
print(md_path)
PY
