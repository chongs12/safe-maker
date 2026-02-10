package main

import (
	"fmt"
	"log"

	"github.com/safeflow-project/safeflow/internal/common"
	"github.com/safeflow-project/safeflow/internal/multimodal"
	"go.uber.org/zap"
)

func main() {
	fmt.Println("=== SafeFlow 多模态审核测试 ===")

	// 加载配置
	cfg, err := common.LoadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化日志
	logger, _ := common.InitLogger()

	// 初始化链路追踪
	if err := common.InitTracing("multimodal-test", cfg.JaegerEndpoint); err != nil {
		logger.Error("链路追踪初始化失败", zap.Error(err))
	}
	defer common.CloseTracing()

	// 测试图片审核功能
	fmt.Println("\n--- 图片审核测试 ---")
	multimodal.TestImageModeration(cfg, logger)

	fmt.Println("\n=== 测试完成 ===")
	fmt.Println("\n💡 下一步建议:")
	fmt.Println("1. 集成实际的OCR服务（如百度OCR）")
	fmt.Println("2. 集成图像审核API（如火山引擎）")
	fmt.Println("3. 添加更多测试用例")
	fmt.Println("4. 部署独立的图片审核服务")
}
