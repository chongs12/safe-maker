package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/safeflow-project/safeflow/internal/common"
	"github.com/safeflow-project/safeflow/internal/rulegen"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow"
	"gorm.io/gorm"
)

func (s *Server) registerAdminRoutes(r *gin.Engine) {
	admin := r.Group("/admin")
	{
		admin.GET("/rules", s.handleListRules)
		admin.POST("/rules", s.handleCreateRule)
		admin.PUT("/rules/:id", s.handleUpdateRule)
		admin.DELETE("/rules/:id", s.handleDeleteRule)

		admin.GET("/cases", s.handleListCases)
		admin.POST("/cases", s.handleCreateCase)

		admin.GET("/audits", s.handleListAudits)

		admin.GET("/reviews", s.handleListReviews)
		admin.GET("/reviews/stats", s.handleReviewStats)
		admin.POST("/reviews/:id/claim", s.handleClaimReview)
		admin.POST("/reviews/:id/decide", s.handleDecideReview)

		admin.GET("/callbacks", s.handleListCallbacks)
		admin.POST("/callbacks/:id/retry", s.handleRetryCallback)

		admin.POST("/versions/snapshot", s.handleSnapshotVersion)

		admin.POST("/prompts", s.handleCreatePrompt)
		admin.GET("/prompts/:scene", s.handleListPrompts)
		admin.PUT("/prompts/:id/activate", s.handleActivatePrompt)

		admin.POST("/rules/generate", s.handleGenerateRules)
		admin.POST("/rules/backtest", s.handleBacktestRules)

		admin.POST("/simulator/start", s.handleStartSimulator)
		admin.POST("/simulator/stop", s.handleStopSimulator)
		admin.GET("/simulator/status", s.handleSimulatorStatus)
	}
}

func (s *Server) handleListRules(c *gin.Context) {
	var rules []common.Rule
	s.db.Order("priority desc").Find(&rules)
	c.JSON(http.StatusOK, rules)
}

func (s *Server) handleCreateRule(c *gin.Context) {
	var rule common.Rule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.db.Create(&rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, rule)
}

func (s *Server) handleUpdateRule(c *gin.Context) {
	id := c.Param("id")
	var rule common.Rule
	if err := s.db.First(&rule, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	s.db.Save(&rule)
	c.JSON(http.StatusOK, rule)
}

func (s *Server) handleDeleteRule(c *gin.Context) {
	id := c.Param("id")
	s.db.Delete(&common.Rule{}, id)
	c.Status(http.StatusNoContent)
}

func (s *Server) handleListCases(c *gin.Context) {
	var cases []common.Case
	s.db.Order("created_at desc").Find(&cases)
	c.JSON(http.StatusOK, cases)
}

func (s *Server) handleCreateCase(c *gin.Context) {
	var kase common.Case
	if err := c.ShouldBindJSON(&kase); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	kase.IsCustom = true
	if err := s.db.Create(&kase).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, kase)
}

