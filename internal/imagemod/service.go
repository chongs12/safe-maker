package imagemod

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/gin-gonic/gin"
	"github.com/safeflow-project/safeflow/internal/common"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow"
	"go.uber.org/zap"
)

// ImageModerationRequest 图像审核请求
type ImageModerationRequest struct {
	ImageData []byte `json:"image_data"` // 图像二进制数据
	ImageURL  string `json:"image_url"`  // 图像URL（二选一）
	Scene     string `json:"scene"`      // 场景: ugc/ad/profile
}

// ImageModerationResult 图像审核结果
type ImageModerationResult struct {
	RequestID   string  `json:"request_id"`
	Action      string  `json:"action"`      // allow/block/review
	RiskScore   float64 `json:"risk_score"`  // 风险分数 0-1
	TextContent string  `json:"text_content"` // OCR提取的文本
	Labels      []Label `json:"labels"`       // 检测到的标签
	Reason      string  `json:"reason"`       // 审核理由
}

// Label 检测标签
type Label struct {
	Name       string  `json:"name"`        // 标签名称
	Confidence float64 `json:"confidence"`  // 置信度
	Positions  []Box   `json:"positions"`   // 位置坐标
}

// Box 边界框
type Box struct {
	X1, Y1, X2, Y2 int `json:"coordinates"`
}

// ImageModerationServiceImpl 图像审核服务实现
type ImageModerationServiceImpl struct {
	cfg               *common.Config
	logger            *zap.Logger
	volcengineClient  *VolcengineImageModerationClient
}

// NewImageModerationServiceImpl 创建服务实例
func NewImageModerationServiceImpl(cfg *common.Config, logger *zap.Logger) *ImageModerationServiceImpl {
	return &ImageModerationServiceImpl{
		cfg:              cfg,
		logger:           logger,
		volcengineClient: NewVolcengineImageModerationClient(cfg, logger),
	}
}

// Moderate 实现图像审核接口
func (s *ImageModerationServiceImpl) Moderate(ctx context.Context, req *safeflow.ImageModerationRequest) (*safeflow.ImageModerationResponse, error) {
	klog.CtxInfof(ctx, "开始图像审核: %s", req.ImageUrl)

	start := time.Now()
	requestID := fmt.Sprintf("img_%d", time.Now().UnixNano())

	// 调用火山引擎API
	volcResult, err := s.volcengineClient.Moderate(ctx, req.ImageUrl)
	if err != nil {
		s.logger.Error("调用火山引擎API失败", zap.Error(err))
		return &safeflow.ImageModerationResponse{
			RequestId: requestID,
			Action:    "review", // API调用失败时默认人工审核
			RiskScore: 0.5,
			Reason:    fmt.Sprintf("服务暂时不可用: %v", err),
		}, nil
	}

	// 转换为内部结果格式
	internalResult := s.volcengineClient.ConvertToInternalResult(volcResult)

	// 记录指标
	if common.GlobalMetrics != nil {
		common.GlobalMetrics.AuditRequestsTotal.WithLabelValues("image", req.Scene).Inc()
		common.GlobalMetrics.AuditActionsTotal.WithLabelValues(internalResult.Action, "image", req.Scene).Inc()
		common.GlobalMetrics.AuditRiskScores.WithLabelValues("image", req.Scene).Observe(internalResult.RiskScore)
	}

	s.logger.Info("图像审核完成",
		zap.String("request_id", requestID),
		zap.String("action", internalResult.Action),
		zap.Float64("risk_score", internalResult.RiskScore),
		zap.Duration("duration", time.Since(start)),
	)

	// 转换为Thrift响应格式
	var labels []*safeflow.Label
	for _, label := range internalResult.Labels {
		thriftLabel := &safeflow.Label{
			Name:       label.Name,
			Confidence: label.Confidence,
		}
		labels = append(labels, thriftLabel)
	}

	return &safeflow.ImageModerationResponse{
		RequestId:   requestID,
		Action:      internalResult.Action,
		RiskScore:   internalResult.RiskScore,
		TextContent: internalResult.TextContent,
		Labels:      labels,
		Reason:      internalResult.Reason,
	}, nil
}

// RegisterRoutes 注册HTTP路由
func (s *ImageModerationServiceImpl) RegisterRoutes(r *gin.Engine) {
	// 图像审核API
	r.POST("/moderate/image", s.handleImageModeration)
	
	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "image-moderation"})
	})
}

// handleImageModeration 处理图像审核请求
func (s *ImageModerationServiceImpl) handleImageModeration(c *gin.Context) {
	var req struct {
		ImageURL string `json:"image_url"`
		Scene    string `json:"scene"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		s.logger.Error("请求参数解析失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	// 验证参数
	if req.ImageURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "必须提供image_url"})
		return
	}

	// 构造Thrift请求
	thriftReq := &safeflow.ImageModerationRequest{
		ImageUrl: req.ImageURL,
		Scene:    req.Scene,
	}

	// 执行审核
	result, err := s.Moderate(c.Request.Context(), thriftReq)
	if err != nil {
		s.logger.Error("图像审核失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "审核服务暂时不可用"})
		return
	}

	// 返回结果
	c.JSON(http.StatusOK, result)
}