package rulegen

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/schema"
	"github.com/safeflow-project/safeflow/internal/common"
	"go.uber.org/zap"
)

// RuleGenerator AI辅助规则生成器
type RuleGenerator struct {
	model  *ark.ChatModel
	logger *zap.Logger
}

// RuleTemplate 规则模板
type RuleTemplate struct {
	Category    string   `json:"category"`    // 规则类别
	Patterns    []string `json:"patterns"`    // 匹配模式
	Description string   `json:"description"` // 描述
	Action      string   `json:"action"`      // 动作: block/review/allow
	RiskLevel   string   `json:"risk_level"`  // 风险等级: low/medium/high/critical
}

// NewRuleGenerator 创建规则生成器
func NewRuleGenerator(ctx context.Context, cfg *common.Config, logger *zap.Logger) (*RuleGenerator, error) {
	// 初始化 LLM 模型
	model, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey: cfg.ArkAPIKey,
		Model:  cfg.ArkModelID,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化模型失败: %w", err)
	}

	return &RuleGenerator{
		model:  model,
		logger: logger,
	}, nil
}

// GenerateRules 基于违规案例生成规则
func (rg *RuleGenerator) GenerateRules(ctx context.Context, cases []common.Case) ([]RuleTemplate, error) {
	if len(cases) == 0 {
		return nil, fmt.Errorf("违规案例不能为空")
	}

	// 构造提示词
	prompt := rg.buildPrompt(cases)

	// 调用 LLM
	resp, err := rg.model.Generate(ctx, []*schema.Message{
		{
			Role:    schema.User,
			Content: prompt,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("调用模型失败: %w", err)
	}

	// 解析响应
	var rules []RuleTemplate
	if err := json.Unmarshal([]byte(resp.Content), &rules); err != nil {
		// 如果 JSON 解析失败，尝试提取规则内容
		rules = rg.extractRulesFromText(resp.Content)
	}

	rg.logger.Info("规则生成完成", zap.Int("generated_count", len(rules)))
	return rules, nil
}

// buildPrompt 构造提示词
func (rg *RuleGenerator) buildPrompt(cases []common.Case) string {
	var caseStrs []string
	for i, c := range cases {
		caseStrs = append(caseStrs, fmt.Sprintf(
			"%d. [%s] %s - Label: %s",
			i+1, c.Category, c.Content, c.Label,
		))
	}

	prompt := fmt.Sprintf(`
你是一个内容安全专家，擅长从违规案例中抽象出通用的检测规则。

请分析以下违规案例，为每个案例生成1-2条对应的检测规则：

%s

要求：
1. 每条规则必须包含:
   - category: 规则类别 (与案例类别一致)
   - patterns: 匹配模式数组 (支持正则表达式)
   - description: 简洁描述规则用途
   - action: 执行动作 (block/review/allow)
   - risk_level: 风险等级 (low/medium/high/critical)

2. patterns 应该足够泛化，能捕获同类违规内容的变体
3. 优先使用正则表达式捕获模式
4. 以严格的 JSON 数组格式返回，不要包含任何其他文字

示例输出格式:
[
  {
    "category": "gamble",
    "patterns": ["赌博", "casino", "\\b赌\\w*博\\b"],
    "description": "禁止推广赌博相关内容",
    "action": "block",
    "risk_level": "high"
  }
]

请严格按照上述格式返回JSON:`, strings.Join(caseStrs, "\n"))

	return prompt
}

// extractRulesFromText 从文本中提取规则（备用解析）
func (rg *RuleGenerator) extractRulesFromText(text string) []RuleTemplate {
	// 简单的文本解析逻辑，实际项目中可以更复杂
	lines := strings.Split(text, "\n")
	var rules []RuleTemplate

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			var rule RuleTemplate
			if err := json.Unmarshal([]byte(line), &rule); err == nil {
				rules = append(rules, rule)
			}
		}
	}

	return rules
}

// ValidateRule 验证生成的规则质量
func (rg *RuleGenerator) ValidateRule(ctx context.Context, rule RuleTemplate) (bool, string) {
	// 检查基本字段
	if rule.Category == "" {
		return false, "缺少规则类别"
	}
	if len(rule.Patterns) == 0 {
		return false, "缺少匹配模式"
	}
	if rule.Description == "" {
		return false, "缺少描述"
	}
	if rule.Action == "" {
		return false, "缺少执行动作"
	}

	// 检查正则表达式有效性
	for _, pattern := range rule.Patterns {
		if strings.Contains(pattern, `\`) || strings.ContainsAny(pattern, ".*+?^$|()[]{}") {
			// 是正则表达式，验证语法
			// 注意：这里简化处理，实际应该使用 regexp.Compile 验证
		}
	}

	return true, "规则有效"
}
