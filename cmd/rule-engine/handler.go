package main

import (
	"context"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/safeflow-project/safeflow/internal/common"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow"
	"gorm.io/gorm"
)

// RuleEngineServiceImpl 实现 RuleEngineService 接口
type RuleEngineServiceImpl struct {
	db          *gorm.DB
	rules       []common.Rule
	mu          sync.RWMutex
	lastRefresh time.Time
}

// NewRuleEngineServiceImpl 创建实例
func NewRuleEngineServiceImpl(db *gorm.DB) *RuleEngineServiceImpl {
	s := &RuleEngineServiceImpl{
		db: db,
	}
	// 初始加载规则
	s.loadRules()
	// 启动后台刷新 (每分钟)
	go s.refreshRulesLoop()
	return s
}

func (s *RuleEngineServiceImpl) loadRules() {
	var rules []common.Rule
	if err := s.db.Where("is_enabled = ?", true).Order("priority desc").Find(&rules).Error; err != nil {
		log.Printf("加载规则失败: %v", err)
		return
	}
	s.mu.Lock()
	s.rules = rules
	s.lastRefresh = time.Now()
	s.mu.Unlock()
	log.Printf("已加载 %d 条规则", len(rules))
}

func (s *RuleEngineServiceImpl) refreshRulesLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		s.loadRules()
	}
}

// Scan 处理内容扫描请求
// 使用简单的关键词匹配和正则表达式进行快速过滤
func (s *RuleEngineServiceImpl) Scan(ctx context.Context, req *safeflow.ScanRequest) (resp *safeflow.ScanResponse, err error) {
	log.Printf("[RuleEngine] 收到请求: ID=%s, Content=%s", req.RequestId, req.Content)

	// 1. 定义敏感词库和正则模式
	// 在实际生产中，这些应该从配置中心或数据库加载，并使用 AC 自动机等高效算法
	sensitiveWords := []string{
		"fuck", "gambling", "terror", "bomb", "kill", "suicide",
		"casino", "drugs", "heroin",
		"兼职", "刷单", "加微信", "博彩", "赌博", "炸弹", "自杀", "毒品", "海洛因",
		"高薪", "日入", "不限经验",
	}
	emailRegex := regexp.MustCompile(`[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}`)
	phoneRegex := regexp.MustCompile(`\b\d{11}\b`) // 简单的中国手机号匹配

	// 初始化默认响应 (允许通过)
	resp = &safeflow.ScanResponse{
		RequestId: req.RequestId,
		Source:    "rule-engine",
		Action:    "allow",
	}

	lowerContent := strings.ToLower(req.Content)

	// 2. 检查敏感词
	for _, word := range sensitiveWords {
		if strings.Contains(lowerContent, word) {
			resp.Action = "block"
			resp.Reason = "检测到敏感关键词: " + word
			return resp, nil
		}
	}

	// 3. 检查正则表达式 (个人隐私信息 PII)
	if emailRegex.MatchString(req.Content) {
		resp.Action = "block"
		resp.Reason = "检测到隐私信息: 电子邮箱"
		return resp, nil
	}
	if phoneRegex.MatchString(req.Content) {
		resp.Action = "block"
		resp.Reason = "检测到隐私信息: 手机号码"
		return resp, nil
	}

	return resp, nil
}
