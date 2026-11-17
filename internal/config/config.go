package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/joho/godotenv"
)

const (
	defaultEndpoint = "https://api.deepseek.com/chat/completions"
	defaultModel    = "deepseek-flash" // = DeepSeek-V4.1-Flash-1017
)

type Config struct {
	APIKey       string
	Endpoint     string
	DefaultModel string
}

// 从.env和环境变量读取
func NewConfigFromEnv() (Config, error) {
	cfg := Config{}

	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return cfg, fmt.Errorf("解析.env失败: %w", err)
	}

	cfg.APIKey = os.Getenv("DEEPSEEK_API_KEY")
	cfg.Endpoint = os.Getenv("DEEPSEEK_ENDPOINT")
	if cfg.Endpoint == "" {
		cfg.Endpoint = defaultEndpoint
	}
	if cfg.APIKey == "" {
		return cfg, fmt.Errorf("未找到DEEPSEEK_API_KEY")
	}
	if cfg.DefaultModel == "" {
		cfg.DefaultModel = defaultModel
	}
	return cfg, nil
}
