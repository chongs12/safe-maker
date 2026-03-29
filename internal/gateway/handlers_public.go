package gateway

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/safeflow-project/safeflow/internal/common"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow"
)

func (s *Server) registerPublicRoutes(r *gin.Engine) {
	r.POST("/submit", s.handleSubmit)
	r.POST("/submit/batch", s.handleBatchSubmit)
	r.GET("/moderation/:request_id", s.handleGetModeration)
}

func (s *Server) handleSubmit(c *gin.Context) {
	var reqBody struct {
		Content     string `json:"content" binding:"required"`
		UserID      string `json:"user_id"`
		CallbackURL string `json:"callback_url"`
	}
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	requestID := uuid.New().String()
	ctx := context.Background()
	scanReq := &safeflow.ScanRequest{
		RequestId: requestID,
		UserId:    reqBody.UserID,
		Content:   reqBody.Content,
	}
	ruleResp, err := s.ruleClient.Scan(ctx, scanReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "规则引擎服务错误: " + err.Error()})
		return
	}
	if ruleResp.Action == "block" {
		now := time.Now()
		s.upsertResult(common.ModerationResult{
			RequestID:   requestID,
			UserID:      reqBody.UserID,
			Content:     reqBody.Content,
			Action:      "block",
			FinalAction: "block",
			Status:      "completed",
			Reason:      ruleResp.Reason,
			Source:      ruleResp.Source,
			CallbackURL: reqBody.CallbackURL,
			DecisionAt:  &now,
		})
		s.publishAudit(requestID, reqBody.UserID, "block", ruleResp.Reason, ruleResp.Source)
		s.sendCallback(reqBody.CallbackURL, map[string]any{
			"request_id": requestID,
			"user_id":    reqBody.UserID,
			"action":     "block",
			"status":     "completed",
			"reason":     ruleResp.Reason,
			"source":     ruleResp.Source,
		})
		c.JSON(http.StatusOK, ruleResp)
		return
	}

	llmResp, err := s.llmClient.Scan(ctx, scanReq)
	if err != nil {
		task, taskErr := s.createReviewTask(
			requestID,
			reqBody.UserID,
			reqBody.Content,
			"LLM 服务暂时不可用: "+err.Error(),
			"gateway",
			reqBody.CallbackURL,
		)
		if taskErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建人工复审任务失败: " + taskErr.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"request_id":     requestID,
			"action":         "review",
			"status":         "queued",
			"reason":         "已进入人工复审队列",
			"source":         "gateway",
			"review_task_id": task.ID,
		})
		return
	}
	if llmResp.Action == "review" {
		task, taskErr := s.createReviewTask(
			requestID,
			reqBody.UserID,
			reqBody.Content,
			llmResp.Reason,
			llmResp.Source,
			reqBody.CallbackURL,
		)
		if taskErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建人工复审任务失败: " + taskErr.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"request_id":     requestID,
			"action":         "review",
			"status":         "queued",
			"reason":         llmResp.Reason,
			"source":         llmResp.Source,
			"review_task_id": task.ID,
		})
		return
	}

	now := time.Now()
	s.upsertResult(common.ModerationResult{
		RequestID:   requestID,
		UserID:      reqBody.UserID,
		Content:     reqBody.Content,
		Action:      llmResp.Action,
		FinalAction: llmResp.Action,
		Status:      "completed",
		Reason:      llmResp.Reason,
		Source:      llmResp.Source,
		CallbackURL: reqBody.CallbackURL,
		DecisionAt:  &now,
	})
	s.publishAudit(requestID, reqBody.UserID, llmResp.Action, llmResp.Reason, llmResp.Source)
	s.sendCallback(reqBody.CallbackURL, map[string]any{
		"request_id": requestID,
		"user_id":    reqBody.UserID,
		"action":     llmResp.Action,
		"status":     "completed",
		"reason":     llmResp.Reason,
		"source":     llmResp.Source,
	})
	c.JSON(http.StatusOK, llmResp)
}

