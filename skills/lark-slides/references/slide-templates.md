# Slide XML 模板

可直接复制使用的 slide XML 模板。纯文本/形状模板可使用 `jq` 包装后传给 `xml_presentation.slide.create`：

```bash
lark-cli slides xml_presentation.slide create --as user \
  --params '{"xml_presentation_id":"YOUR_ID"}' \
  --data "$(jq -n --arg content 'PASTE_XML_HERE' '{slide:{content:$content}}')"
```

> **带图模板不要直接按上面的命令提交。** 新建 PPT 时可在 `+create --slides` 中使用 `src="@./local.png"`，CLI 会自动上传并替换为 `file_token`；给已有 PPT 添加或修改图片时，必须先用 `slides +media-upload` 拿到 `file_token`，再写进 `<img src="...">`。

## 深色封面页

```xml
<slide xmlns="http://www.larkoffice.com/sml/2.0">
  <style><fill><fillColor color="linear-gradient(135deg,rgba(15,23,42,1) 0%,rgba(56,97,140,1) 100%)"/></fill></style>
  <data>
    <shape type="text" topLeftX="80" topLeftY="160" width="800" height="70">
      <content><p textAlign="center"><strong><span color="rgb(255,255,255)" fontSize="44">主标题</span></strong></p></content>
    </shape>
    <shape type="text" topLeftX="80" topLeftY="250" width="800" height="35">
      <content><p textAlign="center"><span color="rgb(148,163,184)" fontSize="20">副标题</span></p></content>
    </shape>
    <shape type="text" topLeftX="80" topLeftY="420" width="800" height="25">
      <content><p textAlign="center"><span color="rgb(100,116,139)" fontSize="14">底部信息</span></p></content>
    </shape>
  </data>
</slide>
```

## 浅色内容页

```xml
<slide xmlns="http://www.larkoffice.com/sml/2.0">
  <style><fill><fillColor color="rgb(248,250,252)"/></fill></style>
  <data>
    <shape type="rect" topLeftX="60" topLeftY="40" width="4" height="35">
      <fill><fillColor color="rgb(59,130,246)"/></fill>
    </shape>
    <shape type="text" topLeftX="76" topLeftY="36" width="600" height="45">
      <content><p><strong><span color="rgb(15,23,42)" fontSize="28">页面标题</span></strong></p></content>
    </shape>
    <shape type="text" topLeftX="60" topLeftY="100" width="840" height="380">
      <content textType="body" lineSpacing="multiple:1.8">
        <p><span color="rgb(51,65,85)" fontSize="15">正文段落</span></p>
        <ul>
          <li><p><span color="rgb(51,65,85)" fontSize="15">要点一</span></p></li>
          <li><p><span color="rgb(51,65,85)" fontSize="15">要点二</span></p></li>
          <li><p><span color="rgb(51,65,85)" fontSize="15">要点三</span></p></li>
        </ul>
      </content>
    </shape>
  </data>
</slide>
```

## 数据卡片页（横排指标）

```xml
<slide xmlns="http://www.larkoffice.com/sml/2.0">
  <style><fill><fillColor color="rgb(248,250,252)"/></fill></style>
  <data>
    <shape type="text" topLeftX="60" topLeftY="36" width="600" height="45">
      <content><p><strong><span color="rgb(15,23,42)" fontSize="28">数据概览</span></strong></p></content>
    </shape>
    <!-- 卡片 1 -->
    <shape type="rect" topLeftX="60" topLeftY="100" width="260" height="140">
      <fill><fillColor color="rgb(255,255,255)"/></fill>
      <border color="rgba(0,0,0,0.08)" width="1"/>
    </shape>
    <shape type="text" topLeftX="60" topLeftY="115" width="260" height="50">
      <content><p textAlign="center"><strong><span color="rgb(59,130,246)" fontSize="36">数值</span></strong></p></content>
    </shape>
    <shape type="text" topLeftX="60" topLeftY="175" width="260" height="25">
      <content><p textAlign="center"><span color="rgb(100,116,139)" fontSize="14">指标名称</span></p></content>
    </shape>
    <!-- 卡片 2：topLeftX="350" -->
    <!-- 卡片 3：topLeftX="640" -->
  </data>
</slide>
```

## 带图版式

> **关键提醒**：`<img>` 的 `width:height` = 原图比例时才不会被裁剪。每个模板都标注了图框比例和建议原图比例，**选模板前先对照你的素材比例**，不要硬塞（如把横图放进竖框，会被左右裁掉大半）。把 `@./your-image.jpg` 替换为实际路径（仅 `+create --slides` 支持 `@` 占位符；其他场景需先用 `slides +media-upload` 拿 `file_token`）。

### 封面右图（左字右图）

图框 400×225（**16:9**），建议原图：横幅 16:9（桌面壁纸、产品 banner、landscape 照片）

