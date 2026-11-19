package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"go-agent-framework/internal/agent"
	"go-agent-framework/internal/apis"
	"go-agent-framework/internal/config"
	"go-agent-framework/internal/interaction/cli"
	"go-agent-framework/internal/storage"
)

func main() {
	cfg, err := config.NewConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	opts := apis.StreamOptions{
		Thinking:        true,
		ReasoningEffort: "low",
		Tools: []apis.Tool{
			{
				Type: "function",
				Function: apis.FunctionDef{
					Name:        "bash",
					Description: "经用户确认后，在当前工作目录执行 Bash 命令并返回输出。",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"command": map[string]any{
								"type":        "string",
								"description": "要执行的完整 Bash 命令。",
							},
						},
						"required":             []string{"command"},
						"additionalProperties": false,
					},
				},
			},
		},
	}
	db, err := storage.Open(ctx, "agent.db") // 初始化数据库连接
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	mainLoop := agent.NewAgentLoop(cfg, db, opts)
	if err := mainLoop.NewSession(ctx, "test"); err != nil {
		log.Fatal(err)
	}
	terminal := cli.NewCli(os.Stdin, os.Stdout)
	for {
		err := mainLoop.RunTurn(ctx, terminal.ReadInput, terminal.WriteOutput, terminal.ConfirmOperateByUser)
		fmt.Println()
		if errors.Is(err, io.EOF) {
			fmt.Println("会话终止")
			break
		}
		if err != nil {
			log.Fatal(err)
		}
	}

}
