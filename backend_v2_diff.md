# SafeFlow 后端现状 vs V2 计划对比（结论与差距）

## 1. 结论：是否需要大幅修改？
结论：**不需要推倒重来，但需要显著补齐“人工复审队列 + 结果回传 + 闭环进化”的关键链路**。现有后端已经具备可运行的审核主干（规则引擎、LLM 审核、审计落库），因此核心结构可复用。真正需要改动的是**业务流的“中后段”**：如何把 review 结果变成可运营的队列、如何回传到业务侧、如何把人工结果沉淀为策略迭代输入。

---

## 2. 现有后端能力（可直接复用）

### 2.1 审核主干链路已存在
- 网关先规则再 LLM 的审核主干已实现，且已发布审计事件 [api-gateway](file:///c:/Users/Chen%20Jiaming/prj/otherpro/back/cmd/api-gateway/main.go#L208-L258)
- 规则引擎具备 AC 自动机与正则策略匹配 [rule-engine](file:///c:/Users/Chen%20Jiaming/prj/otherpro/back/cmd/rule-engine/impl/handler.go#L96-L140)
- 审计服务已订阅结果事件并写入 MySQL [audit-service](file:///c:/Users/Chen%20Jiaming/prj/otherpro/back/cmd/audit-service/main.go#L75-L99)

### 2.2 管理与模型基础存在
- 审计日志、规则、案例、Prompt、策略快照的数据结构已定义 [model.go](file:///c:/Users/Chen%20Jiaming/prj/otherpro/back/internal/common/model.go#L36-L102)
- 审计任务（批量任务模型）已存在 [model.go](file:///c:/Users/Chen%20Jiaming/prj/otherpro/back/internal/common/model.go#L72-L82)

---

## 3. V2 计划要求的“审核系统逻辑”

V2 计划强调可运营的真实审核系统闭环 [v2.md](file:///c:/Users/Chen%20Jiaming/prj/otherpro/back/v2.md#L19-L63)：
- 持续内容流（真实业务或模拟流量）
- 自动审核后“分流”到通过/拦截/人工复审
- 人工复审队列是核心操作面
- 复审结果必须回传业务侧
- 人工结果沉淀为案例和策略迭代输入

---

## 4. 关键差异对比（现状 vs V2）

| 业务环节 | 当前实现 | V2 目标 | 差距等级 |
|---|---|---|---|
| 内容持续流入 | 仅支持手动 API 提交 | 支持真实流或模拟流量源 | 中 |
| 自动审核 | 规则 + LLM 已实现 | 保持，需更稳定与可追踪 | 小 |
| review 分流 | 仅在 LLM 异常时返回 review [api-gateway](file:///c:/Users/Chen%20Jiaming/prj/otherpro/back/cmd/api-gateway/main.go#L242-L252) | 任何“存疑”结果进入人工队列 | 大 |
| 人工审核队列 | 无专用队列/任务模型 | review 任务池 + 状态流转 | 大 |
| 结果回传业务 | 仅返回 API 响应 | 回调业务或状态可查询 | 中 |
| 审计与追踪 | 审计日志已落库 [audit-service](file:///c:/Users/Chen%20Jiaming/prj/otherpro/back/cmd/audit-service/main.go#L84-L99) | 需要与人工结果联动 | 中 |
| 闭环进化 | 案例库存在但未接入人工结果 | 人工裁决可沉淀为案例、支持规则回测 | 中 |

---

## 5. 后端需要补齐的核心模块

### 5.1 人工复审队列（必须新增）
- 建立 ReviewTask 数据表或 Redis 队列
- review 结果从 LLM 模型输出进入队列，而不是直接返回
- 提供“领取 / 裁决 / 回传”的 API

### 5.2 业务回传机制（必须新增）
- 支持业务方回调或状态轮询
- 将最终裁决写入可查询状态表

### 5.3 规则回测与闭环输入（新增）
- 将人工裁决一键沉淀为案例
- 规则上线前支持批量历史回测

### 5.4 模拟流量源（建议新增）
- 新增 Mock 内容生产者，驱动系统持续运行

---

## 6. 对后端改动规模判断

**整体为“中等规模改造”而非重构**：
- 核心审核链路、服务架构、审计系统可保留
- 需要新增“人工复审队列”与“结果回传”这两段关键链路
- 调整网关流程，把 review 从“异常返回”提升为“业务分流”

---

## 7. 建议优先级（后端视角）
1. review 结果进入队列而不是直接返回
2. 人工裁决 API + 回传机制
3. 模拟流量源与持续压测
4. 规则回测与案例闭环
