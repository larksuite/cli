---
name: setup-lark-skills
version: 1.0.0
description: "配置飞书 skills 的激活状态：把低频 lark-* 转为 user-invoked 休眠以节省 system prompt，或恢复为自动注入。首次使用或需要调整时输入 /skill:setup-lark-skills。"
disable-model-invocation: true
metadata:
  role: setup
---

# Setup Lark Skills

默认状态：27 个 lark-* skill 全部自动注入（model-invoked），每条 description 常驻 system prompt（实测 27 条合计约 11.8KB，单条平均约 440B）。本 skill 的作用是把**低频**的 lark-* 转为 user-invoked 休眠——休眠后 description 不再注入 system prompt，需要时输入 `/skill:lark-<name>` 临时加载（本次会话有效）；也可以随时删除该行恢复激活。

`disable-model-invocation` 是 Agent Skills 标准的可选 frontmatter 字段；不识别它的 harness 会忽略该行（无害，skill 保持激活）。

## 配置流程

### 1. 定位已安装的 skill 文件

操作对象是用户机器上**已安装**的 skill 文件，安装路径因 harness 而异。以下探测必须定位到**唯一**安装根目录才继续——找不到或有多个匹配时直接报错退出，不要在不确定的位置上执行后续命令：

```bash
# 常见 harness 的 skill 安装目录
hits=()
for d in ~/.claude/skills ~/.agents/skills ~/.codex/skills ~/.cursor/skills; do
  [ -f "$d/lark-im/SKILL.md" ] && hits+=("$d")
done
# 候选都找不到时兜底（范围较大，可能较慢）
if [ ${#hits[@]} -eq 0 ]; then
  while IFS= read -r f; do
    hits+=("$(dirname "$(dirname "$f")")")
  done < <(find ~ -maxdepth 5 -path '*skills/lark-im/SKILL.md' 2>/dev/null | head -5)
fi
case ${#hits[@]} in
  0) echo "ERROR: 未找到已安装的 lark skills，请先安装" >&2; exit 1 ;;
  1) export ROOT="${hits[0]}"; echo "ROOT=$ROOT" ;;
  *) echo "ERROR: 发现多个安装位置，请手动指定 ROOT 后重试：" >&2
     printf '  %s\n' "${hits[@]}" >&2; exit 1 ;;
esac
```

下文统一用 `$ROOT` 指代探测到的安装根目录。

### 2. 读取真实状态

唯一事实源是各 SKILL.md 的 frontmatter，**不是** `.active-profile`（后者仅是记录）。检测必须只匹配 frontmatter 块（第一个 `---` 到第二个 `---` 之间）内的字段——正文（如文档示例、代码块）里出现的同名文本不算。判定标准严格为「frontmatter 内恰好一行该字段且取值为 true」：手工改坏的重复/冲突字段（如同时存在 true 和 false，YAML 解析器可能按 false 生效）不视为休眠——扫描会显示为激活，重跑第 5 步的休眠命令即可将其规范化为单行 true：

```bash
# 只看 frontmatter 的休眠判断（后续步骤复用）：恰好一行字段且值为 true。
# 每行先去掉行尾 \r 再匹配（仓库 parser 明确支持 CRLF 文件），只读不写。
is_disabled() { awk '{l=$0; sub(/\r$/,"",l)} l=="---"{c++;next} c==1 && l ~ /^disable-model-invocation:[[:space:]]*/{t++; if (l ~ /:[[:space:]]*true[[:space:]]*$/) n++} END{exit !(t==1 && n==1)}' "$1"; }

# nullglob：部分安装时 glob 展开为空数组而不是残留字面 "*"；空集合直接报错退出
shopt -s nullglob
files=( "$ROOT"/lark-*/SKILL.md )
if [ ${#files[@]} -eq 0 ]; then
  echo "ERROR: $ROOT 下未找到任何 lark-*/SKILL.md（可能未完整安装），请先安装" >&2
  exit 1
fi
for f in "${files[@]}"; do
  n=$(basename "$(dirname "$f")")
  if is_disabled "$f"; then echo "$n: 休眠"; else echo "$n: 激活"; fi
done
```

### 3. 展示能力菜单

向用户展示以下分组，让用户勾选**日常高频、需要保持自动注入**的；未勾选的低频 skill 全部转为休眠：

**📋 核心办公（推荐保持激活）**
- [ ] lark-im — 消息/群聊/文件传输
- [ ] lark-doc — 云文档读写
- [ ] lark-calendar — 日程/会议室
- [ ] lark-task — 任务/待办

**📊 数据与文档**
- [ ] lark-sheets — 电子表格
- [ ] lark-base — 多维表格
- [ ] lark-drive — 云盘文件管理
- [ ] lark-wiki — 知识库
- [ ] lark-markdown — Markdown 文件
- [ ] lark-slides — 幻灯片
- [ ] lark-whiteboard — 画板