func (s *Server) handleListAudits(c *gin.Context) {
	var audits []common.AuditLog
	query := s.db.Model(&common.AuditLog{}).Order("created_at desc")
	if uid := c.Query("user_id"); uid != "" {
		query = query.Where("user_id = ?", uid)
	}
	if action := c.Query("action"); action != "" {
		query = query.Where("action = ?", action)
	}
	if source := c.Query("source"); source != "" {
		query = query.Where("source = ?", source)
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	offset := (page - 1) * pageSize

	var total int64
	query.Count(&total)
	query.Limit(pageSize).Offset(offset).Find(&audits)
	s.metrics.HTTPRequestTotal.WithLabelValues("GET", "/admin/audits", "200").Inc()

	c.JSON(http.StatusOK, gin.H{
		"total": total,
		"page":  page,
		"data":  audits,
	})
}

func (s *Server) handleListReviews(c *gin.Context) {
	var tasks []common.ReviewTask
	query := s.db.Model(&common.ReviewTask{}).Order("created_at desc")
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if uid := c.Query("user_id"); uid != "" {
		query = query.Where("user_id = ?", uid)
	}
	if reviewer := c.Query("reviewer"); reviewer != "" {
		query = query.Where("reviewer = ?", reviewer)
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	offset := (page - 1) * pageSize
	var total int64
	query.Count(&total)
	query.Limit(pageSize).Offset(offset).Find(&tasks)
	c.JSON(http.StatusOK, gin.H{
		"total": total,
		"page":  page,
		"data":  tasks,
	})
}

func (s *Server) handleClaimReview(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Reviewer string `json:"reviewer" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var task common.ReviewTask
	if err := s.db.Where("id = ?", id).First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "review task not found"})
		return
	}
	if task.Status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"error": "review task is not pending"})
		return
	}
	now := time.Now()
	if err := s.db.Model(&task).Updates(map[string]any{
		"status":     "processing",
		"reviewer":   req.Reviewer,
		"claimed_at": &now,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = s.db.First(&task, "id = ?", id)
	c.JSON(http.StatusOK, task)
}

func (s *Server) handleDecideReview(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Action   string `json:"action" binding:"required,oneof=allow block"`
		Reason   string `json:"reason"`
		Reviewer string `json:"reviewer" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var task common.ReviewTask
	if err := s.db.Where("id = ?", id).First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "review task not found"})
		return
	}
	if task.Status == "resolved" {
		c.JSON(http.StatusConflict, gin.H{"error": "review task already resolved"})
		return
	}
	finalReason := strings.TrimSpace(req.Reason)
	if finalReason == "" {
		finalReason = task.Reason
	}
	now := time.Now()
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&common.ReviewTask{}).Where("id = ?", id).Updates(map[string]any{
			"status":          "resolved",
			"reviewer":        req.Reviewer,
			"final_action":    req.Action,
			"decision_reason": finalReason,
			"resolved_at":     &now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&common.ModerationResult{}).Where("request_id = ?", task.RequestID).Updates(map[string]any{
			"final_action": req.Action,
			"status":       "completed",
			"reason":       finalReason,
			"source":       "human-review",
			"reviewer":     req.Reviewer,
			"decision_at":  &now,
		}).Error; err != nil {
			return err
		}
		label := "safe"
		if req.Action == "block" {
			label = "unsafe"
		}
		if err := tx.Create(&common.Case{
			Content:   task.Content,
			Label:     label,
			Category:  "human-review",
			IsCustom:  true,
			CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		return nil
	})
	if txErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": txErr.Error()})
		return
	}
	var result common.ModerationResult
	_ = s.db.Where("request_id = ?", task.RequestID).First(&result).Error
	s.publishAudit(task.RequestID, task.UserID, req.Action, finalReason, "human-review")
	s.sendCallback(result.CallbackURL, map[string]any{
		"request_id": task.RequestID,
		"user_id":    task.UserID,
		"action":     req.Action,
		"status":     "completed",
		"reason":     finalReason,
		"source":     "human-review",
		"reviewer":   req.Reviewer,
	})
	_ = s.db.Where("id = ?", id).First(&task).Error
	c.JSON(http.StatusOK, gin.H{
		"review_task": task,
		"result":      result,
	})
}

func (s *Server) handleSnapshotVersion(c *gin.Context) {
	var rules []common.Rule
	s.db.Where("is_enabled = ?", true).Find(&rules)
	configBytes, _ := json.Marshal(rules)
	version := common.PolicyVersion{
		Version:   time.Now().Format("v20060102150405"),
		Type:      "rule",
		Config:    string(configBytes),
		CreatedAt: time.Now(),
	}
	if err := s.db.Create(&version).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, version)
}

