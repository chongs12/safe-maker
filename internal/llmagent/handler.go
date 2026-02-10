package llmagent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/safeflow-project/safeflow/internal/agent"
	"github.com/safeflow-project/safeflow/internal/common"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow"
)

// LLMAgentServiceImpl 实现 LLMAgentService 接口
type LLMAgentServiceImpl struct {
	einoAgent     *agent.EinoAgent     // 旧版 Eino Graph Agent (保留兼容)
	pipelineAgent *agent.PipelineAgent // 新版多阶段流水线 Agent
	usePipeline   bool                 // 是否使用流水线模式
}

// NewLLMAgentServiceImpl 创建新的服务实现实例
func NewLLMAgentServiceImpl(ctx context.Context, cfg *common.Config) *LLMAgentServiceImpl {
	// 初始化 Eino Agent (旧版)
	einoAgent, chatModel, retrieverObj, err := agent.NewEinoAgent(ctx, cfg)
	if err != nil {
		panic(err)
	}

	// 初始化 Pipeline Agent (新版)
	pipelineAgent := agent.NewPipelineAgent(chatModel, retrieverObj)

	return &LLMAgentServiceImpl{
		einoAgent:     einoAgent,
		pipelineAgent: pipelineAgent,
		usePipeline:   true, // 默认使用新版流水线
	}
}

// Scan 处理内容扫描请求
func (s *LLMAgentServiceImpl) Scan(ctx context.Context, req *safeflow.ScanRequest) (resp *safeflow.ScanResponse, err error) {
	start := time.Now()

	// 初始化默认响应 (Review - 需要人工复核)
	resp = &safeflow.ScanResponse{
		RequestId: req.RequestId,
		Source:    "llm-agent",
		Action:    "review",
	}

	var resultStr string

	// 根据配置选择 Agent
	if s.usePipeline {
		// 使用新版多阶段流水线
		resultStr, err = s.pipelineAgent.Run(ctx, req.Content)
		if err != nil {
			// 流水线失败时降级到旧版
			resp.Reason = "[流水线失败] " + err.Error() + "，降级到旧版处理"
			resultStr, err = s.einoAgent.Run(ctx, req.Content)
			if err != nil {
				resp.Reason += "；旧版也失败: " + err.Error()
				return resp, nil
			}
		}
	} else {
		// 使用旧版 Eino Graph
		resultStr, err = s.einoAgent.Run(ctx, req.Content)
		if err != nil {
			resp.Reason = "Agent 运行错误: " + err.Error()
			return resp, nil
		}
	}

	// 解析 Agent 返回的 JSON 结果
	cleanedResult := cleanJSON(resultStr)

	// 尝试解析新版流水线格式
	var pipelineResp struct {
		Action     string  `json:"action"`
		Reason     string  `json:"reason"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(cleanedResult), &pipelineResp); err == nil && pipelineResp.Action != "" {
		resp.Action = pipelineResp.Action
		resp.Reason = pipelineResp.Reason
		if s.usePipeline {
			resp.Reason += " (流水线模式)"
		}
		return resp, nil
	}

	// 降级解析旧版格式
	var decision struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(cleanedResult), &decision); err != nil {
		resp.Reason = "解析结果失败。原始输出: " + resultStr
	} else {
		resp.Action = decision.Action
		resp.Reason = decision.Reason
		if !s.usePipeline {
			resp.Reason += " (旧版模式)"
		}
	}

	// 记录处理时间和动作统计
	duration := time.Since(start).Seconds()
	if common.GlobalMetrics != nil {
		common.GlobalMetrics.AuditDuration.WithLabelValues("llm", "unknown").Observe(duration)
		common.GlobalMetrics.AuditActionsTotal.WithLabelValues(resp.Action, "llm", "unknown").Inc()
		if s.usePipeline {
			common.GlobalMetrics.LLMCallsTotal.WithLabelValues("pipeline", "complete").Inc()
		} else {
			common.GlobalMetrics.LLMCallsTotal.WithLabelValues("eino", "complete").Inc()
		}
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
