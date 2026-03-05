package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudwego/kitex/server"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/safeflow-project/safeflow/internal/common"
	"github.com/safeflow-project/safeflow/internal/llmagent"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow/llmagentservice"
	"go.uber.org/zap"
)

func main() {
	cfg, err := common.LoadConfig()
	if err != nil {
		log.Fatalf("无法加载配置: %v", err)
	}
	logger, _ := common.InitLogger()
	addr, _ := net.ResolveTCPAddr("tcp", "0.0.0.0:"+cfg.LLMAgentPort)

	// 初始化 Prometheus 指标
	metrics := common.InitMetrics("llm-agent")
	logger.Info("Prometheus 指标初始化完成")

	// 记录服务启动指标
	metrics.LLMCallsTotal.WithLabelValues("service", "startup").Inc()

	// 启动 metrics HTTP 服务
	go func() {
		r := gin.Default()
		r.GET("/metrics", gin.WrapH(promhttp.Handler()))
		r.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "llm-agent"})
		})
		logger.Info("Metrics 服务启动", zap.String("port", ":9093"))
		r.Run(":9093")
	}()

	// 初始化服务实现 (包含 Eino Agent 的初始化)
	impl := llmagent.NewLLMAgentServiceImpl(context.Background(), cfg)

	// 创建 Kitex 服务端
	svr := safeflow.NewServer(impl, server.WithServiceAddr(addr))

	// 服务注册到 Etcd (如果启用)
	var registry *common.ServiceRegistry
	if cfg.UseEtcdRegistry && len(cfg.EtcdEndpoints) > 0 {
		registry, err = common.NewServiceRegistry(cfg.EtcdEndpoints, logger)
		if err != nil {
			logger.Fatal("连接 Etcd 失败", zap.Error(err))
		}
		defer registry.Close()

		// 注册服务
		serviceAddr := "localhost:" + cfg.LLMAgentPort
		if err := registry.Register(context.Background(), "safeflow.llm-agent", serviceAddr, 10); err != nil {
			logger.Fatal("注册服务失败", zap.Error(err))
		}

		// 优雅退出时注销服务
		go func() {
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
			<-sigChan
			logger.Info("正在注销服务...")
			registry.Deregister(context.Background(), "safeflow.llm-agent", serviceAddr)
			os.Exit(0)
		}()

		logger.Info("服务已注册到 Etcd", zap.Strings("endpoints", cfg.EtcdEndpoints))
	}

	// 启动服务
	logger.Info("LLM Agent 服务启动", zap.String("addr", addr.String()))
	err = svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}
