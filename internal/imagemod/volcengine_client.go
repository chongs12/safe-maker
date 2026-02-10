package imagemod

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/safeflow-project/safeflow/internal/common"
	"go.uber.org/zap"
)

// VolcengineImageModerationClient 火山引擎图像审核客户端
type VolcengineImageModerationClient struct {
	accessKey string
	secretKey string
	region    string
	logger    *zap.Logger
}

// NewVolcengineImageModerationClient 创建客户端
func NewVolcengineImageModerationClient(cfg *common.Config, logger *zap.Logger) *VolcengineImageModerationClient {
	return &VolcengineImageModerationClient{
		accessKey: cfg.VolcengineAccessKey,
		secretKey: cfg.VolcengineSecretKey,
		region:    cfg.VolcengineRegion,
		logger:    logger,
	}
}

// ModerationRequest 图像审核请求
type ModerationRequest struct {
	Service string `json:"Service"`
	Version string `json:"Version"`
	Action  string `json:"Action"`
	Region  string `json:"Region"`
	ImageURL string `json:"ImageURL"`
}

// ModerationResult 图像审核结果
type ModerationResult struct {
	RequestId string `json:"RequestId"`
	Code      int    `json:"Code"`
	Message   string `json:"Message"`
	Data      Data   `json:"Data"`
}

// Data 审核数据
type Data struct {
	Antispam Antispam `json:"antispam"`
	Quality  Quality  `json:"quality"`
	Face     Face     `json:"face"`
}

// Antispam 反垃圾检测结果
type Antispam struct {
	Suggestion string  `json:"suggestion"` // pass/block/review
	Label      string  `json:"label"`      // 检测标签
	Rate       float64 `json:"rate"`       // 置信度
}

// Quality 图像质量检测结果
type Quality struct {
	Suggestion string `json:"suggestion"`
	Label      string `json:"label"`
	Rate       int    `json:"rate"`
}

// Face 人脸识别结果
type Face struct {
	Suggestion string `json:"suggestion"`
	Label      string `json:"label"`
	Rate       int    `json:"rate"`
}

// Moderate 图像审核主方法
func (c *VolcengineImageModerationClient) Moderate(ctx context.Context, imageURL string) (*ModerationResult, error) {
	if c.accessKey == "" || c.secretKey == "" {
		return nil, fmt.Errorf("火山引擎密钥未配置")
	}

	// 构造请求参数
	params := map[string]string{
		"Service":  "imageaudit",
		"Version":  "2021-06-01",
		"Action":   "ImageContentRisk",
		"Region":   c.region,
		"ImageURL": imageURL,
	}

	// 构造签名
	signature := c.signRequest(params)
	params["Signature"] = signature

	// 发送请求
	result, err := c.sendRequest(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("调用火山引擎API失败: %w", err)
	}

	c.logger.Info("图像审核完成",
		zap.String("image_url", imageURL),
		zap.String("suggestion", result.Data.Antispam.Suggestion),
		zap.String("label", result.Data.Antispam.Label),
		zap.Float64("rate", result.Data.Antispam.Rate),
	)

	return result, nil
}

// signRequest 构造请求签名
func (c *VolcengineImageModerationClient) signRequest(params map[string]string) string {
	// 1. 构造规范请求字符串
	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var canonicalQueryString []string
	for _, k := range keys {
		canonicalQueryString = append(canonicalQueryString, 
			fmt.Sprintf("%s=%s", url.QueryEscape(k), url.QueryEscape(params[k])))
	}
	canonicalString := strings.Join(canonicalQueryString, "&")

	// 2. 构造待签名字符串
	stringToSign := fmt.Sprintf("GET\n/\n%s", canonicalString)

	// 3. 计算签名
	h := hmac.New(sha256.New, []byte(c.secretKey))
	h.Write([]byte(stringToSign))
	signature := hex.EncodeToString(h.Sum(nil))

	return signature
}

// sendRequest 发送HTTP请求
func (c *VolcengineImageModerationClient) sendRequest(ctx context.Context, params map[string]string) (*ModerationResult, error) {
	// 构造查询参数
	var queryParams []string
	for k, v := range params {
		queryParams = append(queryParams, fmt.Sprintf("%s=%s", k, url.QueryEscape(v)))
	}
	queryString := strings.Join(queryParams, "&")

	// 构造请求URL
	apiURL := fmt.Sprintf("https://imageaudit.volcengineapi.com/?%s", queryString)

	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("TC3-HMAC-SHA256 Credential=%s/%s/imageaudit/request", 
		c.accessKey, time.Now().UTC().Format("20060102")))

	// 发送请求
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 解析响应
	var result ModerationResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w, 响应内容: %s", err, string(body))
	}

	// 检查API调用是否成功
	if result.Code != 0 {
		return nil, fmt.Errorf("API调用失败: %d - %s", result.Code, result.Message)
	}

	return &result, nil
}

// ConvertToInternalResult 转换为内部结果格式
func (c *VolcengineImageModerationClient) ConvertToInternalResult(volcResult *ModerationResult) *ImageModerationResult {
	internal := &ImageModerationResult{
		RequestID: volcResult.RequestId,
		Labels:    []Label{},
	}

	// 转换反垃圾检测结果
	if volcResult.Data.Antispam.Suggestion != "" {
		label := Label{
			Name:       volcResult.Data.Antispam.Label,
			Confidence: volcResult.Data.Antispam.Rate,
		}
		
		// 设置审核动作
		switch volcResult.Data.Antispam.Suggestion {
		case "pass":
			internal.Action = "allow"
		case "block":
			internal.Action = "block"
		case "review":
			internal.Action = "review"
		default:
			internal.Action = "review"
		}
		
		internal.Labels = append(internal.Labels, label)
		internal.RiskScore = volcResult.Data.Antispam.Rate
	}

	// 转换质量检测结果
	if volcResult.Data.Quality.Suggestion != "" {
		label := Label{
			Name:       "quality_" + volcResult.Data.Quality.Label,
			Confidence: float64(volcResult.Data.Quality.Rate) / 100.0,
		}
		internal.Labels = append(internal.Labels, label)
	}

	// 转换人脸识别结果
	if volcResult.Data.Face.Suggestion != "" {
		label := Label{
			Name:       "face_" + volcResult.Data.Face.Label,
			Confidence: float64(volcResult.Data.Face.Rate) / 100.0,
		}
		internal.Labels = append(internal.Labels, label)
	}

	// 生成审核理由
	internal.Reason = c.generateReason(internal.Labels, internal.RiskScore)

	return internal
}

// generateReason 生成审核理由
func (c *VolcengineImageModerationClient) generateReason(labels []Label, riskScore float64) string {
	if len(labels) == 0 {
		return fmt.Sprintf("图像审核通过，风险评分%.2f", riskScore)
	}

	var reasons []string
	for _, label := range labels {
		if label.Confidence > 0.5 {
			reasons = append(reasons, fmt.Sprintf("%s(置信度%.2f)", label.Name, label.Confidence))
		}
	}

	if len(reasons) == 0 {
		return fmt.Sprintf("图像审核通过，风险评分%.2f", riskScore)
	}

	return fmt.Sprintf("检测到: %s，风险评分%.2f", strings.Join(reasons, ", "), riskScore)
}