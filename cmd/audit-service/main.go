package main

import (
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/safeflow-project/safeflow/internal/common"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// AuditLog 定义审计日志的数据库模型
type AuditLog struct {
	ID        uint      `gorm:"primaryKey"` // 自增主键
	RequestID string    `gorm:"index"`      // 请求 ID (建立索引以加速查询)
	UserID    string    // 用户 ID
	Action    string    // 动作 (allow, block, review)
	Reason    string    // 原因
	Source    string    // 来源 (rule-engine, llm-agent)
	CreatedAt time.Time // 创建时间
}

func main() {
	// 加载配置和日志
	cfg, _ := common.LoadConfig()
	logger, _ := common.InitLogger()

	// 1. 连接 MySQL 数据库
	// 使用重试机制，因为 MySQL 容器可能启动较慢
	var db *gorm.DB
	var err error
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

	// 自动迁移数据库结构 (创建表)
	db.AutoMigrate(&AuditLog{})

	// 2. 连接 NATS
	nc, _, err := common.InitNATS(cfg.NatsURL)
	if err != nil {
		logger.Fatal("连接 NATS 失败", zap.Error(err))
	}
	defer nc.Close()

	// 3. 订阅审计日志主题 (content.result)
	// 使用 JetStream 或普通订阅均可，这里使用普通订阅演示
	_, err = nc.Subscribe(common.SubjectContentResult, func(msg *nats.Msg) {
		var event common.ContentResultEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			logger.Error("反序列化事件失败", zap.Error(err))
			return
		}

		// 构建日志对象
		logEntry := AuditLog{
			RequestID: event.RequestID,
			UserID:    event.UserID,
			Action:    event.Action,
			Reason:    event.Reason,
			Source:    event.Source,
			CreatedAt: time.Now(),
		}

		// 写入数据库
		if err := db.Create(&logEntry).Error; err != nil {
			logger.Error("保存审计日志失败", zap.Error(err))
		} else {
			logger.Info("审计日志已保存", zap.String("id", event.RequestID), zap.String("action", event.Action))
		}
	})

	if err != nil {
		logger.Fatal("订阅主题失败", zap.Error(err))
	}

	logger.Info("审计服务已启动")
	// 阻塞主进程
	select {}
}
