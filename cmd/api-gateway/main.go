package main

import (
	"context"
	"log"
	"time"

	"github.com/cloudwego/kitex/client"
	"github.com/safeflow-project/safeflow/internal/common"
	"github.com/safeflow-project/safeflow/internal/gateway"
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

	// 初始化 Prometheus 指标
	metrics := common.InitMetrics("api-gateway")
	logger.Info("Prometheus 指标初始化完成")

	// 初始化链路追踪
	if err := common.InitTracing("api-gateway", cfg.JaegerEndpoint); err != nil {
		logger.Error("链路追踪初始化失败", zap.Error(err))
	} else {
		defer common.CloseTracing()
		logger.Info("链路追踪初始化完成")
	}

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

	// 自动迁移数据库表
	logger.Info("开始数据库迁移...")
	if err := db.AutoMigrate(
		&common.Rule{},
		&common.Case{},
		&common.AuditLog{},
		&common.AuditTask{},
		&common.ReviewTask{},
		&common.ModerationResult{},
		&common.PolicyVersion{},
		&common.CallbackTask{},
	); err != nil {
		logger.Fatal("数据库迁移失败", zap.Error(err))
	}
	logger.Info("数据库迁移完成")

	// 3. 初始化 NATS (用于发布审核审计日志)
	nc, _, err := common.InitNATS(cfg.NatsURL)
	if err != nil {
		logger.Fatal("连接 NATS 失败", zap.Error(err))
	}
	defer nc.Close()
	// 4. 初始化 Kitex 客户端 (RPC)
	var ruleClient ruleengineservice.Client
	var llmClient llmagentservice.Client

	if cfg.UseEtcdRegistry && len(cfg.EtcdEndpoints) > 0 {
		// 使用 Etcd 服务发现
		logger.Info("使用 Etcd 服务发现", zap.Strings("endpoints", cfg.EtcdEndpoints))

		discovery, err := common.NewServiceDiscovery(cfg.EtcdEndpoints, logger)
		if err != nil {
			logger.Fatal("连接 Etcd 失败", zap.Error(err))
		}
		defer discovery.Close()

		// 发现规则引擎服务
		ruleAddrs, err := discovery.Discover(context.Background(), "safeflow.rule-engine")
		if err != nil {
			logger.Fatal("发现规则引擎服务失败", zap.Error(err))
		}
		logger.Info("发现规则引擎服务", zap.Strings("addrs", ruleAddrs))

		// 发现 LLM Agent 服务
		llmAddrs, err := discovery.Discover(context.Background(), "safeflow.llm-agent")
		if err != nil {
			logger.Fatal("发现 LLM Agent 服务失败", zap.Error(err))
		}
		logger.Info("发现 LLM Agent 服务", zap.Strings("addrs", llmAddrs))

		// 初始化客户端
		ruleClient, err = ruleengineservice.NewClient("safeflow.rule-engine", client.WithHostPorts(ruleAddrs...))
		if err != nil {
			logger.Fatal("初始化规则引擎客户端失败", zap.Error(err))
		}

		llmClient, err = llmagentservice.NewClient("safeflow.llm-agent", client.WithHostPorts(llmAddrs...))
		if err != nil {
			logger.Fatal("初始化 LLM 客户端失败", zap.Error(err))
		}
	} else {
		// 使用静态地址
		logger.Info("使用静态服务地址", zap.String("rule_engine", cfg.RuleEngineAddr), zap.String("llm_agent", cfg.LLMAgentAddr))

		ruleClient, err = ruleengineservice.NewClient("safeflow.rule-engine", client.WithHostPorts(cfg.RuleEngineAddr))
		if err != nil {
			logger.Fatal("初始化规则引擎客户端失败", zap.Error(err))
		}

		llmClient, err = llmagentservice.NewClient("safeflow.llm-agent", client.WithHostPorts(cfg.LLMAgentAddr))
		if err != nil {
			logger.Fatal("初始化 LLM 客户端失败", zap.Error(err))
		}
	}

	server := gateway.NewServer(cfg, logger, metrics, db, nc, ruleClient, llmClient)
	r := server.Router()
	logger.Info("API 网关正在启动...", zap.String("port", cfg.GatewayPort))
	r.Run(":" + cfg.GatewayPort)
}