```xml
<slide xmlns="http://www.larkoffice.com/sml/2.0">
  <style><fill><fillColor color="linear-gradient(135deg,rgba(15,23,42,1) 0%,rgba(56,97,140,1) 100%)"/></fill></style>
  <data>
    <shape type="text" topLeftX="60" topLeftY="180" width="450" height="80">
      <content><p><strong><span color="rgb(255,255,255)" fontSize="44">主标题</span></strong></p></content>
    </shape>
    <shape type="text" topLeftX="60" topLeftY="270" width="450" height="40">
      <content><p><span color="rgb(186,230,253)" fontSize="20">副标题</span></p></content>
    </shape>
    <line startX="60" startY="350" endX="180" endY="350">
      <border color="rgb(59,130,246)" width="3"/>
    </line>
    <shape type="text" topLeftX="60" topLeftY="370" width="450" height="30">
      <content><p><span color="rgb(203,213,225)" fontSize="13">底部信息</span></p></content>
    </shape>
    <!-- 图框 400×225 = 16:9；原图建议 16:9 横幅 -->
    <img src="@./your-landscape.jpg" topLeftX="540" topLeftY="157" width="400" height="225"/>
  </data>
</slide>
```

### 三卡片带图（上图下文）

每个图框 240×180（**4:3**），建议原图：4:3 或接近正方形的图（产品照、截图、icon 类）

```xml
<slide xmlns="http://www.larkoffice.com/sml/2.0">
  <style><fill><fillColor color="rgb(248,250,252)"/></fill></style>
  <data>
    <shape type="text" topLeftX="60" topLeftY="40" width="600" height="45">
      <content><p><strong><span color="rgb(15,23,42)" fontSize="28">核心亮点</span></strong></p></content>
    </shape>
    <line startX="60" startY="95" endX="140" endY="95">
      <border color="rgb(59,130,246)" width="3"/>
    </line>

    <!-- 卡片 1 -->
    <shape type="rect" topLeftX="60" topLeftY="130" width="270" height="360">
      <fill><fillColor color="rgb(255,255,255)"/></fill>
      <border color="rgba(0,0,0,0.08)" width="1"/>
    </shape>
    <!-- 图框 240×180 = 4:3；原图建议 4:3 -->
    <img src="@./your-image-1.jpg" topLeftX="75" topLeftY="150" width="240" height="180"/>
    <shape type="text" topLeftX="75" topLeftY="345" width="240" height="30">
      <content><p><strong><span color="rgb(15,23,42)" fontSize="18">特性一</span></strong></p></content>
    </shape>
    <shape type="text" topLeftX="75" topLeftY="380" width="240" height="90">
      <content><p><span color="rgb(71,85,105)" fontSize="14">简短描述文案，控制在两行以内。</span></p></content>
    </shape>

    <!-- 卡片 2：复制卡片 1，shape/img 的 topLeftX 改为 345 / 360 -->
    <!-- 卡片 3：复制卡片 1，shape/img 的 topLeftX 改为 630 / 645 -->
  </data>
</slide>
```

### 左右分栏（图在左，文在右）

图框 360×540（**2:3 竖幅**），建议原图：2:3 或 3:4 竖幅（人像照、产品竖拍、海报）

> 如果你只有横幅图，不要硬塞进这个竖框 —— 改用"顶部横幅图 + 下方文字"的版式（把这里的图框改成 960×240 横条放在顶部）。

```xml
<slide xmlns="http://www.larkoffice.com/sml/2.0">
  <style><fill><fillColor color="rgb(255,255,255)"/></fill></style>
  <data>
    <!-- 图框 360×540 = 2:3；原图建议 2:3 或 3:4 竖幅 -->
    <img src="@./your-portrait.jpg" topLeftX="0" topLeftY="0" width="360" height="540"/>

    <shape type="text" topLeftX="410" topLeftY="80" width="490" height="50">
      <content><p><strong><span color="rgb(15,23,42)" fontSize="30">场景标题</span></strong></p></content>
    </shape>
    <line startX="410" startY="140" endX="490" endY="140">
      <border color="rgb(59,130,246)" width="3"/>
    </line>
    <shape type="text" topLeftX="410" topLeftY="160" width="490" height="50">
      <content><p><span color="rgb(71,85,105)" fontSize="16">一句话描述这个场景的价值。</span></p></content>
    </shape>
    <shape type="text" topLeftX="410" topLeftY="230" width="490" height="250">
      <content textType="body" lineSpacing="multiple:1.8">
        <ul>
          <li><p><span color="rgb(51,65,85)" fontSize="15">要点一</span></p></li>
          <li><p><span color="rgb(51,65,85)" fontSize="15">要点二</span></p></li>
          <li><p><span color="rgb(51,65,85)" fontSize="15">要点三</span></p></li>
        </ul>
      </content>
    </shape>
  </data>
</slide>
```

## 深色结尾页

```xml
<slide xmlns="http://www.larkoffice.com/sml/2.0">
  <style><fill><fillColor color="linear-gradient(135deg,rgba(15,23,42,1) 0%,rgba(56,97,140,1) 100%)"/></fill></style>
  <data>
    <shape type="text" topLeftX="80" topLeftY="190" width="800" height="55">
      <content><p textAlign="center"><strong><span color="rgb(255,255,255)" fontSize="36">感谢语或行动号召</span></strong></p></content>
    </shape>
    <line startX="410" startY="260" endX="550" endY="260">
      <border color="rgb(59,130,246)" width="2"/>
    </line>
    <shape type="text" topLeftX="80" topLeftY="280" width="800" height="30">
      <content><p textAlign="center"><span color="rgb(148,163,184)" fontSize="16">补充说明</span></p></content>
    </shape>
  </data>
</slide>
```
