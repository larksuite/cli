# SVG 编辑路径

通过导出画板的 SVG → 编辑 SVG → 回写画板，实现对已有画板的可视化编辑。

---

## ⚠️ 有损性警告

SVG 导出是**纯视觉快照**，再次导入后画板语义（思维导图层级/表格结构/连线绑定/容器类型/mention/节点 ID/锁定/评论）会丢失。

**保留的信息**：形状几何（位置/大小/路径）、文本内容与基本格式（字号/粗体/斜体/对齐）、填充色/描边色/透明度（线性渐变降级为第一个 stop-color 纯色）、连接器路径形状与箭头样式、`<g>` 嵌套的基本分组关系（≥2 子元素时重建为 DirectFocusGroup）。

---

## Workflow

### 0. 用户确认（强制）

在执行任何编辑前，**必须**向用户说明：

> SVG 编辑只保证视觉层面对齐，画板语义（层级/节点类型/思维导图结构/表格结构/连线绑定/容器类型/mention 等）将不可恢复，是否继续？

**用户未确认前不得执行后续步骤。**

### 1. 导出当前画板 SVG

```bash
lark-cli whiteboard +query \
  --whiteboard-token <TOKEN> \
  --output_as svg \
  --output <dir>/original.svg \
  --as user
```

### 2. 编辑 SVG

在导出的 SVG 上进行修改。参考 [`svg.md` § 画板怎么处理 SVG](./svg.md#画板怎么处理-svg) 了解可识别元素与不支持的装饰特性。

**技术约束**：
- 新增文字必须用 `<text>`（不是 `<path>`），容器宽度留够（CJK ≈ 1em / Latin ≈ 0.6em）
- 避免 `skewX` / `skewY` / `matrix(...)` 变换
- 禁止使用 `<radialGradient>` / `<filter>` / `<pattern>` / `<clipPath>` / `<mask>`

**编辑原则**（区别于从零创作）：

- **风格一致**：新增/修改元素应匹配导出 SVG 中已有的配色、字号、线宽、间距风格，不引入突兀的视觉差异
- **最小改动**：只修改用户要求的部分，不主动"优化"或重排无关区域
- **结构稳定**：尽量保留原有 `<g>` 层级结构，避免不必要的重组导致分组关系变化
- **连线协调**：连接器端点绑定已丢失，若移动了形状，必须手动同步调整视觉上连接到该形状的 connector path 端点坐标，否则连线会"断开"
- **内部引用完整性**：不要随意删改 `<defs>` 中被 `url(#id)` 引用的元素（`<marker>`/`<linearGradient>` 等）或修改其 `id`，否则引用方会失效

### 3. 渲染审查

```bash
# 渲染 PNG 预览
npx -y @larksuite/whiteboard-cli@^0.2.13 -i <dir>/edited.svg -o <dir>/edited.png -f svg

# 几何检查（text-overflow / node-overlap）
npx -y @larksuite/whiteboard-cli@^0.2.13 -i <dir>/edited.svg -f svg --check
```

结合 PNG 视觉效果和 `--check` 报告进行调整，有问题则修改 SVG 后重新渲染（最多 2 轮）。
- SVG 本地渲染预览时，画板中的图片因 session 原因无法正常显示，属于预期内的行为。

### 4. 写回画板

`--overwrite` 会清空原画板内容，确认后再执行

```bash
# dry-run 探测
lark-cli whiteboard +update \
  --whiteboard-token <TOKEN> \
  --source @<dir>/edited.svg \
  --input_format svg \
  --idempotent-token <10+字符唯一串> \
  --overwrite --dry-run --as user

# 用户确认后执行
lark-cli whiteboard +update \
  --whiteboard-token <TOKEN> \
  --source @<dir>/edited.svg \
  --input_format svg \
  --idempotent-token <10+字符唯一串> \
  --overwrite --as user
```
