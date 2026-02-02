package main

import (
	"log"
	"net"
	"time"

	"github.com/cloudwego/kitex/server"
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
	svr := safeflow.NewServer(NewRuleEngineServiceImpl(db), server.WithServiceAddr(addr))

	// 启动服务
	err = svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}
