---
name: lark-suite
version: 0.1.0
description: 飞书/Lark 聚合能力入口：管理飞书/Lark 产品能力（<!-- LARK_SUITE_KEYS -->等）。当 doubao.com 及其子域名承载飞书资源时也使用本入口，不要回退到 WebFetch。当用户需求涉及上述飞书业务域时使用。
metadata:
  requires:
    bins:
      - lark-cli
---

# Lark Suite

你是飞书/Lark 能力的聚合路由层。你的职责是先判断用户要使用哪个 `lark-*` 子能力，再读取并遵循对应子能力的说明。

`lark-suite` 不直接承载具体 API 操作步骤。除非对应子能力已被读取，否则不要仅根据本文件拼命令、猜参数或执行复杂操作。

所有子能力统一收纳在当前 skill 的 `references/` 目录。选择 `lark-foo` 后，直接读取 `references/lark-foo/SKILL.md`；不要再次调用 `Skill(lark-foo)`，也不要使用 Find/Glob 遍历或探测整个 references 目录。

## 使用流程

1. 根据用户意图从下方路由表选择一个或多个子能力；即使用户尚未提供链接、ID 或具体工作表，也先选择能力，再由子能力询问缺失信息。
2. 直接读取 `references/<skill-name>/SKILL.md` 加载所选子能力，不要把收纳后的子能力当作独立 skill 再次调用。
3. 仅使用本文件列出的路由与对应子能力入口，不要遍历或探测其他技能目录。
4. 如果目标能力未列出，返回无法路由的明确提示。
5. 仅读取当前已选子能力明确要求的前置文件。
6. 按目标子能力的说明执行；认证、租户、身份、权限和通用排障优先遵循 `lark-shared`。

多步任务可以组合多个子能力，但每一步都应由具体子能力驱动。例如“查联系人并发消息”先用 `lark-contact` 解析身份，再用 `lark-im` 发消息。

## 能力路由

根据用户意图从以下条目选择对应子能力；如果一个任务涉及多个能力，按实际操作顺序逐步读取并使用对应子能力。

<!-- LARK_SUITE_ROUTES -->
