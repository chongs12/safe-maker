package common

import "time"

// ContentSubmittedEvent 是用户提交内容后发布的事件
// 主题: content.submitted
type ContentSubmittedEvent struct {
	RequestID string    `json:"request_id"` // 请求唯一 ID
	UserID    string    `json:"user_id"`    // 用户 ID
	Content   string    `json:"content"`    // 提交的内容
	Timestamp time.Time `json:"timestamp"`  // 事件发生时间
}

// ContentResultEvent 是审核完成后的结果事件
// 主题: content.result
// 用于通知审计服务或其他下游服务
type ContentResultEvent struct {
	RequestID string    `json:"request_id"`
	UserID    string    `json:"user_id"`
	Action    string    `json:"action"` // 动作: allow(通过), block(拦截), review(需复核)
	Reason    string    `json:"reason"` // 审核理由
	Source    string    `json:"source"` // 决策来源: rule-engine(规则引擎), llm-agent(大模型)
	Timestamp time.Time `json:"timestamp"`
}

const (
	// SubjectContentSubmitted 内容提交事件主题
	SubjectContentSubmitted = "content.submitted"
	// SubjectContentResult 审核结果事件主题
	SubjectContentResult = "content.result"

	// StreamName NATS JetStream 流名称
	StreamName = "SAFEFLOW"
)

// AuditLog 定义审计日志的数据库模型
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`                     // 自增主键
	RequestID string    `gorm:"type:varchar(64);index" json:"request_id"` // 请求 ID (建立索引以加速查询)
	UserID    string    `gorm:"type:varchar(64)" json:"user_id"`          // 用户 ID
	Action    string    `json:"action"`                                   // 动作 (allow, block, review)
	Reason    string    `json:"reason"`                                   // 原因
	Source    string    `json:"source"`                                   // 来源 (rule-engine, llm-agent)
	CreatedAt time.Time `json:"created_at"`                               // 创建时间
}

// Rule 定义规则引擎的规则
type Rule struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Pattern     string    `gorm:"type:varchar(255);not null" json:"pattern"` // 关键词或正则表达式
	Type        string    `gorm:"type:varchar(20);not null" json:"type"`     // "keyword", "regex"
	Action      string    `gorm:"type:varchar(20);not null" json:"action"`   // "block", "allow"
	Group       string    `gorm:"type:varchar(50)" json:"group"`             // 分组 (如 "politics", "ads")
	Priority    int       `gorm:"default:0" json:"priority"`                 // 优先级 (数字越大优先级越高)
	IsEnabled   bool      `gorm:"default:true" json:"is_enabled"`            // 是否启用
	Description string    `gorm:"type:varchar(255)" json:"description"`      // 描述
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Case 定义知识库案例 (RAG 源)
type Case struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Content   string    `gorm:"type:text" json:"content"`
	Label     string    `gorm:"type:varchar(20)" json:"label"` // "safe", "unsafe"
	Category  string    `gorm:"type:varchar(50)" json:"category"`
	VectorID  int64     `json:"vector_id"`                      // Milvus 中的 ID
	IsCustom  bool      `gorm:"default:false" json:"is_custom"` // 是否为用户上传的自定义案例
	CreatedAt time.Time `json:"created_at"`
}

// AuditTask 定义批量审核任务
type AuditTask struct {
	ID        string    `gorm:"type:varchar(64);primaryKey" json:"id"` // UUID
	UserID    string    `gorm:"type:varchar(64);index" json:"user_id"`
	Status    string    `gorm:"type:varchar(20)" json:"status"` // "pending", "processing", "completed", "failed"
	Total     int       `json:"total"`
	Processed int       `json:"processed"`
	Result    string    `gorm:"type:longtext" json:"result"` // JSON 格式的结果 (仅存储简要信息或 URL)
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ReviewTask struct {
	ID             string     `gorm:"type:varchar(64);primaryKey" json:"id"`
	RequestID      string     `gorm:"type:varchar(64);index" json:"request_id"`
	UserID         string     `gorm:"type:varchar(64);index" json:"user_id"`
	Content        string     `gorm:"type:text" json:"content"`
	Status         string     `gorm:"type:varchar(20);index" json:"status"`
	Source         string     `gorm:"type:varchar(30)" json:"source"`
	Reason         string     `gorm:"type:text" json:"reason"`
	Reviewer       string     `gorm:"type:varchar(64)" json:"reviewer"`
	FinalAction    string     `gorm:"type:varchar(20)" json:"final_action"`
	DecisionReason string     `gorm:"type:text" json:"decision_reason"`
	ClaimedAt      *time.Time `json:"claimed_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type ModerationResult struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	RequestID   string     `gorm:"type:varchar(64);uniqueIndex" json:"request_id"`
	UserID      string     `gorm:"type:varchar(64);index" json:"user_id"`
	Content     string     `gorm:"type:text" json:"content"`
	Action      string     `gorm:"type:varchar(20)" json:"action"`
	FinalAction string     `gorm:"type:varchar(20)" json:"final_action"`
	Status      string     `gorm:"type:varchar(30);index" json:"status"`
	Reason      string     `gorm:"type:text" json:"reason"`
	Source      string     `gorm:"type:varchar(30)" json:"source"`
	Reviewer    string     `gorm:"type:varchar(64)" json:"reviewer"`
	CallbackURL string     `gorm:"type:text" json:"callback_url"`
	DecisionAt  *time.Time `json:"decision_at"`
	LastError   string     `gorm:"type:text" json:"last_error"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// PolicyVersion 定义策略版本
type PolicyVersion struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Version   string    `gorm:"type:varchar(50)" json:"version"` // 版本号 (如 v1.0.1)
	Type      string    `gorm:"type:varchar(20)" json:"type"`    // "rule", "model", "prompt"
	Config    string    `gorm:"type:text" json:"config"`         // 配置快照 (JSON)
	Comment   string    `gorm:"type:varchar(255)" json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