func (s *Server) handleCreatePrompt(c *gin.Context) {
	var req struct {
		Scene        string   `json:"scene" binding:"required,oneof=im ugc ad guardrail"`
		SystemPrompt string   `json:"system_prompt" binding:"required"`
		Temperature  *float64 `json:"temperature"`
		MaxTokens    *int     `json:"max_tokens"`
		Tools        []string `json:"tools"`
		Comment      string   `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	promptPolicy := common.PromptPolicy{
		Scene:        req.Scene,
		SystemPrompt: req.SystemPrompt,
		Temperature:  req.Temperature,
		MaxTokens:    req.MaxTokens,
		Tools:        req.Tools,
		IsActive:     true,
	}
	configBytes, _ := json.Marshal(promptPolicy)
	version := common.PolicyVersion{
		Version:   time.Now().Format("v20060102150405"),
		Type:      "prompt",
		Config:    string(configBytes),
		Comment:   req.Comment,
		CreatedAt: time.Now(),
	}
	if err := s.db.Create(&version).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, version)
}

func (s *Server) handleListPrompts(c *gin.Context) {
	scene := c.Param("scene")
	var versions []common.PolicyVersion
	s.db.Where("type = ? AND config LIKE ?", "prompt", "%\"scene\":\""+scene+"\"%").
		Order("created_at DESC").Find(&versions)
	c.JSON(http.StatusOK, versions)
}

func (s *Server) handleActivatePrompt(c *gin.Context) {
	id := c.Param("id")
	var version common.PolicyVersion
	if err := s.db.First(&version, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
		return
	}
	var policy common.PromptPolicy
	json.Unmarshal([]byte(version.Config), &policy)
	s.db.Model(&common.PolicyVersion{}).
		Where("type = ? AND config LIKE ? AND id != ?", "prompt", "%\"scene\":\""+policy.Scene+"\"%", id).
		Update("config", gorm.Expr("REPLACE(config, '\"is_active\":true', '\"is_active\":false')"))
	updatedConfig := strings.Replace(version.Config, `"is_active":false`, `"is_active":true`, 1)
	s.db.Model(&version).Update("config", updatedConfig)
	c.JSON(http.StatusOK, gin.H{"message": "Activated"})
}

func (s *Server) handleGenerateRules(c *gin.Context) {
	var req struct {
		Category string `json:"category" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var cases []common.Case
	query := s.db.Where("label = ?", "unsafe")
	if req.Category != "" {
		query = query.Where("category = ?", req.Category)
	}
	query.Order("created_at DESC").Limit(10).Find(&cases)
	if len(cases) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有找到相关违规案例"})
		return
	}
	ruleGen, err := rulegen.NewRuleGenerator(context.Background(), s.cfg, s.logger)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "初始化规则生成器失败: " + err.Error()})
		return
	}
	rules, err := ruleGen.GenerateRules(context.Background(), cases)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成规则失败: " + err.Error()})
		return
	}
	validRules := make([]rulegen.RuleTemplate, 0, len(rules))
	for _, rule := range rules {
		if ok, _ := ruleGen.ValidateRule(context.Background(), rule); ok {
			validRules = append(validRules, rule)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"generated_count": len(rules),
		"valid_count":     len(validRules),
		"rules":           validRules,
	})
}

