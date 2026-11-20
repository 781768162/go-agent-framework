package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (c *Cli) SelectSession(ctx context.Context) (int64, error) {
	fmt.Println("输入 new 新建会话，或输入 sessionId 来加载会话")
	input, err := c.ReadInput(ctx)
	if err != nil {
		return -1, err
	}

	if strings.TrimSpace(input) == "new" {
		return 0, nil
	}

	sessionId, err := strconv.ParseInt(strings.TrimSpace(input), 10, 64)
	if err != nil || sessionId <= 0 {
		return -1, err
	}
	return sessionId, err
}
