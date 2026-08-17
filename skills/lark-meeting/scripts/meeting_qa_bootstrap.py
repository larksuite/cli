#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
# -----------------------------------------------------------------------------
# meeting_qa_bootstrap.py
#
# 设计目标（会议问答加速入口，一次调用完成四件事）：
#   1. 会议列表探测：同时查询「进行中会议」+「今天已结束会议」+「今日日程」，不做互斥判定；
#   2. 轻量单页（绝不 --page-all / 不翻页取全）：
#      - 进行中会议：为每场进行中会议拉取一页事件（pretty 格式，page-size 可控），
#        直接展示最新事件内容；
#      - 今天已结束会议：只取 +search 单页（page-size 可控）作候选列表，
#        不把当天所有会议翻页拉全（避免无效候选灌入上下文）；
#      - 今日日程（未来会议）：通过 calendar +agenda 获取当天日程，仅保留
#        未来时间的会议，覆盖 vc +search 无法搜到的未来会议；
#   3. 简洁命令引导：不读取、不内联任何 skill/reference 文件内容（避免多次
#      执行或上下文已有 skills 时重复灌入），只给出可直接复制执行的命令模板
#      和关键参数说明，agent 按需 Read reference 获取详细字段说明。
#
# 职责边界（重要）：
#   - bootstrap **不锁定任何会议**：即便只有一场进行中会议，也以列表形式展示，
#     同时拉取一页事件内容供快速浏览；
#   - bootstrap 只取单页内容：进行中会议取一页事件、已结束会议取一页列表，
#     不翻页取全；
#   - 更多内容拉取（翻页、+detail / +recording 等）由 agent 读完本脚本输出后
#     按需自行调用 lark-cli 完成，建议使用 --page-size 控制单页大小，
#     用 --page-token 做增量拉取。
#
# 为什么做成脚本：lark-meeting/SKILL.md 里「未给 meeting_id 时查进行中+查当天结束」
#   这条路由是**确定性逻辑**，却每次都要 agent 用自然语言重推一遍。把它下沉
#   成脚本，等价于把当前真实的会议列表一次性喂给 agent，省掉多次工具往返。
#
# Agent 标准调用（零参数，不接受任何 flag，「今天」取系统当前日期）：
#   python3 skills/lark-meeting/scripts/meeting_qa_bootstrap.py
#
# 契约：
#   - stdout 只装结构化 Markdown 简报（「## 会议问答上下文」开头），agent 直接消费；
#   - 所有进度/降级原因走 stderr，绝不污染 stdout；
#   - lark-cli 失败时 returncode 仍可能为 0，成败一律解析顶层 `ok` 字段，不看退出码；
#   - 不读取、不内联 SKILL.md 或 references/ 下任何文件，避免内容重复灌入上下文。
#
# Exit codes:
#   0  正常产出（含「暂无可答会议」—— 无会可答不是错误，是一种正常结论）
#   1  运行异常（lark-cli 缺失、JSON 解析失败等无法产出简报的情况）
# -----------------------------------------------------------------------------

from __future__ import annotations

import json
import re
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import date, datetime, timezone
from typing import Any, Optional

# --- 分页控制 ----------------------------------------------------------------
# bootstrap 内部预取：事件预览用最小 page-size 只拉个开头；会议列表保持合理数量供选择
_SEARCH_PAGE_SIZE = "10"   # +search page-size 1-30，候选会议列 10 场足够选择
_EVENTS_PAGE_SIZE = "20"   # +meeting-events page-size 最小 20，预览只拉开头


# ---- 基础设施 ----------------------------------------------------------------
def log(msg: str) -> None:
    """说明性文字统一走 stderr，永远不污染 stdout 的 Markdown 简报。"""
    print(msg, file=sys.stderr, flush=True)