func (s *Server) handleBacktestRules(c *gin.Context) {
	var req struct {
		Limit    int    `json:"limit"`
		Category string `json:"category"`
		Label    string `json:"label"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Limit <= 0 {
		req.Limit = 200
	}

	query := s.db.Model(&common.Case{}).Order("created_at desc")
	if strings.TrimSpace(req.Category) != "" {
		query = query.Where("category = ?", req.Category)
	}
	if strings.TrimSpace(req.Label) != "" {
		query = query.Where("label = ?", req.Label)
	}
	var cases []common.Case
	query.Limit(req.Limit).Find(&cases)
	if len(cases) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有找到可用于回测的案例"})
		return
	}

	type stats struct {
		Total         int `json:"total"`
		SafeTotal     int `json:"safe_total"`
		UnsafeTotal   int `json:"unsafe_total"`
		BlockCount    int `json:"block_count"`
		AllowCount    int `json:"allow_count"`
		FalsePositive int `json:"false_positive"`
		FalseNegative int `json:"false_negative"`
		RequestErrors int `json:"request_errors"`
		TruePositive  int `json:"true_positive"`
		Precision     any `json:"precision"`
		Recall        any `json:"recall"`
	}
	result := stats{}
	ctx := context.Background()
	for _, kase := range cases {
		result.Total++
		if kase.Label == "unsafe" {
			result.UnsafeTotal++
		} else {
			result.SafeTotal++
		}
		resp, err := s.ruleClient.Scan(ctx, &safeflow.ScanRequest{
			RequestId: uuid.New().String(),
			UserId:    "backtest",
			Content:   kase.Content,
		})
		if err != nil {
			result.RequestErrors++
			continue
		}
		if resp.Action == "block" {
			result.BlockCount++
			if kase.Label != "unsafe" {
				result.FalsePositive++
			}
		} else {
			result.AllowCount++
			if kase.Label == "unsafe" {
				result.FalseNegative++
			}
		}
	}

	result.TruePositive = result.BlockCount - result.FalsePositive
	tp := float64(result.TruePositive)
	fp := float64(result.FalsePositive)
	fn := float64(result.FalseNegative)
	if tp+fp > 0 {
		result.Precision = tp / (tp + fp)
	} else {
		result.Precision = nil
	}
	if tp+fn > 0 {
		result.Recall = tp / (tp + fn)
	} else {
		result.Recall = nil
	}

	c.JSON(http.StatusOK, gin.H{
		"category": req.Category,
		"label":    req.Label,
		"stats":    result,
	})
}

func (s *Server) handleStartSimulator(c *gin.Context) {
	var req struct {
		IntervalMs int    `json:"interval_ms"`
		UserID     string `json:"user_id"`
		Callback   string `json:"callback_url"`
		Limit      int    `json:"limit"`
		Category   string `json:"category"`
	}
	_ = c.ShouldBindJSON(&req)
	cfg := simulatorConfig{
		Interval:    time.Duration(req.IntervalMs) * time.Millisecond,
		UserID:      req.UserID,
		CallbackURL: req.Callback,
		Limit:       req.Limit,
		Category:    req.Category,
	}
	if err := s.startSimulator(cfg); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s.simulatorStatus())
}

func (s *Server) handleStopSimulator(c *gin.Context) {
	if !s.stopSimulator() {
		c.JSON(http.StatusConflict, gin.H{"error": "simulator not running"})
		return
	}
	c.JSON(http.StatusOK, s.simulatorStatus())
}

func (s *Server) handleSimulatorStatus(c *gin.Context) {
	c.JSON(http.StatusOK, s.simulatorStatus())
}

func (s *Server) handleReviewStats(c *gin.Context) {
	var stats common.ReviewQueueStats
	s.db.Model(&common.ReviewTask{}).Count(&stats.Total)
	s.db.Model(&common.ReviewTask{}).Where("status = ?", "pending").Count(&stats.Pending)
	s.db.Model(&common.ReviewTask{}).Where("status = ?", "processing").Count(&stats.Processing)
	s.db.Model(&common.ReviewTask{}).Where("status = ?", "resolved").Count(&stats.Resolved)
	c.JSON(http.StatusOK, stats)
}

func (s *Server) handleListCallbacks(c *gin.Context) {
	var tasks []common.CallbackTask
	query := s.db.Model(&common.CallbackTask{}).Order("created_at desc")
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if requestID := c.Query("request_id"); requestID != "" {
		query = query.Where("request_id = ?", requestID)
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	offset := (page - 1) * pageSize
	var total int64
	query.Count(&total)
	query.Limit(pageSize).Offset(offset).Find(&tasks)
	c.JSON(http.StatusOK, gin.H{
		"total": total,
		"page":  page,
		"data":  tasks,
	})
}

func (s *Server) handleRetryCallback(c *gin.Context) {
	id := c.Param("id")
	var task common.CallbackTask
	if err := s.db.Where("id = ?", id).First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "callback task not found"})
		return
	}
	if task.Status == "success" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "callback already succeeded, no need to retry"})
		return
	}
	go s.executeCallbackWithRetry(task.ID, task.CallbackURL, []byte(task.Payload), 0)
	c.JSON(http.StatusOK, gin.H{"message": "retry scheduled", "task_id": task.ID})
}
