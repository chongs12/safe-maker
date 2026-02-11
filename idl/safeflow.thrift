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

struct Box {
    1: i32 x1
    2: i32 y1
    3: i32 x2
    4: i32 y2
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
    5: bool enable_ocr  // 是否启用OCR
}

struct ImageModerationResponse {
    1: string request_id
    2: string action // allow, block, review
    3: double risk_score
    4: string text_content  // OCR提取的文本
    5: list<Label> labels
    6: string reason
    7: OCRResult ocr_result // OCR详细结果
}

struct OCRResult {
    1: string extracted_text
    2: double confidence
    3: list<TextBlock> text_blocks
    4: string language
}

struct TextBlock {
    1: string text
    2: double confidence
    3: Box bounding_box
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