def run_lark_json(args: list[str]) -> Optional[dict[str, Any]]:
    """
    执行一条 lark-cli 命令并解析其 JSON 输出。
    lark-cli 在出错（returncode ≠ 0）时 JSON 会写到 stderr，所以需要同时捕获 stdout 和 stderr，
    优先从 stdout 解析；若 stdout 为空或解析失败，再尝试 stderr。
    返回解析后的 dict；命令缺失 / 输出均非 JSON 返回 None（由调用方决定降级）。
    注意：lark-cli 部分命令成功时 returncode 也为 0，成败请由调用方读顶层 `ok`。
    """
    try:
        proc = subprocess.run(
            ["lark-cli", *args],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            check=False,
        )
    except FileNotFoundError:
        log("[fatal] lark-cli 未找到（PATH 未安装？），无法采集会议上下文。")
        return None
    except Exception as exc:
        log(f"[error] lark-cli 调用异常 type={type(exc).__name__} msg={exc!r}")
        return None

    # 优先解析 stdout；stdout 为空或解析失败时尝试 stderr（错误响应常走 stderr）
    for raw in (proc.stdout or "", proc.stderr or ""):
        if not raw.strip():
            continue
        try:
            return json.loads(raw)
        except json.JSONDecodeError:
            continue

    # 两种输出都解析不了，记录错误日志（截断避免过长）
    combined = (proc.stdout or "") + (proc.stderr or "")
    preview = combined[:300].replace("\n", " ")
    log(f"[warn] lark-cli 输出非 JSON（args={' '.join(args)}），预览：{preview}")
    return None


def run_lark_text(args: list[str]) -> Optional[str]:
    """
    执行一条 lark-cli 命令并返回原始 stdout 文本（用于 pretty 格式输出）。
    返回文本内容；命令缺失返回 None（由调用方决定降级）。
    注意：pretty 格式不解析 ok 字段，失败时内容可能为空或包含错误信息。
    """
    try:
        proc = subprocess.run(
            ["lark-cli", *args],
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            text=True,
            check=False,
        )
    except FileNotFoundError:
        log("[fatal] lark-cli 未找到（PATH 未安装？），无法采集会议上下文。")
        return None
    except Exception as exc:
        log(f"[error] lark-cli 调用异常 type={type(exc).__name__} msg={exc!r}")
        return None
    return proc.stdout or ""


# ---- 数据采集 ----------------------------------------------------------------
def fetch_active_meetings() -> Optional[list[dict[str, Any]]]:
    """
    查询进行中会议。返回会议列表；采集失败返回 None（区别于「空列表」的正常态）。
    字段实测：data.meetings[] 每项含 meeting_id / meeting_no / meeting_title。
    """
    data = run_lark_json(["vc", "+meeting-list-active", "--as", "user", "--format", "json"])
    if data is None:
        return None
    if not data.get("ok"):
        log(f"[warn] +meeting-list-active 返回 ok=false: {data.get('error')}")
        return None
    return data.get("data", {}).get("meetings", []) or []


def fetch_ended_meetings_today(today: str) -> tuple[Optional[list[dict[str, Any]]], bool, str]:
    """
    只取今天已结束会议的 +search **单页**作候选引导——bootstrap 不负责翻页取全。
    使用 --page-size 控制单页大小，足够 agent 判断「今天开过哪些会」并挑一场继续；
    若 has_more，page_token 带回供 agent 后续翻页。
    字段实测：data.items[] 每项含 id / display_info / meta_data；透传 display_info。
    返回 (items, has_more, page_token)；采集失败返回 (None, False, "")。
    """
    data = run_lark_json([
        "vc", "+search",
        "--start", today,
        "--end", today,
        "--page-size", _SEARCH_PAGE_SIZE,
        "--format", "json",
    ])
    if data is None:
        return None, False, ""
    if not data.get("ok"):
        log(f"[warn] +search 返回 ok=false: {data.get('error')}")
        return None, False, ""
    block = data.get("data", {})
    items = block.get("items", []) or []
    has_more = bool(block.get("has_more"))
    page_token = block.get("page_token", "") or ""
    return items, has_more, page_token


