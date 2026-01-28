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
	SubjectContentResult    = "content.result"
	
	// StreamName NATS JetStream 流名称
	StreamName = "SAFEFLOW"
)
