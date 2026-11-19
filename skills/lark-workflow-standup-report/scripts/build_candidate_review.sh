#!/usr/bin/env bash
set -euo pipefail

DATE=""
SOURCE_MANIFEST=""
AGENT_EVIDENCE=""
OUT_FILE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --date|-Date) DATE=$2; shift 2 ;;
    --source-manifest-path|-SourceManifestPath) SOURCE_MANIFEST=$2; shift 2 ;;
    --agent-evidence-json-path|-AgentEvidenceJsonPath) AGENT_EVIDENCE=$2; shift 2 ;;
    --out-file|-OutFile) OUT_FILE=$2; shift 2 ;;
    *) echo "Unknown argument: $1" >&2; exit 1 ;;
  esac
done

[[ -n "$DATE" && -n "$SOURCE_MANIFEST" && -n "$AGENT_EVIDENCE" && -n "$OUT_FILE" ]] || {
  echo "Required: --date --source-manifest-path --agent-evidence-json-path --out-file" >&2
  exit 1
}

python3 - "$DATE" "$SOURCE_MANIFEST" "$AGENT_EVIDENCE" "$OUT_FILE" <<'PY'
from __future__ import annotations
import json, re, sys
from pathlib import Path

date, manifest_path, agent_path, out_file = sys.argv[1:5]
manifest = json.loads(Path(manifest_path).read_text(encoding="utf-8"))
agent = json.loads(Path(agent_path).read_text(encoding="utf-8"))

def as_list(v):
    if not v:
        return []
    return v if isinstance(v, list) else [v]

def load_json(path):
    try:
        return json.loads(Path(path).read_text(encoding="utf-8"))
    except Exception:
        return {}

def nested(obj, path, default=None):
    cur = obj
    for p in path:
        if not isinstance(cur, dict) or p not in cur:
            return default
        cur = cur[p]
    return cur

def clean_cell(s):
    return re.sub(r"[\r\n|]+", " ", str(s or "")).strip()

def chat_label(message, p2p_names):
    if message.get("chat_name"):
        return message["chat_name"]
    if message.get("chat_type") == "p2p":
        return "与%s私聊" % p2p_names.get(message.get("chat_id"), "未命名联系人")
    return message.get("chat_type") or message.get("chat_id") or "unknown_chat"

source_dir = Path(manifest_path).parent
all_messages = []
for f in as_list(nested(manifest, ["files", "im_all"], [])):
    j = load_json(f)
    all_messages.extend(nested(j, ["data", "messages"], []) or [])

current_user_name = manifest.get("current_user_name")
p2p_names = {}
for m in all_messages:
    if m.get("chat_type") != "p2p" or not m.get("chat_id"):
        continue
    sender_name = nested(m, ["sender", "name"])
    if sender_name and current_user_name and sender_name != current_user_name:
        p2p_names[m["chat_id"]] = sender_name
    elif nested(m, ["chat_partner", "name"]):
        p2p_names[m["chat_id"]] = nested(m, ["chat_partner", "name"])

rows = []
self_messages = []
for f in as_list(nested(manifest, ["files", "im_self"], [])):
    j = load_json(f)
    self_messages.extend(nested(j, ["data", "messages"], []) or [])

groups = {}
for m in self_messages:
    content = str(m.get("content") or "").strip()
    if m.get("msg_type") == "image" or not content:
        continue
    groups.setdefault(chat_label(m, p2p_names), []).append(m)
for name, messages in sorted(groups.items(), key=lambda kv: len(kv[1]), reverse=True)[:30]:
    sample = " / ".join(clean_cell(m.get("content"))[:80] for m in messages[:3])
    rows.append([name, "飞书本人消息", f"本人发言 {len(messages)} 条；样例：{sample}", "待纳入判断", "满足本人相关的最低证据，但仍需判断是否为工作事项、是否有产出或后续责任。"])

