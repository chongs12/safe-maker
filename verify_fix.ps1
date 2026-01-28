# 定义端口变量
$env:GATEWAY_PORT="8081"
$env:RULE_ENGINE_PORT="8891"
$env:LLM_AGENT_PORT="8892"
$env:RULE_ENGINE_ADDR="localhost:8891"
$env:LLM_AGENT_ADDR="localhost:8892"
$env:CONFIG_FILE="config.yaml"

Write-Host "🚀 正在启动测试环境 (Shadow Stack)..."

# 启动服务 (使用 Start-Process -NoNewWindow 后台运行)
# 注意：在某些环境中 Start-Process 可能无法捕获输出，但我们需要它们在后台运行
$p1 = Start-Process -FilePath "go" -ArgumentList "run", "./cmd/rule-engine" -PassThru -NoNewWindow
$p2 = Start-Process -FilePath "go" -ArgumentList "run", "./cmd/llm-agent" -PassThru -NoNewWindow
$p3 = Start-Process -FilePath "go" -ArgumentList "run", "./cmd/api-gateway" -PassThru -NoNewWindow

Write-Host "⏳ 等待服务启动 (15秒)..."
Start-Sleep -Seconds 15

Write-Host "🧪 开始运行验证测试..."
go run ./cmd/verify/main.go

# 保存退出码
$exitCode = $LASTEXITCODE

Write-Host "🛑 正在清理测试进程..."
Stop-Process -Id $p1.Id -ErrorAction SilentlyContinue
Stop-Process -Id $p2.Id -ErrorAction SilentlyContinue
Stop-Process -Id $p3.Id -ErrorAction SilentlyContinue

if ($exitCode -eq 0) {
    Write-Host "✅ 验证成功！代码逻辑正确。"
} else {
    Write-Host "❌ 验证失败。"
}
