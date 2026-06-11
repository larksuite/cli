// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Generates the umbrella skill `skills/lark/` from all sibling `lark-*` skills.
//
// Why an umbrella: agent harnesses (Claude Code, Codex, ...) keep every
// registered skill's frontmatter description resident in the model context.
// 26 lark-* skills cost 26 description entries; the umbrella collapses them
// into one routing skill whose domain guides are loaded on demand via Read.
//
// How it works:
//   skills/lark-<domain>/SKILL.md   -> skills/lark/references/subskills/lark-<domain>/GUIDE.md
//   skills/lark-<domain>/<anything> -> skills/lark/references/subskills/lark-<domain>/<anything>
//   skills/lark/SKILL.md            -> generated routing table (frontmatter descriptions verbatim)
//
// The mirror keeps every skill as a sibling directory, so existing cross-skill
// relative links (../lark-shared/SKILL.md, ../../lark-doc/references/x.md, ...)
// keep the exact same depth — the only rewrite needed is the SKILL.md ->
// GUIDE.md filename swap. SKILL.md is renamed so nested copies are never
// re-registered as standalone skills by harnesses that scan recursively.
//
// Run from anywhere: node scripts/skill-umbrella-sync/index.js
// Idempotent: wipes and regenerates skills/lark/references/subskills and
// skills/lark/SKILL.md on every run. Do not hand-edit generated files; edit
// the source lark-* skills (or this script's template) and re-run.

const fs = require('fs');
const path = require('path');

const SKILLS_DIR = path.resolve(__dirname, '../../skills');
const UMBRELLA_NAME = 'lark';
const UMBRELLA_DIR = path.join(SKILLS_DIR, UMBRELLA_NAME);
const SUBSKILLS_DIR = path.join(UMBRELLA_DIR, 'references', 'subskills');

// Rewrite cross-skill / self references to the mirrored filename. Paths
// prefixed with ../ or lark-<x>/ are always rewritten; bare "SKILL.md"
// mentions (prose like "见 SKILL.md「xx」" pointing at the skill's own doc)
// are rewritten too, EXCEPT in lark-skill-maker — it teaches authoring new
// standalone skills and its literal SKILL.md prose must survive mirroring.
const SKILL_MD_REF = /((?:\.\.\/)+(?:lark-[a-z0-9-]+\/)?|lark-[a-z0-9-]+\/)SKILL\.md/g;
const BARE_SKILL_MD = /\bSKILL\.md\b/g;
const BARE_REWRITE_EXEMPT = new Set(['lark-skill-maker']);

function listSourceSkills() {
  return fs
    .readdirSync(SKILLS_DIR, { withFileTypes: true })
    .filter(e => e.isDirectory() && e.name.startsWith('lark-'))
    .map(e => e.name)
    .sort();
}

