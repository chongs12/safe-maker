package multimodal

import (
	"bytes"
	"context"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/safeflow-project/safeflow/internal/common"
	"go.uber.org/zap"
)

// ImageModerationAPI 图片审核API处理器
type ImageModerationAPI struct {
	moderator *ImageModerator
	logger    *zap.Logger
}

// NewImageModerationAPI 创建API处理器
func NewImageModerationAPI(cfg *common.Config, logger *zap.Logger) *ImageModerationAPI {
	return &ImageModerationAPI{
		moderator: NewImageModerator(cfg, logger),
		logger:    logger,
	}
}

// RegisterRoutes 注册路由
func (api *ImageModerationAPI) RegisterRoutes(r *gin.Engine) {
	// 图片审核API
	r.POST("/moderate/image", api.handleImageModeration)

	// 健康检查
	r.GET("/health/image", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "image-moderation"})
	})
}

// handleImageModeration 处理图片审核请求
func (api *ImageModerationAPI) handleImageModeration(c *gin.Context) {
	start := common.GetTracer().Start(c.Request.Context(), "image.moderation")
	defer start.End()

	var req struct {
		ImageURL string `json:"image_url"`
		Scene    string `json:"scene"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		api.logger.Error("请求参数解析失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	// 验证参数
	if req.ImageURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "必须提供image_url"})
		return
	}

	// 构造审核请求
	modReq := &ImageModerationRequest{
		ImageURL: req.ImageURL,
		Scene:    req.Scene,
	}

	// 执行审核
	result, err := api.moderator.Moderate(c.Request.Context(), modReq)
	if err != nil {
		api.logger.Error("图片审核失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "审核服务暂时不可用"})
		return
	}

	// 记录指标
	if common.GlobalMetrics != nil {
		common.GlobalMetrics.AuditRequestsTotal.WithLabelValues("image", req.Scene).Inc()
		common.GlobalMetrics.AuditActionsTotal.WithLabelValues(result.Action, "image", req.Scene).Inc()
		common.GlobalMetrics.AuditRiskScores.WithLabelValues("image", req.Scene).Observe(result.RiskScore)
	}

	// 返回结果
	c.JSON(http.StatusOK, result)
}

// 文件上传版本的API
func (api *ImageModerationAPI) handleImageUpload(c *gin.Context) {
	// 获取上传的文件
	file, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择要上传的图片文件"})
		return
	}

	// 限制文件大小（例如5MB）
	if file.Size > 5*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "图片文件过大，最大支持5MB"})
		return
	}

	// 读取文件内容
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取上传的文件"})
		return
	}
	defer src.Close()

	imageData, err := ioutil.ReadAll(src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败"})
		return
	}

	// 验证图片格式
	if !isValidImageFormat(imageData) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的图片格式，仅支持JPEG/PNG"})
		return
	}

	// 构造审核请求
	scene := c.PostForm("scene")
	if scene == "" {
		scene = "ugc"
	}

	modReq := &ImageModerationRequest{
		ImageData: imageData,
		Scene:     scene,
	}

	// 执行审核
	result, err := api.moderator.Moderate(c.Request.Context(), modReq)
	if err != nil {
		api.logger.Error("图片审核失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "审核服务暂时不可用"})
		return
	}

	// 返回结果
	c.JSON(http.StatusOK, result)
}

// isValidImageFormat 验证图片格式
func isValidImageFormat(data []byte) bool {
	// 检查JPEG
	_, err := jpeg.Decode(bytes.NewReader(data))
	if err == nil {
		return true
	}

	// 检查PNG
	_, err = png.Decode(bytes.NewReader(data))
	if err == nil {
		return true
	}

	return false
}

// TestImageModeration 测试函数
func TestImageModeration(cfg *common.Config, logger *zap.Logger) {
	api := NewImageModerationAPI(cfg, logger)

	// 模拟测试数据
	testCases := []struct {
		url   string
		scene string
	}{
		{"https://example.com/test1.jpg", "ugc"},
		{"https://example.com/test2.png", "ad"},
	}

	for _, tc := range testCases {
		req := &ImageModerationRequest{
			ImageURL: tc.url,
			Scene:    tc.scene,
		}

		result, err := api.moderator.Moderate(context.Background(), req)
		if err != nil {
			logger.Error("测试失败", zap.String("url", tc.url), zap.Error(err))
			continue
		}

		logger.Info("测试结果",
			zap.String("url", tc.url),
			zap.String("action", result.Action),
			zap.Float64("risk_score", result.RiskScore),
		)
	}
}
