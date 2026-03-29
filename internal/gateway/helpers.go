package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/safeflow-project/safeflow/internal/common"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"
)

func (s *Server) publishAudit(requestID, userID, action, reason, source string) {
	event := common.ContentResultEvent{
		RequestID: requestID,
		UserID:    userID,
		Action:    action,
		Reason:    reason,
		Source:    source,
		Timestamp: time.Now(),
	}
	data, _ := json.Marshal(event)
	_ = s.nc.Publish(common.SubjectContentResult, data)
}

func (s *Server) upsertResult(result common.ModerationResult) {
	now := time.Now()
	result.UpdatedAt = now
	if result.CreatedAt.IsZero() {
		result.CreatedAt = now
	}
	if err := s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "request_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"user_id", "content", "action", "final_action", "status", "reason", "source",
			"reviewer", "callback_url", "decision_at", "last_error", "updated_at",
		}),
	}).Create(&result).Error; err != nil {
		s.logger.Warn("写入审核结果失败", zap.Error(err), zap.String("request_id", result.RequestID))
	}
}

func (s *Server) sendCallback(callbackURL string, payload map[string]any) {
	if strings.TrimSpace(callbackURL) == "" {
		return
	}
	requestID, _ := payload["request_id"].(string)
	payloadBytes, _ := json.Marshal(payload)

	task := &common.CallbackTask{
		ID:          uuid.New().String(),
		RequestID:   requestID,
		CallbackURL: callbackURL,
		Payload:     string(payloadBytes),
		Status:      "pending",
		MaxRetries:  3,
	}
	_ = s.db.Create(task)

	go s.executeCallbackWithRetry(task.ID, callbackURL, payloadBytes, 0)
}

func (s *Server) executeCallbackWithRetry(taskID, callbackURL string, body []byte, retryCount int) {
	maxRetries := 3
	backoff := []time.Duration{1 * time.Minute, 5 * time.Minute, 30 * time.Minute}

	req, err := http.NewRequest(http.MethodPost, callbackURL, bytes.NewReader(body))
	if err != nil {
		s.logger.Warn("构造回调请求失败", zap.Error(err), zap.String("callback_url", callbackURL))
		s.updateCallbackTask(taskID, "failed", 0, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Warn("回调请求失败", zap.Error(err), zap.String("callback_url", callbackURL))
		if retryCount < maxRetries {
			s.updateCallbackTask(taskID, "pending", retryCount+1, err.Error())
			time.Sleep(backoff[retryCount])
			go s.executeCallbackWithRetry(taskID, callbackURL, body, retryCount+1)
		} else {
			s.updateCallbackTask(taskID, "failed", retryCount, err.Error())
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		s.updateCallbackTask(taskID, "success", retryCount, "")
		s.logger.Info("回调成功", zap.String("callback_url", callbackURL), zap.Int("status_code", resp.StatusCode))
	} else {
		s.logger.Warn("回调返回错误状态码", zap.String("callback_url", callbackURL), zap.Int("status_code", resp.StatusCode))
		if retryCount < maxRetries {
			s.updateCallbackTask(taskID, "pending", retryCount+1, "")
			time.Sleep(backoff[retryCount])
			go s.executeCallbackWithRetry(taskID, callbackURL, body, retryCount+1)
		} else {
			s.updateCallbackTask(taskID, "failed", retryCount, "")
		}
	}
}

func (s *Server) updateCallbackTask(taskID, status string, retryCount int, lastError string) {
	now := time.Now()
	updates := map[string]any{
		"status":      status,
		"retry_count": retryCount,
		"last_try_at": now,
	}
	if status == "failed" {
		updates["response_body"] = lastError
	}
	if status == "pending" && retryCount > 0 {
		backoff := []time.Duration{1 * time.Minute, 5 * time.Minute, 30 * time.Minute}
		nextTry := now.Add(backoff[retryCount-1])
		updates["next_try_at"] = nextTry
	}
	_ = s.db.Model(&common.CallbackTask{}).Where("id = ?", taskID).Updates(updates)
}

func (s *Server) createReviewTask(requestID, userID, content, reason, source, callbackURL string) (*common.ReviewTask, error) {
	task := &common.ReviewTask{
		ID:        uuid.New().String(),
		RequestID: requestID,
		UserID:    userID,
		Content:   content,
		Status:    "pending",
		Source:    source,
		Reason:    reason,
	}
	if err := s.db.Create(task).Error; err != nil {
		return nil, err
	}
	s.upsertResult(common.ModerationResult{
		RequestID:   requestID,
		UserID:      userID,
		Content:     content,
		Action:      "review",
		FinalAction: "review",
		Status:      "pending_review",
		Reason:      reason,
		Source:      source,
		CallbackURL: callbackURL,
	})
	s.publishAudit(requestID, userID, "review", reason, source)
	return task, nil
}

type simulatorConfig struct {
	Interval    time.Duration
	UserID      string
	CallbackURL string
	Limit       int
	Category    string
}

func (s *Server) startSimulator(cfg simulatorConfig) error {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Second
	}
	s.simulatorMu.Lock()
	if s.simulatorRunning {
		s.simulatorMu.Unlock()
		return errors.New("simulator already running")
	}
	s.simulatorRunning = true
	s.simulatorStop = make(chan struct{})
	s.simulatorConfig = cfg
	s.simulatorCount = 0
	stopCh := s.simulatorStop
	s.simulatorMu.Unlock()

	go s.runSimulator(cfg, stopCh)
	return nil
}

func (s *Server) stopSimulator() bool {
	s.simulatorMu.Lock()
	if !s.simulatorRunning {
		s.simulatorMu.Unlock()
		return false
	}
	stopCh := s.simulatorStop
	s.simulatorRunning = false
	s.simulatorStop = nil
	s.simulatorMu.Unlock()
	if stopCh != nil {
		close(stopCh)
	}
	return true
}

func (s *Server) simulatorStatus() map[string]any {
	s.simulatorMu.Lock()
	defer s.simulatorMu.Unlock()
	return map[string]any{
		"running":     s.simulatorRunning,
		"sent_count":  s.simulatorCount,
		"interval_ms": int(s.simulatorConfig.Interval / time.Millisecond),
		"limit":       s.simulatorConfig.Limit,
		"user_id":     s.simulatorConfig.UserID,
		"category":    s.simulatorConfig.Category,
	}
}

func (s *Server) runSimulator(cfg simulatorConfig, stopCh chan struct{}) {
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.simulatorOnce(cfg)
		case <-stopCh:
			return
		}
	}
}

