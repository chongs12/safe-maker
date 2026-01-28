package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	// 获取网关地址，默认为 8080 (用户环境) 或 8081 (测试环境)
	port := os.Getenv("GATEWAY_PORT")
	if port == "" {
		port = "8080"
	}
	url := fmt.Sprintf("http://localhost:%s/submit", port)

	tests := []struct {
		name    string
		content string
		want    string // 期望的 action
	}{
		{
			name:    "规则引擎拦截-兼职刷单",
			content: "兼职刷单，加微信 123456",
			want:    "block",
		},
		{
			name:    "LLM拦截-高薪诈骗",
			content: "我们需要有进取心的人，只需少量押金，日入过万不是梦，无需经验。",
			want:    "block", // 或 review
		},
		{
			name:    "正常内容",
			content: "今天天气真不错，适合出去公园散步。",
			want:    "allow",
		},
		{
			name:    "规则引擎拦截-博彩赌博",
			content: "博彩赌博，加微信 123456",
			want:    "block",
		},
		{
			name:    "LLM拦截-阶级对立",
			content: "我支持户晨风，他被封杀只是因为说中了很多实话",
			want:    "block",
		},
	}

	successCount := 0
	for _, tt := range tests {
		fmt.Printf("正在测试: [%s]...\n", tt.name)

		payload := map[string]string{
			"user_id": "test_verify",
			"content": tt.content,
		}
		jsonData, _ := json.Marshal(payload)

		resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			fmt.Printf("❌ 请求失败: %v\n", err)
			continue
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)

		var result struct {
			Action string `json:"action"`
			Reason string `json:"reason"`
			Source string `json:"source"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			fmt.Printf("❌ 解析响应失败: %v. Body: %s\n", err, string(body))
			continue
		}

		fmt.Printf("   结果: Action=%s, Source=%s, Reason=%s\n", result.Action, result.Source, result.Reason)

		// 验证逻辑
		pass := false
		if tt.want == "block" {
			if result.Action == "block" || result.Action == "review" {
				pass = true
			}
		} else {
			if result.Action == tt.want {
				pass = true
			}
		}

		if pass {
			fmt.Println("   ✅ 测试通过")
			successCount++
		} else {
			fmt.Printf("   ❌ 测试失败 (期望 %s, 实际 %s)\n", tt.want, result.Action)
		}
		fmt.Println("---------------------------------------------------")
		time.Sleep(500 * time.Millisecond)
	}

	if successCount == len(tests) {
		fmt.Println("🎉 所有测试用例均通过！服务运行正常。")
		os.Exit(0)
	} else {
		fmt.Println("⚠️ 部分测试失败，请检查日志。")
		os.Exit(1)
	}
}
