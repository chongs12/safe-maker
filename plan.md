# SafeFlow AI审核平台开发规划

> 目标：打造"规则引擎 + LLM Orchestration + RAG + 评估闭环"的企业级内容安全平台

---

## 一、总体架构目标

```
┌─────────────────────────────────────────────────────────────────┐
│                         API Gateway (Gin)                        │
│                    RESTful API + Admin Console                   │
└────────────────────┬────────────────────────────────────────────┘
                     │
        ┌────────────┴────────────┐
        ▼                         ▼
┌──────────────────┐    ┌──────────────────────────────────────┐
│   Rule Engine    │    │           LLM Agent (Eino)            │
│  (毫秒级初筛)     │    │  ┌─────────┐   ┌─────────┐   ┌──────┐ │
│                  │    │  │Classify │ → │  RAG    │ → │Decide│ │
│  - 关键词/正则    │    │  │  Node   │   │  Node   │   │ Node │ │
│  - AC自动机       │    │  └─────────┘   └─────────┘   └──────┘ │
│  - 规则热更新     │    │         ↓              ↓              │
└────────┬─────────┘    │    ┌─────────┐    ┌─────────┐         │
         │              │    │ Validate│    │  Tools  │         │
         │              │    │  Node   │    │ (多工具) │         │
         │              │    └─────────┘    └─────────┘         │
         │              └──────────────────────────────────────┘
         │                              │
         └──────────────┬───────────────┘
                        ▼
              ┌──────────────────┐
              │   NATS (Event)   │
              └────────┬─────────┘
                       ▼
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
┌──────────────┐ ┌──────────┐ ┌──────────────┐
│Audit Service │ │Evaluation│ │   Metrics    │
│  (MySQL)     │ │  Service │ │ (Prometheus) │
└──────────────┘ └──────────┘ └──────────────┘
```

---

## 二、六大工作流（Workstreams）

### A. 多阶段决策流水线（Multi-Stage Pipeline）

**目标**：将单次 LLM 调用升级为多阶段决策图

**当前状态**：
- `EinoAgent` 是简单 "Model → Tools → Model" 流程
- 缺少中间结果的结构化管理

**改造方案**：

| 阶段 | 节点名称 | 功能 | 输出 |
|------|---------|------|------|
| 1 | `ClassifyNode` | 内容分类 + 风险打分 | `{category, risk_score, need_rag}` |
| 2 | `RAGNode` (可选) | 检索相似案例 | `[similar_cases]` |
| 3 | `DecideNode` | 综合决策 | `{action, reason, evidence}` |
| 4 | `ValidateNode` | 结果自洽检查 | `{final_action, confidence}` |

**关键代码位置**：`internal/agent/eino.go`

---

### B. Tool 体系化（Tool Ecosystem）

**目标**：构建完整的 AI 审核工具箱

**现有 Tools**：
- `search_sensitive_cases` ✅
- `check_political_entities` (Mock)

**新增 Tools**：

| 类别 | Tool 名称 | 功能描述 |
|------|----------|---------|
| 知识检索 | `search_policy_docs` | 检索平台规范/政策条款 |
| 信息抽取 | `extract_entities` | NER：人名、地名、组织 |
| 信息抽取 | `detect_pii` | 检测邮箱、手机号、身份证 |
| 辅助决策 | `get_rule_suggestion` | 基于历史推荐规则模式 |
| 辅助决策 | `suggest_user_feedback` | 生成用户解释文案 |
| 运维排障 | `fetch_recent_audit_stats` | 获取近期审计统计 |

**关键代码位置**：`internal/agent/tools/` (新建目录)

---

### C. Prompt/策略版本管理（Policy as Code）

**目标**：将审核策略抽象为可版本化的配置

**数据模型扩展**：

```go
// PolicyVersion 扩展
const (
    PolicyTypeRule   = "rule"
    PolicyTypeModel  = "model"  
    PolicyTypePrompt = "prompt" // 新增
)

type PromptPolicy struct {
    Scene       string   // im / ugc / ad / guardrail
    SystemPrompt string
    Temperature float32
    MaxTokens   int
    Tools       []string // 启用的 tools
}
```

**管理接口**：
- `GET /admin/prompts` - 列出所有 prompt 模板
- `POST /admin/prompts` - 创建新版本
- `POST /admin/prompts/:id/activate` - 激活指定版本
- `GET /admin/prompts/:id/diff` - 对比版本差异

**关键代码位置**：`internal/common/model.go`, `cmd/api-gateway/main.go`

---

### D. 历史数据评估闭环（Offline Evaluation）

**目标**：建立基于历史审计数据的 LLM 效果评估体系

**评估维度**：

| 指标 | 说明 | 计算方式 |
|------|------|---------|
| 准确率 | 与基准标签一致的比例 | TP+TN / Total |
| 误杀率 | 应 allow 被 block | FP / (FP+TN) |
| 放过率 | 应 block 被 allow | FN / (FN+TP) |
| 平均耗时 | P50/P95/P99 响应时间 | - |
| 成本估算 | Token 消耗统计 | - |

**评估流程**：
1. 从 `AuditLog` 抽取样本（含人工复核结果）
2. 批量重放给当前 LLM 流水线
3. 对比输出与基准标签
4. 生成评估报告

**关键代码位置**：`cmd/verify/main.go` (已有，需扩展)

---

### E. AI 辅助写规则（AI-Assisted Rule Generation）

