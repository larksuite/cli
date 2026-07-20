# IconPark 图标

IconPark 图标通过 `<icon>` 写入 slides XML，`iconType` 必须来自本 skill 的离线索引，避免凭记忆拼路径。

## 机器优先流程

```bash
python3 skills/lark-slides/scripts/iconpark_tool.py search --query "增长趋势" --limit 8
python3 skills/lark-slides/scripts/iconpark_tool.py resolve --name chart-line
python3 skills/lark-slides/scripts/iconpark_tool.py list-categories
```

`search` 返回 JSON 数组，每项包含 `iconType`、`category`、`name`、`tags`、`score`。直接把选中的 `iconType` 写入 XML，并为图标指定可见颜色：

```xml
<icon iconType="iconpark/Charts/chart-line.svg" topLeftX="80" topLeftY="120" width="32" height="32">
  <fill>
    <fillColor color="rgba(37, 99, 235, 1)"/>
  </fill>
</icon>
```

## 使用规则

- 默认先检索：语义图标需求必须先用 `iconpark_tool.py search --limit 8` 或 `--limit 10`，让 agent 从候选里结合版面语义二次判断；不要阅读全文索引，也不要编造不存在的 `iconType`。
- 图标用于概念提示、步骤、状态、指标、角色和导航；不要用无关装饰图标填充版面。
- 常用尺寸：行内状态图标 16-24px，卡片标题图标 28-40px，主视觉图标 56-96px。
- 视觉规范要求图标设置非透明 `fillColor`，显式指定颜色并和背景有足够对比；深色背景优先放在浅色圆形/方形底上，或使用 `rgba(255, 255, 255, 1)` 作为图标填充色。
- 查不到合适图标时，用 shape、line、text 画 XML-native fallback，不留空图标位。

## 高频示例

| 语义 | iconType |
|---|---|
| 设置/配置 | `iconpark/Base/setting.svg` |
| 目标 | `iconpark/Base/aiming.svg` |
| 增长趋势 | `iconpark/Charts/positive-dynamics.svg` |
| 折线趋势 | `iconpark/Charts/chart-line.svg` |
| 占比 | `iconpark/Charts/chart-proportion.svg` |
| 数据看板 | `iconpark/Charts/data-screen.svg` |
| 成功 | `iconpark/Character/check-one.svg` |
| 失败/风险 | `iconpark/Character/close-one.svg` |
| 团队/用户 | `iconpark/Peoples/peoples.svg` |
| 安全防护 | `iconpark/Safe/protect.svg` |
| 全球/市场 | `iconpark/Travel/world.svg` |
| 邮件/联系 | `iconpark/Office/envelope-one.svg` |
