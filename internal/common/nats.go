package common

import (
	"context"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// InitNATS 连接 NATS 并初始化 JetStream 上下文
// url: NATS 服务器地址
func InitNATS(url string) (*nats.Conn, jetstream.JetStream, error) {
	// 连接 NATS，配置重连策略
	nc, err := nats.Connect(url, nats.RetryOnFailedConnect(true), nats.MaxReconnects(5), nats.ReconnectWait(time.Second))
	if err != nil {
		return nil, nil, err
	}

	// 创建 JetStream 上下文
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, nil, err
	}

	// 确保 Stream (消息流) 存在
	// 如果 Stream 不存在则创建，如果存在则更新配置
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      StreamName,
		Subjects:  []string{"content.>"}, // 匹配 content 开头的所有主题
		Retention: jetstream.WorkQueuePolicy, // 工作队列模式 (消息被消费后删除)
	})
	if err != nil {
		log.Printf("警告: 创建或更新 Stream 失败 (可能已存在): %v", err)
	}

	return nc, js, nil
}
