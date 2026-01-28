package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	ark_embed "github.com/cloudwego/eino-ext/components/embedding/ark"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"github.com/safeflow-project/safeflow/internal/common"
)

// Case 定义案例数据结构
type Case struct {
	Text     string `json:"text"`     // 文本内容
	Label    string `json:"label"`    // 标签 (safe/unsafe)
	Category string `json:"category"` // 类别
}

const (
	CollectionName = "sensitive_cases" // 集合名称
	Dim            = 4096              // 向量维度 (Ark embedding 为 4096)
)

func main() {
	ctx := context.Background()

	// 0. 加载配置
	cfg, err := common.LoadConfig()
	if err != nil {
		log.Fatal("加载配置失败:", err)
	}

	// 1. 连接 Milvus 向量数据库
	c, err := client.NewClient(ctx, client.Config{
		Address: cfg.MilvusAddr,
	})
	if err != nil {
		log.Fatal("连接 milvus 失败:", err)
	}
	defer c.Close()

	// 2. 初始化 Embedder (用于将文本转换为向量)
	emb, err := ark_embed.NewEmbedder(ctx, &ark_embed.EmbeddingConfig{
		APIKey: cfg.ArkAPIKey,
		Model:  cfg.ArkEmbeddingModel,
	})
	if err != nil {
		log.Fatal("初始化 embedder 失败:", err)
	}

	// 3. 创建集合 (Collection)
	// 检查集合是否存在，如果存在则删除 (重新初始化)
	has, err := c.HasCollection(ctx, CollectionName)
	if err != nil {
		log.Fatal("检查集合失败:", err)
	}
	if has {
		err = c.DropCollection(ctx, CollectionName)
		if err != nil {
			log.Fatal("删除集合失败:", err)
		}
	}

	// 定义集合 Schema
	schema := &entity.Schema{
		CollectionName: CollectionName,
		Description:    "SafeFlow 敏感案例库",
		Fields: []*entity.Field{
			{
				Name:       "id",
				DataType:   entity.FieldTypeInt64,
				PrimaryKey: true,
				AutoID:     true, // 自动生成 ID
			},
			{
				Name:     "vector",
				DataType: entity.FieldTypeFloatVector,
				TypeParams: map[string]string{
					"dim": "4096", // 必须与 Embedding 模型维度一致
				},
			},
			{
				Name:     "content",
				DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{
					"max_length": "2048",
				},
			},
			{
				Name:     "label",
				DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{
					"max_length": "64",
				},
			},
		},
	}

	err = c.CreateCollection(ctx, schema, entity.DefaultShardNumber)
	if err != nil {
		log.Fatal("创建集合失败:", err)
	}

	// 4. 创建索引 (Index)
	// 使用 IVF_FLAT 索引，L2 (欧氏距离)
	idx, err := entity.NewIndexIvfFlat(entity.L2, Dim)
	if err != nil {
		log.Fatal("创建索引实体失败:", err)
	}
	err = c.CreateIndex(ctx, CollectionName, "vector", idx, false)
	if err != nil {
		log.Fatal("创建索引失败:", err)
	}

	// 5. 加载数据 (cases.json)
	file, err := os.ReadFile("assets/examples/cases.json")
	if err != nil {
		log.Fatal("读取 cases.json 失败:", err)
	}
	var cases []Case
	if err := json.Unmarshal(file, &cases); err != nil {
		log.Fatal("解析 cases 失败:", err)
	}

	// 6. 批量 Embedding 并插入数据
	var vectors [][]float32
	var contents []string
	var labels []string
	var texts []string

	for _, item := range cases {
		texts = append(texts, item.Text)
		contents = append(contents, item.Text)
		labels = append(labels, item.Label)
	}

	// 调用 API 获取 Embedding
	embeddings, err := emb.EmbedStrings(ctx, texts)
	if err != nil {
		log.Fatal("embedding 失败:", err)
	}
	log.Printf("Embedding 成功: 输入 %d 条, 返回 %d 条, 维度 %d", len(texts), len(embeddings), len(embeddings[0]))

	// 转换 [][]float64 为 [][]float32 (Milvus SDK 要求 float32)
	for _, v64 := range embeddings {
		v32 := make([]float32, len(v64))
		for i, f := range v64 {
			v32[i] = float32(f)
		}
		vectors = append(vectors, v32)
	}

	// 插入数据
	_, err = c.Insert(ctx, CollectionName, "",
		entity.NewColumnFloatVector("vector", Dim, vectors),
		entity.NewColumnVarChar("content", contents),
		entity.NewColumnVarChar("label", labels),
	)
	if err != nil {
		log.Fatal("插入数据失败:", err)
	}

	// 加载集合到内存 (以便进行搜索)
	err = c.LoadCollection(ctx, CollectionName, false)
	if err != nil {
		log.Fatal("加载集合失败:", err)
	}

	log.Println("成功初始化 Milvus，共导入", len(cases), "条案例。")
}
