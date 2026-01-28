package common

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config 定义应用程序的配置结构
type Config struct {
	NatsURL           string `mapstructure:"NATS_URL"`    // NATS 连接地址
	ServerPort        string `mapstructure:"SERVER_PORT"` // 服务监听端口
	MySQLDSN          string `mapstructure:"MYSQL_DSN"`   // MySQL 连接字符串
	OllamaHost        string `mapstructure:"OLLAMA_HOST"` // Ollama 地址 (已废弃，保留兼容)
	ChromaURL         string `mapstructure:"CHROMA_URL"`  // Chroma 地址 (已废弃，保留兼容)
	GatewayPort       string `mapstructure:"GATEWAY_PORT"`
	RuleEnginePort    string `mapstructure:"RULE_ENGINE_PORT"`
	LLMAgentPort      string `mapstructure:"LLM_AGENT_PORT"`
	RuleEngineAddr    string `mapstructure:"RULE_ENGINE_ADDR"`
	LLMAgentAddr      string `mapstructure:"LLM_AGENT_ADDR"`
	ArkAPIKey         string `mapstructure:"ARK_API_KEY"`
	ArkModelID        string `mapstructure:"ARK_MODEL_ID"`
	ArkEmbeddingModel string `mapstructure:"ARK_EMBEDDING_MODEL"`
	MilvusAddr        string `mapstructure:"MILVUS_ADDR"`
}

// LoadConfig 从环境变量加载配置
// 使用 Viper 库自动绑定环境变量
func LoadConfig() (*Config, error) {
	viper.SetDefault("NATS_URL", "nats://localhost:4222")
	viper.SetDefault("SERVER_PORT", "8080")
	viper.SetDefault("GATEWAY_PORT", "8080")
	viper.SetDefault("RULE_ENGINE_PORT", "8881")
	viper.SetDefault("LLM_AGENT_PORT", "8882")
	viper.SetDefault("RULE_ENGINE_ADDR", "rule-engine:8881")
	viper.SetDefault("LLM_AGENT_ADDR", "llm-agent:8882")
	viper.SetDefault("MYSQL_DSN", "root:root@tcp(localhost:3306)/safeflow?charset=utf8mb4&parseTime=True&loc=Local")
	viper.SetDefault("OLLAMA_HOST", "http://localhost:11434")
	viper.SetDefault("CHROMA_URL", "http://localhost:8000")
	viper.SetDefault("MILVUS_ADDR", "localhost:19530")
	viper.SetDefault("ARK_API_KEY", "")
	viper.SetDefault("ARK_MODEL_ID", "")
	viper.SetDefault("ARK_EMBEDDING_MODEL", "")

	configFile := os.Getenv("CONFIG_FILE")
	if configFile != "" {
		viper.SetConfigFile(configFile)
		_ = viper.ReadInConfig()
	} else {
		// 自动寻找配置文件
		wd, _ := os.Getwd()
		configs := []struct {
			name string
			ext  string
		}{
			{"config", "yaml"},
			{"config", "yml"},
			{".env", "env"},
		}

		found := false
		curr := wd
		for i := 0; i < 3; i++ {
			for _, c := range configs {
				path := filepath.Join(curr, c.name)
				if c.ext != "env" {
					path += "." + c.ext
				}
				if _, err := os.Stat(path); err == nil {
					viper.SetConfigFile(path)
					viper.SetConfigType(c.ext)
					if err := viper.ReadInConfig(); err == nil {
						found = true
						break
					}
				}
			}
			if found {
				break
			}
			curr = filepath.Dir(curr)
		}
	}

	viper.AutomaticEnv()

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}
	if config.GatewayPort == "" {
		config.GatewayPort = config.ServerPort
	}
	return &config, nil
}
