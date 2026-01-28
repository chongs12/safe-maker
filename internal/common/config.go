package common

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config 定义应用程序的配置结构
type Config struct {
	NatsURL        string `mapstructure:"NATS_URL"`    // NATS 连接地址
	ServerPort     string `mapstructure:"SERVER_PORT"` // 服务监听端口
	MySQLDSN       string `mapstructure:"MYSQL_DSN"`   // MySQL 连接字符串
	OllamaHost     string `mapstructure:"OLLAMA_HOST"` // Ollama 地址 (已废弃，保留兼容)
	ChromaURL      string `mapstructure:"CHROMA_URL"`  // Chroma 地址 (已废弃，保留兼容)
	GatewayPort    string `mapstructure:"GATEWAY_PORT"`
	RuleEnginePort string `mapstructure:"RULE_ENGINE_PORT"`
	LLMAgentPort   string `mapstructure:"LLM_AGENT_PORT"`
	RuleEngineAddr string `mapstructure:"RULE_ENGINE_ADDR"`
	LLMAgentAddr   string `mapstructure:"LLM_AGENT_ADDR"`
}

// LoadConfig 从环境变量加载配置
// 使用 Viper 库自动绑定环境变量
func LoadConfig() (*Config, error) {
	source := os.Getenv("CONFIG_SOURCE")
	if source == "" {
		source = "env"
	}
	configFile := os.Getenv("CONFIG_FILE")

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

	var config Config
	if source == "yaml" {
		viper.SetConfigType("yaml")
		if configFile != "" {
			viper.SetConfigFile(configFile)
			_ = viper.ReadInConfig()
		} else if wd, err := os.Getwd(); err == nil {
			candidates := []string{
				filepath.Join(wd, "config.yaml"),
				filepath.Join(filepath.Dir(wd), "config.yaml"),
				filepath.Join(filepath.Dir(filepath.Dir(wd)), "config.yaml"),
			}
			for _, file := range candidates {
				if _, err := os.Stat(file); err == nil {
					viper.SetConfigFile(file)
					_ = viper.ReadInConfig()
					break
				}
			}
		}
	} else {
		viper.SetConfigType("env")
		if configFile != "" {
			viper.SetConfigFile(configFile)
			_ = viper.ReadInConfig()
		} else if wd, err := os.Getwd(); err == nil {
			candidates := []string{
				filepath.Join(wd, ".env"),
				filepath.Join(filepath.Dir(wd), ".env"),
				filepath.Join(filepath.Dir(filepath.Dir(wd)), ".env"),
			}
			for _, file := range candidates {
				if _, err := os.Stat(file); err == nil {
					viper.SetConfigFile(file)
					_ = viper.ReadInConfig()
					break
				}
			}
		}
		viper.AutomaticEnv()
	}
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}
	if config.GatewayPort == "" {
		config.GatewayPort = config.ServerPort
	}
	return &config, nil
}
