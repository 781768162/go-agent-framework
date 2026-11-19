package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExecBashWorkingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "工作 目录")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "input.txt"), []byte("目标文件"), 0600); err != nil {
		t.Fatal(err)
	}

	output, err := ExecBash(t.Context(), dir, "cat input.txt")
	if err != nil || output != "目标文件" {
		t.Fatalf("输出 = %q，错误 = %v", output, err)
	}
}
