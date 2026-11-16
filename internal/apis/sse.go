package apis

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"
)

const (
	defaultModel = "deepseek-flash" // = DeepSeek-V4.1-Flash-1017
	dataPrefix   = "data:"
	doneSentinel = "[DONE]"
	apiKey       = "should-read-from-config"
)

var client *http.Client = initHttpClient()

// 流式对话入口，返回iter.Seq2回调函数用于range
func StreamChat(ctx context.Context, endpoint string, msgs []Message) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		reqBody, err := json.Marshal(ChatRequest{ // 初始化reqBody
			Model:    defaultModel,
			Messages: msgs,
			Stream:   true,
			Thinking: &enableThinking{Type: "disabled"},
		})
		if err != nil {
			yield("", err)
			return
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
		if err != nil {
			yield("", err)
			return
		}
		initStreamReqHeader(req, apiKey) // 初始化Header

		resp, err := client.Do(req) // 发送请求
		if err != nil {
			yield("", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			errMsg, _ := io.ReadAll(io.LimitReader(resp.Body, 512)) // 错误时读取API报错内容
			yield("", fmt.Errorf("HTTP %d 请求失败: %s", resp.StatusCode, bytes.TrimSpace(errMsg)))
			return
		}

		if err := handleStream(resp.Body, yield); err != nil {
			yield("", err)
		}
	}

}

// 流式解析SSE
func handleStream(body io.ReadCloser, yield func(string, error) bool) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), dataPrefix)
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)

		if data == doneSentinel { // 流结束
			return nil
		}

		chunk, err := parseStreamChunk(data)
		if err != nil {
			return fmt.Errorf("解析 chunk 失败: %w", err)
		}
		// 首块只带 role、末块只带 finish_reason，content 均为空
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
			continue
		}

		if !yield(chunk.Choices[0].Delta.Content, nil) {
			return nil
		}
	}
	return scanner.Err()
}

// 解析ChatResponseChunk
func parseStreamChunk(raw string) (ChatResponseChunk, error) {
	var chunk ChatResponseChunk

	err := json.Unmarshal([]byte(raw), &chunk)
	return chunk, err
}
