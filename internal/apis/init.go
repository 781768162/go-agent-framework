package apis

import (
	"go-agent-framework/internal/config"

	"net/http"
	"time"
)

func initStreamReqHeader(req *http.Request, apiKey string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+apiKey)
}

func initHttpClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			ResponseHeaderTimeout: 60 * time.Second,
		},
	}
}

func newChatRequest(msgs []Message, opts StreamOptions, cfg config.Config) ChatRequest {
	thinkingType := thinkingDisabled
	effort := ""
	if opts.Thinking {
		thinkingType = thinkingEnabled
		effort = opts.ReasoningEffort
	}

	return ChatRequest{ // 初始化reqBody
		Model:           cfg.DefaultModel,
		Messages:        msgs,
		Stream:          true,
		Thinking:        &enableThinking{Type: thinkingType},
		ReasoningEffort: effort,
		Tools:           opts.Tools,
	}

}
