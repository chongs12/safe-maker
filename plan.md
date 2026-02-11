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

### M1：多阶段流水线 + Tool 体系（**✅ 已完成**）

**交付物**：
- [x] `ClassifyNode` + `DecideNode` + `ValidateNode` 实现
- [x] `RAGNode` 实现（Milvus 检索）
- [x] 重构 `EinoAgent` 为多阶段 Graph
- [x] 新增 4+ 个 Tools（保留原有）
- [x] Pipeline 协调器实现
- [x] 向后兼容：同时支持新旧两种模式
- [x] 端到端测试验证通过
- [x] 冗余代码清理完成

**验证标准**：
- ✅ 流水线可正常编译运行
- ✅ 中间结果可观测（日志）
- ✅ Tools 可被 LLM 正确调用
- ✅ 端到端测试 4/4 通过
- ✅ 代码整洁，无冗余

**核心特性**：
1. **四阶段处理**：Classify → (Optional RAG) → Decide → Validate
2. **结构化输出**：每个阶段都有明确的输入输出结构
3. **容错机制**：RAG 失败不影响主流程，验证失败可降级
4. **可解释性**：详细的日志记录每个阶段的决策过程

---

---

### M2：Prompt 版本管理 + AI 辅助规则（**✅ 已完成**）

**交付物**：
- [x] `PolicyVersion` 表扩展支持 `type: prompt`
- [x] `PromptPolicy` 结构体定义（Scene/SystemPrompt/Temperature/Tools）
- [x] Admin API：prompt CRUD + 版本切换
- [x] `AI Rule Generator` 实现（基于 LLM 从案例生成规则）
- [x] 提示词工程：结构化 Prompt + JSON Schema 输出
- [x] 规则验证机制

**验证标准**：
- ✅ 可通过 API 创建/切换 prompt 版本
- ✅ AI 生成的规则可被人工审核后入库
- ✅ Prompt 策略可版本化管理
- ✅ 支持多场景独立配置
- ✅ AI 可基于历史案例生成新规则

**核心特性**：
1. **Prompt 版本管理**：不同场景可独立维护系统提示词
2. **AI 规则生成**：从违规案例自动抽象通用规则
3. **结构化输出**：严格的 JSON Schema 确保一致性
4. **质量控制**：内置规则验证机制

---

### M3：可观测性增强（**✅ 已完成**）

**交付物**：
- [x] Prometheus 指标收集框架 (internal/common/metrics.go)
- [x] API Gateway 集成指标收集和 /metrics 端点
- [x] Rule Engine 和 LLM Agent 集成指标收集
- [x] Grafana 仪表板配置和可视化
- [x] Docker Compose 部署 Prometheus + Grafana + Jaeger
- [x] OpenTelemetry 链路追踪集成

**验证标准**：
- ✅ 自定义业务指标可被 Prometheus 抓取
- ✅ 各服务关键性能指标可视化
- ✅ 请求链路可追踪（Jaeger UI）
- ✅ 告警规则配置

**核心特性**：
1. **多维度指标**：HTTP、审核、LLM、规则引擎等各层面指标
2. **标准化输出**：Prometheus 格式，易于集成监控系统
3. **性能洞察**：延迟分布、错误率、吞吐量等关键指标
4. **服务健康**：Go 运行时、资源使用情况监控
5. **可视化界面**：Grafana 仪表板实时展示各项指标
6. **链路追踪**：Jaeger UI 展示请求调用链路

---

### M4：整合优化 + 文档（1周）

**交付物**：
- [ ] 端到端测试
- [ ] README 更新（架构图、快速开始）
- [ ] 面试话术整理
- [ ] 演示视频/GIF（可选）

---

## 五、未来拓展方向

### M5：多模态内容审核（深度集成 ✅）

**背景**：基于微服务架构，深度集成火山引擎图像审核和OCR服务。

#### Phase 1: 独立图像审核服务（✅ 已完成）
- [x] 独立的 image-moderation 微服务
- [x] 火山引擎图像审核API集成
- [x] Kitex RPC服务框架
- [x] RESTful API接口
- [x] Prometheus指标收集
- [x] 链路追踪集成
- [x] Etcd服务注册发现
- [x] Thrift接口定义扩展

#### Phase 2: OCR文字识别集成（✅ 真实API集成完成）
- [x] 火山OCR API客户端真实集成
- [x] 图像审核API真实集成  
- [x] TC3-HMAC-SHA256签名算法实现
- [x] OCR文本提取功能
- [x] 文本后处理（去噪、格式化）
- [x] 与现有文本审核服务联动
- [ ] OCR质量评估指标（待完善）
- [ ] 多语言支持（中英日韩）（待完善）

**技术架构升级**：
```
┌─────────────────┐    ┌─────────────────────┐    ┌─────────────────┐
│   API Gateway   │───▶│  Image Moderation   │───▶│ Volcengine APIs │
│                 │    │   (独立服务)        │    │  ├─ 图像审核    │
└─────────────────┘    └─────────────────────┘    │  └─ OCR识别     │
         │                      │                   └─────────────────┘
         │                      │                            │
         ▼                      ▼                            ▼
┌─────────────────┐    ┌─────────────────────┐    ┌─────────────────┐
│ Text Moderation │◀───┤   文本审核服务       │◀───┤  OCR提取文本    │
│   Service       │    │                     │    │                 │
└─────────────────┘    └─────────────────────┘    └─────────────────┘
```

**技术架构**：
```
┌─────────────────┐    ┌─────────────────────┐    ┌─────────────────┐
│   API Gateway   │───▶│  Image Moderation   │───▶│ Volcengine API  │
│                 │    │   (独立服务)        │    │  图像审核       │
└─────────────────┘    └─────────────────────┘    └─────────────────┘
                            │         ▲
                            │         │
                            ▼         │
                    ┌─────────────────────┐
                    │   Prometheus        │
                    │   Metrics收集       │
                    └─────────────────────┘
```

**核心特性**：
1. **专业分工**：独立服务专门处理图像审核
2. **高可用性**：API调用失败时默认人工审核
3. **可观测性**：完整的指标和链路追踪
4. **标准化**：统一的Thrift接口定义
5. **可扩展**：易于添加OCR、人脸识别等功能

**部署方式**：
- 端口配置：HTTP 8081, RPC 8883
- 服务发现：Etcd注册
- 监控集成：Prometheus + Grafana
- 链路追踪：Jaeger

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