def fetch_calendar_agenda(today: str) -> tuple[Optional[list[dict[str, Any]]], Optional[str]]:
    """
    获取今日日程中未来时间的会议（calendar +agenda），覆盖 vc +search 无法搜到的未来会议。
    vc +search 只能搜到已结束且用户参与过的会议，calendar +agenda 可补全未来会议场景。
    仅返回 start_time 在当前时间之后的日程（未来会议），已过期的日程由 vc +search 覆盖。
    返回 (events, scope_hint)；scope_hint 不为空表示缺少权限，调用方应提示用户授权。
    字段实测：data.events[] 每项含 event_id / summary / start_time / end_time / status 等。
    """
    data = run_lark_json([
        "calendar", "+agenda",
        "--start", today,
        "--end", today,
        "--format", "json",
    ])
    if data is None:
        return None, None
    if not data.get("ok"):
        err = data.get("error", {})
        if isinstance(err, dict) and err.get("subtype") == "missing_scope":
            scopes = err.get("missing_scopes", [])
            scopes_str = " ".join(scopes)
            hint = (
                f"lark-cli auth login --scope \"{scopes_str}\" --no-wait --json"
            )
            log(f"[warn] calendar +agenda 缺少 scope: {scopes_str}，需授权后重试。")
            return None, hint
        log(f"[warn] calendar +agenda 返回 ok=false: {data.get('error')}")
        return None, None
    all_events = data.get("data", []) or []

    # 仅保留未来时间的日程
    # start_time 是嵌套对象 {"datetime": "...", "timezone": "..."}
    now = datetime.now(timezone.utc)
    future: list[dict[str, Any]] = []
    for ev in all_events:
        if not isinstance(ev, dict):
            continue
        start_obj = ev.get("start_time", {})
        start_str = start_obj.get("datetime", "") if isinstance(start_obj, dict) else ""
        if not start_str:
            future.append(ev)  # 无时间信息的日程保守保留
            continue
        try:
            # 尝试解析 ISO 8601 格式
            start_dt = datetime.fromisoformat(start_str)
            if start_dt > now:
                future.append(ev)
        except (ValueError, TypeError):
            future.append(ev)  # 解析失败保守保留
    return future, None


def _parse_page_token_from_text(text: str) -> str:
    """从 pretty 格式输出中提取 page_token（pretty 末尾会打印 `page_token: <token>`）。"""
    if not text:
        return ""
    m = re.search(r"^page_token:\s*(\S+)", text, re.MULTILINE)
    return m.group(1) if m else ""


def fetch_events_page(meeting_id: str) -> tuple[Optional[str], str]:
    """
    拉取一场会议的一页事件：返回 (pretty_text, page_token)。
    只调用一次 pretty 格式，从输出末尾解析 page_token，避免二次 JSON 调用。
    失败时 (None, "")。
    """
    text = run_lark_text([
        "vc", "+meeting-events",
        "--as", "user",
        "--meeting-id", meeting_id,
        "--format", "pretty",
        "--page-size", _EVENTS_PAGE_SIZE,
    ])
    page_token = _parse_page_token_from_text(text or "")
    return text, page_token


def _fetch_content_for_meeting(meeting_id: str) -> dict[str, str]:
    """
    预取一场会议的所有内容 token：vc +detail → 并行(note +detail, minutes minutes get)。
    返回 note_id / minute_token / note_doc_token / verbatim_doc_token / minute_title / minute_url。
    失败的字段留空字符串。
    """

    def _note_tokens(note_id: str) -> tuple[str, str]:
        if not note_id:
            return "", ""
        detail = run_lark_json(["note", "+detail", "--note-id", note_id, "--format", "json"])
        if detail and detail.get("ok"):
            note = (detail.get("data") or {}).get("note") or {}
            return note.get("note_doc_token", "") or "", note.get("verbatim_doc_token", "") or ""
        return "", ""

    def _minute_info(minute_token: str) -> tuple[str, str, str]:
        if not minute_token:
            return "", "", ""
        info = run_lark_json([
            "minutes", "minutes", "get",
            "--params", f'{{"minute_token":"{minute_token}"}}',
            "--format", "json",
        ])
        if info and info.get("ok"):
            mi = (info.get("data") or {}).get("minute") or {}
            return mi.get("title", "") or "", mi.get("url", "") or "", ""
        # 失败时返回错误信息
        err = ""
        if info:
            e = info.get("error") or {}
            if isinstance(e, dict):
                err = e.get("hint") or e.get("message") or ""
                if not err:
                    err = json.dumps(e, ensure_ascii=False)
        return "", "", err

    tokens: dict[str, str] = {
        "note_id": "", "minute_token": "",
        "note_doc_token": "", "verbatim_doc_token": "",
        "minute_title": "", "minute_url": "", "minute_error": "",
    }
    # 1) vc +detail 拿 note_id / minute_token
    detail = run_lark_json([
        "vc", "+detail",
        "--meeting-ids", meeting_id,
        "--format", "json",
    ])
    if not detail or not detail.get("ok"):
        return tokens
    meetings = (detail.get("data") or {}).get("meetings") or []
    if not meetings:
        return tokens
    m = meetings[0]
    tokens["note_id"] = m.get("note_id", "") or ""
    tokens["minute_token"] = m.get("minute_token", "") or ""

    # 2) 并行：note +detail 和 minutes minutes get
    with ThreadPoolExecutor(max_workers=2) as pool:
        fut_note = pool.submit(_note_tokens, tokens["note_id"])
        fut_minute = pool.submit(_minute_info, tokens["minute_token"])
    tokens["note_doc_token"], tokens["verbatim_doc_token"] = fut_note.result()
    mt, mu, me = fut_minute.result()
    tokens["minute_title"] = mt
    tokens["minute_url"] = mu
    tokens["minute_error"] = me
    return tokens