**🤝 协作与沟通**
- [ ] lark-approval — 审批
- [ ] lark-mail — 邮件
- [ ] lark-contact — 通讯录/查人
- [ ] lark-vc — 已结束会议记录查询
- [ ] lark-minutes — 妙记/音视频转写
- [ ] lark-vc-agent — 会中实时能力
- [ ] lark-note — 已知 note_id 纪要直查

**🏢 组织与效率**
- [ ] lark-okr — OKR 管理
- [ ] lark-attendance — 考勤打卡
- [ ] lark-apps — 妙搭应用开发

**🔧 开发者工具**
- [ ] lark-event — 实时事件监听
- [ ] lark-shared — 认证/权限管理
- [ ] lark-skill-maker — 创建新 skill
- [ ] lark-openapi-explorer — 探索原生 API

**🔄 工作流**
- [ ] lark-workflow-meeting-summary — 会议纪要汇总报告
- [ ] lark-workflow-standup-report — 日程待办摘要

### 4. 确认选择

展示用户的选择（保持激活的集合 + 将转休眠的集合），确认后再执行修改。

### 5. 执行休眠/激活（幂等）

编辑同样只作用于 frontmatter 块——休眠只在 frontmatter 内生效（已有该字段则改写为 `true`，没有才插入），激活只删 frontmatter 内的该字段，正文里的同名文本原样保留。写入走「`cp -a` 克隆元数据 → awk 改写 → mv 替换」，保留原文件的权限/ACL；任一步失败都会清理临时文件并报错退出：

```bash
f="$ROOT/lark-<name>/SKILL.md"

# 休眠（幂等：frontmatter 内已有 disable-model-invocation 字段则改写为 true，
# 没有则插在 name: 行之后，避开多行 description 陷阱；不会产生重复字段。
# 匹配用去掉行尾 \r 的副本，输出一律用原始行，新插入的行跟随该文件的行尾风格）
is_disabled "$f" || {
  cp -a "$f" "$f.tmp" &&
  awk '
    { cr = (/\r$/ ? "\r" : ""); l = $0; sub(/\r$/, "", l) }
    l == "---" { c++; print; next }
    c==1 && l ~ /^disable-model-invocation:[[:space:]]*/ { if (!d) { print "disable-model-invocation: true" cr; d=1 }; next }
    c==1 && l ~ /^name:/ && !d { print; print "disable-model-invocation: true" cr; d=1; next }
    { print }
  ' "$f" > "$f.tmp" &&
  mv "$f.tmp" "$f"
} || { rm -f "$f.tmp"; echo "ERROR: 休眠写入失败: $f" >&2; exit 1; }

# 激活（幂等：删除 frontmatter 内该字段的全部取值，正文同名文本保留，原始行尾原样输出）
cp -a "$f" "$f.tmp" &&
awk '
  { l = $0; sub(/\r$/, "", l) }
  l == "---" { c++; print; next }
  c==1 && l ~ /^disable-model-invocation:[[:space:]]*/ { next }
  { print }
' "$f" > "$f.tmp" &&
mv "$f.tmp" "$f" ||
{ rm -f "$f.tmp"; echo "ERROR: 激活写入失败: $f" >&2; exit 1; }
```

（awk 在 macOS / Linux 行为一致，无需区分平台；匹配前剥离行尾 `\r`，CRLF 格式的 SKILL.md 同样正确处理。）

### 6. 写入配置记录

将选择结果写入 `$ROOT/lark/.active-profile`，格式：

```
# Lark Skills Active Profile
# Updated: <timestamp>
lark-im
lark-doc
```

下次运行时以第 2 步的扫描结果为准，`.active-profile` 仅供人类参考。

## 预设方案

预设含义 = **保持激活**的集合，其余低频 skill 转为休眠：

| 预设 | 保持激活 |
|------|------|
| `minimal` | lark-im, lark-doc, lark-calendar |
| `office` | minimal + lark-task, lark-approval, lark-drive, lark-contact |
| `full` | 全部保持激活（默认状态，27 条 description 常驻，约 11.8KB） |
| `dev` | lark-shared, lark-event, lark-skill-maker, lark-openapi-explorer, lark-apps |

## 注意事项

- 修改后需**重启当前 agent 会话**才能生效（skill 列表一般在会话启动时加载）。
- 任何时候都可以用 `/skill:lark-<name>` 临时加载休眠的 skill（本次会话有效）。
- `lark`（路由）与本 skill 在仓库中即带 `disable-model-invocation: true`，永远保持 user-invoked，不占 system prompt；配置时不要改动这两个文件。
