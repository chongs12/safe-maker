package multimodal

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/safeflow-project/safeflow/internal/common"
	"go.uber.org/zap"
)

// ImageModerationRequest 图片审核请求
type ImageModerationRequest struct {
	ImageData []byte `json:"image_data"` // 图片二进制数据
	ImageURL  string `json:"image_url"`  // 图片URL（二选一）
	Scene     string `json:"scene"`      // 场景: ugc/ad/profile
}

// ImageModerationResult 图片审核结果
type ImageModerationResult struct {
	RequestID   string  `json:"request_id"`
	Action      string  `json:"action"`       // allow/block/review
	RiskScore   float64 `json:"risk_score"`   // 风险分数 0-1
	TextContent string  `json:"text_content"` // OCR提取的文本
	Labels      []Label `json:"labels"`       // 检测到的标签
	Reason      string  `json:"reason"`       // 审核理由
}

// Label 检测标签
type Label struct {
	Name       string  `json:"name"`       // 标签名称
	Confidence float64 `json:"confidence"` // 置信度
	Positions  []Box   `json:"positions"`  // 位置坐标
}

// Box 边界框
type Box struct {
	X1, Y1, X2, Y2 int `json:"coordinates"`
}

// ImageModerator 图片审核器
type ImageModerator struct {
	cfg    *common.Config
	logger *zap.Logger
}

// NewImageModerator 创建图片审核器
func NewImageModerator(cfg *common.Config, logger *zap.Logger) *ImageModerator {
	return &ImageModerator{
		cfg:    cfg,
		logger: logger,
	}
}

// Moderate 审核图片
func (im *ImageModerator) Moderate(ctx context.Context, req *ImageModerationRequest) (*ImageModerationResult, error) {
	start := time.Now()
	requestID := fmt.Sprintf("img_%d", time.Now().UnixNano())

	// 1. 获取图片数据
	imageData, err := im.getImageData(req)
	if err != nil {
		return nil, fmt.Errorf("获取图片数据失败: %w", err)
	}

	result := &ImageModerationResult{
		RequestID: requestID,
		Action:    "allow", // 默认允许
	}

	// 2. OCR文本提取
	textContent, err := im.extractText(imageData)
	if err != nil {
		im.logger.Warn("OCR提取失败", zap.Error(err))
	} else {
		result.TextContent = textContent
		// 对提取的文本进行审核
		if textContent != "" {
			// TODO: 调用文本审核服务
			im.logger.Info("OCR提取文本", zap.String("text", textContent))
		}
	}

	// 3. 视觉内容检测
	labels, err := im.detectVisualContent(imageData)
	if err != nil {
		im.logger.Warn("视觉检测失败", zap.Error(err))
	} else {
		result.Labels = labels
	}

	// 4. 风险评估和决策
	result.RiskScore = im.calculateRiskScore(labels, textContent)
	result.Action = im.determineAction(result.RiskScore)
	result.Reason = im.generateReason(labels, textContent, result.RiskScore)

	im.logger.Info("图片审核完成",
		zap.String("request_id", requestID),
		zap.String("action", result.Action),
		zap.Float64("risk_score", result.RiskScore),
		zap.Duration("duration", time.Since(start)),
	)

	return result, nil
}

// getImageData 获取图片数据
func (im *ImageModerator) getImageData(req *ImageModerationRequest) ([]byte, error) {
	if len(req.ImageData) > 0 {
		return req.ImageData, nil
	}

	if req.ImageURL != "" {
		resp, err := http.Get(req.ImageURL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		return ioutil.ReadAll(resp.Body)
	}

	return nil, fmt.Errorf("必须提供ImageData或ImageURL")
}

// extractText OCR文本提取（占位实现）
func (im *ImageModerator) extractText(imageData []byte) (string, error) {
	// TODO: 集成OCR服务
	// 示例：调用百度OCR或PaddleOCR
	base64Img := base64.StdEncoding.EncodeToString(imageData)

	// 模拟OCR结果
	im.logger.Debug("模拟OCR提取", zap.String("image_size", fmt.Sprintf("%d bytes", len(imageData))))

	// 实际实现示例：
	/*
		client := ocr.NewClient(im.cfg.OcrAPIKey)
		result, err := client.Recognize(imageData)
		if err != nil {
			return "", err
		}
		return result.Text, nil
	*/

	return "", nil // 暂时返回空文本
}

// detectVisualContent 视觉内容检测（占位实现）
func (im *ImageModerator) detectVisualContent(imageData []byte) ([]Label, error) {
	// TODO: 集成图像审核API
	// 示例：调用火山引擎图像审核

	// 模拟检测结果
	labels := []Label{
		{
			Name:       "normal",
			Confidence: 0.95,
			Positions:  []Box{{0, 0, 100, 100}},
		},
	}

	im.logger.Debug("模拟视觉检测", zap.Int("label_count", len(labels)))

	// 实际实现示例：
	/*
		client := vision.NewClient(im.cfg.VisionAPIKey)
		results, err := client.Detect(imageData)
		if err != nil {
			return nil, err
		}

		var labels []Label
		for _, result := range results {
			labels = append(labels, Label{
				Name:       result.Label,
				Confidence: result.Confidence,
				Positions:  convertToBoxes(result.BoundingBoxes),
			})
		}
	*/

	return labels, nil
}

// calculateRiskScore 计算风险分数
func (im *ImageModerator) calculateRiskScore(labels []Label, textContent string) float64 {
	score := 0.0

	// 基于标签的风险评分
	for _, label := range labels {
		switch label.Name {
		case "porn", "sexy":
			score += label.Confidence * 0.8
		case "violence", "bloody":
			score += label.Confidence * 0.9
		case "political":
			score += label.Confidence * 0.7
		case "ads":
			score += label.Confidence * 0.3
		}
	}

	// 基于文本内容的风险评分
	if textContent != "" {
		// 可以调用文本审核服务获取分数
		// 这里简化处理
		if len(textContent) > 50 {
			score += 0.1 // 长文本略微增加风险
		}
	}

	// 确保分数在0-1范围内
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// determineAction 确定审核动作
func (im *ImageModerator) determineAction(riskScore float64) string {
	switch {
	case riskScore >= 0.8:
		return "block"
	case riskScore >= 0.5:
		return "review"
	default:
		return "allow"
	}
}

// generateReason 生成审核理由
func (im *ImageModerator) generateReason(labels []Label, textContent string, riskScore float64) string {
	reasons := []string{}

	// 添加标签相关的理由
	for _, label := range labels {
		if label.Confidence > 0.5 {
			reasons = append(reasons, fmt.Sprintf("检测到%s内容(置信度%.2f)", label.Name, label.Confidence))
		}
	}

	// 添加文本相关理由
	if textContent != "" {
		reasons = append(reasons, fmt.Sprintf("包含文本内容(%d字符)", len(textContent)))
	}

	// 添加风险评分理由
	reasons = append(reasons, fmt.Sprintf("综合风险评分%.2f", riskScore))

	if len(reasons) == 0 {
		return "内容正常，无违规风险"
	}

	return "检测到: " + joinStrings(reasons, "; ")
}

// 辅助函数
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}

	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