for f in as_list(nested(manifest, ["files", "vc_meeting_details"], [])):
    j = load_json(f)
    data = j.get("data") or {}
    topic = next((data.get(k) for k in ["topic", "title", "meeting_topic", "name"] if data.get(k)), "飞书会议")
    evidence = []
    for label, keys in [("组织者", ["organizer_name", "owner_name", "host_name"]), ("时间", ["start_time", "start_time_iso", "meeting_start_time"])]:
        value = next((data.get(k) for k in keys if data.get(k)), None)
        if value:
            evidence.append(f"{label}：{value}")
    if data.get("meeting_id"):
        evidence.append("meeting_id 已采集")
    rows.append([topic, "飞书会议", "；".join(evidence), "待纳入判断", "会议是高价值证据源；应优先结合会议纪要、妙记和相关文档判断是否纳入。"])

for f in as_list(nested(manifest, ["files", "docs"], [])):
    j = load_json(f)
    for result in (nested(j, ["data", "results"], []) or [])[:20]:
        meta = result.get("result_meta") or {}
        title = result.get("title_highlighted") or meta.get("title") or "飞书文档"
        evidence = []
        if result.get("entity_type"):
            evidence.append("类型：" + str(result["entity_type"]))
        if meta.get("owner_name"):
            evidence.append("所有者：" + str(meta["owner_name"]))
        if meta.get("last_open_time_iso"):
            evidence.append("最近打开：" + str(meta["last_open_time_iso"]))
        rows.append([title, "飞书文档", "；".join(evidence), "待纳入判断", "文档打开或编辑本身不是结论，但若标题和时间与当天主线一致，应优先纳入候选审查。"])

for p in agent.get("project_candidates", []):
    status = p.get("status")
    recommendation = "待纳入判断" if status == "has_today_files" else "默认不纳入"
    reason = {
        "has_today_files": "有当天本地文件证据；需结合会话与产物判断是否为正式工作包。",
        "project_timestamp_only": "仅目录时间变化，缺少产物证据。",
        "important_project_no_today_evidence": "历史重点项目，但无当天证据。",
    }.get(status, "缺少当天证据。")
    rows.append([p.get("name"), "本地项目", f"{p.get('path')}；状态：{status}；当天文件数：{len(p.get('recent_files') or [])}", recommendation, reason])

for s in agent.get("codex_sessions", [])[:40]:
    name = s.get("thread_name") or s.get("path") or s.get("id")
    rows.append([name, "Codex 会话", "更新时间：" + str(s.get("updated_at")), "待纳入判断", "需要读取会话摘要和产物；不能只因会话存在就写入日报。"])

lines = [
    f"## 日报候选事项审查（{date}）",
    "",
    "### 数据覆盖",
    "",
    f"- 日历：{nested(manifest, ['counts', 'calendar'], 0)}",
    f"- 视频会议：{nested(manifest, ['counts', 'vc'], 0)}",
    f"- 群聊全量：{nested(manifest, ['counts', 'im_all'], 0)}",
    f"- 本人发言：{nested(manifest, ['counts', 'im_self'], 0)}",
    f"- 云文档：{nested(manifest, ['counts', 'docs'], 0)}",
]
if manifest.get("errors"):
    lines.append(f"- 采集错误：{len(manifest['errors'])} 项，见 source_manifest.json")
lines += [
    "",
    "### 纳入门槛",
    "",
    "- 纳入日报必须有本人主导、本人明确推进、本人实际产出或本人后续责任。",
    "- 全量群聊只作为上下文；无本人相关证据时默认不纳入。",
    "- 日报自动化普通采集/创建文档不作为业务工作项。",
    "",
    "### 候选审查表",
    "",
    "| 候选事项 | 来源类型 | 本人相关证据 | 建议 | 未纳入/待确认原因 |",
    "| --- | --- | --- | --- | --- |",
]
if rows:
    for row in rows:
        lines.append("| " + " | ".join(clean_cell(c) for c in row) + " |")
else:
    lines.append("| 无 | - | - | 默认不纳入 | 未发现候选事项。 |")

Path(out_file).parent.mkdir(parents=True, exist_ok=True)
Path(out_file).write_text("\n".join(lines), encoding="utf-8")
print(out_file)
PY
