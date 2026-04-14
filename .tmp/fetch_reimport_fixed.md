> 🔗 原文链接： [https://mp.weixin.qq.com/s/dJitOPF5...](https%3A%2F%2Fmp.weixin.qq.com%2Fs%2FdJitOPF5wM4ticTxTocvKA)
>
> ⏰ 剪存时间：2026-03-30 14:30:09 (UTC+8)
>
> ✂️ 本文档由 [飞书剪存 ](https%3A%2F%2Fwww.feishu.cn%2Fhc%2Fzh-CN%2Farticles%2F606278856233%3Ffrom%3Din_ccm_clip_doc)一键生成

原创 劲松 Leo 劲松 Leo BeforeAGI*2026年3月29日 16:48*

# Harness Engineering：

**最近AI圈最火的新概念，**

**到底在说啥？** {align="center"}

<text color="gray">一文读懂：从Prompt Engineering到Harness Engineering的进化之路</text> {align="center"}

如果你关注AI领域，最近一定被一个词刷屏了：

<text color="orange">Harness Engineering（驾驭工程）</text> {align="center"}

<text color="gray">*Harness 直译为马具我觉得也挺传神的*</text> {align="center"}

Anthropic、OpenAI、LangChain 等大厂纷纷发文，大佬们激动地说： <text color="blue">"这是AI工程的第三次范式跃迁！"</text>

听起来很厉害对不对？但到底是什么意思呢？

别急，今天这篇文章，我用 最通俗的语言 ，带你从头搞懂。

## 先讲个故事：你新招了个超强实习生

想象下，你的公司招了个 天赋异禀 的实习生：

📚 他读过所有教科书，知识面极广

⚡ 他打字速度飞快，一天能写别人一个月的代码

🧠 他理解力超强，你说什么他都能快速上手

但问题来了：

😅 他不了解你们公司的规矩和代码规范

🤷 他有时会"自由发挥"，写出你根本不想要的

🔄 他犯了一个错，如果你不说，他会反复犯

💥 他做事太快了，一旦方向错了，会错上加错

这个"超强实习生"，就是今天的 **AI Agent。**

那么问题来了： 你要怎么管理这个实习生？

<text color="gray">🤔 是每次都口头叮嘱他注意事项？ </text>

<text color="gray">（这就是 Prompt Engineering ）</text>

<text color="gray">🤔 是把项目资料整理好给他看？ </text>

<text color="gray">（这就是 Context Engineering ）</text>

<text color="gray">🤔 还是给他搭一套完整的工作环境——规范手册、代码检查工具、自动化测试、定期复盘机制？ </text>

<text color="gray">（这就是 Harness Engineering ）</text>

看到了吗？ <text color="orange">Harness Engineering 解决的核心问题，不是"让 AI 更聪明"，而是"让 AI 更可靠"。</text>

## 三代AI工程范式：进化之路

在解释 Harness Engineering 之前，我们先回顾一下 AI 工程是怎么一步步演进过来的：

<text color="gray" bgcolor="blue">**2023 ~ 2024**</text>

<text bgcolor="light-gray">**第一代：Prompt Engineering（提示词工程）**</text>

<text bgcolor="light-gray">核心问题： </text><text bgcolor="light-gray">*怎么把话说清楚？*</text>

<text bgcolor="light-gray">你可能经历过——同一个问题，换个问法，ChatGPT 给出的答案天差地别。那个时代，大家都在研究"怎么写出更好的提示词"。</text>

# <text color="gray">⬇️</text> {align="center"}

<text color="gray" bgcolor="orange">**2025**</text>

**第二代：Context Engineering（上下文工程）**

核心问题： *怎么给 AI 喂正确的信息？*

大家发现，光靠提示词不够，还要把相关的文档、数据、背景知识整理好"喂"给 AI。RAG（检索增强生成）就是这个时代的代表产物。

# <text color="gray">⬇️</text> {align="center"}

<text color="gray" bgcolor="green">**2026 🔥**</text>

**第三代：Harness Engineering（驾驭工程）**

核心问题： *怎么让 AI Agent 可靠地、稳定地、不翻车地工作？*

当 AI 不再只是"回答问题"，而是真正上手写代码、做决策、执行任务时，整个游戏规则都变了。

我们用一个更生动的比喻来理解这三代的区别：

假设 AI 是一匹马 🐴：

<image token="BX1kb9xnXoDeLQxtL0icbvECnMf" width="1080" height="1069" align="center"/>

<text color="gray" bgcolor="blue">类比理解</text>

<text bgcolor="light-gray">🗣️ Prompt Engineering = 对马喊话的技巧 </text>

<text color="gray" bgcolor="light-gray">研究怎么下指令，马才能听懂、跑对方向</text>

<text bgcolor="light-gray">🗺️ Context Engineering = 给马看的地图 </text>

<text color="gray" bgcolor="light-gray">把路线规划好、标注好，让马知道该往哪跑</text>

<text bgcolor="light-gray">🛣️ Harness Engineering = 修一条高速公路，装上护栏、限速牌和加油站 </text>

<text color="gray" bgcolor="light-gray">不管马跑多快，都有护栏防止它冲出去，有路标引导方向，有加油站续航</text>

一句话总结：

<text color="orange">Prompt 管的是"说什么"， </text>

<text color="orange">Context 管的是"看什么"， </text>

<text color="orange">Harness 管的是"整个跑道怎么建"。</text> {align="center"}

下面这张对比表，让你一眼看清三者的区别：

<lark-table rows="6" cols="4" column-widths="183,183,183,183">

  <lark-tr>
    <lark-td>
      <text color="gray" bgcolor="blue">维度</text> {align="center"}
    </lark-td>
    <lark-td>
      <text color="gray" bgcolor="blue">Prompt提示词</text> {align="center"}
    </lark-td>
    <lark-td>
      <text color="gray" bgcolor="blue">Context上下文</text> {align="center"}
    </lark-td>
    <lark-td>
      <text color="gray" bgcolor="blue">Harness驾驭</text> {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      火热年份 {align="center"}
    </lark-td>
    <lark-td>
      2023-2024 {align="center"}
    </lark-td>
    <lark-td>
      2025 {align="center"}
    </lark-td>
    <lark-td>
      2026 {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      优化对象 {align="center"}
    </lark-td>
    <lark-td>
      输入的措辞 {align="center"}
    </lark-td>
    <lark-td>
      输入的信息 {align="center"}
    </lark-td>
    <lark-td>
      运行的环境 {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      核心问题 {align="center"}
    </lark-td>
    <lark-td>
      怎么把话说清楚？ {align="center"}
    </lark-td>
    <lark-td>
      怎么给AI喂信息？ {align="center"}
    </lark-td>
    <lark-td>
      怎么让Agent可靠？ {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      交互模式 {align="center"}
    </lark-td>
    <lark-td>
      一问一答 {align="center"}
    </lark-td>
    <lark-td>
      信息注入→生成 {align="center"}
    </lark-td>
    <lark-td>
      人类掌舵→Agent执行 {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      关注重点 {align="center"}
    </lark-td>
    <lark-td>
      单次对话质量 {align="center"}
    </lark-td>
    <lark-td>
      单次任务质量 {align="center"}
    </lark-td>
    <lark-td>
      系统级长期质量 {align="center"}
    </lark-td>
  </lark-tr>
</lark-table>

## 那 Harness Engineering 到底要做什么？

"Harness"这个词，原意是 <text color="blue">马具 </text>——缰绳、马鞍、嚼子。

所以 Harness Engineering 的核心哲学就八个字：

<text color="orange">人类掌舵，智能体执行 </text>

<text color="yellow">Human Steer, Agent Execute</text> {align="center"}

它不是要削弱 AI 的能力，而是为 AI 打造一套 <text color="blue">"黄金缰绳" </text>——让它跑得又快又稳，不翻车。

具体来说，Harness Engineering 包含 四大核心组件 ，我把它们叫做 <text color="orange">"四根护栏" </text>：

<lark-table rows="2" cols="2" column-widths="365,365">

  <lark-tr>
    <lark-td>
      # <text bgcolor="light-gray">📋</text> {align="center"}

      <text bgcolor="light-gray">**知识管理**</text> {align="center"}

      <text color="gray" bgcolor="light-gray">把公司的规矩、技术标准变成AI能读懂的"新人手册"</text> {align="center"}
    </lark-td>
    <lark-td>
      # 🚧 {align="center"}

      **架构约束** {align="center"}

      <text color="gray">把"口头约定"变成"自动化法律"，AI违规就会被拦截</text> {align="center"}
    </lark-td>
  </lark-tr>
  <lark-tr>
    <lark-td>
      # 🔄 {align="center"}

      **反馈循环** {align="center"}

      <text color="gray">让AI知道自己做对了没有，自动发现并修正错误</text> {align="center"}
    </lark-td>
    <lark-td>
      # <text bgcolor="light-gray">🧹</text> {align="center"}

      <text bgcolor="light-gray">**熵管理**</text> {align="center"}

      <text color="gray" bgcolor="light-gray">定期"打扫卫生"，防止AI产生的混乱越积越多</text> {align="center"}
    </lark-td>
  </lark-tr>
</lark-table>

我们一个个来聊：

### 护栏一：知识管理 📋

问题： AI Agent 不了解你公司的背景、规范和习惯。

解法： 写一份结构化的"速查手册"（业内叫 AGENTS.md），告诉 AI：

📌 我们用的技术栈是什么

📌 代码风格有哪些要求

📌 哪些操作是绝对禁止的

📌 遇到某类问题该怎么处理

就像你给新员工准备的入职手册，但它是专门为 AI 写的，而且要 小巧精炼、按需加载 。

### 护栏二：架构约束 🚧

问题： AI 非常擅长"复制粘贴"——如果代码库里有坏代码，AI 会照着写更多坏代码。

解法： 用自动化工具（比如代码检查器 Linter）来强制执行规则。AI 一旦违规，代码直接无法提交。

<text color="gray">💡 通俗理解： 如果说"知识管理"是公司贴在墙上的规章制度，那"架构约束"就是门禁系统——你不刷卡就进不去，不是靠自觉。</text>

### 护栏三：反馈循环 🔄

问题： AI Agent 做完事后不知道自己做得对不对，有时候还会"自信地宣布大功告成"——但其实一团糟。

解法： 建立自动化的检查机制：

✅ AI 写完代码 → 自动跑测试 → 告诉 AI 哪些通过哪些没通过

✅ 用另一个 AI 来检查这个 AI 的工作（Agent审Agent）

✅ 把错误信号反馈回去，让 AI 自我修正

<text color="gray">💡 通俗理解： 就像老师批改作业后把错题标红还回去，学生改完再交，直到全对为止。</text>

### 护栏四：熵管理 🧹

问题： AI 干活特别快，但"快"意味着技术债务（代码垃圾）也积累得特别快。

解法： 安排一个"清洁工 Agent"在后台定期扫描和清理——

🧹 自动发现过时的文档并更新

🧹 检测偏离规范的代码并标记

🧹 持续进行小规模的"技术债偿还"

<text color="gray">💡 通俗理解： 如果你家每天都做饭但从不洗碗，厨房三天就没法看了。熵管理就是那个"每天顺手洗碗"的习惯。</text>

## 为什么 Harness Engineering 在2026年突然爆火？

两个字： <text color="orange">必要 </text>。

2025 年，AI Agent 已经证明了自己 能干活 。但真正用起来后，大家发现了一个扎心的事实：

<text color="orange">同样的模型，在不同的系统里，表现天壤之别。</text> {align="center"}

举个真实的例子：OpenAI 的 3 人团队用 AI Agent 在 5 个月内写了 100 万行代码 。他们发现：

<text color="gray">📊 仅仅改变了 AI 的"编辑格式"（一种 Harness 优化），性能就提升了 </text><text color="orange">10 倍 </text><text color="gray">！</text>

<text color="gray">模型还是同一个模型，但运行环境的优化带来了天翻地覆的变化。</text>

于是，行业里开始流传一句话：

<text color="orange">模型不是瓶颈， </text>

<text color="orange">模型之外的一切才是。</text> {align="center"}

这也是为什么 Anthropic 喊出了：

<text color="gray">"别等下一代模型了，现在就来做 Harness Engineering！"</text>

## 跟我有什么关系？

如果你是以下任何一种人，Harness Engineering 都跟你有关：

<lark-table rows="1" cols="1" column-widths="730">

  <lark-tr>
    <lark-td>
      开发者/程序员：

      你的角色正在从"写代码的人"变成"设计让 AI 可靠写代码的系统的人"。不会做 Harness 的工程师，可能很快会被会做 Harness 的工程师替代。
    </lark-td>
  </lark-tr>
</lark-table>

<lark-table rows="1" cols="1" column-widths="730">

  <lark-tr>
    <lark-td>
      技术管理者：

      你的团队可能已经在用 AI 写代码了。没有 Harness，AI 写的代码越多，你的技术债越重，系统越混乱。
    </lark-td>
  </lark-tr>
</lark-table>

<lark-table rows="1" cols="1" column-widths="730">

  <lark-tr>
    <lark-td>
      创业者/产品经理：

      选择 AI 产品时，不要只看"用了什么模型"，更要看"有没有做好 Harness"。同样用 GPT-4，有 Harness 和没 Harness 的产品，体验可能差 10 倍。
    </lark-td>
  </lark-tr>
</lark-table>

<lark-table rows="1" cols="1" column-widths="730">

  <lark-tr>
    <lark-td>
      对 AI 感兴趣的普通人：

      理解了这个概念，你就能明白为什么有些 AI 产品用起来很稳、有些则各种翻车—— 大多数问题不在 AI 本身，而在它运行的"环境"。
    </lark-td>
  </lark-tr>
</lark-table>

## 最后，总结

<text color="blue" bgcolor="light-gray">**📌 三句话记住 Harness Engineering**</text>

<text bgcolor="light-gray">• 是什么： 围绕 AI 智能体构建约束、反馈和控制系统的工程方法论</text>

<text bgcolor="light-gray">• 为什么： AI 够聪明了，但不够可靠；模型不是瓶颈，环境才是</text>

<text bgcolor="light-gray">• 怎么做： 知识管理 + 架构约束 + 反馈循环 + 熵管理 = 四根护栏</text>

<text color="orange">**🔑 核心公式**</text>

• Prompt Engineering = 把话说对 → 优化对话

• Context Engineering = 把料备齐 → 优化任务

• Harness Engineering = 把路修好 → 优化系统

AI 的时代不是"谁能调出最好的 Prompt"的竞赛，而是 "谁能建出最稳的系统" 的竞赛。

Harness Engineering 告诉我们一个道理：

<text color="orange">真正厉害的不是拥有最快的马， </text>

<text color="orange">而是能修出最好的路。 🛣️</text> {align="center"}

Prompt 和 Context 的概念很好理解，通过和大模型进行简单问答也能体会到他们的价值和作用， **但 Harness Engineering 需要真正用 Agent 完成复杂的工程时（不一定是写代码，只是写代码比较典型），才能体会到所谓的 Harness 到底是什么，以及为什么需要 Harness Engineering**。

本质上， Harness Engineering 是为我们使用 Agent 完成复杂工程时提供的一个最佳实践，但我认为这种最佳实践也会随着大模型的能力迭代变得越来越简单。

<text color="gray">如果这篇文章让你搞懂了 Harness Engineering， </text>

<text color="gray">欢迎 </text><text color="gray">**关注我**</text><text color="gray">， 点赞、在看、转发 给还在疑惑的朋友 👇</text> {align="center"}

<text color="gray">参考来源：OpenAI Blog, Anthropic, Mitchell Hashimoto, Phil Schmid (HuggingFace), Martin Fowler</text> {align="center"}
