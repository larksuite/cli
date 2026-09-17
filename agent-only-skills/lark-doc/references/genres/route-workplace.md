# Genre Router: Workplace (`router.workplace`)

组织内决策、执行、留档走本类；先按读者任务与生命周期选择且只读一个 leaf，关键词仅用于召回，排除信号优先于同名词。

| 读者任务 / 关键词、强信号与排除 | Leaf |
|-|-|
| 快速知悉、短判断或会前准备；备忘录、决策摘要、会前材料。完整批准论证走 Proposal，周期状态走 Weekly | [`memo-brief.md`](memo-brief.md) |
| 按周期判断相对承诺的状态、偏差、风险和下一步；周报、日报、月报、项目状态。原因学习走 Retrospective，完整分析走 Report | [`weekly-report.md`](weekly-report.md) |
| 请具名决策者批准方向、预算、资源或执行承诺；提案、立项、资源申请。已定产品行为走 PRD | [`proposal.md`](proposal.md) |
| 将方向已定的一次性项目、变更、专项行动或营销战役转成可协同推进的交付、依赖、里程碑与验收；项目计划、执行方案、实施计划。仍在比较方向或请求批准走 Proposal / Report，重复稳定路径走 SOP | [`execution-plan.md`](execution-plan.md) |
| 将已授权的内部规则 / 安排、可复核的检查整改记录或已核定组织立场写成正式载体；制度、公司通知、整改记录、讲话底稿。待批准方向走 Proposal，复杂执行走 Execution Plan，法定公文走 Official；`正式`单独不触发 | [`formal-doc.md`](formal-doc.md) |
| 党政机关法定公文拟制、审校或制发；明确要求公文 / 红头 / 套红 / 正式发文，或法定文种与机关行文关系、文号、主送等制发要素共同出现。`通知 / 报告 / 公告 / 纪要 / 正式 / 官方`单独不触发 | [`official-redhead.md`](official-redhead.md) |
| 记录已发生会议的决定、异议、行动和批准状态；会议记录、行动项。逐字稿不走本 leaf，法定公文纪要走 Official | [`meeting-minutes.md`](meeting-minutes.md) |
| 从已结束周期 / 事件提炼证据化学习并改变下一轮；复盘、回顾、经验教训。当前状态走 Weekly，活跃未知事故走 Technical | [`retrospective.md`](retrospective.md) |
| 方向已定，定义用户问题、范围、产品行为与验收；PRD、用户故事、验收标准。是否投入走 Proposal，实现取舍走 Technical | [`prd.md`](prd.md) |
| 评审未来技术设计、查询精确契约或调查未知故障；RFC、API、架构、事故调查。产品行为走 PRD，已定重复路径走 SOP | [`technical-doc.md`](technical-doc.md) |
| 按已批准、可验证路径重复达到终态，或为已知事件类别预置响应路径；SOP、runbook、值班 / 操作手册、BCP / 处置预案。应急预案若主要发布权威职责走 Formal，法定制发走 Official，活跃未知事故走 Technical，一次学习教程走 Knowledge | [`sop-tutorial.md`](sop-tutorial.md) |
