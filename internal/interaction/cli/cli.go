package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"go-agent-framework/internal/agent"
	"go-agent-framework/internal/apis"
)

type Cli struct {
	Scanner *bufio.Scanner
	out     io.Writer
}

func NewCli(in io.Reader, out io.Writer) *Cli {
	return &Cli{
		Scanner: bufio.NewScanner(in),
		out:     out,
	}
}

func (c *Cli) ReadInput(ctx context.Context) (string, error) {
	for c.Scanner.Scan() {
		text := c.Scanner.Text()
		if strings.TrimSpace(text) == "" {
			continue
		}
		return text, nil
	}

	if err := c.Scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

func (c *Cli) WriteOutput(ctx context.Context, text string, outputRole agent.OutputRole) error {
	switch outputRole {
	case agent.IsContent:
		_, err := fmt.Fprint(c.out, text)
		return err

	case agent.IsReasoningContent:
		const style = "\x1b[3;90m" // 斜体、灰色
		const reset = "\x1b[0m"    // 恢复默认

		_, err := fmt.Fprint(c.out, style, text, reset)
		return err
	case agent.IsToolContent:
		const style = "\x1b[36m" // 青色
		const reset = "\x1b[0m"  // 恢复默认

		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		if _, err := fmt.Fprint(c.out, style, "[工具输出] ", reset); err != nil {
			return err
		}
		_, err := fmt.Fprint(c.out, text)
		return err

	default:
		_, err := fmt.Fprint(c.out, text)
		return err
	}
}

func (c *Cli) ConfirmOperateByUser(ctx context.Context, dir, command string) (bool, error) {
	_, err := fmt.Fprintf(c.out, "\n目标目录: %s\n命令: %s\n是否执行?[y/n]", dir, command)
	if err != nil {
		return false, err
	}

	for ctx.Err() == nil { // 循环读取 y/n
		ans, err := c.ReadInput(ctx)
		if err != nil {
			return false, err
		}

		switch strings.ToLower(strings.TrimSpace(ans)) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			if _, err := fmt.Fprint(c.out, "请输入 y 或 n:"); err != nil {
				return false, err
			}
		}
	}
	return false, ctx.Err()
}

func (c *Cli) PrintHistory(ctx context.Context, msgs []apis.Message) {
	for _, msg := range msgs {
		splitMark := "\n"
		if msg.Role == "tool" {
			c.WriteOutput(ctx, msg.Content, agent.IsToolContent)
			continue
		}
		if strings.TrimSpace(msg.ReasoningContent) != "" {
			c.WriteOutput(ctx, splitMark+msg.ReasoningContent, agent.IsReasoningContent)
		}
		if strings.TrimSpace(msg.Content) != "" {
			c.WriteOutput(ctx, splitMark+msg.Content, agent.IsContent)
		}
		for _, call := range msg.ToolCalls {
			c.WriteOutput(ctx, splitMark+"[工具调用] "+call.Function.Name+" "+call.Function.Args+"\n", agent.IsContent)
		}

	}
}
