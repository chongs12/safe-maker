package nodes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"
)

// DecideInput 决策节点输入
type DecideInput struct {
	Content     string          // 原始内容
	ClassifyOut *ClassifyOutput // 分类结果
	RAGOut      *RAGOutput      // RAG 结果（可选）
}

// DecideOutput 决策节点输出
type DecideOutput struct {
	Action     string  `json:"action"`     // allow/block/review
	Reason     string  `json:"reason"`     // 决策原因
	Confidence float64 `json:"confidence"` // 置信度
	Evidence   string  `json:"evidence"`   // 证据摘要
}

// DecideNode 决策节点
type DecideNode struct {
	model *ark.ChatModel
}

// NewDecideNode 创建决策节点
func NewDecideNode(model *ark.ChatModel) *DecideNode {
	return &DecideNode{model: model}
}

// Invoke 执行决策
func (n *DecideNode) Invoke(ctx context.Context, input *DecideInput) (*DecideOutput, error) {
	// 构造系统提示词
	systemPrompt := `你是一个内容安全审核专家。请综合分析以下信息做出最终审核决策。

输入信息：
1. 原始内容
2. 内容分类结果（类别、风险分数、是否需要RAG）
3. RAG检索结果（相似案例）

决策标准：
- allow: 内容合规，无风险
- block: 明确违规，应拦截
- review: 边界情况，需人工复核

输出要求：
1. 必须是严格的 JSON 格式
2. 不要包含任何 markdown 标记
3. confidence 范围 0.0-1.0
4. reason 应简洁明了，说明决策依据

输出格式：
{
  "action": "allow|block|review",
  "reason": "决策原因说明",
  "confidence": 0.95,
  "evidence": "证据摘要"
}`

	// 构造用户消息
	userMsg := fmt.Sprintf(`原始内容：%s

分类结果：
- 类别：%s
- 风险分数：%.2f
- 置信度：%.2f
- 需要RAG：%t`,
		input.Content,
		input.ClassifyOut.Category,
		input.ClassifyOut.RiskScore,
		input.ClassifyOut.Confidence,
		input.ClassifyOut.NeedRAG,
	)

	// 添加 RAG 信息（如果有）
	if input.RAGOut != nil {
		userMsg += fmt.Sprintf(`

RAG 检索结果：
%s`, input.RAGOut.Summary)
	}

	messages := []*schema.Message{
		{Role: schema.System, Content: systemPrompt},
		{Role: schema.User, Content: userMsg},
	}

	resp, err := n.model.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("决策模型调用失败: %w", err)
	}

	// 解析输出
	var output DecideOutput
	if err := json.Unmarshal([]byte(resp.Content), &output); err != nil {
		return nil, fmt.Errorf("解析决策结果失败: %w, 原始输出: %s", err, resp.Content)
	}

	// 验证输出合理性
	if output.Action == "" {
		output.Action = "review" // 默认需要复核
	}
	if output.Confidence < 0 {
		output.Confidence = 0
	}
	if output.Confidence > 1 {
		output.Confidence = 1
	}

	return &output, nil
}
