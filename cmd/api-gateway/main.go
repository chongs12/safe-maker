package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/cloudwego/kitex/client"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/safeflow-project/safeflow/internal/common"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow"
	"github.com/safeflow-project/safeflow/kitex_gen/safeflow/llmagentservice"
	"github.com/safeflow-project/safeflow/kitex_gen/safeflow/ruleengineservice"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
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

	// 连接数据库 (用于管理 API)
	var db *gorm.DB
	for i := 0; i < 30; i++ {
		db, err = gorm.Open(mysql.Open(cfg.MySQLDSN), &gorm.Config{})
		if err == nil {
			break
		}
		logger.Warn("等待 MySQL 启动...", zap.Error(err))
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		logger.Fatal("连接 MySQL 失败", zap.Error(err))
	}

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
				Timestamp: time.Now(),
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

	// 批量审核 API
	r.POST("/submit/batch", func(c *gin.Context) {
		var reqBody struct {
			BatchID  string   `json:"batch_id"`
			Contents []string `json:"contents" binding:"required"`
			UserID   string   `json:"user_id"`
		}
		if err := c.ShouldBindJSON(&reqBody); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		results := make([]interface{}, 0, len(reqBody.Contents))
		ctx := context.Background()

		// 简单串行处理 (生产环境应改为并行)
		for _, content := range reqBody.Contents {
			reqID := uuid.New().String()
			scanReq := &safeflow.ScanRequest{RequestId: reqID, UserId: reqBody.UserID, Content: content}

			// 1. Rule Engine
			ruleResp, err := ruleClient.Scan(ctx, scanReq)
			if err != nil || ruleResp.Action == "block" {
				if err != nil {
					results = append(results, map[string]interface{}{"content": content, "error": err.Error()})
				} else {
					results = append(results, ruleResp)
				}
				continue
			}

			// 2. LLM Agent
			llmResp, err := llmClient.Scan(ctx, scanReq)
			if err != nil {
				results = append(results, map[string]interface{}{"content": content, "error": err.Error()})
			} else {
				results = append(results, llmResp)
			}
		}

		c.JSON(http.StatusOK, gin.H{"batch_id": reqBody.BatchID, "results": results})
	})

	// 管理 API (Admin) - Rule Studio
	admin := r.Group("/admin")
	{
		// 规则管理
		admin.GET("/rules", func(c *gin.Context) {
			var rules []common.Rule
			db.Order("priority desc").Find(&rules)
			c.JSON(http.StatusOK, rules)
		})
		admin.POST("/rules", func(c *gin.Context) {
			var rule common.Rule
			if err := c.ShouldBindJSON(&rule); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if err := db.Create(&rule).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusCreated, rule)
		})
		admin.PUT("/rules/:id", func(c *gin.Context) {
			id := c.Param("id")
			var rule common.Rule
			if err := db.First(&rule, id).Error; err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
				return
			}
			if err := c.ShouldBindJSON(&rule); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			db.Save(&rule)
			c.JSON(http.StatusOK, rule)
		})
		admin.DELETE("/rules/:id", func(c *gin.Context) {
			id := c.Param("id")
			db.Delete(&common.Rule{}, id)
			c.Status(http.StatusNoContent)
		})

		// 案例库管理 (Case Knowledge Base)
		admin.GET("/cases", func(c *gin.Context) {
			var cases []common.Case
			db.Order("created_at desc").Find(&cases)
			c.JSON(http.StatusOK, cases)
		})
		admin.POST("/cases", func(c *gin.Context) {
			var kase common.Case
			if err := c.ShouldBindJSON(&kase); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			// 这里应该同时调用 Milvus 插入向量，简化起见仅存 MySQL
			kase.IsCustom = true
			if err := db.Create(&kase).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusCreated, kase)
		})

		// 审计日志中心
		admin.GET("/audits", func(c *gin.Context) {
			var audits []common.AuditLog
			query := db.Model(&common.AuditLog{}).Order("created_at desc")

			if uid := c.Query("user_id"); uid != "" {
				query = query.Where("user_id = ?", uid)
			}
			if action := c.Query("action"); action != "" {
				query = query.Where("action = ?", action)
			}
			if source := c.Query("source"); source != "" {
				query = query.Where("source = ?", source)
			}

			page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
			pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
			offset := (page - 1) * pageSize

			var total int64
			query.Count(&total)
			query.Limit(pageSize).Offset(offset).Find(&audits)

			c.JSON(http.StatusOK, gin.H{
				"total": total,
				"page":  page,
				"data":  audits,
			})
		})

		// 版本管理 (快照)
		admin.POST("/versions/snapshot", func(c *gin.Context) {
			// 简单实现：将当前启用的规则导出为 JSON 并保存
			var rules []common.Rule
			db.Where("is_enabled = ?", true).Find(&rules)

			configBytes, _ := json.Marshal(rules)
			version := common.PolicyVersion{
				Version:   time.Now().Format("v20060102150405"),
				Type:      "rule",
				Config:    string(configBytes),
				CreatedAt: time.Now(),
			}

			if err := db.Create(&version).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusCreated, version)
		})
	}

	logger.Info("API 网关正在启动...", zap.String("port", cfg.GatewayPort))
	r.Run(":" + cfg.GatewayPort)
}
