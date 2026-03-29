package common

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config 定义应用程序的配置结构
type Config struct {
	NatsURL                string   `mapstructure:"NATS_URL"`    // NATS 连接地址
	ServerPort             string   `mapstructure:"SERVER_PORT"` // 服务监听端口
	MySQLDSN               string   `mapstructure:"MYSQL_DSN"`   // MySQL 连接字符串
	OllamaHost             string   `mapstructure:"OLLAMA_HOST"` // Ollama 地址 (已废弃，保留兼容)
	ChromaURL              string   `mapstructure:"CHROMA_URL"`  // Chroma 地址 (已废弃，保留兼容)
	GatewayPort            string   `mapstructure:"GATEWAY_PORT"`
	RuleEnginePort         string   `mapstructure:"RULE_ENGINE_PORT"`
	LLMAgentPort           string   `mapstructure:"LLM_AGENT_PORT"`
	ImageModerationPort    string   `mapstructure:"IMAGE_MODERATION_PORT"`     // 图片审核HTTP端口
	ImageModerationRPCPort string   `mapstructure:"IMAGE_MODERATION_RPC_PORT"` // 图片审核RPC端口
	RuleEngineAddr         string   `mapstructure:"RULE_ENGINE_ADDR"`
	LLMAgentAddr           string   `mapstructure:"LLM_AGENT_ADDR"`
	ImageModerationAddr    string   `mapstructure:"IMAGE_MODERATION_ADDR"` // 图片审核服务地址
	ArkAPIKey              string   `mapstructure:"ARK_API_KEY"`
	ArkModelID             string   `mapstructure:"ARK_MODEL_ID"`
	ArkEmbeddingModel      string   `mapstructure:"ARK_EMBEDDING_MODEL"`
	ArkEndpoint            string   `mapstructure:"ARK_ENDPOINT"` // Ark API Endpoint，如 arn:bc:ark:::v1/character/xxx
	MilvusAddr             string   `mapstructure:"MILVUS_ADDR"`
	EtcdEndpoints          []string `mapstructure:"ETCD_ENDPOINTS"`        // Etcd 地址列表
	UseEtcdRegistry        bool     `mapstructure:"USE_ETCD_REGISTRY"`     // 是否使用 Etcd 服务注册
	JaegerEndpoint         string   `mapstructure:"JAEGER_ENDPOINT"`       // Jaeger 收集器地址
	VolcengineAccessKey    string   `mapstructure:"VOLCENGINE_ACCESS_KEY"` // 火山引擎AccessKey
	VolcengineSecretKey    string   `mapstructure:"VOLCENGINE_SECRET_KEY"` // 火山引擎SecretKey
	VolcengineRegion       string   `mapstructure:"VOLCENGINE_REGION"`     // 火山引擎区域
}

// LoadConfig 从环境变量加载配置
// 使用 Viper 库自动绑定环境变量
func LoadConfig() (*Config, error) {
	// 首先尝试加载.env文件
	if err := godotenv.Load(); err != nil {
		// .env文件不存在不是致命错误，继续使用系统环境变量
	}

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
	viper.SetDefault("ARK_ENDPOINT", "")
	viper.SetDefault("ETCD_ENDPOINTS", []string{"localhost:2379"})
	viper.SetDefault("USE_ETCD_REGISTRY", false)
	viper.SetDefault("JAEGER_ENDPOINT", "http://localhost:14268/api/traces") // Jaeger 默认地址
	viper.SetDefault("IMAGE_MODERATION_PORT", "8081")
	viper.SetDefault("IMAGE_MODERATION_RPC_PORT", "8883")
	viper.SetDefault("IMAGE_MODERATION_ADDR", "image-moderation:8883")
	viper.SetDefault("VOLCENGINE_ACCESS_KEY", "")
	viper.SetDefault("VOLCENGINE_SECRET_KEY", "")
	viper.SetDefault("VOLCENGINE_REGION", "cn-north-1")

	configFile := os.Getenv("CONFIG_FILE")
	if configFile != "" {
		if !readConfigAtPath(configFile) && !autoReadConfig() {
			return nil, fmt.Errorf("无法加载配置文件: %s", configFile)
		}
	} else {
		_ = autoReadConfig()
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

func readConfigAtPath(path string) bool {
	if _, err := os.Stat(path); err == nil {
		viper.SetConfigFile(path)
		return viper.ReadInConfig() == nil
	}
	wd, _ := os.Getwd()
	joined := filepath.Join(wd, path)
	if _, err := os.Stat(joined); err == nil {
		viper.SetConfigFile(joined)
		return viper.ReadInConfig() == nil
	}
	return false
}

func autoReadConfig() bool {
	wd, _ := os.Getwd()
	configs := []struct {
		name string
		ext  string
	}{
		{"config", "yaml"},
		{"config", "yml"},
		{".env", "env"},
	}
	curr := wd
	for i := 0; i < 5; i++ {
		for _, c := range configs {
			path := filepath.Join(curr, c.name)
			if c.ext != "env" {
				path += "." + c.ext
			}
			if _, err := os.Stat(path); err == nil {
				viper.SetConfigFile(path)
				viper.SetConfigType(c.ext)
				if err := viper.ReadInConfig(); err == nil {
					return true
				}
			}
		}
		next := filepath.Dir(curr)
		if next == curr {
			break
		}
		curr = next
	}
	return false
}
