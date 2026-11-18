package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"go-agent-framework/internal/apis"
	"go-agent-framework/internal/config"
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
	}
	msgs := []apis.Message{}
	scanner := bufio.NewScanner(os.Stdin)
	db, err := storage.Open(ctx, "agent.db") // 初始化数据库连接
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	sessionId, err := storage.CreateSession(ctx, db, "test") // 创建session记录
	if err != nil {
		log.Fatal(err)
	}

	// 对话轮次循环
	for scanner.Scan() { // 读取stdin流
		input := scanner.Text()
		if strings.TrimSpace(input) == "" {
			fmt.Println("输入为空")
			continue
		}
		msg := apis.Message{
			Role:    "user",
			Content: input,
		}
		msgs = append(msgs, msg)
		msgStorage := storage.Message{
			SessionId: sessionId,
			Role:      "user",
			Content:   input,
		}
		if err := storage.CreateMessage(ctx, db, msgStorage); err != nil { // 储存用户输入
			fmt.Fprintln(os.Stderr, "储存出错: ", err)
			break
		}

		var output, reasoningOutput strings.Builder
		var calls []apis.ToolCall
		done := false // 标识本轮是否输出完毕，流结束不代表轮次结束
		for event, err := range apis.StreamChat(ctx, msgs, opts, cfg) { // 解析sse流
			if err != nil {
				fmt.Fprintln(os.Stderr, "流式对话失败:", err)
				return
			}
			switch event.Kind {
			case apis.EventReasoning:
				//fmt.Printf("[Thinking] %s \n", event.Text)
				reasoningOutput.WriteString(event.Text)
			case apis.EventContent:
				//fmt.Printf("Deepseek: %s \n", event.Text)
				output.WriteString(event.Text)
			case apis.EventFinish:
				if event.FinishReason == "tool_calls" {
					calls = event.ToolCalls
				}
				fmt.Printf("轮次结束原因: %s \n", event.FinishReason)
			case apis.EventDone:
				done = true
				fmt.Printf("会话轮次结束\n")
			}
		}
		if done && len(calls) > 0 {
			if err := apis.CheckToolCalls(calls); err != nil {
				fmt.Fprintln(os.Stderr, "工具调用校验失败", err)
				continue
			}
		}

		if done && (output.Len() > 0 || reasoningOutput.Len() > 0) {
			msg = apis.Message{
				Role:             "assistant",
				Content:          output.String(),
				ReasoningContent: reasoningOutput.String(),
			}
			msgs = append(msgs, msg)
			msgStorage = storage.Message{
				SessionId:        sessionId,
				Role:             "assistant",
				Content:          output.String(),
				ReasoningContent: reasoningOutput.String(),
			}
			if err := storage.CreateMessage(ctx, db, msgStorage); err != nil { // 储存模型输出
				fmt.Fprintln(os.Stderr, "储存出错: ", err)
				break
			}
			fmt.Printf("[think] %s [/think]\n", reasoningOutput.String())
			fmt.Printf("Deepseek: %s\n", output.String())
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(fmt.Errorf("输入流出错: %w", err))
	}

}
