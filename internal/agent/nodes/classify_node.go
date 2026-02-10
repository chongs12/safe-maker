package nodes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"
)

// ClassifyInput 分类节点输入
type ClassifyInput struct {
	Content string // 原始内容
}

// ClassifyOutput 分类节点输出
type ClassifyOutput struct {
	Category   string  `json:"category"`   // 分类: sexual/violence/spam/normal
	RiskScore  float64 `json:"risk_score"` // 风险分数 0-1
	NeedRAG    bool    `json:"need_rag"`   // 是否需要 RAG 检索
	Confidence float64 `json:"confidence"` // 置信度
}

// ClassifyNode 分类节点
type ClassifyNode struct {
	model *ark.ChatModel
}

// NewClassifyNode 创建分类节点
func NewClassifyNode(model *ark.ChatModel) *ClassifyNode {
	return &ClassifyNode{model: model}
}

// Invoke 执行分类
func (n *ClassifyNode) Invoke(ctx context.Context, input *ClassifyInput) (*ClassifyOutput, error) {
	// 构造系统提示词
	systemPrompt := `你是一个内容安全分类专家。请分析用户输入的内容，进行精确分类。

分类标准：
- sexual: 色情低俗、性暗示、成人内容
- violence: 暴力、血腥、恐怖、自残
- spam: 广告、营销、刷屏、无关内容
- politics: 敏感政治话题、不当言论
- normal: 正常、合规内容

输出要求：
1. 必须是严格的 JSON 格式
2. 不要包含任何 markdown 标记
3. risk_score 范围 0.0-1.0，分数越高风险越大
4. need_rag 为 true 表示需要进一步检索案例库确认

输出格式：
{
  "category": "分类标签",
  "risk_score": 0.85,
  "need_rag": true,
  "confidence": 0.92
}`

	messages := []*schema.Message{
		{Role: schema.System, Content: systemPrompt},
		{Role: schema.User, Content: input.Content},
	}

	resp, err := n.model.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("分类模型调用失败: %w", err)
	}

	// 解析输出
	var output ClassifyOutput
	if err := json.Unmarshal([]byte(resp.Content), &output); err != nil {
		return nil, fmt.Errorf("解析分类结果失败: %w, 原始输出: %s", err, resp.Content)
	}

	// 验证输出合理性
	if output.Category == "" {
		output.Category = "normal"
	}
	if output.RiskScore < 0 {
		output.RiskScore = 0
	}
	if output.RiskScore > 1 {
		output.RiskScore = 1
	}
	if output.Confidence < 0 {
		output.Confidence = 0
	}
	if output.Confidence > 1 {
		output.Confidence = 1
	}

	return &output, nil
}