func (s *Server) handleBatchSubmit(c *gin.Context) {
	var reqBody struct {
		BatchID     string   `json:"batch_id"`
		Contents    []string `json:"contents" binding:"required"`
		UserID      string   `json:"user_id"`
		CallbackURL string   `json:"callback_url"`
	}
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	results := make([]interface{}, 0, len(reqBody.Contents))
	ctx := context.Background()

	for _, content := range reqBody.Contents {
		reqID := uuid.New().String()
		scanReq := &safeflow.ScanRequest{RequestId: reqID, UserId: reqBody.UserID, Content: content}
		ruleResp, err := s.ruleClient.Scan(ctx, scanReq)
		if err != nil {
			s.upsertResult(common.ModerationResult{
				RequestID:   reqID,
				UserID:      reqBody.UserID,
				Content:     content,
				Action:      "review",
				FinalAction: "review",
				Status:      "pending_review",
				Reason:      "规则引擎服务错误: " + err.Error(),
				Source:      "gateway",
				CallbackURL: reqBody.CallbackURL,
				LastError:   err.Error(),
			})
			results = append(results, map[string]interface{}{
				"request_id": reqID,
				"content":    content,
				"action":     "review",
				"status":     "queued",
				"source":     "gateway",
				"reason":     "规则引擎服务错误，已进入人工复审队列",
			})
			continue
		}
		if ruleResp.Action == "block" {
			now := time.Now()
			s.upsertResult(common.ModerationResult{
				RequestID:   reqID,
				UserID:      reqBody.UserID,
				Content:     content,
				Action:      "block",
				FinalAction: "block",
				Status:      "completed",
				Reason:      ruleResp.Reason,
				Source:      ruleResp.Source,
				CallbackURL: reqBody.CallbackURL,
				DecisionAt:  &now,
			})
			s.publishAudit(reqID, reqBody.UserID, "block", ruleResp.Reason, ruleResp.Source)
			results = append(results, ruleResp)
			continue
		}

		llmResp, err := s.llmClient.Scan(ctx, scanReq)
		if err != nil {
			task, taskErr := s.createReviewTask(reqID, reqBody.UserID, content, "LLM 服务暂时不可用: "+err.Error(), "gateway", reqBody.CallbackURL)
			if taskErr != nil {
				results = append(results, map[string]interface{}{"request_id": reqID, "content": content, "error": taskErr.Error()})
				continue
			}
			results = append(results, map[string]interface{}{
				"request_id":     reqID,
				"action":         "review",
				"status":         "queued",
				"reason":         "已进入人工复审队列",
				"source":         "gateway",
				"review_task_id": task.ID,
			})
			continue
		}
		if llmResp.Action == "review" {
			task, taskErr := s.createReviewTask(reqID, reqBody.UserID, content, llmResp.Reason, llmResp.Source, reqBody.CallbackURL)
			if taskErr != nil {
				results = append(results, map[string]interface{}{"request_id": reqID, "content": content, "error": taskErr.Error()})
				continue
			}
			results = append(results, map[string]interface{}{
				"request_id":     reqID,
				"action":         "review",
				"status":         "queued",
				"reason":         llmResp.Reason,
				"source":         llmResp.Source,
				"review_task_id": task.ID,
			})
			continue
		}
		now := time.Now()
		s.upsertResult(common.ModerationResult{
			RequestID:   reqID,
			UserID:      reqBody.UserID,
			Content:     content,
			Action:      llmResp.Action,
			FinalAction: llmResp.Action,
			Status:      "completed",
			Reason:      llmResp.Reason,
			Source:      llmResp.Source,
			CallbackURL: reqBody.CallbackURL,
			DecisionAt:  &now,
		})
		s.publishAudit(reqID, reqBody.UserID, llmResp.Action, llmResp.Reason, llmResp.Source)
		results = append(results, llmResp)
	}

	c.JSON(http.StatusOK, gin.H{"batch_id": reqBody.BatchID, "results": results})
}

func (s *Server) handleGetModeration(c *gin.Context) {
	requestID := c.Param("request_id")
	var result common.ModerationResult
	if err := s.db.Where("request_id = ?", requestID).First(&result).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "result not found"})
		return
	}

	var reviewTask common.ReviewTask
	reviewTaskResp := map[string]any{}
	if err := s.db.Where("request_id = ?", requestID).Order("created_at desc").First(&reviewTask).Error; err == nil {
		reviewTaskResp = map[string]any{
			"id":              reviewTask.ID,
			"status":          reviewTask.Status,
			"reviewer":        reviewTask.Reviewer,
			"final_action":    reviewTask.FinalAction,
			"decision_reason": reviewTask.DecisionReason,
			"claimed_at":      reviewTask.ClaimedAt,
			"resolved_at":     reviewTask.ResolvedAt,
		}
	}

	var callbackTasks []common.CallbackTask
	s.db.Where("request_id = ?", requestID).Order("created_at desc").Find(&callbackTasks)
	callbackResp := make([]map[string]any, 0, len(callbackTasks))
	for _, ct := range callbackTasks {
		callbackResp = append(callbackResp, map[string]any{
			"id":            ct.ID,
			"callback_url":  ct.CallbackURL,
			"status":        ct.Status,
			"retry_count":   ct.RetryCount,
			"response_code": ct.ResponseCode,
			"last_try_at":   ct.LastTryAt,
			"next_try_at":   ct.NextTryAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"request_id":   result.RequestID,
		"user_id":      result.UserID,
		"content":      result.Content,
		"action":       result.Action,
		"final_action": result.FinalAction,
		"status":       result.Status,
		"reason":       result.Reason,
		"source":       result.Source,
		"review_task":  reviewTaskResp,
		"callbacks":    callbackResp,
	})
}
