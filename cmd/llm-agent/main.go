package main

import (
	"context"
	"log"
	"net"

	"github.com/cloudwego/kitex/server"
	"github.com/safeflow-project/safeflow/internal/common"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow/llmagentservice"
)

func main() {
	cfg, err := common.LoadConfig()
	if err != nil {
		log.Fatalf("无法加载配置: %v", err)
	}
	addr, _ := net.ResolveTCPAddr("tcp", "0.0.0.0:"+cfg.LLMAgentPort)

	// 初始化服务实现 (包含 Eino Agent 的初始化)
	impl := NewLLMAgentServiceImpl(context.Background())

	// 创建 Kitex 服务端
	svr := safeflow.NewServer(impl, server.WithServiceAddr(addr))

	// 启动服务
	err = svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}
