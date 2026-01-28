package main

import (
	"log"
	"net"

	"github.com/cloudwego/kitex/server"
	"github.com/safeflow-project/safeflow/internal/common"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow/ruleengineservice"
)

func main() {
	cfg, err := common.LoadConfig()
	if err != nil {
		log.Fatalf("无法加载配置: %v", err)
	}
	addr, _ := net.ResolveTCPAddr("tcp", "0.0.0.0:"+cfg.RuleEnginePort)

	// 创建 Kitex 服务端实例
	// 注入 RuleEngineServiceImpl 实现
	svr := safeflow.NewServer(new(RuleEngineServiceImpl), server.WithServiceAddr(addr))

	// 启动服务
	err = svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}
