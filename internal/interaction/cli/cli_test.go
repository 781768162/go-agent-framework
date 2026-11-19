package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestConfirmOperateByUser(t *testing.T) {
	for _, tc := range []struct {
		name, input     string
		approved, retry bool
		err             error
	}{
		{"同意", "y\n", true, false, nil},
		{"重试后同意", "不确定\n YES \n", true, true, nil},
		{"拒绝", "N\n", false, false, nil},
		{"完整拒绝", "no\n", false, false, nil},
		{"输入结束", "", false, false, io.EOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			const dir, command = "工作 目录", "cat 'test file'"
			terminal := NewCli(strings.NewReader(tc.input), &out)
			ok, err := terminal.ConfirmOperateByUser(t.Context(), dir, command)
			if ok != tc.approved || !errors.Is(err, tc.err) {
				t.Fatalf("确认 = %v，错误 = %v", ok, err)
			}

			prompt := out.String()
			if !strings.Contains(prompt, dir) || !strings.Contains(prompt, command) || strings.Contains(prompt, "请输入") != tc.retry {
				t.Fatalf("确认提示 = %q", prompt)
			}
		})
	}
}
