package agent

import (
	"context"
	"encoding/json"
	"log"
	"os"

	ark_embed "github.com/cloudwego/eino-ext/components/embedding/ark"
	ark_model "github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/retriever/milvus2"
	"github.com/cloudwego/eino-ext/components/retriever/milvus2/search_mode"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// EinoAgent 封装了 Eino 运行图
type EinoAgent struct {
	runnable compose.Runnable[[]*schema.Message, *schema.Message]
}

// Arguments structs
// 定义工具参数结构体，用于 JSON 反序列化

type SearchArgs struct {
	Keyword string `json:"keyword"`
}

type CheckPoliticalArgs struct {
	Text string `json:"text"`
}

// NewEinoAgent 初始化并构建 Eino Agent
func NewEinoAgent(ctx context.Context) (*EinoAgent, error) {
	// 1. 初始化 Embedding (用于 Retriever)
	// 使用火山引擎 Ark Embedding 服务
	emb, err := ark_embed.NewEmbedder(ctx, &ark_embed.EmbeddingConfig{
		APIKey: os.Getenv("ARK_API_KEY"),
		Model:  os.Getenv("ARK_EMBEDDING_MODEL"),
	})
	if err != nil {
		log.Printf("警告: 初始化 embedding 失败: %v", err)
	}

	// 2. 初始化 Milvus Retriever (向量检索)
	// 用于从向量数据库中检索相似的历史违规案例
	var retriever *milvus2.Retriever
	if emb != nil {
		retriever, err = milvus2.NewRetriever(ctx, &milvus2.RetrieverConfig{
			ClientConfig: &milvusclient.ClientConfig{
				Address: os.Getenv("MILVUS_ADDR"),
			},
			Collection: "sensitive_cases",                          // 集合名称
			TopK:       3,                                          // 返回前 3 个最相似结果
			SearchMode: search_mode.NewApproximate(milvus2.COSINE), // 使用余弦相似度
			Embedding:  emb,                                        // 注入 Embedder
		})
		if err != nil {
			log.Printf("警告: 初始化 milvus retriever 失败: %v", err)
		}
	}

	// 3. 定义工具 (Tools)

	// 工具 1: 搜索敏感案例 (RAG)
	searchInfo := &schema.ToolInfo{
		Name: "search_sensitive_cases",
		Desc: "从数据库中搜索相似的敏感或违规案例。当无法判断时，请使用此工具查找参考。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"keyword": {
				Type:     schema.String,
				Desc:     "用于搜索的关键词",
				Required: true,
			},
		}),
	}
	searchTool := utils.NewTool(searchInfo, func(ctx context.Context, args *SearchArgs) (string, error) {
		if retriever == nil {
			return "错误: Retriever 未初始化", nil
		}
		docs, err := retriever.Retrieve(ctx, args.Keyword)
		if err != nil {
			return "错误: " + err.Error(), nil
		}
		if len(docs) == 0 {
			return "未找到相似案例。", nil
		}
		res, _ := json.Marshal(docs)
		return string(res), nil
	})

	// 工具 2: 检查政治实体 (Mock)
	// 这是一个示例工具，实际可以调用 NER 模型 API
	politicalInfo := &schema.ToolInfo{
		Name: "check_political_entities",
		Desc: "检查文本是否提及特定的政治实体或敏感人物。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"text": {
				Type:     schema.String,
				Desc:     "需要分析的文本",
				Required: true,
			},
		}),
	}
	politicalTool := utils.NewTool(politicalInfo, func(ctx context.Context, args *CheckPoliticalArgs) (string, error) {
		return "未发现敏感政治实体。", nil
	})

	tools := []tool.BaseTool{searchTool, politicalTool}

	// 4. 初始化 Chat Model (Ark)
	// 使用火山引擎 Ark 大语言模型服务
	chatModel, err := ark_model.NewChatModel(ctx, &ark_model.ChatModelConfig{
		APIKey: os.Getenv("ARK_API_KEY"),
		Model:  os.Getenv("ARK_MODEL_ID"),
	})
	if err != nil {
		return nil, err
	}

	// 绑定工具到模型
	// 这让模型知道有哪些工具可用，以及如何调用它们
	var toolInfos []*schema.ToolInfo
	for _, t := range tools {
		info, _ := t.Info(ctx)
		toolInfos = append(toolInfos, info)
	}

	// 绑定工具信息 (Model Bind Tools)
	err = chatModel.BindTools(toolInfos)
	if err != nil {
		return nil, err
	}
	toolModel := chatModel

	// 5. 构建 Eino Graph (ReAct 模式)
	// 节点：Model -> Tools

	// 创建 ToolsNode (负责执行工具调用)
	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools: tools,
	})
	if err != nil {
		return nil, err
	}

	// 创建图
	g := compose.NewGraph[[]*schema.Message, *schema.Message]()

	// 添加节点
	_ = g.AddChatModelNode("model", toolModel)
	_ = g.AddToolsNode("tools", toolsNode)

	// 添加边: Start -> Model
	_ = g.AddEdge(compose.START, "model")

	// 添加分支: Model -> Tools (如果模型决定调用工具) OR End (如果模型生成了最终回复)
	branch := compose.NewGraphBranch(func(_ context.Context, msg *schema.Message) (string, error) {
		if len(msg.ToolCalls) > 0 {
			return "tools", nil
		}
		return compose.END, nil
	}, map[string]bool{"tools": true, compose.END: true})

	_ = g.AddBranch("model", branch)

	// 添加边: Tools -> Model (工具执行结果返回给模型，形成循环)
	_ = g.AddEdge("tools", "model")

	// 编译图
	runnable, err := g.Compile(ctx)
	if err != nil {
		return nil, err
	}

	return &EinoAgent{runnable: runnable}, nil
}

// Run 执行 Agent 逻辑
func (a *EinoAgent) Run(ctx context.Context, content string) (string, error) {
	// 构造输入消息
	input := []*schema.Message{
		{
			Role:    schema.System,
			Content: "你是一个内容安全审核员。请分析用户的输入。如有必要，请使用工具。请以 JSON 格式回复：{\"action\": \"allow\"|\"block\"|\"review\", \"reason\": \"...\"}。",
		},
		{
			Role:    schema.User,
			Content: content,
		},
	}

	// 调用图
	resp, err := a.runnable.Invoke(ctx, input)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}
