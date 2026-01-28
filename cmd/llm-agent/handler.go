package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/safeflow-project/safeflow/internal/agent"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow"
)

// LLMAgentServiceImpl 实现 LLMAgentService 接口
type LLMAgentServiceImpl struct {
	agent *agent.EinoAgent
}

// NewLLMAgentServiceImpl 创建新的服务实现实例
func NewLLMAgentServiceImpl(ctx context.Context) *LLMAgentServiceImpl {
	// 初始化 Eino Agent (包含图编排、模型加载等)
	a, err := agent.NewEinoAgent(ctx)
	if err != nil {
		// 在生产环境中，应该优雅地处理错误
		// 这里如果配置缺失或初始化失败，我们选择快速失败 (Panic)
		panic(err)
	}
	return &LLMAgentServiceImpl{agent: a}
}

// Scan 处理内容扫描请求
func (s *LLMAgentServiceImpl) Scan(ctx context.Context, req *safeflow.ScanRequest) (resp *safeflow.ScanResponse, err error) {
	// 初始化默认响应 (Review - 需要人工复核)
	resp = &safeflow.ScanResponse{
		RequestId: req.RequestId,
		Source:    "llm-agent",
		Action:    "review",
	}

	// 运行 Eino Agent
	resultStr, err := s.agent.Run(ctx, req.Content)
	if err != nil {
		resp.Reason = "Agent 运行错误: " + err.Error()
		return resp, nil
	}

	// 解析 Agent 返回的 JSON 结果
	// 清理可能存在的 Markdown 代码块标记
	cleanedResult := cleanJSON(resultStr)
	var decision struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(cleanedResult), &decision); err != nil {
		resp.Reason = "解析结果失败。原始输出: " + resultStr
	} else {
		resp.Action = decision.Action
		resp.Reason = decision.Reason
	}

	return resp, nil
}

// cleanJSON 清理 JSON 字符串中的 Markdown 标记
func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
