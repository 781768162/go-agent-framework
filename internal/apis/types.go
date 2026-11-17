package apis

// 请求

type Message struct {
	Role    string `json:"role"` // "system" / "user" / "assistant"
	Content string `json:"content"`
}

type enableThinking struct {
	Type string `json:"type"` // "enabled" / "disabled"
}

const (
	thinkingEnabled  = "enabled"
	thinkingDisabled = "disabled"
)

type StreamOptions struct {
	Thinking        bool
	ReasoningEffort string // "low" / "high" / "max"
}

type EventKind int // 用于标识输出流类型

const (
	EventReasoning EventKind = iota
	EventContent
	EventFinish
)

func (k EventKind) String() string {
	switch k {
	case EventReasoning:
		return "reasoning"
	case EventContent:
		return "content"
	case EventFinish:
		return "finish"
	default:
		return "unknown"
	}
}

// 流式事件
type StreamEvent struct {
	Kind         EventKind
	Text         string
	FinishReason string
}

type ChatRequest struct {
	Model           string          `json:"model"`
	Messages        []Message       `json:"messages"`
	Stream          bool            `json:"stream"`
	Thinking        *enableThinking `json:"thinking,omitzero"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
}

// 响应

type delta struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

type ChatResponseChunk struct {
	Choices []struct {
		Delta        delta  `json:"delta"`
		FinishReason string `json:"finish_reason"` // ""表示流尚未结束
	} `json:"choices"`
}
