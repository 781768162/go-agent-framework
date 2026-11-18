package storage

type Session struct {
	Title string
}

type Message struct {
	SessionId        int64
	Role             string
	Content          string
	ReasoningContent string
	ToolCalls        string
	ToolCallId       string
}

type ApiCall struct {
	SessionId             int
	Model                 string
	Thinking              int
	PromptTokens          int
	CompletionTokens      int
	PromptCacheHitTokens  int
	PromptCacheMissTokens int
	FinishReason          string
	DurationMs            int
}