def _render_active_meeting_item(
    idx: int,
    m: dict[str, Any],
    events_text: str,
    page_token: str,
) -> list[str]:
    """渲染单场进行中会议：基础信息 + 事件预览 + 共享文档提示 + 可直接执行的拉取命令。"""
    mid = m.get("meeting_id", "")
    title = m.get("meeting_title", "(无标题)")
    meeting_no = m.get("meeting_no", "")
    lines: list[str] = [
        f"{idx}. **{title}**",
        f"   - 会议 ID：`{mid}`",
        f"   - 会议号：{meeting_no}",
        "",
    ]
    if events_text and events_text.strip():
        lines += [
            f"   第一页 {_EVENTS_PAGE_SIZE} 条事件（预览）：",
            "",
            "   ```",
        ]
        for line in events_text.strip().splitlines():
            lines.append(f"   {line}")
        lines += ["   ```", ""]
    lines += [
        "   如果会中共享了文档，可以用以下命令拉取内容：",
        "   > `--doc-format markdown` 配合 `--format pretty` 输出更短、读起来更快",
        "",
        "   ```bash",
        f"   lark-cli docs +fetch --doc <doc_token / url> --doc-format markdown --format pretty",
        "   ```",
        "",
    ]
    # 直接给出可执行命令（参数已填充）
    lines += [
        "   ```bash",
        "   # 首次全量拉取",
        f"   lark-cli vc +meeting-events --as user --meeting-id {mid} --format pretty --page-all",
    ]
    if page_token:
        lines += [
            "",
            "   # 后续增量拉取（基于已保存的 page_token）",
            f"   lark-cli vc +meeting-events --as user --meeting-id {mid} --format pretty --page-all --page-token {page_token}",
        ]
    lines += ["   ```", ""]
    return lines


def _render_ended_meeting_item(
    idx: int,
    it: dict[str, Any],
    tokens: dict[str, str],
) -> list[str]:
    """渲染单场已结束会议：基础信息 + 直接可执行的最终命令（token 已填充）。"""
    mid = it.get("id", "")
    display_info = " ".join(str(it.get("display_info", "")).split())
    note_doc = tokens.get("note_doc_token", "")
    verbatim_doc = tokens.get("verbatim_doc_token", "")
    minute_token = tokens.get("minute_token", "")
    minute_title = tokens.get("minute_title", "")
    minute_url = tokens.get("minute_url", "")
    note_id = tokens.get("note_id", "")

    lines: list[str] = [
        f"{idx}. 会议 ID：`{mid}`",
        f"   - {display_info}",
        "",
    ]
    # 直接给出可执行命令；收集所有命令行后统一处理空行
    # --doc-format markdown 输出更短（无 XML blockId），--format pretty 也更短
    cmd_lines: list[str] = []
    if note_doc:
        cmd_lines.append(f"# 拉取智能纪要正文")
        cmd_lines.append(f"lark-cli docs +fetch --doc {note_doc} --doc-format markdown --format pretty")
    if verbatim_doc:
        cmd_lines.append(f"# 拉取逐字稿正文")
        cmd_lines.append(f"lark-cli docs +fetch --doc {verbatim_doc} --doc-format markdown --format pretty")
    if minute_token:
        if minute_title and minute_url:
            cmd_lines.append(f"# 妙记：[{minute_title}]({minute_url})")
        elif tokens.get("minute_error"):
            cmd_lines.append(f"# 妙记：{tokens['minute_error']}")
        else:
            cmd_lines.append(f"# 妙记基础信息（标题/时长/所有者/链接）")
            cmd_lines.append(f"lark-cli minutes minutes get --params '{{\"minute_token\":\"{minute_token}\"}}'")

    if cmd_lines:
        lines.append("   ```bash")
        for cl in cmd_lines:
            lines.append(f"   {cl}")
        lines.append("   ```")
        lines.append("")
    else:
        lines += [
            "   > 该会议暂无纪要产物（无智能纪要、逐字稿、妙记）。",
            "   > 如需查看参会人快照：`lark-cli vc meeting get --params '{\"meeting_id\":\"" + mid + "\",\"with_participants\":true}'`",
            "",
        ]
    return lines


