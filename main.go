package main

import (
	"bufio"
	"context"
	"encoding/json"
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
		Tools: []apis.Tool{
			apis.Tool{
				Type: "function",
				Function: apis.FunctionDef{
					Name:        "get_mock_result",
					Description: "获取模拟工具结果",
				},
			},
		},
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
		for {
			var output, reasoningOutput strings.Builder
			var calls []apis.ToolCall
			var finishReason string
			done := false                                                   // 标识本轮是否输出完毕，流结束不代表轮次结束
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
					finishReason = event.FinishReason
					if event.FinishReason == "tool_calls" {
						calls = event.ToolCalls
					}
					fmt.Printf("轮次结束原因: %s \n", event.FinishReason)
				case apis.EventDone:
					done = true
					fmt.Printf("会话轮次结束\n")
				}
			}

			if !done { // 结束模型输出循环，回到用户输入
				break
			}

			if finishReason == "tool_calls" {
				if err := apis.CheckToolCalls(calls); err != nil {
					fmt.Fprintln(os.Stderr, "工具调用校验失败", err)
					return
				}
			}

			if output.Len() > 0 || reasoningOutput.Len() > 0 || len(calls) > 0 {
				msg = apis.Message{
					Role:             "assistant",
					Content:          output.String(),
					ReasoningContent: reasoningOutput.String(),
					ToolCalls:        calls,
				}
				msgs = append(msgs, msg)

				callsString := "[]"
				if len(calls) > 0 {
					data, err := json.Marshal(calls)
					if err != nil {
						fmt.Fprintln(os.Stderr, "工具调用编码失败: ", err)
						return
					}
					callsString = string(data)
				}

				msgStorage = storage.Message{
					SessionId:        sessionId,
					Role:             "assistant",
					Content:          output.String(),
					ReasoningContent: reasoningOutput.String(),
					ToolCalls:        callsString,
				}
				if err := storage.CreateMessage(ctx, db, msgStorage); err != nil { // 储存模型输出
					fmt.Fprintln(os.Stderr, "储存出错: ", err)
					return
				}

				for _, call := range calls {
					toolStorage := storage.Message{
						SessionId:  sessionId,
						Role:       "tool",
						Content:    "模拟工具结果，尚未实现！",
						ToolCallId: call.Id,
					}

					if err := storage.CreateMessage(ctx, db, toolStorage); err != nil {
						fmt.Fprintln(os.Stderr, "储存出错: ", err)
						return
					}

					msgs = append(msgs, apis.Message{
						Role:       "tool",
						Content:    "模拟工具结果，尚未实现！",
						ToolCallId: call.Id,
					})

				}

				fmt.Printf("[think] %s [/think]\n", reasoningOutput.String())
				fmt.Printf("Deepseek: %s\n", output.String())
			}

			if len(calls) == 0 { // 结束模型输出循环，回到用户输入
				break
			}
		} // 用户输入循环结束
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(fmt.Errorf("输入流出错: %w", err))
	}

}
