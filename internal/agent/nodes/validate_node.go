package nodes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"
)

// ValidateInput 验证节点输入
type ValidateInput struct {
	Content     string          // 原始内容
	ClassifyOut *ClassifyOutput // 分类结果
	DecideOut   *DecideOutput   // 决策结果
	RAGOut      *RAGOutput      // RAG 结果（可选）
}

// ValidateOutput 验证节点输出
type ValidateOutput struct {
	FinalAction string  `json:"final_action"` // 最终动作
	IsValid     bool    `json:"is_valid"`     // 是否有效
	Feedback    string  `json:"feedback"`     // 验证反馈
	Confidence  float64 `json:"confidence"`   // 最终置信度
}

// ValidateNode 验证节点
type ValidateNode struct {
	model *ark.ChatModel
}

// NewValidateNode 创建验证节点
func NewValidateNode(model *ark.ChatModel) *ValidateNode {
	return &ValidateNode{model: model}
}

// Invoke 执行验证
func (n *ValidateNode) Invoke(ctx context.Context, input *ValidateInput) (*ValidateOutput, error) {
	// 构造系统提示词
	systemPrompt := `你是一个审核结果验证专家。请检查决策过程是否合理、一致。

验证要点：
1. 决策(action)与风险分数(risk_score)是否匹配
   - risk_score > 0.8 应该是 block
   - risk_score < 0.3 应该是 allow
   - 中间值可以是 review

2. 决策理由(reason)是否充分支撑决策

3. 置信度(confidence)是否合理

输出要求：
1. 必须是严格的 JSON 格式
2. 不要包含任何 markdown 标记
3. is_valid 为 false 时表示发现不一致，需要修正

输出格式：
{
  "final_action": "allow|block|review",
  "is_valid": true,
  "feedback": "验证反馈说明",
  "confidence": 0.92
}`

	// 构造用户消息
	userMsg := fmt.Sprintf(`原始内容：%s

分类结果：
- 类别：%s
- 风险分数：%.2f
- 置信度：%.2f

决策结果：
- 动作：%s
- 理由：%s
- 置信度：%.2f`,
		input.Content,
		input.ClassifyOut.Category,
		input.ClassifyOut.RiskScore,
		input.ClassifyOut.Confidence,
		input.DecideOut.Action,
		input.DecideOut.Reason,
		input.DecideOut.Confidence,
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
		// 验证节点失败时，默认通过验证
		return &ValidateOutput{
			FinalAction: input.DecideOut.Action,
			IsValid:     true,
			Feedback:    "验证模型调用失败，使用原始决策",
			Confidence:  input.DecideOut.Confidence,
		}, nil
	}

	// 解析输出
	var output ValidateOutput
	if err := json.Unmarshal([]byte(resp.Content), &output); err != nil {
		// 解析失败时，默认通过验证
		return &ValidateOutput{
			FinalAction: input.DecideOut.Action,
			IsValid:     true,
			Feedback:    "验证结果解析失败，使用原始决策",
			Confidence:  input.DecideOut.Confidence,
		}, nil
	}

	// 如果验证不通过，强制降级为 review
	if !output.IsValid {
		output.FinalAction = "review"
		output.Feedback = "[已修正] " + output.Feedback
	}

	// 确保最终动作有效
	if output.FinalAction == "" {
		output.FinalAction = "review"
	}

	return &output, nil
}