def _render_calendar_event_item(idx: int, ev: dict[str, Any]) -> list[str]:
    """渲染单条未来日程：标题、时间、状态，以及如何获取关联会议。"""
    event_id = ev.get("event_id", "")
    summary = ev.get("summary", "(无标题)")
    start_obj = ev.get("start_time", {})
    end_obj = ev.get("end_time", {})
    start_time = start_obj.get("datetime", "") if isinstance(start_obj, dict) else ""
    end_time = end_obj.get("datetime", "") if isinstance(end_obj, dict) else ""
    status = ev.get("status", "") or ev.get("self_rsvp_status", "")

    # 时间显示：从 ISO 8601 "2026-08-17T17:00:00+08:00" 中提取 HH:MM
    start_short = start_time[11:16] if len(start_time) >= 16 and "T" in start_time else start_time
    end_short = end_time[11:16] if len(end_time) >= 16 and "T" in end_time else end_time

    status_label = {"confirmed": "已确认", "tentative": "待定", "cancelled": "已取消",
                    "accept": "已接受", "decline": "已拒绝",
                    "needs_action": "待回复", "remove": "已移除"}.get(status, status)

    lines: list[str] = [
        f"{idx}. **{summary}**",
        f"   - 日程 ID：`{event_id}`",
        f"   - 时间：{start_short} ~ {end_short}",
        f"   - 状态：{status_label}",
        "",
    ]
    return lines


def _guide_post_meeting_section(has_more: bool, page_token: str) -> list[str]:
    """会后章节末尾补充说明。"""
    lines: list[str] = []
    if has_more and page_token:
        lines += [
            f"> 今天还有更多会议未列出，翻页：`lark-cli vc +search --page-token {page_token} --format pretty`",
            "",
        ]
    lines += [
        "> 上面命令中的 doc_token / note_id 已预取填充，直接执行即可获取正文。",
    ]
    return lines


def render_meetings(
    today: str,
    active: list[dict[str, Any]],
    active_events: dict[str, str],
    active_page_tokens: dict[str, str],
    ended: list[dict[str, Any]],
    ended_tokens: dict[str, dict[str, str]],
    ended_has_more: bool,
    ended_page_token: str,
    calendar_events: list[dict[str, Any]],
    scope_hint: str = "",
) -> str:
    """
    统一渲染：进行中会议（含一页 pretty 事件预览）+ 今天已结束会议（含可直接执行的最终命令）+ 未来日程。
    ended_tokens: {meeting_id: {note_id, minute_token, note_doc_token, verbatim_doc_token}}
    scope_hint: 不为空时在末尾追加授权提示。
    """
    parts = ["## 会议问答上下文", ""]

    has_active = bool(active)
    has_ended = bool(ended)
    has_calendar = bool(calendar_events)
    if has_active and has_ended:
        scene = "当前有进行中会议（附第一页事件预览），同时列出今天已结束的会议"
    elif has_active:
        scene = "当前有进行中的会议（附第一页事件预览）"
    else:
        scene = "当前无进行中会议，以下为今天已结束的会议"
    if has_calendar:
        scene += "以及今日未来日程"
    parts += [f"场景：{scene}", f"今天：{today}", ""]

    seq = 1

    if has_active:
        parts.append("### 进行中会议")
        parts.append("")
        for m in active:
            mid = m.get("meeting_id", "")
            parts += _render_active_meeting_item(
                seq, m,
                active_events.get(mid, ""),
                active_page_tokens.get(mid, ""),
            )
            seq += 1

    if has_ended:
        parts.append("### 今天已结束的会议")
        parts.append("")
        for it in ended:
            mid = it.get("id", "")
            parts += _render_ended_meeting_item(seq, it, ended_tokens.get(mid, {}))
            seq += 1
        parts += _guide_post_meeting_section(ended_has_more, ended_page_token)

    if has_calendar:
        parts.append("### 今日未来日程")
        parts.append("")
        parts.append("> 今天尚未开始的日程会议。")
        parts.append("")
        for ev in calendar_events:
            parts += _render_calendar_event_item(seq, ev)
            seq += 1

    parts += [
        "搜索其他日期会议：",
        "",
        "```bash",
        "lark-cli vc +search --start <YYYY-MM-DD> --end <YYYY-MM-DD> --format pretty",
        "```",
        "",
        "如需查看命令字段详情，可 Read references/ 下对应文档。",
    ]
    if scope_hint:
        parts += [
            "",
            "> 日程权限不足，如需获取未来日程请先授权：",
            f"> `{scope_hint}`",
            "> 执行后根据提示完成授权，再重新运行本脚本。",
        ]
    return "\n".join(parts)


