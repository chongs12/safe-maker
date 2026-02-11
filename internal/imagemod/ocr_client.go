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
	"strings"
	"time"

	"github.com/safeflow-project/safeflow/internal/common"
	safeflow "github.com/safeflow-project/safeflow/kitex_gen/safeflow"
	"go.uber.org/zap"
)

// VolcengineOCRClient 火山引擎OCR客户端
type VolcengineOCRClient struct {
	accessKey string
	secretKey string
	region    string
	logger    *zap.Logger
}

// NewVolcengineOCRClient 创建OCR客户端
func NewVolcengineOCRClient(cfg *common.Config, logger *zap.Logger) *VolcengineOCRClient {
	return &VolcengineOCRClient{
		accessKey: cfg.VolcengineAccessKey,
		secretKey: cfg.VolcengineSecretKey,
		region:    cfg.VolcengineRegion,
		logger:    logger,
	}
}

// OCRRequest OCR请求参数
type OCRRequest struct {
	ImageURL string `json:"image_url,omitempty"`
	Image    []byte `json:"image,omitempty"`    // base64编码
	Language string `json:"language,omitempty"` // zh/en/ja/ko
	Detect   string `json:"detect,omitempty"`   // text/table/qrcode
}

// OCRResponse OCR响应结果
type OCRResponse struct {
	Code    int     `json:"code"`
	Message string  `json:"message"`
	Data    OCRData `json:"data"`
}

// OCRData OCR数据
type OCRData struct {
	LineTexts []string   `json:"line_texts"` // 识别出的文本行
	LineRects []LineRect `json:"line_rects"` // 文本行位置
	LineProbs []float64  `json:"line_probs"` // 置信度
}

// LineRect 文本行矩形位置
type LineRect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Recognize 执行OCR识别
func (c *VolcengineOCRClient) Recognize(ctx context.Context, req *OCRRequest) (*safeflow.OCRResult_, error) {
	if c.accessKey == "" || c.secretKey == "" {
		return nil, fmt.Errorf("火山引擎密钥未配置")
	}

	// 构造API请求
	apiReq := map[string]interface{}{
		"Service": "ocr",
		"Version": "2020-08-26",
		"Action":  "OCRNormal",
		"Region":  c.region,
	}

	if req.ImageURL != "" {
		apiReq["image_url"] = req.ImageURL
	}

	if len(req.Image) > 0 {
		// TODO: 实现base64编码
		apiReq["image_base64"] = req.Image
	}

	if req.Language != "" {
		// 火山引擎OCR会自动识别语言
	} else {
		// 默认中文
	}

	if req.Detect != "" {
		// 不同的检测类型对应不同的Action
		switch req.Detect {
		case "text":
			apiReq["Action"] = "OCRNormal" // 通用文字识别
		case "table":
			apiReq["Action"] = "OCRPdf" // 智能文档解析
		case "qrcode":
			// TODO: 实现二维码识别
		}
	}

	// 调用API
	response, err := c.callOCRService(ctx, apiReq)
	if err != nil {
		return nil, fmt.Errorf("调用OCR服务失败: %w", err)
	}

	// 转换为Thrift格式
	extractedText := c.extractFullTextFromLines(response.Data.LineTexts, response.Data.LineProbs)
	confidence := c.calculateAverageConfidence(response.Data.LineProbs)

	ocrResult := &safeflow.OCRResult_{
		ExtractedText: extractedText,
		Confidence:    confidence,
		Language:      "zh", // 默认中文
	}

	// 转换文本块
	var textBlocks []*safeflow.TextBlock
	for i, lineText := range response.Data.LineTexts {
		if i < len(response.Data.LineRects) && i < len(response.Data.LineProbs) {
			rect := response.Data.LineRects[i]
			prob := response.Data.LineProbs[i]

			if prob > 0.3 { // 过滤低置信度文本
				textBlock := &safeflow.TextBlock{
					Text:       lineText,
					Confidence: prob,
					BoundingBox: &safeflow.Box{
						X1: int32(rect.X),
						Y1: int32(rect.Y),
						X2: int32(rect.X + rect.Width),
						Y2: int32(rect.Y + rect.Height),
					},
				}
				textBlocks = append(textBlocks, textBlock)
			}
		}
	}
	ocrResult.TextBlocks = textBlocks

	c.logger.Info("OCR识别完成",
		zap.String("extracted_text", extractedText),
		zap.Float64("confidence", confidence),
		zap.Int("text_blocks", len(textBlocks)),
	)

	return ocrResult, nil
}

