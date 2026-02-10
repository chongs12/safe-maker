package nodes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino-ext/components/retriever/milvus2"
	"github.com/cloudwego/eino/schema"
)

// RAGInput RAG 节点输入
type RAGInput struct {
	Content  string         // 原始内容
	Category string         // 分类结果
	Metadata map[string]any // 其他元数据
}

// RAGOutput RAG 节点输出
type RAGOutput struct {
	CaseCount int    `json:"case_count"` // 案例数量
	Summary   string `json:"summary"`    // 案例摘要
	// SimilarCases 字段暂不暴露，避免序列化问题
}

// RAGNode RAG 检索节点
type RAGNode struct {
	retriever *milvus2.Retriever
	topK      int
}

// NewRAGNode 创建 RAG 节点
func NewRAGNode(retriever *milvus2.Retriever, topK int) *RAGNode {
	if topK <= 0 {
		topK = 3
	}
	return &RAGNode{
		retriever: retriever,
		topK:      topK,
	}
}

// Invoke 执行 RAG 检索
func (n *RAGNode) Invoke(ctx context.Context, input *RAGInput) (*RAGOutput, error) {
	// 使用原始内容进行检索
	docs, err := n.retriever.Retrieve(ctx, input.Content)
	if err != nil {
		return nil, fmt.Errorf("RAG 检索失败: %w", err)
	}

	// 限制返回数量
	if len(docs) > n.topK {
		docs = docs[:n.topK]
	}

	// 生成案例摘要
	summary := n.generateSummary(docs)

	return &RAGOutput{
		CaseCount: len(docs),
		Summary:   summary,
	}, nil
}

// generateSummary 生成案例摘要
func (n *RAGNode) generateSummary(docs []*schema.Document) string {
	if len(docs) == 0 {
		return "未找到相关历史案例"
	}

	// 简单摘要：列出前几个案例的标签
	labels := make(map[string]int)
	for _, doc := range docs {
		if label, ok := doc.MetaData["label"].(string); ok {
			labels[label]++
		}
	}

	summary := fmt.Sprintf("共检索到 %d 个相似案例: ", len(docs))
	for label, count := range labels {
		summary += fmt.Sprintf("%s(%d) ", label, count)
	}

	return summary
}

// MarshalJSON 自定义 JSON 序列化
func (o *RAGOutput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"case_count": o.CaseCount,
		"summary":    o.Summary,
	})
}
