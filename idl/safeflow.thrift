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

struct Label {
    1: string name
    2: double confidence
}

struct ImageModerationRequest {
    1: string request_id
    2: string image_url
    3: binary image_data
    4: string scene
}

struct ImageModerationResponse {
    1: string request_id
    2: string action // allow, block, review
    3: double risk_score
    4: string text_content
    5: list<Label> labels
    6: string reason
}

service RuleEngineService {
    ScanResponse Scan(1: ScanRequest req)
}

service LLMAgentService {
    ScanResponse Scan(1: ScanRequest req)
}

service ImageModerationService {
    ImageModerationResponse Moderate(1: ImageModerationRequest req)
}