func (s *Server) simulatorOnce(cfg simulatorConfig) {
	content := s.pickSimulatorContent(cfg.Category)
	if strings.TrimSpace(content) == "" {
		return
	}
	s.simulateModeration(content, cfg.UserID, cfg.CallbackURL)
	stopCh := s.recordSimulatorCount(cfg.Limit)
	if stopCh != nil {
		close(stopCh)
	}
}

func (s *Server) recordSimulatorCount(limit int) chan struct{} {
	s.simulatorMu.Lock()
	defer s.simulatorMu.Unlock()
	s.simulatorCount++
	if limit > 0 && s.simulatorCount >= limit && s.simulatorRunning {
		stopCh := s.simulatorStop
		s.simulatorRunning = false
		s.simulatorStop = nil
		return stopCh
	}
	return nil
}

func (s *Server) pickSimulatorContent(category string) string {
	query := s.db.Model(&common.Case{})
	if strings.TrimSpace(category) != "" {
		query = query.Where("category = ?", category)
	}
	var total int64
	query.Count(&total)
	if total > 0 {
		offset := s.simulatorRandIntn(int(total))
		var kase common.Case
		query.Offset(offset).Limit(1).Find(&kase)
		if strings.TrimSpace(kase.Content) != "" {
			return kase.Content
		}
	}
	fallback := []string{
		"这是一条正常的用户评论，内容友善",
		"限时促销，点击链接领取优惠券",
		"联系我加微信领取兼职机会",
		"欢迎加入我们的社区讨论",
		"这里出现了敏感词汇用于测试",
	}
	return fallback[s.simulatorRandIntn(len(fallback))]
}

func (s *Server) simulatorRandIntn(n int) int {
	s.simulatorMu.Lock()
	defer s.simulatorMu.Unlock()
	if n <= 0 {
		return 0
	}
	return s.simulatorRand.Intn(n)
}

func (s *Server) simulateModeration(content, userID, callbackURL string) {
	requestID := uuid.New().String()
	ctx := context.Background()
	scanReq := &safeflow.ScanRequest{RequestId: requestID, UserId: userID, Content: content}

	ruleResp, err := s.ruleClient.Scan(ctx, scanReq)
	if err != nil {
		_, _ = s.createReviewTask(requestID, userID, content, "规则引擎服务错误: "+err.Error(), "simulator", callbackURL)
		return
	}
	if ruleResp.Action == "block" {
		now := time.Now()
		s.upsertResult(common.ModerationResult{
			RequestID:   requestID,
			UserID:      userID,
			Content:     content,
			Action:      "block",
			FinalAction: "block",
			Status:      "completed",
			Reason:      ruleResp.Reason,
			Source:      ruleResp.Source,
			CallbackURL: callbackURL,
			DecisionAt:  &now,
		})
		s.publishAudit(requestID, userID, "block", ruleResp.Reason, ruleResp.Source)
		s.sendCallback(callbackURL, map[string]any{
			"request_id": requestID,
			"user_id":    userID,
			"action":     "block",
			"status":     "completed",
			"reason":     ruleResp.Reason,
			"source":     ruleResp.Source,
		})
		return
	}

	llmResp, err := s.llmClient.Scan(ctx, scanReq)
	if err != nil {
		_, _ = s.createReviewTask(requestID, userID, content, "LLM 服务暂时不可用: "+err.Error(), "simulator", callbackURL)
		return
	}
	if llmResp.Action == "review" {
		_, _ = s.createReviewTask(requestID, userID, content, llmResp.Reason, llmResp.Source, callbackURL)
		return
	}
	now := time.Now()
	s.upsertResult(common.ModerationResult{
		RequestID:   requestID,
		UserID:      userID,
		Content:     content,
		Action:      llmResp.Action,
		FinalAction: llmResp.Action,
		Status:      "completed",
		Reason:      llmResp.Reason,
		Source:      llmResp.Source,
		CallbackURL: callbackURL,
		DecisionAt:  &now,
	})
	s.publishAudit(requestID, userID, llmResp.Action, llmResp.Reason, llmResp.Source)
	s.sendCallback(callbackURL, map[string]any{
		"request_id": requestID,
		"user_id":    userID,
		"action":     llmResp.Action,
		"status":     "completed",
		"reason":     llmResp.Reason,
		"source":     llmResp.Source,
	})
}
