# 终端 1 (Milvus 初始化 - 仅需一次)
# 终端 2 (Audit)
$env:CONFIG_FILE="config.yaml"; go run ./cmd/audit-service
# 终端 3 (Rule)
$env:CONFIG_FILE="config.yaml"; go run ./cmd/rule-engine
# 终端 4 (LLM)
$env:CONFIG_FILE="config.yaml"; go run ./cmd/llm-agent
# 终端 5 (Gateway)
$env:CONFIG_FILE="config.yaml"; go run ./cmd/api-gateway