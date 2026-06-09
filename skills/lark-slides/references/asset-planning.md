# Asset Planning

新建演示文稿或大幅改写页面时，在写入 `slide_plan.json` 前后都可以参考本文件。目标是让 agent 主动识别有价值的图、图标、图表、流程图、时序图、架构图、装饰图案、截图或示意图需求，同时保持 deck 在没有真实素材时也能完整执行。

本文件只定义轻量资产规划。不要把它理解成素材采集流程。

## Core Rules

- `asset_need` is metadata only. It can guide page design, but it must not require web search, local download, media upload, or external tools.
- Every planned asset must include a fallback visual plan so the slide can be generated with XML shapes, text, arrows, tables, simple charts, whiteboard diagrams, or placeholder regions.
- Asset needs must serve the page's `key_message` and `visual_focus`. Do not add decorative assets that do not clarify the page.
- Prefer a few high-value asset plans over one asset on every page. For a 6-page technical or business deck, plan assets on at least 3 pages when the content allows.
- If a real local asset already exists or the user provides one, it can be used through the normal media-upload workflow. Still keep `fallback_if_missing` in the plan.
- Do not leave blank image boxes in final XML. If the asset is missing, render the fallback visual.

## JSON Shape

Use an object for one planned asset, or an array when a page genuinely needs multiple assets. Keep each item compact.

```json
{
  "asset_type": "architecture_diagram",
  "purpose": "Show how API gateway, planner, XML generator, and Slides API interact.",
  "suggested_query": "agent native slides runtime architecture diagram",
  "fallback_if_missing": "Draw grouped boxes connected by arrows with short labels."
}
```

For a page without a meaningful asset need, use:

```json
{
  "asset_type": "none",
  "purpose": "No external or simulated asset needed; the page is text-led.",
  "suggested_query": "",
  "fallback_if_missing": "Use typography, spacing, and simple accent shapes only."
}
```

## Supported Asset Types

- `paper_figure`: figure from a paper or technical article.
- `architecture_diagram`: system components, data flow, dependency map, or model structure.
- `icon`: small semantic symbol for a concept, step, role, or status.
- `logo`: brand, product, team, or customer mark.
- `chart`: line, bar, pie, radar, area, or combo data visual. Note: `<chart>` does not support funnel or scatter — map those to `<whiteboard>` SVG at generation time.
- `infographic`: composed visual explanation, usually combining labels, numbers, and simple shapes.
- `screenshot`: product UI, terminal output, workflow state, or page capture.
- `flow_diagram`: process, sequence, decision tree, or mechanism diagram.
- `none`: explicitly no asset needed.

Do not invent new asset types unless the user asks for a special visual format. If a need is close to these types, choose the closest one and explain the detail in `purpose`.

## Planning Guidance

Match asset type to slide role:

- `architecture-diagram` layout usually pairs with `architecture_diagram` or `flow_diagram`.
- `process-flow` layout usually pairs with `flow_diagram`, `icon`, or `infographic`.
- `comparison` layout often works with `icon`, `chart`, or `infographic`.
- `timeline` layout often works with `icon`, `chart`, or shape-based milestone markers.
- `big-number` layout often works with `chart` or `infographic`, but only if it supports the metric.
- `image-left-text-right` and `image-right-text-left` can use `screenshot`, `paper_figure`, `logo`, or `infographic`; if missing, use a large placeholder diagram or stylized panel.

`suggested_query` is only a future lookup hint. Write it as a short phrase a human or later workflow could search, but do not execute the search unless the user separately requests real assets.

`fallback_if_missing` must be concrete enough to turn into XML, for example:

- "Draw a simplified attention matrix with 5 token labels, semi-transparent cells, and arrows to output token."
- "Use three grouped boxes with arrows from client to gateway to service; add small protocol labels."
- "Render a mini bar chart with 4 bars using shapes and value labels."
- "Use a bordered placeholder panel with product area labels, not an empty image."

Weak fallbacks to avoid:

- "Use a placeholder."
- "Find another image."
- "Leave blank if unavailable."
- "Use generic decoration."

## Examples

Transformer Self-Attention page:

```json
{
  "asset_type": "paper_figure",
  "purpose": "Explain token-to-token attention and why each output token mixes context.",
  "suggested_query": "Transformer self attention attention matrix diagram",
  "fallback_if_missing": "Draw a simplified attention matrix with token labels, colored weights, and arrows from input tokens to one highlighted output token."
}
```

System architecture page:

```json
{
  "asset_type": "architecture_diagram",
  "purpose": "Show the runtime path from user prompt to plan, XML generation, Slides API creation, and fetch verification.",
  "suggested_query": "slides generation runtime architecture planner XML API verification",
  "fallback_if_missing": "Draw four grouped boxes connected left-to-right with arrows; put verification as a return arrow from Slides API to agent."
}
```

Business comparison page:

```json
{
  "asset_type": "infographic",
  "purpose": "Make before/after differences scannable without dense bullet lists.",
  "suggested_query": "before after product workflow comparison infographic",
  "fallback_if_missing": "Use two side-by-side panels with matching icon circles and three parallel rows of concise labels."
}
```

## Plan To XML Contract

When generating XML:

1. If an asset exists and the workflow supports it, place it in the planned visual region.
2. If no asset exists, immediately render `fallback_if_missing` with XML-native shapes, text, lines, arrows, tables, whiteboard diagrams, or chart-like elements.
3. Size the fallback to satisfy `visual_focus`; it should be a real page element, not a tiny decoration.
4. Keep text-density limits. Do not compensate for missing assets by adding long bullet text.
5. After creation, fetch the presentation and verify asset pages are not blank and that each planned fallback is visible when no real asset was used.
