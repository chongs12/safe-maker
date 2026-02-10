package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/retriever/milvus2"
	"github.com/safeflow-project/safeflow/internal/agent/nodes"
)

// PipelineAgent 多阶段流水线 Agent
type PipelineAgent struct {
	classifyNode *nodes.ClassifyNode
	ragNode      *nodes.RAGNode
	decideNode   *nodes.DecideNode
	validateNode *nodes.ValidateNode
	chatModel    *ark.ChatModel
	retriever    *milvus2.Retriever
}

// NewPipelineAgent 创建流水线 Agent
func NewPipelineAgent(
	chatModel *ark.ChatModel,
	retriever *milvus2.Retriever,
) *PipelineAgent {
	return &PipelineAgent{
		classifyNode: nodes.NewClassifyNode(chatModel),
		ragNode:      nodes.NewRAGNode(retriever, 3),
		decideNode:   nodes.NewDecideNode(chatModel),
		validateNode: nodes.NewValidateNode(chatModel),
		chatModel:    chatModel,
		retriever:    retriever,
	}
}

// Run 执行多阶段流水线
func (p *PipelineAgent) Run(ctx context.Context, content string) (string, error) {
	log.Printf("[PipelineAgent] 开始处理内容: %s", content)

	// 阶段 1: 分类
	log.Printf("[Pipeline] 阶段 1: 分类")
	classifyInput := &nodes.ClassifyInput{Content: content}
	classifyOut, err := p.classifyNode.Invoke(ctx, classifyInput)
	if err != nil {
		return "", fmt.Errorf("分类阶段失败: %w", err)
	}
	log.Printf("[Pipeline] 分类结果: %+v", classifyOut)

	// 阶段 2: RAG 检索（如果需要）
	var ragOut *nodes.RAGOutput
	if classifyOut.NeedRAG {
		log.Printf("[Pipeline] 阶段 2: RAG 检索")
		ragInput := &nodes.RAGInput{
			Content:  content,
			Category: classifyOut.Category,
		}
		ragOut, err = p.ragNode.Invoke(ctx, ragInput)
		if err != nil {
			log.Printf("[Pipeline] RAG 检索失败: %v", err)
			// RAG 失败不影响流程继续
		} else {
			log.Printf("[Pipeline] RAG 结果: %s", ragOut.Summary)
		}
	}

	// 阶段 3: 决策
	log.Printf("[Pipeline] 阶段 3: 决策")
	decideInput := &nodes.DecideInput{
		Content:     content,
		ClassifyOut: classifyOut,
		RAGOut:      ragOut,
	}
	decideOut, err := p.decideNode.Invoke(ctx, decideInput)
	if err != nil {
		return "", fmt.Errorf("决策阶段失败: %w", err)
	}
	log.Printf("[Pipeline] 决策结果: %+v", decideOut)

	// 阶段 4: 验证
	log.Printf("[Pipeline] 阶段 4: 验证")
	validateInput := &nodes.ValidateInput{
		Content:     content,
		ClassifyOut: classifyOut,
		DecideOut:   decideOut,
		RAGOut:      ragOut,
	}
	validateOut, err := p.validateNode.Invoke(ctx, validateInput)
	if err != nil {
		log.Printf("[Pipeline] 验证阶段失败: %v", err)
		// 验证失败不影响最终结果
		validateOut = &nodes.ValidateOutput{
			FinalAction: decideOut.Action,
			IsValid:     true,
			Feedback:    "验证跳过",
			Confidence:  decideOut.Confidence,
		}
	}
	log.Printf("[Pipeline] 验证结果: %+v", validateOut)

	// 构造最终响应
	finalResp := map[string]any{
		"action":          validateOut.FinalAction,
		"reason":          decideOut.Reason,
		"confidence":      validateOut.Confidence,
		"classify_result": classifyOut,
		"rag_result":      ragOut,
		"decision_result": decideOut,
		"validate_result": validateOut,
	}

	jsonResp, err := json.Marshal(finalResp)
	if err != nil {
		return "", fmt.Errorf("序列化最终结果失败: %w", err)
	}

	log.Printf("[PipelineAgent] 处理完成")
	return string(jsonResp), nil
}
