package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"go-agent-framework/internal/apis"
	"go-agent-framework/internal/config"
)

func main() {
	cfg, err := config.NewConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	msg1 := apis.Message{
		Role:    "user",
		Content: "你好，Deepseek！",
	}
	msg2 := apis.Message{
		Role:    "user",
		Content: "这里是来自go-agent-framework的问候！",
	}
	msgs := []apis.Message{msg1, msg2}
	opts := apis.StreamOptions{
		Thinking:        true,
		ReasoningEffort: "low",
	}
	for event, err := range apis.StreamChat(ctx, msgs, opts, cfg) {
		if err != nil {
			fmt.Fprintln(os.Stderr, "流式对话失败:", err)
			return
		}
		switch event.Kind {
		case apis.EventReasoning:
			fmt.Printf("[Thinking] %s \n", event.Text)
		case apis.EventContent:
			fmt.Printf("Deepseek: %s \n", event.Text)
		case apis.EventFinish:
			fmt.Printf("会话流结束，原因: %s", event.FinishReason)
		}
	}
}
