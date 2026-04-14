# ModelHub平台-模型使用手册

# 一、平台定位

ModelHub 是公司统一的大模型与外部 AI 服务接入平台。我们致力于帮助各业务线**安全、稳定、低成本**地使用来自多家厂商的大模型能力。

平台提供以下核心功能：

- **模型广场与体验中心**：在线体验、对比不同模型的效果。

- **统一接入与密钥管理**：简化的 API 密钥管理与权限控制。

- **合规与审批流程**：内置法务、安全审批流程，确保业务合规。

- **资源与成本管理**：提供配额设置、用量监控、成本计算与账单结算。

平台地址：[https://aidp.bytedance.net/modelhub/model-access](https%3A%2F%2Faidp.bytedance.net%2Fmodelhub%2Fmodel-access)

### 一分钟速览：三步开始使用

<grid cols="3">

  <column width="33">

    **第 1 步：**[**完成场景审批**](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23GIUMdoH8PoGSsQxKM7mcQ5YonPf)
    <callout emoji="💡" background-color="light-orange" border-color="light-yellow">

    发起场景的法务合规审批

    - **正式场景**：约 1-2 天

    - **试用场景**：仅需 +1 审批，约 10 分钟
    </callout>

  </column>

  <column width="33">

    **第 2 步：**[**申请模型配额**](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23StDUd11pqoZ9ohxUfiQciPaXnwb)
    <callout emoji="💡" background-color="light-blue" border-color="light-blue">

    申请模型调用配额

    - **申请提交**：约 10 分钟

    - **配额审批**：约 30 分钟
    
    </callout>

  </column>

  <column width="34">

    **第 3 步：**[**获取密钥与API调用**](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23OfiHdRNsYo9t4WxMjrVcxmsanOd)
    <callout emoji="💡" background-color="light-green" border-color="light-green">

    获取密钥，调用 API

    - **即时生效**

    - **提供代码示例**
    
    </callout>

  </column>

</grid>

# 二、**平台能做什么：核心功能概览**
<callout emoji="💡" background-color="light-orange" border-color="light-blue">

ModelHub 平台集成了从模型体验、资源申请到监控的全链路功能。下表汇总了各核心模块的入口及简介，方便你快速找到所需功能
</callout>

平台地址：https://aidp.bytedance.net/modelhub/model-access

<image token="YMqFbpvgHoiuAyxCVt3cCvNnn6f" width="3708" height="1674" align="center"/>

主流程

<add-ons component-id="" component-type-id="blk_631fefbbae02400430b8f9f4" record="{"data":"graph LR\nA[发起场景审批] --\u003e B[申请模型配额]\nB --\u003e C[获取API密钥]\nC --\u003e D[模型调用与监控]\nD --\u003e E[追踪用量与费用]\nE --\u003e F[问题排查与解决]","theme":"default","view":"chart"}"/>

### 任务导航

如果你有明确的目标，请参考下面的指引：

- 我要开始接入：先看【[三、模型申请和使用](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-HnOOdDAvJoY0gGx6k6RcdEL4nGf)】

- 我要了解平台支持什么能力：看[【能力地图】](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-EJrydhXuyo1N1dxdTcNcAJbLnif)

- 我要优化成本和性能：看【[成本与性能](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-YreOdzhOwoOwZpxNCXZcjUHqnCf)】和【[六、专属资源（PTU）与成本管理](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-D0WwdM82coWIlNxjZSxcMsfOnNe)】

- 我要排查线上问题：看【[五、如何追踪请求与监控用量](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-AwIqdGJL1ofesYxOBF6cn2HAnmc)】和【[七、问题排查与处理（FAQ）](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-TE5QdnsVyo4eASxGkTQcm7yln9b)】

如果您想体验完整的核心流程，可按下列任务进行：

<grid cols="3">

  <column width="33">

    **任务：体验与对比模型**

    - 目标：先看效果，再决定接入哪个模型

    - 入口：[模型广场](https%3A%2F%2Faidp.bytedance.net%2Fmodelhub%2Fmodel-square) / [体验中心](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-JhiDdnrKdoudJ5xiuTzc6It0nPh)

  </column>

  <column width="33">

    **任务：发起场景审批并完成接入**

    - 目标：让业务场景通过法务、安全合规审批

    - 入口：[发起场景审批](https%3A%2F%2Faidp.bytedance.net%2Fmodelhub%2Fmodel-access)

  </column>

  <column width="34">

    **任务：申请模型配额并选择资源类型**

    - 目标：拿到足够稳定的 QPM / TPM 资源

    - 入口：[申请模型配额](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-GBGud3u4jopNcXx9jsmc3jgXndf)

  </column>

</grid>

<grid cols="3">

  <column width="33">

    **任务：获取 AK 并完成首次 API 调用**

    - 目标：拿到密钥，按示例代码跑通一次请求

    - 入口：[获取密钥与API调用](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-M6jgdLj1WofjGfxxIJVcMy5jnSg)

  </column>

  <column width="33">

    **任务：排查问题与观察流量**

    - 目标：发现/定位模型调用错误或性能问题

    - 入口：[Trace、监控看板](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-UvHTdvmpDoWUd0xOeJeclHpUnDe)

  </column>

  <column width="34">

    **任务：管理成本与升级资源**

    - 目标：看清每日费用，按需扩缩容或升级到 PTU

    - 入口：[账单与用量、费用计算器、独立 PTU 管理](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-OCytdhUf0oQxzvxtWrochizgn7C)

  </column>

</grid>

### 能力地图

<lark-table rows="11" cols="4" header-row="true" column-widths="183,314,215,521">

  <lark-tr>
    <lark-td>
      模块
    </lark-td>
    <lark-td>
      说明
    </lark-td>
    <lark-td>
      地址
    </lark-td>
    <lark-td>
      截图
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      模型广场
    </lark-td>
    <lark-td>
      平台支持的模型全部会展示在此处，支持查看模型示例代码和快速跳转到模型体验页（一期支持文本模型）
    </lark-td>
    <lark-td>
      https://aidp.bytedance.net/modelhub/model-square
    </lark-td>
    <lark-td>
      <image token="Ot6nbjIPpoCXsDx1VuAc3Yiqndb" width="3700" height="1668" align="center"/>
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      体验中心 - 文本模型
    </lark-td>
    <lark-td>
      **支持模型体验和对比**，目前支持文本模型，多模态模型体验正在规划中～
    </lark-td>
    <lark-td>
      https://aidp.bytedance.net/modelhub/trial
    </lark-td>
    <lark-td>
      <image token="XsH5bn3ErozVZqxDs7scH9Dynrg" width="3710" height="1676" align="center"/>
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      账单与用量 - 数据看板
    </lark-td>
    <lark-td>
      支持查看**模型每日实际费用**，包括`PTU`费用、`paygo`费用、各类token 费用等等
    </lark-td>
    <lark-td>
      https://aidp.bytedance.net/modelhub/dashboard
    </lark-td>
    <lark-td>
      <image token="G73kbWuZro7N5exXwNncMkGQnIb" width="3704" height="1678" align="center"/>
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      账单与用量 - 费用计算器
    </lark-td>
    <lark-td>
      支持模型**费用预估、对比**，填入预计用量，平均请求tokens数，就能自动计算出预计费用
    </lark-td>
    <lark-td>
      https://aidp.bytedance.net/modelhub/cost
    </lark-td>
    <lark-td>
      <image token="Cyk7bs8wdo1mI1x9FdUcxn1Ln3g" width="3714" height="1674" align="center"/>
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      产品接入 - 模型接入
    </lark-td>
    <lark-td>
      核心模块，支持发起各类模型申请的**合规审批流程、模型资源管理流程、管理密钥、示例代码、监控看板、代理设置、安全防护、账号粘性开关**。详细接入流程参考本文档【[三、如何申请模型配额](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-Ycb1dNETpo0FlaxbxntcDcgUnpg)】一节
    </lark-td>
    <lark-td>
      https://aidp.bytedance.net/modelhub/model-access
    </lark-td>
    <lark-td>
      <image token="VdFPbKRwQoL7f2xMcDrcFQ6Xn1e" width="3708" height="1674" align="center"/>
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      产品接入 - 独立 PTU 管理
    </lark-td>
    <lark-td>
      支持独立**PTU的申请和管理**，详见本文档【[五、专属资源（PTU）与成本管理](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23HPfTdFjqCoj5bNxVMEec9jfJnqe)】一节
    </lark-td>
    <lark-td>
      https://aidp.bytedance.net/modelhub/ptu-manage
    </lark-td>
    <lark-td>
      <image token="FSqIbJYxaoeVg3xKZWdcj0HQnHR" width="4354" height="1862" align="center"/>
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      模型微调
    </lark-td>
    <lark-td>
      支持GPT和Gemini部分<text color="red">**模型的SFT/DPO**</text>，详见本文档【[模型SFT/DPO](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-REhRd5evGojDJWxW797chmmcnBe)】一节
    </lark-td>
    <lark-td>
      https://aidp.bytedance.net/modelhub/fine-tuning
    </lark-td>
    <lark-td>
      <image token="DF31bXsROoI8k9xKlfucE14snId" width="4350" height="1896" align="center"/>
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      排行榜
    </lark-td>
    <lark-td>
      支持查看 Arena <text color="red">**模型排行榜**</text>和模型开源评测排行榜
    </lark-td>
    <lark-td>
      https://aidp.bytedance.net/modelhub/ranking
    </lark-td>
    <lark-td>
      <image token="Xs6Cb7wrYoZTcoxHHQrc6DC1n5d" width="4352" height="1948" align="center"/>
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      审批中心
    </lark-td>
    <lark-td>
      平台上发起的各类<text color="red">**审批流程**</text>，可在此处查看详情
    </lark-td>
    <lark-td>
      https://aidp.bytedance.net/modelhub/approval
    </lark-td>
    <lark-td>
      <image token="Ksv7bg56poJoI0xTFDuc9sOqnrU" width="4336" height="1794" align="center"/>
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      Trace
    </lark-td>
    <lark-td>
      平台接入了fornax trace，支持查询所有请求的<text color="red">**trace日志**</text>，包括用户与ModelHub的交互日志和ModelHub与下游厂商的交互日志。详细用法参考本文档【[四、如何追踪请求与监控用量](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23FuBIda1vAoOxbgxIHKacPiIqnwe)】一节
    </lark-td>
    <lark-td>
      https://aidp.bytedance.net/modelhub/fornax_panel
    </lark-td>
    <lark-td>
      <image token="QlsUbfKMaoeFJPxSowPcxvkPnmc" width="4350" height="2028" align="center"/>
    </lark-td>
  </lark-tr>
</lark-table>

# 三、模型申请和使用

## 接入流程

<whiteboard token="Sgntwys4Qh5K49bMvITcqo7fnfd"/>

## 接入原则&风险评估
<callout emoji="exclamation" background-color="light-orange" border-color="light-orange">

请遵守以下接入原则：

1. **保障请求内容合规**：ModelHub涉及将消息发送到外部公司，且受监管侧重点关注，请大家务必保证使用方式的安全合规，尤其输入信息中不可以包含[**个人信息、重要数据**](https%3A%2F%2Fbytedance.feishu.cn%2Fdocs%2FdoccnQaPeVBl6JoNrZa6xQascge%3Ffrom%3Dfrom_copylink)**、**[**商业秘密**](https%3A%2F%2Fbytedance.feishu.cn%2Fwiki%2FwikcnnVhSQ8T5Sw6ppvPcX09Dle%3Ffrom%3Dfrom_copylink)**和出口管制内容****（如源代码、技术资料）**等。

1. **遵守Azure****non-compete条款**：不允许使用Azure服务或数据用于竞争目的，各业务发起申请时请谨慎评估或向法务BP咨询确认。关于微软non-compete 条款的初步解读，可参考<mention-doc token="QS9wdf6DyoCLHHxTkzScv71in3c" type="docx">OpenAI合规/安全使用指南与风险提示（持续更新）</mention-doc>的Q/A章节

1. **中国业务****优先使用豆包模型**：根据公司管理规范，如豆包大模型合规、效果、性能方面能满足场景的则推荐使用豆包大模型，不建议使用外部三方大模型

1. **妥善保管AK，防止泄漏**：AK是唯一用户身份标识，**请妥善保管，请勿进行分享，禁止前端直接传递AK**。

1. **各****厂商核心条款汇总**：<mention-doc token="MhdFdcjfco8obNxTy6PcmZPfnHg" type="docx">【持续更新】【P&C】AI接口采买协议核心条款汇总</mention-doc>，如果业务法务需要了解更详细的协议条款，请联系：国内<mention-user id="ou_8758ce423480d8a8c4df760702c0a495"/> 海外<mention-user id="ou_a8d52106bc5f5968e59a7a8eea9dae45"/> ，非法务同事请勿直接联系
</callout>

<callout emoji="thought_balloon" background-color="light-gray" border-color="gray">

**风险评估指南**

在提交场景审批前，请根据【数据敏感性】【合作保密性】【来源合法性】三个维度，对业务场景进行初步自评：

- 初步判断风险等级（高 / 中高 / 中 / 低）。

- 在审批表单中如实填写评估结果和降险措施。

- 需要更精细判断时，可对照下方表格中的判定标准。
</callout>

<lark-table rows="5" cols="2" column-widths="100,714">

  <lark-tr>
    <lark-td>
      **风险等级** {align="center"}
    </lark-td>
    <lark-td>
      **判定标准** {align="center"}

      <text color="red">**维度:【数据敏感性】【合作保密性】【来源合法性】**</text> {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      <text bgcolor="light-red">高风险</text> {align="center"}
    </lark-td>
    <lark-td>
      - 可能存在**刑事立案、监管调查**等高法律风险。
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      <text bgcolor="light-orange">中高风险</text> {align="center"}
    </lark-td>
    <lark-td>
      - 存在中风险的情况下，由于内外部整体形势，可能**因人为因素使风险上升**，但不存在高风险场景。例如涉及未**经授权的个人信息、定制采购**、数据源有竞品投资等等。
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      <text bgcolor="light-yellow">中风险</text> {align="center"}
    </lark-td>
    <lark-td>
      - 可能存在**不正当竞争、知识产权等经济法、竞争法层面的民事及行政责任**法律风险，或者被约谈、引发舆情等风险。
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      <text bgcolor="light-green">低风险</text> {align="center"}
    </lark-td>
    <lark-td>
      - 导致我方触发法律风险的可能性相对较低。
    </lark-td>
  </lark-tr>
</lark-table>

## 步骤一：完成场景审批

### CN/SG/NonTT区域申请场景
<callout emoji="💡" background-color="light-orange" border-color="light-yellow">

接入 ModelHub 的第一步是完成场景审批，确保业务在法务、安全层面合规，并为后续资源分配提供依据。此步骤的核心是完成**风险评估**。根据场景的紧急性和规模，我们提供“试用”和“正式”两种审批路径，它们在流程耗时与权限上有显著差异，请根据您的实际需求选择。
</callout>

1. 登录[平台](https%3A%2F%2Faidp.bytedance.net%2Fmodelhub%2Fdashboard%2Fresult)，点击【创建场景】

  <image token="TIbTb31F3oKUquxCFR9cItI5nFb" width="4168" height="2402" align="center"/>

1. 选择接入场景类型：根据业务需求选择合适的场景类型

  <image token="AffdbcGe5o46PMx1V10ct726n4b" width="4162" height="2390" align="center"/>

  1. 如果是外部模型的试用，选择【外部厂商-试用】

  1. 如果是外部模型的正式使用，选择【外部厂商-正式】

  <grid cols="2">

    <column width="50">
      <callout emoji="speech_balloon" background-color="light-orange" border-color="light-yellow">

      **快速上手：选择“试用”场景**

      - **目的**：功能验证、快速体验

      - **审批流程**：仅需 +1 审批

      - **预估耗时**：约 10 分钟

      - **AK 有效期**：30 天

      - **配额限制**：固定小额配额 (如 5 `QPM`)，**不支持扩容**

      - **适用**：开发阶段的临时测试
      </callout>

    </column>

    <column width="50">
      <callout emoji="⭐" background-color="light-green" border-color="light-green">

      **长期使用：选择“正式”场景**

      - **目的**：线上服务、生产环境

      - **审批流程**：法务、安全、内控多方审批

      - **预估耗时**：约 1-2 天

      - **AK 有效期**：长期有效

      - **配额限制**：按需申请，**支持扩缩容**

      - **适用**：所有线上/生产环境
      </callout>

    </column>

  </grid>

  1. 如果是方舟模型的试用/正式使用，选择【字节云方舟】 。

注意：

- 【外部厂商-试用】需上级审批；【外部厂商-正式】的审批流程需要法务、安全、内控审批，【字节云方舟】只需要上级审批

- 【外部厂商-试用】的AK有效期仅30天，单模型最高5QPM，且不支持扩容
  <callout emoji="exclamation" background-color="light-red" border-color="light-red">

  **红线警告：禁止将试用 AK 用于生产环境**

  【外部厂商-试用】场景获取的 API Key 仅供短期功能验证和内部测试使用。严禁将试用 AK 用于面向终端用户的生产环境或产生线上收入的业务，一经发现将触发安全红线。
  </callout>

1. 填写审批表单：确保场景描述准确，提供所需的合规和风险评估

  不同接入模型类型，需要填写的字段不一致，如下所示：

  <grid cols="3">

    <column width="33">

      外部厂商-试用

      <image token="DeYYbL35OoljU6x86Z9cPDaunOh" width="4180" height="2412" align="center"/>

    </column>

    <column width="33">

      外部厂商-正式

      <image token="FYQtbYlAyoJOqvxaCG4cGlWNn4e" width="4158" height="2398" align="center"/>

    </column>

    <column width="33">

      字节云方舟

      <image token="W6NJbpSFcoSBd5x1TQjc4U5FnZd" width="4176" height="2390" align="center"/>

    </column>

  </grid>

  <lark-table rows="11" cols="4" header-row="true" column-widths="163,142,120,208">

    <lark-tr>
      <lark-td colspan="4">
        **各类型场景字段说明**
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        **字段 \ 表单**
      </lark-td>
      <lark-td>
        **外部厂商-试用**
      </lark-td>
      <lark-td>
        **外部厂商-正式**
      </lark-td>
      <lark-td>
        **字节云方舟**
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        **模型AK的授权范围**
      </lark-td>
      <lark-td colspan="3">
        AK将会给哪些团队、用户使用，该字段只是用于记录，按真实使用情况填写即可
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        **法务BP**
      </lark-td>
      <lark-td colspan="3">
        业务方的法务BP，点击https://mybp.bytedance.com/查询
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        **财务BP**
      </lark-td>
      <lark-td colspan="3">
        业务方的财务BP，点击https://mybp.bytedance.com/查询
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        **场景名称**
      </lark-td>
      <lark-td colspan="3">
        场景中文名称，能简单概括该场景，例如：trae线上模型接入
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        **场景英文名称**
      </lark-td>
      <lark-td colspan="3">
        场景英文名称，能简单概括该场景，例如：trae_online_model_access
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        **管理员**
      </lark-td>
      <lark-td colspan="3">
        具有该场景管理权限的用户
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        **所属产品**
      </lark-td>
      <lark-td colspan="3">
        业务方的产品/平台名称，例如：trae、aime、seed评测等等
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        **用户类型**
      </lark-td>
      <lark-td>
        -
      </lark-td>
      <lark-td>
        -
      </lark-td>
      <lark-td>
        模型最终用户是公司内部用户，还是公司外部用户
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        **需求描述**
      </lark-td>
      <lark-td>
        -
      </lark-td>
      <lark-td>
        -
      </lark-td>
      <lark-td>
        描述落地场景,比如用于代码生成,如果有产品需求文档也可附上文档链接
      </lark-td>
    </lark-tr>
  </lark-table>

1. 如果是申请【外部厂商-正式】场景，还需选择厂商类型，并填写各厂商相关内容：

  <image token="Knqdbx5vDojFlvxM5sWcGFEInBg" width="4168" height="2376" align="center"/>

  1. 如果是接入OpenAI的模型如GPT、sora等模型，选择【Azure协议类厂商】

  1. 如果是接入非OpenAI厂商：Google（gemini），MiniMax、智谱、百度千帆、百川、阿里通义千问、Google（Gemini）等，选择【其他厂商】

  1. 如果是接入搜索API厂商如you、brave、bayou、serp等，选择【搜索引擎类厂商】

  每类厂商由于法务合规流程不同，需要填写的字段不同：

  <grid cols="3">

    <column width="33">

      **Azure 类协议厂商**

      <image token="ZqU1bxRRiodSJex7ymLcbRCHnfe" width="4150" height="2386" align="center"/>

    </column>

    <column width="33">

      **其他厂商**

      <image token="ZVL1bTRNYoAxdhx8666c2pQynCc" width="4160" height="2386" align="center"/>

    </column>

    <column width="33">

      **搜索引擎类厂商**

      <image token="CYuhbukTko2bA2x7csccheMZnmg" width="4166" height="2388" align="center"/>

    </column>

  </grid>

注意：

- 【你的服务可能部署的区域】字段表示你的服务所在的区域，例如你的服务可能在SG或者US_TTP访问ModelHub，就要同时勾选【i18N_TT(SG)】和【US_TTP】，否则后续无法申请对应区域的资源

  <image token="Cm4wbpdyxoySrdxJOGncO9q6nkc" width="1084" height="448" align="left"/>

- 其他字段按照表格中的说明填写或勾选即可

1. 审批发起后，需求方和自己的法务BP充分沟通使用方式，包括输入信息（指prompt）、返回结果的使用等，请法务BP作为收口人，拉上需求方数据法务、安全、PA同学一起输出评估意见，评估规则可参考 <mention-doc token="QS9wdf6DyoCLHHxTkzScv71in3c" type="docx">OpenAI合规/安全使用指南与风险提示（持续更新）</mention-doc>

  <lark-table rows="7" cols="2" column-widths="164,518">

    <lark-tr>
      <lark-td colspan="2">
        **【外部厂商-正式】的审批人员和顺序**
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        角色
      </lark-td>
      <lark-td>
        事项
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        1. 申请人
      </lark-td>
      <lark-td>
        - 填写申请表单
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        2. 申请人上级
      </lark-td>
      <lark-td>
        - 对申请人提出的使用场景进行评估：保证使用场景需求的真实性、必要性、最小化使用原则
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        3. 申请人法务
      </lark-td>
      <lark-td>
        - 填写审批意见，包括风险等级评估和降险建议，具体可参考<mention-doc token="QS9wdf6DyoCLHHxTkzScv71in3c" type="docx">【参考】Azure OpenAI-搜索openapi（更新中）</mention-doc> 

        - 作为申请方合规POC，如果需要安全、PA、PR审批，可加签（加签时不要选择后加签）

        - 可从[https://mybp.bytedance.com/](https%3A%2F%2Fmybp.bytedance.com%2F)查询法务BP
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        4. 搜索法务、安全、PA、PR、
      </lark-td>
      <lark-td>
        - 填写审批意见
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        5. 内控
      </lark-td>
      <lark-td>
        - 加签申请人内控，根据各方意见确认加签或者知会到申请人的哪一级
      </lark-td>
    </lark-tr>
  </lark-table>

1. 场景审批通过后，进入<mention-doc token="wikcnUPXCY2idGyg2AXKPvay4pd" type="wiki">GPT-OpenAPI接入手册</mention-doc> "步骤二：模型接入"，开始申请模型
  <callout emoji="thought_balloon" background-color="gray" border-color="gray">

  **常见问题**

  - **问：为什么“试用”场景也需要审批？**

    - **答**：尽管流程简化（仅需+1审批），但试用同样会产生团队成本和消耗平台资源，因此需要进行基础的审批留档。

  - **问：试用账号的配额限制是多少？可以用于线上测试吗？**

    - **答**：试用账号有严格的低配额限制（例如 5 QPM, 4K `TPM`），严禁直接用于生产环境。对于线上环境的内部测试，需确保使用量在配额范围内，并建议尽快转为正式场景以获取稳定资源和更高配额。

  - **问：审批时，我的法务/财务 BP 是谁？**

    - **答**：您可以通过访问 [MyBP](https%3A%2F%2Fmybp.bytedance.com%2F) 查询您所属业务线的法务和财务 BP。
  </callout>

### TTP区域申请场景

<reference-synced source-block-id="I7ZUdHSOKsQn3Sb223bcQ6IonVd" source-document-id="N7o6dsvxUoYIKFxyFwvcmHuKnBe">

  1. TTP 的支持情况：

    1. 目前 ModelHub 支持 US-TTP、 US-TTP2、 EU-TTP (ie/no1a) 和 GCP 接入使用。

    1. TTP 的计费方式统一为**按量计费**，每日按照实际使用量收取费用并通过babi推送。

    1. TTP 目前已经支持大部分 模型的接入。并且，只提供在线（online）接入，不支持离线批量处理的接入方式。

    1. **所有的 API 都没有接 OG 和 DES，只支持 TTP 内服务间调用，目前没有支持办公网 / TTP外访问的计划。**

  1. 整体流程：

    1. USTTP整体流程：具体审核链接可以参考<mention-doc token="WVYZdDzZKo2CmYxaTEhuKTq6sXb" type="docx">How to Onboard External AI Models (Example: Gemini) in USTTP</mention-doc>

    <image token="MuNNbIfKxoBRTWxglVGc87JVnzW" width="1445" height="629" align="center"/>

    1. EUTTP 整体流程：目前没有特殊的合规要求，业务法务评估确认合规即可。确认后带业务法务确认截图，提oncall接入

  1. 上述第二步审核通过后， 可以在 ModelHub 平台申请场景：

    1. 如果还没有创建过场景，可以在平台上创建一个新的US-TTP场景，进行法务审批流程

      <image token="Tb5Rb3wXro9IHOxlOPZcisqsnKe" width="2682" height="1738" align="center"/>

    1. 第一次接入US-TTP，需遵循<mention-doc token="WVYZdDzZKo2CmYxaTEhuKTq6sXb" type="docx">How to Onboard External AI Models (Ex. Gemini) in USTTP</mention-doc>, 包括TPRM审批和合规审批

    1. 如果是已有场景，需要新增US_TTP的接入账号，可以按如下步骤：

      1. 点击编辑场景

      <image token="P62WbvVNEov8eoxlxe6cNP3fnod" width="2680" height="1724" align="center"/>

      1. 勾选上需要新增的TTP场景，然后点击【立即修改】

      <image token="Pt7Vb2FLfouuOJxV12bcBT8xnlf" width="2666" height="1722" align="center"/>

      1. 点击【创建接入账号】，来创建一个TTP的账号

      <image token="NMoGbc0DWoB3JuxNUaycTEsHnTc" width="2680" height="1726" align="center"/>

      <image token="GyXdb2BoooQJMIxlZg2c03FBnco" width="2670" height="1724" align="center"/>

</reference-synced>

## 步骤二：申请模型配额
<callout emoji="💡" background-color="light-blue" border-color="light-blue">

场景审批通过后，您需要为该场景申请具体的模型调用配额。此环节的核心是**决策资源类型**与**预估成本**。您需要根据业务的并发量、稳定性要求和预算，在按量付费（Pay-as-you-go）和专属物理部署单元（PTU）之间做出选择。平台提供了成本计算器，帮助您在申请时做出更精确的成本预估。
</callout>

1. 业务方自查：已通过"步骤一：场景接入：法务安全合规审批" 

1. 明确要使用的模型，平台支持下列模型：

  - 三方厂商模型： <mention-doc token="HcXSwkmYniQiM2kWFVxcLQlqndh" type="wiki">ModelHub 模型列表</mention-doc>

  - 字节云方舟模型：<mention-doc token="T3GvwdlXqiX5VwksFAcclVQTnqc" type="wiki">ModelHub—Doubao模型/字节云方舟模型接入</mention-doc>

  - AGI-Hub上部署在merlin的模型：<mention-doc token="Zp6Ywg5OciYJywky1v3cVjmPnrc" type="wiki">AGI Hub 用户部署模型调用代码示例</mention-doc>

  如果你需要的模型在平台上没有，请参考：<mention-doc token="TMfww9sVhiU5n8kYNSicdyu2n5c" type="wiki">ModelHub 新模型新功能提需指南</mention-doc> 来提需接入

1. 登录平台：https://aidp.bytedance.net/modelhub/model-access，切换到需要申请模型的场景

  <image token="X1EBbRvZyoTDIGxCst5cHc6enIh" width="4166" height="2400" align="center"/>

1. 如果场景下【接入账号】列表为空，需要点击右上角【创建接入账号】

  <image token="OrFFbSYR3oklgrxkYiScma2fn1d" width="4168" height="2386" align="center"/>

  <image token="DDcfbGWcToIyxwx3JvZcgV3CnNf" width="4174" height="2390" align="center"/>

  <lark-table rows="4" cols="2" header-row="true" column-widths="117,445">

    <lark-tr>
      <lark-td>
        字段
      </lark-td>
      <lark-td>
        说明
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        PSM
      </lark-td>
      <lark-td>
        用于接收babi账单的`psm`，不会限制请求必须来自此psm（建议联系RD来获取自己服务的psm）
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        服务部署位置
      </lark-td>
      <lark-td>
        使用该接入账号的区域，例如你将从sg请求ModelHub，那就选择【SG】
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        接入账号
      </lark-td>
      <lark-td>
        需要填写自定义部分来保证唯一性
      </lark-td>
    </lark-tr>
  </lark-table>

1. 点击对应接入账号的【模型接入】，填写表单后点提交

  <image token="Fip3b2S00obvWsxf0pfcr5AjnMg" width="4172" height="2402" align="center"/>

三种场景要填写的表单字段不完全相同：

<grid cols="3">

  <column width="33">

    外部厂商-正式

    <image token="B5tAbx8QVosnyDxyKtmcPbNInbb" width="4168" height="3366" align="center"/>

  </column>

  <column width="33">

    外部厂商-试用

    <image token="WUvAbceUCoS0EVxz1Bdc9a5Nn3c" width="4178" height="2404" align="center"/>

    
  </column>

  <column width="33">

    字节云方舟

    <image token="LUIkbdjIpoVeX5xstRzcbh2WnHe" width="4174" height="2378" align="center"/>

  </column>

</grid>

<lark-table rows="12" cols="4" header-row="true" column-widths="250,303,163,208">

  <lark-tr>
    <lark-td>
      字段 \ 表单
    </lark-td>
    <lark-td>
      **外部厂商-正式**
    </lark-td>
    <lark-td>
      **外部厂商-试用**
    </lark-td>
    <lark-td>
      **字节云方舟**
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      模型
    </lark-td>
    <lark-td colspan="3">
      需要访问的模型。注意如果你需要的模型是灰色的无法选择，请先参考下面的[ 补充步骤：新增厂商](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%3FcontentTheme%3DDARK%26last_doc_message_id%3D7611489077619772381%26preview_comment_id%3D7611488260807789521%26sourceType%3Dfeed%26theme%3Dlight%23F9B3dMwLYo9wkfxH3nicc7SLnww) 一节来走对应厂商的合规审批流程
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      你的 QPM(Queries Per Minute)
    </lark-td>
    <lark-td colspan="3">
      预估QPM（每分钟请求数）
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      给出 QPM 的预估逻辑
    </lark-td>
    <lark-td colspan="3">
      QPM（每分钟请求数）的预估逻辑
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      平均单次请求的prompt_tokens
    </lark-td>
    <lark-td>
      请求的平均prompt token长度，用于系统自动计算TPM
    </lark-td>
    <lark-td>
      -
    </lark-td>
    <lark-td>
      -
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      平均单次请求的completion_tokens
    </lark-td>
    <lark-td>
      请求的平均completion token长度，用于系统自动计算TPM
    </lark-td>
    <lark-td>
      -
    </lark-td>
    <lark-td>
      -
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      平均单次请求的`max_tokens`
    </lark-td>
    <lark-td>
      请求传入的max_tokens，用于系统自动计算TPM
    </lark-td>
    <lark-td>
      -
    </lark-td>
    <lark-td>
      -
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      你的 TPM(Tokens Per Minute)
    </lark-td>
    <lark-td>
      -
    </lark-td>
    <lark-td>
    </lark-td>
    <lark-td>
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      系统计算 TPM(Tokens Per Minute)
    </lark-td>
    <lark-td>
      系统自动计算出的TPM
    </lark-td>
    <lark-td>
      -
    </lark-td>
    <lark-td>
      -
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      使用频率
    </lark-td>
    <lark-td>
      使用频率
    </lark-td>
    <lark-td>
      -
    </lark-td>
    <lark-td>
      -
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      全年预估使用天数
    </lark-td>
    <lark-td>
      每年预估有多少天在访问API
    </lark-td>
    <lark-td>
      -
    </lark-td>
    <lark-td>
      -
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      预算单元
    </lark-td>
    <lark-td>
      预算单元根据模型区域和PSM自动匹配

      预算单元将用于匹配业务POC人员进行审批,建议向自己所在E业务咨询预算单元。若有其他问题,请发起Oncall处理。
    </lark-td>
    <lark-td>
      -
    </lark-td>
    <lark-td>
      -
    </lark-td>
  </lark-tr>
</lark-table>

<lark-table rows="7" cols="2" header-row="true" column-widths="168,665">

  <lark-tr>
    <lark-td>
      术语
    </lark-td>
    <lark-td>
      说明
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **Tokens**
    </lark-td>
    <lark-td>
      Tokens 是大模型处理文本的基本单位。一个 token 可以是一个单词、一个字符、或一个音节，具体取决于模型的分词方式。平台的所有计费和用量限制都与 Tokens 相关。

      - **输入 Tokens**：您发送给模型的文本（如 Prompt、上下文）所包含的 Tokens 数量。

      - **输出 Tokens**：模型为您生成的文本（如回答、代码）所包含的 Tokens 数量。
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **TPM**

      (Tokens Per Minute)
    </lark-td>
    <lark-td>
      每分钟处理的总 Tokens 数量。这是衡量模型吞吐量的核心指标，限制了您在一分钟内可以发送给模型并由模型生成的 Tokens 总和。TPM 是一个比 QPM 更精确的用量控制单位，因为它同时考虑了请求频率和请求的文本长度。
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **QPM**

      (Queries Per Minute)
    </lark-td>
    <lark-td>
      每分钟的请求（查询）次数。该指标限制了您在一分钟内可以向模型 API 发起调用的总次数，不论每次调用的文本量大小。当 TPM 和 QPM 同时设置时，任一指标达到上限都会触发限流。
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **max_tokens**
    </lark-td>
    <lark-td>
      一个 API 请求参数，用于限制模型单次调用返回内容（输出 Tokens）的最大长度。合理设置此参数可以控制成本和返回内容的简洁性。

      **默认值说明：**

      - **GPT 系列模型**：若不指定，默认为 `1000`。

      - **Gemini 系列模型**：没有默认值，必须显式指定 `max_tokens`，否则请求可能会失败。
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **Paygo**

      (Pay-as-you-go)
    </lark-td>
    <lark-td>
      按量计费模式。这是一种基于实际使用量（消耗的 Tokens 数量）进行计费的资源模式。此模式下，您将与其他用户共享厂商的公共资源池，不保证资源的稳定性和性能，适合用量波动较大或对性能要求不高的场景。
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **PTU**

      (Provisioned Throughput Unit)
    </lark-td>
    <lark-td>
      预留吞吐量单元，即专属资源/集群。这是一种包月计费模式，为您提供专用的模型处理能力，厂商保障资源的稳定性和性能 SLA。适合对性能、稳定性有高要求或用量可预测的大规模业务场景。
    </lark-td>
  </lark-tr>
</lark-table>

1. 补充步骤：新增厂商
<callout emoji="exclamation" background-color="light-orange" border-color="light-orange">

如果【模型接入】表单中选择模型时模型为灰色无法选中，说明场景接入时，没有勾选该模型所属的协议厂商，如下图所示，可以点击【新增厂商】来完成对应的合规审批流程
</callout>

<image token="V9gKbyRL3oygV2xPuaPc6N3fn6c" width="2692" height="1726" align="center"/>

勾选需要新增的厂商，并填写对应表单，字段说明请参考[这里](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23YFn8dqWvJoWYRkx8ovXccQTQnvc)

<image token="PEvJb5imqojy1jxQMADcqQGyned" width="4168" height="2404" align="center"/>

1. 审批通过后，可以点击【模型接入详情】查看该接入账号下已经申请的模型

  <image token="C0lIbe09KoWqI7xDMhgcj08FnNb" width="2678" height="1724" align="center"/>

  <image token="IA8HbitDQoGlBixt2ONcvwUHnBd" width="2686" height="1728" align="center"/>

1. 如果已接入模型，需要扩缩容，需要在模型接入详情页面中点击对应模型的【扩容】或者【缩容】按钮申请扩缩容即可

  <image token="Dg7bbSvlPoMKjYx4RiAcOqw4nlh" width="4180" height="2378" align="center"/>

<grid cols="2">

  <column width="50">

    扩容

    <image token="MNi1bldSmo8U79xD0hEcgQMWnwc" width="2674" height="1720" align="center"/>

  </column>

  <column width="50">

    缩容

    <image token="G3KSbBSNYogiRxxmWBycR39rnVb" width="2686" height="1726" align="center"/>

  </column>

</grid>

1. 确认是否需要使用PTU

<grid cols="2">

  <column width="50">
    <callout emoji="💡" background-color="light-gray" border-color="gray">

    **Paygo（Pay-as-you-go，按量计费)**

    - **特点**：按实际使用 Tokens 计费，无需预投入。

    - **资源**：使用厂商的**公共资源池**。

    - **SLA**：厂商**不保障**稳定性和性能。

    - **适合场景**：

      - 业务初期，用量波动大

      - 对性能和稳定性不敏感的离线任务

      - 成本优先的探索性项目
    </callout>

  </column>

  <column width="50">
    <callout emoji="💡" background-color="light-gray" border-color="gray">

    **PTU (专属资源)**

    - **特点**：包月固定费用，获取专用处理能力。

    - **资源**：厂商分配的**专属物理集群**。

    - **SLA**：厂商**保障**稳定性和性能。

    - **适合场景**：

      - 核心线上业务，对稳定性要求高

      - 可预测的大规模、高并发请求

      - 对延迟敏感的实时交互应用
    </callout>

  </column>

</grid>

可以通过下图确认是否要使用PTU

<whiteboard token="E0OswtTqNh7nhPbO1EVcZbzFnaf" align="left"/>

各厂商对PTU的支持情况如下，如果确认要使用PTU，点击 [申请PTU](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FFEbnwYza1iFqmSkCjlwc8wtYnbf)

<reference-synced source-block-id="J3wcdfuovsygqtbnJr0cjdQ4npd" source-document-id="TuepdL16Bo4mfAxD5QEcEiNwnPd">

  <lark-table rows="7" cols="4" column-widths="328,240,360,335">

    <lark-tr>
      <lark-td>
        厂商/资源类型
        <quote-container>

        业务申请用量占字节整体用量50%以内，ModelHub会直接和厂商沟通协调资源。

        业务申请用量占字节整体用量50%以上，ModelHub会和商务一起与厂商沟通协调资源。
        </quote-container>
      </lark-td>
      <lark-td>
        普通资源(paygo)
      </lark-td>
      <lark-td>
        专属资源(PTU)
      </lark-td>
      <lark-td>
        Priority 普通资源
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        资源特点
      </lark-td>
      <lark-td>
        按量计费，使用全球公共资源。

        <text bgcolor="light-red">**厂商不保障稳定性SLA/性能**</text>
      </lark-td>
      <lark-td>
        包月固定计费，费用与资源量正相关。厂商保障稳定性/性能
      </lark-td>
      <lark-td>
        按量计费，价格是普通资源2倍，厂商说明稳定性会较普通资源更好，但也不保障稳定性/性能
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        azure厂商(GPT模型)
      </lark-td>
      <lark-td rowspan="2">
        - 平台按照历史经验设定业务申请资源上限阈值，确保所有申请业务都能使用
        - 超过上限阈值推荐业务购买PTU资源
      </lark-td>
      <lark-td>
        模型上新后，厂商一般在<text bgcolor="light-red">2-3</text><text bgcolor="light-red">周</text>后提供PTU资源申请

        - 如期望PTU上线后第一时间有资源，<text bgcolor="light-green">**需提前找**</text><text bgcolor="light-green">**ModelHub**</text><text bgcolor="light-green">**平台报备，等资源到位后会使用**</text>

        - PTU 上线初期，全球资源相对紧张，业务需参与资源优先级评估，具体可参考下面的优先级评估细节

        - PTU申请后，会在1-7个工作日内资源到位(非美国公共假期及厂商特殊情况)
      </lark-td>
      <lark-td>
        暂无
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        gcp厂商(Gemini模型)
      </lark-td>
      <lark-td>
        模型上新后，厂商一般会同步提供PTU

        - 新模型发布保密因素，无法预知模型上新时间以及报备申请资源

        - PTU申请后，根据资源申请情况，会在1-7个工作日内资源到位(非美国公共假期及厂商特殊情况)，最快次日到位
      </lark-td>
      <lark-td>
        preview阶段，暂时只有固定quota
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        azure厂商(sora/deepseek等)
      </lark-td>
      <lark-td rowspan="3">
        - 平台按照历史经验设定业务申请资源上限阈值，确保所有申请业务都能使用
        - 业务申请超过阈值资源，需联系<text bgcolor="light-green">**ModelHub**</text>平台，平台和商务/系统部SRE等沟通对齐后，会尽力向厂商申请
      </lark-td>
      <lark-td rowspan="3">
        暂无
      </lark-td>
      <lark-td rowspan="3">
        暂无
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        gcp厂商(veo等)
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        其他厂商(Kimi、GLM、minimax、Qwen等)
      </lark-td>
    </lark-tr>
  </lark-table>

</reference-synced>

## 步骤三：获取密钥与API调用
<callout emoji="💡" background-color="light-green" border-color="light-green">

完成配额申请后，您将获得用于身份认证的 API 密钥（AK），这是调用模型的唯一凭证。此步骤的核心是**获取密钥**并**参考示例代码**完成 API 调用。平台为不同模型和编程语言提供了详细的代码示例，您可以直接使用或稍作修改，即可快速将大模型能力集成到您的应用程序中。
</callout>

### 各区域域名
<callout emoji="exclamation" background-color="light-orange" border-color="light-yellow">

**域名访问排错提示**

- **问题：为什么从办公网（本机）访问 SG/US-TTP/EU-TTP 等区域的域名，会显示“无法访问”或 404？**

  - **答：这是正常现象。** 出于合规要求，绝大部分生产环境域名（如 `aidp.tiktokd.net`）仅支持从其所在区域的服务器内部进行调用，无法从办公网络直接访问。

- **问题：我需要在办公网调试非国内区的模型，应该用哪个域名？**

  - **答**：对于新加坡（SG）区域，我们提供了专门的**OG 域名** (`aidp-i18ntt-sg.tiktok-row.net`) 供办公网调试使用。请在开发或测试时使用此域名。其他 TTP 区域目前没有提供 OG 域名。

- **总结**：在服务器上部署时，请使用生产域名；在本地开发调试时，请使用对应的 OG 域名（如有）。
</callout>

<source-synced align="1">

  <lark-table rows="7" cols="4" header-row="true" column-widths="161,100,238,320">

    <lark-tr>
      <lark-td>
        支持区域
      </lark-td>
      <lark-td>
        环境
      </lark-td>
      <lark-td>
        域名
      </lark-td>
      <lark-td>
        说明
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        **国内（CN）**
      </lark-td>
      <lark-td>
        生产/办公网
      </lark-td>
      <lark-td>
        [aidp.bytedance.net](https%3A%2F%2Fcloud.bytedance.net%2Fnetlink%2Fv2%2Fmain%2Fbusiness%2Fchildren%2Fnetlink%2Fmain%2Fnamespace%2Finsert-manage%3Fdomain_search%3D%257B%2522fuzzySearch%2522%253A%2522aidp.byte%2522%252C%2522compositionSearch%2522%253A%257B%257D%257D%26show_type%3Ddomain%26tdm_domain%3Daidp.bytedance.net%26tdm_servername%3D12208%26tdm_visible%3D1%26ti_business_id%3D10082692%26tlb_tab%3Dticket%26x-bc-region-id%3Dbytedance%26x-resource-account%3Dpublic)
      </lark-td>
      <lark-td>
        办公网络环境和 CN 生产环境均可访问
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td rowspan="2">
        **新加坡（SG）**
      </lark-td>
      <lark-td>
        生产
      </lark-td>
      <lark-td>
        [aidp-i18ntt-sg.byteintl.net](https%3A%2F%2Fcloud.tiktok-row.net%2Fnetlink%2Fv2%2Fmain%2Fbusiness%2Fchildren%2Fnetlink%2Fmain%2Fnamespace%2Finsert-manage%3Fshow_type%3Ddomain%26tdm_domain%3Daidp-i18ntt-sg.byteintl.net%26tdm_visible%3D1%26ti_business_id%3D10171271%26x-bc-region-id%3Dbytedance)
      </lark-td>
      <lark-td>
        仅限部署在 SG 生产环境内的服务进行调用，无法从办公网直接访问。
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        办公网
      </lark-td>
      <lark-td>
        [<text color="blue">aidp-i18ntt-sg.tiktok-row.net</text>](https%3A%2F%2Fcloud.tiktok-row.net%2Fnetlink%2Fv2%2Fmain%2Fbusiness%2Fchildren%2Fnetlink%2Fmain%2Fnamespace%2Finsert-manage%3Fsdmwc_domain%3Dlabel.bytedance.net%26sdmwc_selecting_servername_id%3D5979%26show_type%3Ddomain%26tdm_domain%3Daidp-i18ntt-sg.tiktok-row.net%26tdm_servername%3D42905%26tdm_visible%3D1%26ti_business_id%3D10171271%26x-bc-region-id%3Dbytedance%26x-resource-account%3Di18n)
      </lark-td>
      <lark-td>
        仅供在**办公网络**环境下进行测试或调试时使用。
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        **NonTT**
      </lark-td>
      <lark-td>
        生产/办公网
      </lark-td>
      <lark-td>
        [aidp.byteintl.net](https%3A%2F%2Fcloud.byteintl.net%2Fnetlink%2Fv2%2Fmain%2Fbusiness%2Fchildren%2Fnetlink%2Fmain%2Fnamespace%2Finsert-manage%3Fsdmwc_domain%3Daidp.byteintl.net%26sdmwc_selecting_servername_id%3D40120%26sdmwc_visible%3D1%26show_type%3Ddomain%26ti_business_id%3D10171271%26x-bc-region-id%3Dbytedance)
      </lark-td>
      <lark-td>
        办公网络环境和 NonTT 生产环境均可访问
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        **美国（US-TTP）**
      </lark-td>
      <lark-td>
        生产
      </lark-td>
      <lark-td>
        [aidp.tiktokd.net](https%3A%2F%2Fcloud-ttp-us.bytedance.net%2Fnetlink%2Fv2%2Fmain%2Fbusiness%2Fchildren%2Fnetlink%2Fmain%2Fnamespace%2Finsert-manage%3Fshow_type%3Ddomain%26tdm_domain%3Daidp.tiktokd.net%26tdm_visible%3D1%26ti_business_id%3D14109765)
      </lark-td>
      <lark-td>
        仅限部署在 US-TTP 生产环境内的服务进行调用，无法从办公网直接访问。
      </lark-td>
    </lark-tr>
    <lark-tr>
      <lark-td>
        **欧洲（EU-TTP/GCP）**
      </lark-td>
      <lark-td>
        生产
      </lark-td>
      <lark-td>
        [aidp.tiktoke.org](https%3A%2F%2Fcloud-eu.tiktok-row.net%2Fnetlink%2Fv2%2Fmain%2Fbusiness%2Fchildren%2Fnetlink%2Fmain%2Fnamespace%2Finsert-manage%3Fshow_type%3Ddomain%26tdm_domain%3Daidp.tiktoke.org%26tdm_servername%3D21613%26tdm_visible%3D1%26ti_business_id%3D14109765)
      </lark-td>
      <lark-td>
        仅限部署在 EU-TTP 生产环境内的服务进行调用，无法从办公网直接访问。
      </lark-td>
    </lark-tr>
  </lark-table>

</source-synced>

### 在线API访问方式

**创建密钥**

1. 点击【AK管理】

<image token="SY2IbpCpboykEVxROqucguyLnBf" width="2700" height="1458" align="center"/>

1. 点击【创建AK】即可完成AK的快速创建

<image token="MHVbbJwTVoklGIxnG3AcSROTnoh" width="2688" height="1434" align="center"/>

**参考平台示例代码进行模型调用**

1. 进入模型接入详情页

<image token="PyoubWBSroV8P9x9PLqchFFnnjg" width="2698" height="1516" align="center"/>

1. 点击想要访问的模型的示例代码，参考示例代码接入

<image token="BA7mbCZdsoHT8GxEGAJc9EzonBc" width="4352" height="2090" align="center"/>

<callout emoji="thought_balloon" background-color="gray" border-color="gray">

**常见问题**

- **问：我想用的模型没有提供示例代码怎么办？**

  - **答**：部分新上线或小众模型可能暂时缺少平台统一的示例代码。您可以：

    1. **查看官方文档**：直接参考模型提供方的官方 API 文档进行调用。

    1. **联系我们**：通过 [Oncall](https%3A%2F%2Fcloud.bytedance.net%2Foncall%2Fchats%2Fuser%2Findex%3Fx-resource-account%3Dpublic%26x-bc-region-id%3Dbytedance%26region%3Dcn%26tenantId%3D6074%26step%3Dself-solve) 或在用户群中反馈，我们会评估并提升补充示例代码的优先级。

- 我们也欢迎您分享自己的模型接入实践，共建更完善的示例库。
</callout>

<image token="Gxrkbg6LnoUH9qxQ95icJMeenhb" width="2658" height="1710" align="center"/>

# 四、进阶功能与第三方集成

以下能力适用于已经完成基础接入的业务。你可以根据目标选择对应能力：

- 想降成本、提性能：看【[成本与性能](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-P5ocd84M3o3FBhxMPvNcyGx4npd)】

- 想扩展检索、协议能力：看【[能力扩展](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-DePUdabL9o3djOxesp5cqD8UnNh)】

- 想做模型定制：看【[模型优化](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-AFBZd0RZFo6muXxMtNhcRsXtnCc)】

- 想在三方 agent 或开发工具中使用 ModelHub：看【[三方 agent 对接](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FwikcnUPXCY2idGyg2AXKPvay4pd%23share-QGNRdnADkohKvqxWliOcUYfsn8c)】

<lark-table rows="5" cols="2" header-row="true" column-widths="218,862">

  <lark-tr>
    <lark-td>
      功能
    </lark-td>
    <lark-td>
      使用说明
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **成本与性能**
    </lark-td>
    <lark-td>
      **Prompt Cache**

      通过缓存策略优化接口响应时间，降低API调用成本。参考：

      <mention-doc token="ODGmwD8MliLaaMkQUgJcb1W1nIi" type="wiki">ModelHub—使用Prompt Cache功能说明</mention-doc>

      **离线****Batch API访问方式**

      部分GPT模型提供离线批处理能力，参考：

      <mention-doc token="VhtIwOGNJil4gNkLWorcAJz1nef" type="wiki">ModelHub—离线Batch API功能说明</mention-doc>

      **精准Cache功能**

      ModelHub现已支持精准cache功能，针对完全重复请求不用再实际请求大模型，而是使用缓存的结果，以减少业务方成本和提升响应速度，目前只支持非多模态模型chat/completion接口，详情请看：

       [ModelHub 精准cache功能用户手册](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FEzukwouTXi7uaSkGIKucpPGrn9b)
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **能力扩展**
    </lark-td>
    <lark-td>
      **Responses API**

      OpenAI提供的新协议API，相比Chat Completions API支持更多能力：

      <mention-doc token="JyLQwnrJFiczHyklYiFccYebnFf" type="wiki">ModelHub—Azure Responses API</mention-doc>

      **Search API******

      Search API 适用于需要让模型获取实时外部信息的场景，例如网页搜索、新闻搜索、图片搜索等。

      目前平台支持接入 You 和 Brave 两种搜索能力，业务可根据效果、性能、成本和支持响应情况选择。

      <mention-doc token="GhJtwljajieoEdk4BpscEhNhnFd" type="wiki">ModelHub—You Search接入</mention-doc>

      <mention-doc token="OBqOwvkg2in6P8k5XMRcSQjtnth" type="wiki">ModelHub—Brave Search接入</mention-doc>

      <source-synced align="1">
        <lark-table rows="8" cols="3" header-row="true" column-widths="153,315,335">
          <lark-tr>
            <lark-td>
              对比项
            </lark-td>
            <lark-td>
              You
            </lark-td>
            <lark-td>
              Brave
            </lark-td>
          </lark-tr>
          <lark-tr>
            <lark-td>
              能力差异
            </lark-td>
            <lark-td>
              支持web search、image search、news search、smart search（ai搜）
            </lark-td>
            <lark-td>
              支持web search、image search、news search、video search
            </lark-td>
          </lark-tr>
          <lark-tr>
            <lark-td>
              搜索效果
            </lark-td>
            <lark-td>
              搜索效果好
            </lark-td>
            <lark-td>
              搜索效果中等
            </lark-td>
          </lark-tr>
          <lark-tr>
            <lark-td>
              稳定性
            </lark-td>
            <lark-td>
              SLA > 99.9%
            </lark-td>
            <lark-td>
              SLA > 99.9%
            </lark-td>
          </lark-tr>
          <lark-tr>
            <lark-td>
              技术支持
            </lark-td>
            <lark-td>
              厂商有专人支持，有飞书群，响应较快(1天内)
            </lark-td>
            <lark-td>
              厂商有专人支持，有飞书群，响应较慢(2天内)
            </lark-td>
          </lark-tr>
          <lark-tr>
            <lark-td>
              性能
            </lark-td>
            <lark-td>
              单次请求平均耗时：1.2s
            </lark-td>
            <lark-td>
              单次请求平均耗时：0.6s
            </lark-td>
          </lark-tr>
          <lark-tr>
            <lark-td>
              目前使用量
            </lark-td>
            <lark-td>
              高
            </lark-td>
            <lark-td>
              低
            </lark-td>
          </lark-tr>
          <lark-tr>
            <lark-td>
              成本
            </lark-td>
            <lark-td>
              高
            </lark-td>
            <lark-td>
              低
            </lark-td>
          </lark-tr>
        </lark-table>
      </source-synced>
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **模型优化**
    </lark-td>
    <lark-td>
      **模型SFT/DPO**

      <mention-doc token="NsZuw7TRPiQF7ZknruqcWE3Wnlf" type="wiki">ModelHub—模型微调功能支持说明</mention-doc>
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      **使用三方agent对接**<text bgcolor="light-green">**ModelHub**</text>
    </lark-td>
    <lark-td>
      **Clawdbot / Moltbot OpenClaw接入**

      <mention-doc token="UXEywm07mimHSykb0oNc9kTKnae" type="wiki">Clawdbot / Moltbot / OpenClaw接入ModelHub模型</mention-doc>

      **Codex / Claude Code接入**

      <mention-doc token="OOYBdDO2MoSU7nxb8iLcQAWMnPf" type="docx">如何在 Claude Code or  Codex 上使用其他模型</mention-doc> 

      感谢 <mention-user id="ou_6ca1fe121f02d2f22e7dc4d9e72f43d7"/>贡献的教程

      <add-ons component-id="" component-type-id="blk_6358a421bca0001c1ce11f5f" record="{"config":{"afterText":null,"beforText":null,"color":"RED","icon":"LIKE","readType":0,"selectVal":4}}"/>

      **Opencode接入**

      <text bgcolor="light-green">**ModelHub**</text> 的 opencode provider <mention-doc token="LFicwPze7iFVwpkSIEqcg1cznad" type="wiki">OpenCode 对接内部模型</mention-doc> 仓库  [[Repository] bytedance/opencode-modelhub-provider](https%3A%2F%2Fcode.byted.org%2Fbytedance%2Fopencode-modelhub-provider)

      感谢<mention-user id="ou_6ad7e77fdff45230cac159a004b5ad52"/>贡献的支持

      <add-ons component-id="" component-type-id="blk_6358a421bca0001c1ce11f5f" record="{"config":{"afterText":null,"beforText":null,"color":"RED","icon":"LIKE","readType":0,"selectVal":4}}"/>
    </lark-td>
  </lark-tr>
</lark-table>

# 五、**如何追踪请求与监控用量**

## 如何使用 Trace 排查问题

1. 登录trace平台：https://aidp.bytedance.net/modelhub/fornax_panel，选择要查看的场景和用户名

<image token="TVJwblaHooppIox6DiNcZ14snuh" width="4344" height="2400" align="center"/>

1. 选择时间范围

<image token="PBYubYWifo8wFVxHIAScXGoVn3g" width="4392" height="2376" align="center"/>

1. 在过滤器中设置过滤条件，支持：

  1. 按【input（输入内容）】模糊搜索

  1. 按【Logid】精确搜索

  1. 按【LatecyFirstResp（请求耗时）】范围搜索，例如在排查延迟升高问题时，可筛选出耗时超过5分钟的请求

<image token="FIhUbt704oPIXkxyp2jc5ilDnPG" width="4346" height="2394" align="center"/>

## 如何使用监控看板查看流量情况

在【模型接入详情】中，每个模型都可以点击查看【监控看板】，监控看板详细说明，参考：<mention-doc token="ONPRw5PWXieI99k12bCc3BQUnIb" type="wiki">ModelHub 模型监控看板</mention-doc>

<image token="PmhObcamZoxquex0zo6cbywtnyc" width="2682" height="1728" align="center"/>

# 六、专属资源（PTU）与成本管理

## 独立PTU

普通账号quota限制较低，性能稳定性差，如果需要的quota量较大或者对性能稳定性要求较高，建议 [申请PTU](https%3A%2F%2Fbytedance.larkoffice.com%2Fwiki%2FFEbnwYza1iFqmSkCjlwc8wtYnbf%23share-YWdAdInL3oegQWxDc2jcuHMHn9b)。PTU的费用、限流规则和更多说明请参考：

平台提供计算器，可以计算相关费用：https://aidp.bytedance.net/modelhub/cost

仅参考，以计算器为准：<mention-doc token="FEbnwYza1iFqmSkCjlwc8wtYnbf" type="wiki">ModelHub —模型quota上限\费用\PTU申请方式\限流规则</mention-doc> 

## 账单推送和查询

账单会通过babi结算推送到psm对应的部门 <mention-user id="ou_6a918e20683348c514037efd83558ecd"/>

1. 费用情况可以在babi平台（https://babi.bytedance.net/finance/index/）查看。由于babi有权限限制，不对所有同学开放成本，但可以申请某个服务树的权限，单独查看关注的服务树下的成本。

1. 申请方法：

  1. 根据接入 ModelHub 时提供的 PSM 找到对应的服务树，在[bytetree](https%3A%2F%2Fcloud.bytedance.net%2Fbytetree%2Fservice%2F4844044%2Foverview)申请对应服务树的babi成本管理员权限。

  1. 申请通过后，就可以在babi查看相应节点的成本了。在平台上选择「收支管理」->「服务树成本」，再搜索关注的服务树节点，选择要查询的账期，根据区域选择大区选项，选择「纯资源成本」。就可以按成本概览、成本商品、成本服务树、成本明细等多维度进行下钻分析。

  1. 成本来源（商品）选择 `GPT_openapi`。

## 普通账号费用说明
<callout emoji="speech_balloon" background-color="light-blue" border-color="light-blue">

**关于计费单位**

本平台所有服务的计费单位均为 **美元（USD）**。最终结算将依据您所在团队的内部账单系统进行处理。在查看各区域模型的定价时，请注意区分国内和海外区域的特定说明，避免混淆。
</callout>

均为实际使用的token数量。如无特殊说明，计费单位均为美元，参考 <mention-doc token="HcXSwkmYniQiM2kWFVxcLQlqndh" type="wiki">GPT-OpenAPI 模型列表</mention-doc> 中「价格」一列

- GPT模型图像计费规则：https://platform.openai.com/docs/guides/vision/calculating-costs

## 通过API查询每日费用

具体可以参考<mention-doc token="Mg8HdyR76oWE4MxA9Lvc2E1kndc" type="docx">Gpt openapi支持api/hive查询账单</mention-doc>，以该文档为准

简单介绍如下：

**普通账号用量**可以通过hive表查询：

1. 打开 [cn hive](https%3A%2F%2Fdata.bytedance.net%2Fcoral%2Fdatamap%2Fdetail%3Ffrom%3Dcoral_copy_link%26groupName%3Ddefault%26qualifiedName%3DHiveTable%253A%252F%252F%252Fgpt_openapi_cost_cn_hive%252Fgpt_openapi_cost_detail_dict%25400%23group%3Ddefault)， 点击权限申请：

<image token="Xl0PbxEkPoPuClxM16tcgqITnag" width="4346" height="338" align="center"/>

1. 按ModelHub用户名维度申请权限：

<image token="QSt8bBQi1ovmM3x1ZS9ccolInCf" width="4348" height="2386" align="center"/>

1. 使用条件date = 'yyyy-MM-hh'（例如2026-03-19) and user = '你的用户名' 在上述hive表中查询每日费用/用量

**PTU账号用量**可以通过下列 hive表查询：

表内date是分区， usage_date是实际用量日期，**中国请求是UTC+8时区，海外请求是UTC+0时区**

CN和VA均有全球数据，任选其一申请即可

CN：[cn hive](https%3A%2F%2Fdata.bytedance.net%2Fcoral%2Fdatamap%2Fdetail%3Ffrom%3Dcoral_copy_link%26groupName%3Ddefault%26qualifiedName%3DHiveTable%253A%252F%252F%252Fgpt_openapi_cost_cn_hive%252Fgpt_openapi_usage_cn%25400%23group%3Ddefault)

VA： [va hive](https%3A%2F%2Fdataleap-va.tiktok-row.net%2Fcoral%2Fdatamap%2Fdetail%3Ffrom%3Dcoral_copy_link%26groupName%3Di18n%26qualifiedName%3DHiveTable%253A%252F%252F%252Fva_gpt_openapi_cost_hive%252Fgpt_openapi_usage_hive%25401%23group%3Di18n)

# 七、问题排查与处理（FAQ）

### 模型调用常见错误与处理方法

- <mention-doc token="KqZywS3zPi4mPtkQWiMcE8fFnwg" type="wiki">ModelHub—Oncall&FAQ</mention-doc> 

- <mention-doc token="CtJVsFbdnhrEjEt3ezWcAR3lnjc" type="sheet">GPT-OpenAPI平台错误码信息 Error Code</mention-doc>

# 八、联系我们

1. 用户群

<chat-card id="oc_d79dc429421631e29909a6a288b34832" name="AIDP ModelHub用户群"/>

<chat-card id="oc_888bce2dabe4288438d0e896689d71d1" name="AIDP ModelHub用户群 2"/>

1. 提oncall

https://cloud.bytedance.net/oncall/chats/user/index?x-resource-account=public&x-bc-region-id=bytedance&region=cn&tenantId=6074&step=self-solve
<callout emoji="exclamation" background-color="light-gray" border-color="gray">

**内容时效性提醒**

本手册内容会随平台功能迭代而持续更新。若手册描述与平台实际功能存在不一致，请以平台最新公告或通过 Oncall 渠道咨询确认为准。
</callout>

