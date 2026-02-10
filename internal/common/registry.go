package common

import (
	"context"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

// ServiceRegistry 封装 Etcd 服务注册功能
type ServiceRegistry struct {
	client  *clientv3.Client
	leaseID clientv3.LeaseID
	logger  *zap.Logger
}

// NewServiceRegistry 创建服务注册中心实例
func NewServiceRegistry(endpoints []string, logger *zap.Logger) (*ServiceRegistry, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("连接 Etcd 失败: %w", err)
	}

	return &ServiceRegistry{
		client: cli,
		logger: logger,
	}, nil
}

// Register 注册服务到 Etcd
// serviceName: 服务名称 (如: safeflow.rule-engine)
// addr: 服务地址 (如: localhost:8881)
// ttl: 租约过期时间(秒)
func (r *ServiceRegistry) Register(ctx context.Context, serviceName, addr string, ttl int64) error {
	// 创建租约
	leaseResp, err := r.client.Grant(ctx, ttl)
	if err != nil {
		return fmt.Errorf("创建租约失败: %w", err)
	}
	r.leaseID = leaseResp.ID

	// 服务注册的 key: /safeflow/services/{serviceName}/{addr}
	key := fmt.Sprintf("/safeflow/services/%s/%s", serviceName, addr)
	value := addr

	// 将服务地址注册到 Etcd，绑定租约
	_, err = r.client.Put(ctx, key, value, clientv3.WithLease(r.leaseID))
	if err != nil {
		return fmt.Errorf("注册服务失败: %w", err)
	}

	r.logger.Info("服务已注册到 Etcd",
		zap.String("service", serviceName),
		zap.String("addr", addr),
		zap.Int64("lease_ttl", ttl),
	)

	// 启动保活协程
	go r.keepAlive(ctx, serviceName)

	return nil
}

// keepAlive 保持租约存活
func (r *ServiceRegistry) keepAlive(ctx context.Context, serviceName string) {
	ch, err := r.client.KeepAlive(ctx, r.leaseID)
	if err != nil {
		r.logger.Error("启动租约保活失败", zap.Error(err))
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ka, ok := <-ch:
			if !ok {
				r.logger.Warn("租约保活通道关闭", zap.String("service", serviceName))
				return
			}
			r.logger.Debug("租约保活成功",
				zap.String("service", serviceName),
				zap.Int64("lease_id", int64(ka.ID)),
			)
		}
	}
}

// Deregister 服务下线
func (r *ServiceRegistry) Deregister(ctx context.Context, serviceName, addr string) error {
	key := fmt.Sprintf("/safeflow/services/%s/%s", serviceName, addr)
	_, err := r.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("注销服务失败: %w", err)
	}

	// 撤销租约
	if r.leaseID != 0 {
		_, err = r.client.Revoke(ctx, r.leaseID)
		if err != nil {
			r.logger.Warn("撤销租约失败", zap.Error(err))
		}
	}

	r.logger.Info("服务已从 Etcd 注销",
		zap.String("service", serviceName),
		zap.String("addr", addr),
	)
	return nil
}

// Close 关闭连接
func (r *ServiceRegistry) Close() error {
	return r.client.Close()
}

// ServiceDiscovery 封装 Etcd 服务发现功能
type ServiceDiscovery struct {
	client *clientv3.Client
	logger *zap.Logger
}

// NewServiceDiscovery 创建服务发现实例
func NewServiceDiscovery(endpoints []string, logger *zap.Logger) (*ServiceDiscovery, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("连接 Etcd 失败: %w", err)
	}

	return &ServiceDiscovery{
		client: cli,
		logger: logger,
	}, nil
}

// Discover 发现服务地址列表
func (d *ServiceDiscovery) Discover(ctx context.Context, serviceName string) ([]string, error) {
	prefix := fmt.Sprintf("/safeflow/services/%s/", serviceName)
	resp, err := d.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("发现服务失败: %w", err)
	}

	var addrs []string
	for _, kv := range resp.Kvs {
		addrs = append(addrs, string(kv.Value))
	}

	if len(addrs) == 0 {
		return nil, fmt.Errorf("未发现可用服务: %s", serviceName)
	}

	d.logger.Debug("发现服务",
		zap.String("service", serviceName),
		zap.Strings("addrs", addrs),
	)

	return addrs, nil
}

// Watch 监听服务变化
func (d *ServiceDiscovery) Watch(ctx context.Context, serviceName string, callback func(addrs []string)) {
	prefix := fmt.Sprintf("/safeflow/services/%s/", serviceName)
	watchChan := d.client.Watch(ctx, prefix, clientv3.WithPrefix())

	go func() {
		for wresp := range watchChan {
			if wresp.Err() != nil {
				d.logger.Error("监听服务变化出错", zap.Error(wresp.Err()))
				continue
			}

			// 重新获取服务列表
			addrs, err := d.Discover(ctx, serviceName)
			if err != nil {
				d.logger.Warn("重新发现服务失败", zap.Error(err))
				continue
			}

			callback(addrs)
		}
	}()
}

// Close 关闭连接
func (d *ServiceDiscovery) Close() error {
	return d.client.Close()
}

// EtcdResolver 实现 Kitex 的 Resolver 接口
type EtcdResolver struct {
	discovery   *ServiceDiscovery
	serviceName string
}

// NewEtcdResolver 创建 Etcd 解析器
func NewEtcdResolver(endpoints []string, serviceName string, logger *zap.Logger) (*EtcdResolver, error) {
	discovery, err := NewServiceDiscovery(endpoints, logger)
	if err != nil {
		return nil, err
	}

	return &EtcdResolver{
		discovery:   discovery,
		serviceName: serviceName,
	}, nil
}

// Resolve 解析服务地址 (简化版，实际需实现 Kitex Resolver 接口)
func (r *EtcdResolver) Resolve(ctx context.Context) ([]string, error) {
	return r.discovery.Discover(ctx, r.serviceName)
}

// Close 关闭
func (r *EtcdResolver) Close() error {
	return r.discovery.Close()
}
