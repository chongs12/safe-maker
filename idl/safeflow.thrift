namespace go safeflow

struct ScanRequest {
    1: string request_id
    2: string user_id
    3: string content
}

struct ScanResponse {
    1: string request_id
    2: string action // allow, block, review
    3: string reason
    4: string source // rule-engine, llm-agent
}

service RuleEngineService {
    ScanResponse Scan(1: ScanRequest req)
}

service LLMAgentService {
    ScanResponse Scan(1: ScanRequest req)
}
