package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/cloudwego/kitex/client"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/safeflow-project/safeflow/internal/common"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow"
	"github.com/safeflow-project/safeflow/kitex_gen/safeflow/llmagentservice"
	"github.com/safeflow-project/safeflow/kitex_gen/safeflow/ruleengineservice"
	"go.uber.org/zap"
)

func main() {
	// 1. 加载配置
	cfg, err := common.LoadConfig()
	if err != nil {
		log.Fatalf("无法加载配置: %v", err)
	}

	// 2. 初始化日志记录器
	logger, err := common.InitLogger()
	if err != nil {
		log.Fatalf("无法初始化日志: %v", err)
	}
	defer logger.Sync()

	// 3. 初始化 NATS (用于发布审核审计日志)
	nc, _, err := common.InitNATS(cfg.NatsURL)
	if err != nil {
		logger.Fatal("连接 NATS 失败", zap.Error(err))
	}
	defer nc.Close()

	// 4. 初始化 Kitex 客户端 (RPC)
	// 初始化规则引擎服务客户端
	ruleClient, err := ruleengineservice.NewClient("safeflow.rule-engine", client.WithHostPorts(cfg.RuleEngineAddr))
	if err != nil {
		logger.Fatal("初始化规则引擎客户端失败", zap.Error(err))
	}

	// 初始化 LLM Agent 服务客户端
	llmClient, err := llmagentservice.NewClient("safeflow.llm-agent", client.WithHostPorts(cfg.LLMAgentAddr))
	if err != nil {
		logger.Fatal("初始化 LLM 客户端失败", zap.Error(err))
	}

	// 5. 启动 Gin Web 服务器
	r := gin.Default()

	// 配置 CORS 中间件 (允许跨域请求)
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// 定义提交审核的 API 接口
	r.POST("/submit", func(c *gin.Context) {
		var reqBody struct {
			Content string `json:"content" binding:"required"`
			UserID  string `json:"user_id"`
		}

		if err := c.ShouldBindJSON(&reqBody); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		requestID := uuid.New().String()
		ctx := context.Background()

		// 步骤 1: 调用规则引擎 (快速初筛)
		scanReq := &safeflow.ScanRequest{
			RequestId: requestID,
			UserId:    reqBody.UserID,
			Content:   reqBody.Content,
		}

		ruleResp, err := ruleClient.Scan(ctx, scanReq)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "规则引擎服务错误: " + err.Error()})
			return
		}

		// 辅助函数：发布审计日志事件
		publishAudit := func(resp *safeflow.ScanResponse) {
			event := common.ContentResultEvent{
				RequestID: resp.RequestId,
				UserID:    reqBody.UserID,
				Action:    resp.Action,
				Reason:    resp.Reason,
				Source:    resp.Source,
			}
			data, _ := json.Marshal(event)
			nc.Publish(common.SubjectContentResult, data)
		}

		// 如果规则引擎拦截 (Block)，直接返回，不再调用 LLM
		if ruleResp.Action == "block" {
			publishAudit(ruleResp)
			c.JSON(http.StatusOK, ruleResp)
			return
		}

		// 步骤 2: 调用 LLM Agent (如果通过了规则引擎)
		// 这一步耗时较长，涉及大模型推理和工具调用
		llmResp, err := llmClient.Scan(ctx, scanReq)
		if err != nil {
			// 如果 LLM 服务不可用，降级处理为人工审核 (Review)
			c.JSON(http.StatusOK, gin.H{
				"request_id": requestID,
				"action":     "review",
				"reason":     "LLM 服务暂时不可用: " + err.Error(),
				"source":     "gateway",
			})
			return
		}

		// 发布最终结果的审计日志
		publishAudit(llmResp)
		c.JSON(http.StatusOK, llmResp)
	})

	logger.Info("API 网关正在启动...", zap.String("port", cfg.GatewayPort))
	r.Run(":" + cfg.GatewayPort)
}