// callOCRService 调用OCR服务API
func (c *VolcengineOCRClient) callOCRService(ctx context.Context, params map[string]interface{}) (*OCRResponse, error) {
	// 构造API请求 - 使用正确的火山引擎OCR API规范
	host := "visual.volcengineapi.com"
	action := "OCRNormal"   // 通用文字识别 (中英文)
	version := "2020-08-26" // 对应版本
	service := "ocr"

	// 构造查询参数
	queryParams := fmt.Sprintf("Action=%s&Version=%s", action, version)

	// 构造请求体 - 使用application/x-www-form-urlencoded格式
	var bodyParams []string
	for key, value := range params {
		if strValue, ok := value.(string); ok {
			bodyParams = append(bodyParams, fmt.Sprintf("%s=%s", key, url.QueryEscape(strValue)))
		}
	}
	bodyString := strings.Join(bodyParams, "&")

	// 构造规范请求
	canonicalURI := "/"
	canonicalQueryString := queryParams
	canonicalHeaders := fmt.Sprintf("content-length:%d\ncontent-type:application/x-www-form-urlencoded\nhost:%s\nx-content-sha256-trailers:\n",
		len(bodyString), host)
	signedHeaders := "content-length;content-type;host;x-content-sha256-trailers"

	// 计算payload hash
	hash := sha256.Sum256([]byte(bodyString))
	payloadHash := hex.EncodeToString(hash[:])

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		"POST", canonicalURI, canonicalQueryString, canonicalHeaders, signedHeaders, payloadHash)

	// 构造待签名字符串
	algorithm := "TC3-HMAC-SHA256"
	timestamp := time.Now().Unix()
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, service)

	hash = sha256.Sum256([]byte(canonicalRequest))
	stringToSign := fmt.Sprintf("%s\n%d\n%s\n%s",
		algorithm, timestamp, credentialScope, hex.EncodeToString(hash[:]))

	// 计算签名
	secretDate := hmacSHA256OCR(date, []byte("TC3"+c.secretKey))
	secretService := hmacSHA256OCR(service, secretDate)
	secretSigning := hmacSHA256OCR("tc3_request", secretService)
	signature := hex.EncodeToString(hmacSHA256OCR(stringToSign, secretSigning))

	// 构造Authorization头
	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, c.accessKey, credentialScope, signedHeaders, signature)

	// 构造完整URL
	apiURL := fmt.Sprintf("https://%s%s?%s", host, canonicalURI, queryParams)

	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(bodyString))
	if err != nil {
		return nil, err
	}

	// 设置请求头 - 符合文档要求
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(bodyString)))
	req.Header.Set("Host", host)
	req.Header.Set("Authorization", authorization)

	// 添加X-Date头部（UTC时间）
	utcTime := time.Unix(timestamp, 0).UTC().Format("20060102T150405Z")
	req.Header.Set("X-Date", utcTime)

	req.Header.Set("X-TC-Version", version)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Region", c.region)

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
	var result OCRResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析OCR响应失败: %w, 响应内容: %s", err, string(body))
	}

	// 检查API调用是否成功
	if result.Code != 10000 { // 火山引擎OCR成功码是10000
		// 如果code为0且有message，可能是API错误
		if result.Code == 0 && result.Message != "" {
			return nil, fmt.Errorf("OCR API调用失败: %s", result.Message)
		}
		return nil, fmt.Errorf("OCR API调用失败: %d - %s", result.Code, result.Message)
	}

	return &result, nil
}

// hmacSHA256OCR HMAC-SHA256计算（OCR专用）
func hmacSHA256OCR(message string, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(message))
	return h.Sum(nil)
}

// extractFullTextFromLines 从行文本中提取完整文本
func (c *VolcengineOCRClient) extractFullTextFromLines(lines []string, probs []float64) string {
	if len(lines) == 0 {
		return ""
	}

	var texts []string
	for i, line := range lines {
		// 根据置信度过滤低质量文本
		if i < len(probs) && probs[i] > 0.5 {
			texts = append(texts, line)
		} else if i >= len(probs) {
			// 如果没有置信度信息，保留所有文本
			texts = append(texts, line)
		}
	}

	return strings.Join(texts, "\n")
}

// calculateAverageConfidence 计算平均置信度
func (c *VolcengineOCRClient) calculateAverageConfidence(probs []float64) float64 {
	if len(probs) == 0 {
		return 0.0
	}

	sum := 0.0
	for _, prob := range probs {
		sum += prob
	}
	return sum / float64(len(probs))
}

// PostProcessOCRResult OCR结果后处理
func (c *VolcengineOCRClient) PostProcessOCRResult(ocrResult *safeflow.OCRResult_) *safeflow.OCRResult_ {
	if ocrResult == nil {
		return nil
	}

	// 文本清洗和格式化
	cleanedText := c.cleanText(ocrResult.GetExtractedText())

	// 过滤低质量文本块
	var filteredBlocks []*safeflow.TextBlock
	for _, block := range ocrResult.GetTextBlocks() {
		if block.GetConfidence() > 0.3 && len(block.GetText()) > 1 {
			filteredBlocks = append(filteredBlocks, block)
		}
	}

	return &safeflow.OCRResult_{
		ExtractedText: cleanedText,
		Confidence:    ocrResult.GetConfidence(),
		Language:      ocrResult.GetLanguage(),
		TextBlocks:    filteredBlocks,
	}
}

// cleanText 文本清洗
func (c *VolcengineOCRClient) cleanText(text string) string {
	// 移除多余的空白字符
	text = strings.TrimSpace(text)

	// 标准化换行符
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	// 移除连续的空行
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			cleanedLines = append(cleanedLines, line)
		}
	}

	return strings.Join(cleanedLines, "\n")
}
