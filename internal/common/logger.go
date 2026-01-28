package common

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// InitLogger 初始化 Zap 日志记录器
// 根据环境变量 ENV 决定使用开发模式还是生产模式
func InitLogger() (*zap.Logger, error) {
	var config zap.Config

	if os.Getenv("ENV") == "production" {
		// 生产环境配置: JSON 格式，Info 级别
		config = zap.NewProductionConfig()
	} else {
		// 开发环境配置: 控制台格式，Debug 级别，带颜色
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	return config.Build()
}
