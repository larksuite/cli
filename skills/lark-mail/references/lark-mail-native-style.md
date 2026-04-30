# 飞书原生写法与模板

撰写邮件时优先用 HTML，保持结论前置、标题短、分点清晰、emoji 克制，不写冗长免责声明。

## 通知模板

```html
<div style="line-height:1.6;color:rgb(31,35,41)">
  <p>各位同事：</p>
  <h3>系统维护通知</h3>
  <p><b>结论：</b>本周五 22:00-23:00 将进行例行维护。</p>
  <ul><li>影响范围：后台配置页短暂不可用</li><li>业务接口不受影响</li></ul>
  <table><thead><tr><th>时间</th><th>动作</th></tr></thead><tbody><tr><td>22:00</td><td>开始维护</td></tr></tbody></table>
  <p>[发件人] / [团队] / [日期]</p>
</div>
```

## 周报模板

```html
<div style="line-height:1.6;color:rgb(31,35,41)">
  <p>各位同事：</p>
  <h3>本周进展</h3><ul><li>完成核心功能联调</li><li>补齐风险用例</li></ul>
  <h3>下周计划</h3><ol><li>灰度验证</li><li>整理发布说明</li></ol>
  <h3>关键指标</h3><table><thead><tr><th>指标</th><th>本周</th><th>上周</th></tr></thead><tbody><tr><td>通过率</td><td>99%</td><td>97%</td></tr></tbody></table>
  <p>[发件人] / [团队] / [日期]</p>
</div>
```

## 决策请求模板

```html
<div style="line-height:1.6;color:rgb(31,35,41)">
  <p>Hi 各位 Reviewer，下方是方案审批请求，请协助拍板。</p>
  <h3>请求事项</h3><p><b>请确认是否按方案 A 推进。</b></p>
  <h3>背景</h3><p>当前链路需要在本周内完成收敛。</p>
  <h3>选项与建议</h3><table><thead><tr><th>方案</th><th>优势</th><th>劣势</th><th>推荐</th></tr></thead><tbody><tr><td>A</td><td>风险低</td><td>周期中等</td><td>是</td></tr><tr><td>B</td><td>最快</td><td>回滚复杂</td><td>否</td></tr></tbody></table>
  <h3>需要的决策与时间</h3><p>请在本周四 18:00 前确认。</p>
  <p>[发件人姓名] / [团队] / [日期]</p>
</div>
```
