package apis

// 请求

type Message struct {
	Role    string `json:"role"` // "system" / "user" / "assistant"
	Content string `json:"content"`
}

type enableThinking struct {
	Type string `json:"type"` // "enabled" / "disabled"
}

type ChatRequest struct {
	Model    string          `json:"model"`
	Messages []Message       `json:"messages"`
	Stream   bool            `json:"stream"`
	Thinking *enableThinking `json:"thinking,omitzero"`
}

// 响应

type delta struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponseChunk struct {
	Choices []struct {
		Delta        delta  `json:"delta"`
		FinishReason string `json:"finish_reason"` // ""表示流尚未结束
	} `json:"choices"`
}