**目标**：用 LLM 帮助运营生成/优化规则

**接口设计**：

```http
POST /admin/rules/ai_suggest
{
    "description": "拦截所有含有赌博、博彩、网赌相关的文案，包括英文 casino/gambling",
    "scene": "ugc",
    "severity": "high"
}

Response:
{
    "suggestions": [
        {
            "pattern": "(赌博|博彩|网赌|casino|gambling)",
            "type": "regex",
            "action": "block",
            "group": "ads",
            "confidence": 0.95,
            "reasoning": "覆盖了中英文常见赌博词汇..."
        }
    ],
    "test_cases": ["想玩网赌", "casino night"],
    "estimated_coverage": 0.92
}
```

**关键代码位置**：`internal/agent/rule_generator.go` (新建)

---

### F. 稳定性与降级策略（Resilience & Degradation）

**目标**：确保 LLM 服务在生产环境的稳定性

**降级策略**：

| 场景 | 降级行为 | 触发条件 |
|------|---------|---------|
| LLM 超时 | 返回 `review` + 标记原因 | 响应 > 3s |
| LLM 错误率过高 | 切换备用模型 | 错误率 > 5% |
| Milvus 不可用 | 跳过 RAG，仅用 LLM 自身知识 | 连接失败 |
| 高频限流 | 轻量模型/仅规则引擎 | QPS > 阈值 |

**可观测性**：
- Prometheus Metrics：LLM QPS、Latency、Error Rate、Token Usage
- 链路追踪：每个 Graph Node 的耗时
- 审计日志：记录每次 LLM 调用的 prompt_hash、model_id、tools_used

**关键代码位置**：`internal/agent/`, `internal/common/metrics.go` (新建)

---

## 三、里程碑规划（Milestones）

### M1：多阶段流水线 + Tool 体系（2周）

**交付物**：
- [ ] `ClassifyNode` + `DecideNode` + `ValidateNode` 实现
- [ ] 重构 `EinoAgent` 为多阶段 Graph
- [ ] 新增 4+ 个 Tools
- [ ] 单元测试覆盖

**验证标准**：
- 流水线可正常运行，中间结果可观测
- Tools 可被 LLM 正确调用

---

### M2：Prompt 版本管理 + AI 辅助规则（2周）

**交付物**：
- [ ] `PolicyVersion` 表扩展支持 `type: prompt`
- [ ] Admin API：prompt CRUD + 版本切换
- [ ] `AI Rule Generator` 实现
- [ ] 前端可演示（或 Postman 集合）

**验证标准**：
- 可通过 API 创建/切换 prompt 版本
- AI 生成的规则可被人工审核后入库

---

### M3：评估闭环 + 稳定性（2周）

**交付物**：
- [ ] `cmd/verify` 评估工具完善
- [ ] Prometheus Metrics 接入
- [ ] 降级策略实现（超时/错误率/限流）
- [ ] 链路追踪（可选）

**验证标准**：
- 可运行评估脚本生成报告
- 模拟故障时触发降级

---

### M4：整合优化 + 文档（1周）

**交付物**：
- [ ] 端到端测试
- [ ] README 更新（架构图、快速开始）
- [ ] 面试话术整理
- [ ] 演示视频/GIF（可选）

---

## 四、技术债务与前置任务

### 立即执行（本周）

1. **服务注册发现**
   - 集成 Etcd 作为 Kitex 服务注册中心
   - 替换硬编码地址

2. **规则引擎优化**
   - AC 自动机替换简单遍历
   - 提升匹配性能

3. **基础可观测性**
   - Prometheus + Grafana 基础接入

---

## 五、面试话术模板

### 一句话介绍

> 我做了一个企业级内容审核平台 SafeFlow，底层用规则引擎做毫秒级过滤，上层用 Eino 编排 LLM 多阶段决策流水线，结合 Milvus RAG 和完整的 Tool 体系，外层还有 Prompt 版本管理和离线评估闭环，形成了"规则 + LLM + RAG + 评估"的完整 AI 应用工程化方案。

### 可深入的技术点

1. **多阶段决策**：为什么要拆成 Classify → RAG → Decide → Validate？
2. **Tool 设计**：如何让 LLM 正确选择 Tool？如何设计 Tool 的输入输出？
3. **Prompt 版本**：如何管理 Prompt 的迭代和回滚？
4. **评估闭环**：如何建立离线评估体系？指标怎么选？
5. **稳定性**：LLM 服务如何降级？成本怎么控制？

---

## 六、附录：关键文件结构

```
internal/
├── agent/
│   ├── eino.go                 # 主 Graph 编排（M1 改造）
│   ├── nodes/                  # 各阶段节点实现（M1 新建）
│   │   ├── classify_node.go
│   │   ├── decide_node.go
│   │   └── validate_node.go
│   ├── tools/                  # Tool 体系（M1 新建）
│   │   ├── search.go
│   │   ├── extract.go
│   │   └── suggest.go
│   ├── rule_generator.go       # AI 辅助写规则（M2 新建）
│   └── metrics.go              # LLM 指标采集（M3 新建）
├── common/
│   ├── model.go                # PolicyVersion 扩展（M2）
│   └── metrics.go              # Prometheus 封装（M3 新建）
└── ...

cmd/
├── verify/
│   └── main.go                 # 评估工具（M3 完善）
└── ...
```

---

*Last Updated: 2025-02-10*
*Next Review: M1 完成后*
