# SafeFlow — Enterprise Content Safety Platform with Kitex & Eino

> **Next-Gen Content Moderation Platform powered by CloudWeGo Ecosystem (Kitex + Eino) and Volcano Engine.**

SafeFlow 是一个企业级智能内容安全平台，演示了如何使用字节跳动开源的 CloudWeGo 生态构建高性能、可扩展的 AI 应用。它集成了 **Kitex** 微服务框架和 **Eino** AI 应用框架，并利用 **Milvus** 向量数据库和 **火山引擎 (Volcano Engine)** 的 Ark 大模型服务，实现了基于 RAG 和 Agent 的深度内容审核。

## 🏗 架构设计

SafeFlow 从传统的事件驱动架构演进为高性能的 RPC 微服务架构，结合了规则引擎的极速响应和 LLM Agent 的深度推理能力。

```mermaid
graph TD
    User[User] -->|POST /submit| Gateway[API Gateway (Gin)]
    Gateway -->|RPC (Kitex)| RuleEngine[Rule Engine Service]
    
    RuleEngine -- Match? -->|Block| Gateway
    RuleEngine -- Pass -->|RPC (Kitex)| LLMAgent[LLM Agent Service]
    
    subgraph Eino Agent Logic
        LLMAgent -->|Eino Graph| Agent[Eino ReAct Agent]
        Agent -->|Retrieve| Milvus[Milvus Vector DB]
        Agent -->|Embed/Chat| Ark[Volcano Engine Ark]
    end
    
    Gateway -.->|Async Event (NATS)| Audit[Audit Service]
    Audit -->|Write| MySQL[(MySQL)]
    
    Gateway -->|Response| User
```

### 核心技术栈

- **微服务框架**: [Kitex](https://github.com/cloudwego/kitex) - 高性能、强类型的 Go RPC 框架。
- **AI 应用框架**: [Eino](https://github.com/cloudwego/eino) - 字节跳动开源的大模型应用开发框架，提供极简的 Graph 编排和组件集成。
- **向量数据库**: [Milvus](https://milvus.io/) - 云原生向量数据库，用于存储和检索敏感案例库。
- **大模型服务**: [Volcano Engine Ark](https://www.volcengine.com/product/ark) - 提供高性能的 LLM 推理和 Embedding 服务。
- **网关**: Gin Web Framework。
- **基础设施**: Docker Compose (Etcd, Minio, Milvus, NATS, MySQL).

## ✅ 项目进度（对齐 v2 计划）

- **阶段 S1**：完成内容流入、自动审核与结果分流
- **阶段 S2**：完成人工复审队列、裁决回传与案例沉淀
- **阶段 S3**：已具备规则回测与 Prompt 版本化，评估与质检指标待完善

## ✨ 已实现功能

- 审核主干：规则引擎优先 + LLM 审核兜底
- 结果分流：通过 / 拦截 / 人工复审
- 人工复审队列：领取、裁决、结果回传
- 审核结果查询：按 request_id 拉取结果与复审任务
- 审计日志中心：按用户/动作/来源检索
- 规则治理：增删改查、启停、快照
- Prompt 策略管理：版本化与激活
- 规则回测：基于历史案例评估规则效果
- 模拟流量源：持续投喂内容流

## 🚀 快速开始

### 前置要求

1. **Docker & Docker Compose**: 用于启动基础设施。
2. **Go 1.22+**: 用于本地开发和编译。
3. **火山引擎 API Key**: 需要开通火山引擎方舟平台 (Ark) 服务，并获取 API Key 和 Endpoint。
   - 需部署/接入一个 Chat Model (e.g., Doubao-Pro) 和 Embedding Model。

### 部署步骤

1. **配置环境变量**:
   修改 `docker-compose.yml` 或设置环境变量：
   ```bash
   export ARK_API_KEY="your_volc_api_key"
   export ARK_MODEL_ID="your_endpoint_id_for_chat"
   export ARK_EMBEDDING_MODEL="your_endpoint_id_for_embedding"
   ```

2. **启动基础设施**:
   ```bash
   docker-compose up -d etcd minio milvus nats mysql
   ```
   *等待 Milvus 启动完成 (约 30-60 秒).*

3. **运行微服务**:
   建议在本地分别运行服务以便调试：

   - **Rule Engine**:
     ```bash
     cd cmd/rule-engine
     go run .
     ```
   
   - **LLM Agent**:
     ```bash
     cd cmd/llm-agent
     go run .
     ```

   - **API Gateway**:
     ```bash
     cd cmd/api-gateway
     go run .
     ```

   - **Audit Service**:
     ```bash
     cd cmd/audit-service
     go run .
     ```

   - **Image Moderation**:
     ```bash
     cd cmd/image-moderation
     go run .
     ```

4. **测试请求**:
   ```bash
   curl -X POST http://localhost:8080/submit \
     -H "Content-Type: application/json" \
     -d '{"content": "This is a test message regarding gambling.", "user_id": "test_user"}'
   ```

## 🧪 如何测试

```bash
go test ./...
```

## 🧭 常用管理 API

- 复审队列：`GET /admin/reviews`
- 复审领取：`POST /admin/reviews/:id/claim`
- 复审裁决：`POST /admin/reviews/:id/decide`
- 结果查询：`GET /moderation/:request_id`
- 规则回测：`POST /admin/rules/backtest`
- 模拟流量：`POST /admin/simulator/start` / `POST /admin/simulator/stop` / `GET /admin/simulator/status`

## 🧩 Eino Agent 实现

LLM Agent 服务使用 Eino 框架构建了一个 ReAct Agent：
- **Retriever**: 集成 Milvus，自动检索历史违规案例。
- **Tools**: 定义了 `search_sensitive_cases` 等工具供 LLM 调用。
- **Graph**: 使用 Eino Graph 编排 "思考-行动-观察" 循环。

## 📄 IDL 定义 (Kitex)

项目使用 Thrift 定义服务接口 (`idl/safeflow.thrift`)：

```thrift
struct ScanRequest {
    1: string request_id
    2: string user_id
    3: string content
}

service RuleEngineService {
    ScanResponse Scan(1: ScanRequest req)
}

service LLMAgentService {
    ScanResponse Scan(1: ScanRequest req)
}
```

## 🛠 扩展指南

- **添加新规则**: 修改 `cmd/rule-engine/handler.go` 中的逻辑。
- **添加新工具**: 在 `internal/agent/eino.go` 中注册新的 `schema.SimpleTool`。
- **切换模型**: 修改环境变量中的 `ARK_MODEL_ID`。

## License

MIT
