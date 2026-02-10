package main

import (
	"context"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/safeflow-project/safeflow/internal/common"
	"github.com/safeflow-project/safeflow/internal/rule"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow"
	"gorm.io/gorm"
)

// RuleEngineServiceImpl 实现 RuleEngineService 接口
type RuleEngineServiceImpl struct {
	db          *gorm.DB
	rules       []common.Rule
	mu          sync.RWMutex
	lastRefresh time.Time
	acMatcher   *rule.AhoCorasick // AC自动机匹配器
	useAC       bool              // 是否使用AC自动机
}

// NewRuleEngineServiceImpl 创建实例
func NewRuleEngineServiceImpl(db *gorm.DB) *RuleEngineServiceImpl {
	s := &RuleEngineServiceImpl{
		db:    db,
		useAC: true, // 默认启用AC自动机
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

	// 构建AC自动机
	if s.useAC {
		s.buildACMatcher()
	}

	s.mu.Unlock()
	log.Printf("已加载 %d 条规则，AC自动机构建完成", len(rules))
}

// buildACMatcher 构建AC自动机
func (s *RuleEngineServiceImpl) buildACMatcher() {
	ac := rule.NewAhoCorasick()

	for _, r := range s.rules {
		switch r.Type {
		case "keyword":
			ac.AddPattern(r.Pattern, r.Action, r.Group)
		case "regex":
			if err := ac.AddRegexPattern(r.Pattern, r.Action, r.Group); err != nil {
				log.Printf("添加正则规则失败 [%s]: %v", r.Pattern, err)
			}
		}
	}

	// 添加默认的PII检测规则
	ac.AddRegexPattern(`[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}`, "block", "privacy") // 邮箱
	ac.AddRegexPattern(`\b1[3-9]\d{9}\b`, "block", "privacy")                          // 手机号

	ac.Build()
	s.acMatcher = ac
}

func (s *RuleEngineServiceImpl) refreshRulesLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		s.loadRules()
	}
}

// Scan 处理内容扫描请求
// 使用AC自动机进行高效匹配
func (s *RuleEngineServiceImpl) Scan(ctx context.Context, req *safeflow.ScanRequest) (resp *safeflow.ScanResponse, err error) {
	log.Printf("[RuleEngine] 收到请求: ID=%s, Content=%s", req.RequestId, req.Content)

	// 初始化默认响应 (允许通过)
	resp = &safeflow.ScanResponse{
		RequestId: req.RequestId,
		Source:    "rule-engine",
		Action:    "allow",
	}

	s.mu.RLock()
	ac := s.acMatcher
	useAC := s.useAC
	s.mu.RUnlock()

	// 使用AC自动机匹配
	if useAC && ac != nil {
		if result := ac.SearchWithRegex(req.Content); result != nil {
			resp.Action = result.Action
			if result.IsRegex {
				resp.Reason = "检测到违规内容 (正则匹配): " + result.Group
			} else {
				resp.Reason = "检测到敏感关键词: " + result.Pattern
			}
			return resp, nil
		}
	} else {
		// 降级到简单匹配
		resp = s.scanWithSimpleRules(req)
	}

	return resp, nil
}

// scanWithSimpleRules 使用简单规则匹配（降级方案）
func (s *RuleEngineServiceImpl) scanWithSimpleRules(req *safeflow.ScanRequest) *safeflow.ScanResponse {
	resp := &safeflow.ScanResponse{
		RequestId: req.RequestId,
		Source:    "rule-engine",
		Action:    "allow",
	}

	// 默认敏感词（当数据库不可用时）
	sensitiveWords := []string{
		"fuck", "gambling", "terror", "bomb", "kill", "suicide",
		"casino", "drugs", "heroin",
		"兼职", "刷单", "加微信", "博彩", "赌博", "炸弹", "自杀", "毒品", "海洛因",
		"高薪", "日入", "不限经验",
	}
	emailRegex := regexp.MustCompile(`[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}`)
	phoneRegex := regexp.MustCompile(`\b\d{11}\b`)

	lowerContent := strings.ToLower(req.Content)

	// 检查敏感词
	for _, word := range sensitiveWords {
		if strings.Contains(lowerContent, word) {
			resp.Action = "block"
			resp.Reason = "检测到敏感关键词: " + word
			return resp
		}
	}

	// 检查正则
	if emailRegex.MatchString(req.Content) {
		resp.Action = "block"
		resp.Reason = "检测到隐私信息: 电子邮箱"
		return resp
	}
	if phoneRegex.MatchString(req.Content) {
		resp.Action = "block"
		resp.Reason = "检测到隐私信息: 手机号码"
		return resp
	}

	return resp
}
