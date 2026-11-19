package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
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

func (c *Cli) WriteOutput(ctx context.Context, text string, isReasoning bool) error {
	if isReasoning {
		const style = "\x1b[3;90m" // 斜体、灰色
		const reset = "\x1b[0m"    // 恢复默认

		_, err := fmt.Fprint(c.out, style, text, reset)
		return err
	} else {
		_, err := fmt.Fprint(c.out, text)
		return err
	}
}