def render_none(today: str, calendar_events: Optional[list[dict[str, Any]]] = None) -> str:
    """无会可答：不是错误，引导 agent 向用户补会议线索，不自行扩大时间范围。
    若有今日日程，即便无进行中/已结束会议，也展示日程列表。"""
    if calendar_events:
        # 有日程但无 VC 会议：用 render_meetings 统一渲染
        return render_meetings(today, [], {}, {}, [], {}, False, "", calendar_events)

    return "\n".join([
        "## 会议问答上下文",
        "",
        "场景：暂无可答会议（当前无进行中会议，今天也没有已结束会议和日程）",
        f"今天：{today}",
        "",
        "请向用户询问会议时间 / 主题 / 9 位会议号，不要自行扩大时间范围。",
    ])


# ---- 主流程 ------------------------------------------------------------------
def main() -> int:
    today = date.today().isoformat()
    log("==== 会议问答引导启动 ====")

    # ① 三路并行：进行中（采集+事件页）、已结束（采集+token预取）、未来日程（仅采集）
    #    每路内部有子任务时再并行，尽量减少总耗时。
    def _branch_active():
        active = fetch_active_meetings()
        if active is None:
            return None, {}, {}
        active_events: dict[str, str] = {}
        active_page_tokens: dict[str, str] = {}
        mids = [m.get("meeting_id", "") for m in active if m.get("meeting_id")]
        if mids:
            with ThreadPoolExecutor(max_workers=min(len(mids), 5)) as pool:
                futures = {pool.submit(fetch_events_page, mid): mid for mid in mids}
                for fut in as_completed(futures):
                    mid = futures[fut]
                    text, token = fut.result()
                    if text is not None:
                        active_events[mid] = text
                    if token:
                        active_page_tokens[mid] = token
        return active, active_events, active_page_tokens

    def _branch_ended():
        ended, has_more, page_token = fetch_ended_meetings_today(today)
        if ended is None:
            return None, False, "", {}
        mids = [it.get("id", "") for it in ended if it.get("id")]
        if not mids:
            return ended, has_more, page_token, {}

        ended_tokens: dict[str, dict[str, str]] = {}
        with ThreadPoolExecutor(max_workers=min(len(mids), 5)) as pool:
            futures = {pool.submit(_fetch_content_for_meeting, mid): mid for mid in mids}
            for fut in as_completed(futures):
                ended_tokens[futures[fut]] = fut.result()
        return ended, has_more, page_token, ended_tokens

    def _branch_calendar():
        return fetch_calendar_agenda(today)

    with ThreadPoolExecutor(max_workers=3) as pool:
        fut_active = pool.submit(_branch_active)
        fut_ended = pool.submit(_branch_ended)
        fut_calendar = pool.submit(_branch_calendar)

    # 收集并行结果
    active, active_events, active_page_tokens = fut_active.result()
    if active is None:
        log("[fatal] 进行中会议采集失败。")
        return 1

    ended, ended_has_more, ended_page_token, ended_tokens = fut_ended.result()
    if ended is None:
        log("[fatal] 已结束会议采集失败。")
        return 1

    calendar_events, scope_hint = fut_calendar.result()
    if calendar_events is None:
        calendar_events = []
        if scope_hint:
            log("[warn] 未来日程缺少 scope 权限，输出中附带授权提示。")
        else:
            log("[warn] 未来日程采集失败，继续输出已有结果。")

    # ② 汇总输出
    if not active and not ended and not calendar_events:
        sys.stdout.write(render_none(today) + "\n")
        return 0

    sys.stdout.write(
        render_meetings(today, active, active_events, active_page_tokens, ended, ended_tokens, ended_has_more, ended_page_token, calendar_events, scope_hint) + "\n"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
