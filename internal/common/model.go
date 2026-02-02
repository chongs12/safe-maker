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
	ID        uint      `gorm:"primaryKey" json:"id"`    // 自增主键
	RequestID string    `gorm:"index" json:"request_id"` // 请求 ID (建立索引以加速查询)
	UserID    string    `json:"user_id"`                 // 用户 ID
	Action    string    `json:"action"`                  // 动作 (allow, block, review)
	Reason    string    `json:"reason"`                  // 原因
	Source    string    `json:"source"`                  // 来源 (rule-engine, llm-agent)
	CreatedAt time.Time `json:"created_at"`              // 创建时间
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
	ID        string    `gorm:"primaryKey" json:"id"` // UUID
	UserID    string    `gorm:"index" json:"user_id"`
	Status    string    `gorm:"type:varchar(20)" json:"status"` // "pending", "processing", "completed", "failed"
	Total     int       `json:"total"`
	Processed int       `json:"processed"`
	Result    string    `gorm:"type:longtext" json:"result"` // JSON 格式的结果 (仅存储简要信息或 URL)
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PolicyVersion 定义策略版本
type PolicyVersion struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Version   string    `gorm:"type:varchar(50)" json:"version"` // 版本号 (如 v1.0.1)
	Type      string    `gorm:"type:varchar(20)" json:"type"`    // "rule", "model"
	Config    string    `gorm:"type:text" json:"config"`         // 配置快照 (JSON)
	Comment   string    `gorm:"type:varchar(255)" json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}