// PromptPolicy Prompt 策略配置
type PromptPolicy struct {
	Scene        string   `json:"scene"`                 // 场景: im / ugc / ad / guardrail
	SystemPrompt string   `json:"system_prompt"`         // 系统提示词
	Temperature  *float64 `json:"temperature,omitempty"` // 温度参数
	MaxTokens    *int     `json:"max_tokens,omitempty"`  // 最大 token 数
	Tools        []string `json:"tools,omitempty"`       // 启用的工具列表
	IsActive     bool     `json:"is_active"`             // 是否激活
}

// CallbackTask 回调任务 (用于异步回调重试机制)
type CallbackTask struct {
	ID           string     `gorm:"type:varchar(64);primaryKey" json:"id"`    // UUID
	RequestID    string     `gorm:"type:varchar(64);index" json:"request_id"` // 关联的审核请求ID
	CallbackURL  string     `gorm:"size:500" json:"callback_url"`             // 回调地址
	Payload      string     `gorm:"type:text" json:"payload"`                 // JSON payload
	RetryCount   int        `gorm:"default:0" json:"retry_count"`             // 当前重试次数
	MaxRetries   int        `gorm:"default:3" json:"max_retries"`             // 最大重试次数
	Status       string     `gorm:"type:varchar(20)" json:"status"`           // pending/success/failed
	ResponseCode int        `json:"response_code"`                            // HTTP 响应码
	ResponseBody string     `gorm:"type:text" json:"response_body"`           // 响应内容
	LastTryAt    *time.Time `json:"last_try_at"`                              // 最后尝试时间
	NextTryAt    *time.Time `json:"next_try_at"`                              // 下次尝试时间
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// ReviewQueueStats 复审队列统计
type ReviewQueueStats struct {
	Total      int64 `json:"total"`
	Pending    int64 `json:"pending"`
	Processing int64 `json:"processing"`
	Resolved   int64 `json:"resolved"`
}