function readFrontmatter(skill) {
  const file = path.join(SKILLS_DIR, skill, 'SKILL.md');
  const content = fs.readFileSync(file, 'utf-8').replace(/\r\n/g, '\n');
  const fm = content.match(/^---\n([\s\S]*?)\n---(?:\n|$)/);
  if (!fm) throw new Error(`${skill}: SKILL.md has no YAML frontmatter`);
  const lines = fm[1].split('\n');
  const idx = lines.findIndex(l => /^description:/.test(l));
  if (idx === -1) throw new Error(`${skill}: frontmatter missing description`);
  // Two formats exist in this repo: double-quoted single line, and YAML
  // folded scalar (`description: >` + indented lines, e.g. lark-whiteboard).
  const inline = lines[idx].match(/^description:\s*"(.*)"\s*$/);
  if (inline) return { description: inline[1].replace(/\\"/g, '"') };
  if (/^description:\s*[>|][+-]?\s*$/.test(lines[idx])) {
    const collected = [];
    for (let i = idx + 1; i < lines.length && /^\s+\S/.test(lines[i]); i++) {
      collected.push(lines[i].trim());
    }
    if (collected.length > 0) return { description: collected.join(' ') };
  }
  throw new Error(`${skill}: unsupported description format in frontmatter`);
}

function rewriteMd(content, skill) {
  let out = content.replace(SKILL_MD_REF, '$1GUIDE.md');
  if (!BARE_REWRITE_EXEMPT.has(skill)) out = out.replace(BARE_SKILL_MD, 'GUIDE.md');
  return out;
}

function mirrorSkill(skill) {
  const src = path.join(SKILLS_DIR, skill);
  const dst = path.join(SUBSKILLS_DIR, skill);

  const walk = (rel) => {
    const srcPath = path.join(src, rel);
    for (const entry of fs.readdirSync(srcPath, { withFileTypes: true })) {
      const entryRel = path.join(rel, entry.name);
      if (entry.isDirectory()) {
        fs.mkdirSync(path.join(dst, entryRel), { recursive: true });
        walk(entryRel);
        continue;
      }
      // Top-level SKILL.md becomes GUIDE.md; everything else keeps its name.
      const outRel = entryRel === 'SKILL.md' ? 'GUIDE.md' : entryRel;
      const outPath = path.join(dst, outRel);
      if (entry.name.endsWith('.md')) {
        fs.writeFileSync(outPath, rewriteMd(fs.readFileSync(path.join(src, entryRel), 'utf-8'), skill));
      } else {
        fs.copyFileSync(path.join(src, entryRel), outPath);
      }
    }
  };

  fs.mkdirSync(dst, { recursive: true });
  walk('');
}

function buildUmbrellaSkillMd(skills) {
  const routed = skills.filter(s => s !== 'lark-shared');
  const routeLines = routed
    .map(s => `- [\`${s}\`](references/subskills/${s}/GUIDE.md) — ${readFrontmatter(s).description}`)
    .join('\n');

  return `---
name: ${UMBRELLA_NAME}
version: 1.0.0
description: "飞书/Lark 统一入口 skill（伞形路由）：覆盖云文档、电子表格、多维表格/Base、知识库 Wiki、云盘/云空间、邮件、即时消息与群聊、日历与会议室、视频会议、妙记、任务待办、审批、考勤打卡、OKR、通讯录、画板、幻灯片、Markdown、妙搭应用开发、实时事件订阅、原生 OpenAPI 探索等全部飞书能力。凡涉及飞书/Lark/feishu/larkoffice/larksuite（以及 doubao.com 的 docx/sheets/wiki/base 链接）的 URL 或 token，或要对上述对象做查看、创建、编辑、发送、搜索、下载、订阅等操作时使用。用法：先按本 skill 路由表选定领域，再 Read references/subskills/<域>/GUIDE.md 按指引执行。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli --help"
---

# lark — 飞书/Lark 统一入口

本 skill 将所有 lark-* 领域 skill 收敛为一个入口。各领域完整说明在 \`references/subskills/<域>/GUIDE.md\`，按需读取；不要凭记忆直接执行 lark-cli 命令。

## 使用方式（CRITICAL）

1. **MUST 先用 Read 工具读取** [\`references/subskills/lark-shared/GUIDE.md\`](references/subskills/lark-shared/GUIDE.md) — 认证、身份选择（\`--as user/bot\`）、权限不足处理、安全规则、高风险操作确认协议，所有领域通用。
2. 按下方路由表选定领域，**Read 对应 GUIDE.md**，严格遵循其中的前置条件与指引；GUIDE.md 内部引用的 references/ 等文件，路径相对该 GUIDE.md 所在目录解析。
3. 任务跨领域时（如文档里嵌电子表格、消息里发云盘文件），按 GUIDE.md 内的跨域提示切换到对应领域 GUIDE.md；所有领域都在 \`references/subskills/\` 下互为兄弟目录。
4. 拿不准走哪个领域时，对照路由表各条目 description 末尾的"不负责…走 xx"提示做排除。

## 路由表

${routeLines}
`;
}

function verifyRelativeLinks() {
  // Best-effort guard: every relative markdown link inside the generated
  // umbrella must resolve to a real file. Broken links inherited from source
  // skills are reported but do not fail the build.
  const broken = [];
  const walk = (dir) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const p = path.join(dir, entry.name);
      if (entry.isDirectory()) { walk(p); continue; }
      if (!entry.name.endsWith('.md')) continue;
      const content = fs.readFileSync(p, 'utf-8');
      for (const m of content.matchAll(/\]\(([^)\s#]+)(?:#[^)]*)?\)/g)) {
        const target = m[1];
        if (/^[a-z]+:/i.test(target)) continue; // http(s), mailto, ...
        if (!fs.existsSync(path.resolve(path.dirname(p), target))) {
          broken.push(`${path.relative(UMBRELLA_DIR, p)} -> ${target}`);
        }
      }
    }
  };
  walk(UMBRELLA_DIR);
  return broken;
}

function main() {
  const skills = listSourceSkills();
  if (skills.length === 0) {
    console.error('No lark-* skills found under', SKILLS_DIR);
    process.exit(1);
  }

  fs.rmSync(SUBSKILLS_DIR, { recursive: true, force: true });
  fs.mkdirSync(SUBSKILLS_DIR, { recursive: true });

  for (const skill of skills) mirrorSkill(skill);
  fs.writeFileSync(path.join(UMBRELLA_DIR, 'SKILL.md'), buildUmbrellaSkillMd(skills));

  console.log(`✅ Generated ${UMBRELLA_DIR.replace(process.cwd() + path.sep, '')}: ${skills.length} subskills mirrored`);

  const broken = verifyRelativeLinks();
  if (broken.length > 0) {
    console.warn(`⚠️  ${broken.length} unresolved relative link(s) in generated tree:`);
    for (const b of broken) console.warn('   ', b);
  }
}

main();
