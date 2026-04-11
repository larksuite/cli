# Slide XML 模板

可直接复制使用的 slide XML 模板。使用 `jq` 包装后传给 `xml_presentation.slide.create`：

```bash
lark-cli slides xml_presentation.slide create --as user \
  --params '{"xml_presentation_id":"YOUR_ID"}' \
  --data "$(jq -n --arg content 'PASTE_XML_HERE' '{slide:{content:$content}}')"
```

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
