package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudwego/kitex/server"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/safeflow-project/safeflow/internal/common"
	"github.com/safeflow-project/safeflow/internal/imagemod"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow/imagemoderationservice"
	"go.uber.org/zap"
)

func main() {
	// 1. 加载配置
	cfg, err := common.LoadConfig()
	if err != nil {
		log.Fatalf("无法加载配置: %v", err)
	}

	// 2. 初始化日志
	logger, _ := common.InitLogger()
	defer logger.Sync()

	// 3. 初始化 Prometheus 指标
	_ = common.InitMetrics("image-moderation")
	logger.Info("Prometheus 指标初始化完成")

	// 4. 初始化链路追踪
	if err := common.InitTracing("image-moderation", cfg.JaegerEndpoint); err != nil {
		logger.Error("链路追踪初始化失败", zap.Error(err))
	} else {
		defer common.CloseTracing()
		logger.Info("链路追踪初始化完成")
	}

	// 5. 初始化服务实现
	imageModService := imagemod.NewImageModerationServiceImpl(cfg, logger)

	// 6. 启动 REST API 服务
	go func() {
		r := gin.Default()
		
		// 添加 Prometheus metrics 端点
		r.GET("/metrics", gin.WrapH(promhttp.Handler()))
		
		// 注册图片审核API路由
		imageModService.RegisterRoutes(r)
		
		// 启动HTTP服务
		addr := ":" + cfg.ImageModerationPort
		logger.Info("图片审核API服务启动", zap.String("addr", addr))
		if err := r.Run(addr); err != nil {
			logger.Fatal("启动HTTP服务失败", zap.Error(err))
		}
	}()

	// 7. 启动 Kitex RPC 服务
	addr, _ := net.ResolveTCPAddr("tcp", "0.0.0.0:"+cfg.ImageModerationRPCPort)
	svr := safeflow.NewServer(imageModService, server.WithServiceAddr(addr))

	// 8. 服务注册到 Etcd (如果启用)
	var registry *common.ServiceRegistry
	if cfg.UseEtcdRegistry && len(cfg.EtcdEndpoints) > 0 {
		registry, err = common.NewServiceRegistry(cfg.EtcdEndpoints, logger)
		if err != nil {
			logger.Fatal("连接 Etcd 失败", zap.Error(err))
		}
		defer registry.Close()

		// 注册服务
		serviceAddr := "localhost:" + cfg.ImageModerationRPCPort
		if err := registry.Register(context.Background(), "safeflow.image-moderation", serviceAddr, 10); err != nil {
			logger.Fatal("注册服务失败", zap.Error(err))
		}

		// 优雅退出时注销服务
		go func() {
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
			<-sigChan
			logger.Info("正在注销服务...")
			registry.Deregister(context.Background(), "safeflow.image-moderation", serviceAddr)
			os.Exit(0)
		}()

		logger.Info("服务已注册到 Etcd", zap.Strings("endpoints", cfg.EtcdEndpoints))
	}

	// 9. 启动 RPC 服务
	logger.Info("图片审核RPC服务启动", zap.String("addr", addr.String()))
	err = svr.Run()
	if err != nil {
		log.Println(err.Error())
	}
}