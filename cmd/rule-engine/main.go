package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudwego/kitex/server"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/safeflow-project/safeflow/cmd/rule-engine/impl"
	"github.com/safeflow-project/safeflow/internal/common"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow/ruleengineservice"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	cfg, err := common.LoadConfig()
	if err != nil {
		log.Fatalf("无法加载配置: %v", err)
	}
	logger, _ := common.InitLogger()

	// 初始化 Prometheus 指标
	metrics := common.InitMetrics("rule-engine")
	logger.Info("Prometheus 指标初始化完成")

	// 记录服务启动指标
	metrics.RuleEngineDuration.WithLabelValues("service_startup").Observe(0)

	// 启动 metrics HTTP 服务
	go func() {
		r := gin.Default()
		r.GET("/metrics", gin.WrapH(promhttp.Handler()))
		r.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "rule-engine"})
		})
		logger.Info("Metrics 服务启动", zap.String("port", ":9092"))
		r.Run(":9092")
	}()

	// 连接数据库
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
	// 自动迁移
	db.AutoMigrate(&common.Rule{})

	// 初始化一些默认规则 (如果为空)
	var count int64
	db.Model(&common.Rule{}).Count(&count)
	if count == 0 {
		defaultRules := []common.Rule{
			{Pattern: "gambling", Type: "keyword", Action: "block", Group: "gambling", Description: "Gambling keyword"},
			{Pattern: "兼职", Type: "keyword", Action: "block", Group: "spam", Description: "兼职刷单"},
			{Pattern: "加微信", Type: "keyword", Action: "block", Group: "spam", Description: "引流"},
			{Pattern: `\b\d{11}\b`, Type: "regex", Action: "block", Group: "privacy", Description: "手机号"},
		}
		db.Create(&defaultRules)
		logger.Info("已初始化默认规则")
	}

	addr, _ := net.ResolveTCPAddr("tcp", "0.0.0.0:"+cfg.RuleEnginePort)

	// 创建 Kitex 服务端实例
	// 注入 RuleEngineServiceImpl 实现
	svr := safeflow.NewServer(impl.NewRuleEngineServiceImpl(db), server.WithServiceAddr(addr))

	// 服务注册到 Etcd (如果启用)
	var registry *common.ServiceRegistry
	if cfg.UseEtcdRegistry && len(cfg.EtcdEndpoints) > 0 {
		registry, err = common.NewServiceRegistry(cfg.EtcdEndpoints, logger)
		if err != nil {
			logger.Fatal("连接 Etcd 失败", zap.Error(err))
		}
		defer registry.Close()

		// 注册服务
		serviceAddr := "localhost:" + cfg.RuleEnginePort
		if err := registry.Register(context.Background(), "safeflow.rule-engine", serviceAddr, 10); err != nil {
			logger.Fatal("注册服务失败", zap.Error(err))
		}

		// 优雅退出时注销服务
		go func() {
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
			<-sigChan
			logger.Info("正在注销服务...")
			registry.Deregister(context.Background(), "safeflow.rule-engine", serviceAddr)
			os.Exit(0)
		}()

		logger.Info("服务已注册到 Etcd", zap.Strings("endpoints", cfg.EtcdEndpoints))
	}

	// 启动服务
	logger.Info("规则引擎服务启动", zap.String("addr", addr.String()))
	err = svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}
