package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 各用例显式 t.Setenv 置空或设值，不依赖宿主 shell 是否 export 过这些变量，
// 避免本地绿、CI 红的漂移。godotenv.Load() 在包目录找不到 .env 时静默跳过，
// 因此这些用例走的是纯环境变量路径。

func TestNewConfigFromEnv(t *testing.T) {
	t.Run("缺 APIKey 报错", func(t *testing.T) {
		t.Setenv("DEEPSEEK_API_KEY", "")
		t.Setenv("DEEPSEEK_ENDPOINT", "")

		_, err := NewConfigFromEnv()
		if err == nil {
			t.Fatal("缺 DEEPSEEK_API_KEY 应该报错")
		}
		if !strings.Contains(err.Error(), "DEEPSEEK_API_KEY") {
			t.Errorf("错误信息应包含变量名，方便排查: %v", err)
		}
	})

	t.Run("齐全时成功", func(t *testing.T) {
		t.Setenv("DEEPSEEK_API_KEY", "sk-test")
		t.Setenv("DEEPSEEK_ENDPOINT", "http://localhost:9999")

		cfg, err := NewConfigFromEnv()
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if cfg.APIKey != "sk-test" {
			t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-test")
		}
		if cfg.Endpoint != "http://localhost:9999" {
			t.Errorf("Endpoint = %q, want %q", cfg.Endpoint, "http://localhost:9999")
		}
	})

	t.Run("Endpoint 缺省官方地址", func(t *testing.T) {
		t.Setenv("DEEPSEEK_API_KEY", "sk-test")
		t.Setenv("DEEPSEEK_ENDPOINT", "")

		cfg, err := NewConfigFromEnv()
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if cfg.Endpoint != defaultEndpoint {
			t.Errorf("Endpoint = %q, want %q", cfg.Endpoint, defaultEndpoint)
		}
	})

	t.Run("DefaultModel 为内置默认型号", func(t *testing.T) {
		t.Setenv("DEEPSEEK_API_KEY", "sk-test")
		t.Setenv("DEEPSEEK_ENDPOINT", "")

		cfg, err := NewConfigFromEnv()
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if cfg.DefaultModel != defaultModel {
			t.Errorf("DefaultModel = %q, want %q", cfg.DefaultModel, defaultModel)
		}
	})
}

// .env 存在但格式坏时必须报错而非静默跳过：
// 用户写错了 .env 却以为配置生效，排查成本极高。
// godotenv 对没有等号的行报 unexpected character（非 NotExist，实验验证过），
// t.Chdir 把工作目录切到临时目录让 Load() 读到坏文件，结束后自动恢复。
func TestNewConfigFromEnvBadDotenv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("没有等号的坏行"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	_, err := NewConfigFromEnv()
	if err == nil {
		t.Fatal(".env 格式坏应该报错，实际被静默跳过")
	}
	if !strings.Contains(err.Error(), "解析") {
		t.Errorf("错误应说明是 .env 解析问题: %v", err)
	}
}
