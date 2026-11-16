package apis

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ---------- 测试辅助 ----------

// sseBody 把 SSE 文本包成 handleStream 需要的 io.ReadCloser。
func sseBody(s string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(s))
}

// chunkLine 拼一行 SSE data，content 为增量内容。
func chunkLine(content string) string {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"delta": map[string]string{"content": content}},
		},
	})
	return "data: " + string(b) + "\n\n"
}

// drain 消费 handleStream，返回聚合内容与终止错误。
func drain(body io.ReadCloser) (string, error) {
	var sb strings.Builder
	err := handleStream(body, func(content string, err error) bool {
		if err != nil {
			// handleStream 的契约是不通过 yield 报错，这里出现即为违约
			panic(fmt.Sprintf("handleStream 不该用 yield 传错误: %v", err))
		}
		sb.WriteString(content)
		return true
	})
	return sb.String(), err
}

// ---------- 层 3：parseStreamChunk（纯函数）----------

func TestParseStreamChunk(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantContent string
		wantFinish  string
		wantChoices int
		wantErr     bool
	}{
		{
			name:        "正常增量块",
			raw:         `{"choices":[{"delta":{"content":"你好"},"index":0}]}`,
			wantContent: "你好",
			wantChoices: 1,
		},
		{
			name:        "首块只带 role，content 为空",
			raw:         `{"choices":[{"delta":{"role":"assistant","content":""},"index":0}]}`,
			wantContent: "",
			wantChoices: 1,
		},
		{
			name:        "末块带 finish_reason",
			raw:         `{"choices":[{"delta":{},"finish_reason":"stop","index":0}]}`,
			wantFinish:  "stop",
			wantChoices: 1,
		},
		{
			name:        "choices 为空数组",
			raw:         `{"choices":[]}`,
			wantChoices: 0,
		},
		{
			// DeepSeek 的真实响应带大量本结构体未声明的字段，必须被静默忽略
			name:        "含未声明字段（id/created/usage/logprobs）",
			raw:         `{"id":"x","created":1,"model":"deepseek-flash","usage":{"total_tokens":18},"choices":[{"delta":{"content":"a"},"logprobs":null,"index":0}],"system_fingerprint":"f"}`,
			wantContent: "a",
			wantChoices: 1,
		},
		{
			name:    "畸形 JSON 返回错误",
			raw:     `{"choices":`,
			wantErr: true,
		},
		{
			name:    "空字符串返回错误",
			raw:     ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunk, err := parseStreamChunk(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got := len(chunk.Choices); got != tt.wantChoices {
				t.Fatalf("len(Choices) = %d, want %d", got, tt.wantChoices)
			}
			if tt.wantChoices == 0 {
				return
			}
			if got := chunk.Choices[0].Delta.Content; got != tt.wantContent {
				t.Errorf("Content = %q, want %q", got, tt.wantContent)
			}
			if got := chunk.Choices[0].FinishReason; got != tt.wantFinish {
				t.Errorf("FinishReason = %q, want %q", got, tt.wantFinish)
			}
		})
	}
}

// ---------- 层 2：handleStream ----------

func TestHandleStream(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{
			name: "正常流：跳过 role 块与空 content 块，[DONE] 终止",
			body: `data: {"choices":[{"delta":{"role":"assistant","content":""}}]}` + "\n\n" +
				chunkLine("你好") +
				chunkLine("，") +
				chunkLine("世界") +
				`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n" +
				"data: [DONE]\n\n",
			want: "你好，世界",
		},
		{
			name: "无 [DONE] 直接 EOF 也算正常结束",
			body: chunkLine("abc"),
			want: "abc",
		},
		{
			name: "空输入",
			body: "",
			want: "",
		},
		{
			name: "跳过空行、心跳注释行与非 data 字段",
			body: "\n" +
				": keep-alive\n" +
				"\n" +
				"event: message\n" +
				"id: 42\n" +
				chunkLine("ok") +
				"\n" +
				"data: [DONE]\n\n",
			want: "ok",
		},
		{
			name: "CRLF 行尾",
			body: "data: {\"choices\":[{\"delta\":{\"content\":\"crlf\"}}]}\r\n" +
				"\r\n" +
				"data: [DONE]\r\n\r\n",
			want: "crlf",
		},
		{
			name: "data: 后无空格也能解析",
			body: "data:{\"choices\":[{\"delta\":{\"content\":\"nospace\"}}]}\n\n" +
				"data: [DONE]\n\n",
			want: "nospace",
		},
		{
			name: "choices 为空数组的块被跳过",
			body: "data: {\"choices\":[]}\n\n" +
				chunkLine("a") +
				"data: [DONE]\n\n",
			want: "a",
		},
		{
			name: "content 含转义与多字节字符",
			body: chunkLine("引号\"反斜杠\\换行\n emoji 👋") +
				"data: [DONE]\n\n",
			want: "引号\"反斜杠\\换行\n emoji 👋",
		},
		{
			name: "[DONE] 之后的数据不再被处理",
			body: chunkLine("before") +
				"data: [DONE]\n\n" +
				chunkLine("after"),
			want: "before",
		},
		{
			name:    "畸形 JSON 报错而非静默跳过",
			body:    "data: {\"choices\":\n\n",
			wantErr: true,
		},
		{
			name:    "[DONE] 不是合法 JSON，但必须先被哨兵拦截",
			body:    "data: [DONE]\n\n",
			want:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := drain(sseBody(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// 消费者 break 时 handleStream 必须返回 nil，且不再继续读流。
func TestHandleStreamBreakIsNotError(t *testing.T) {
	body := chunkLine("a") + chunkLine("b") + chunkLine("c")

	var n int
	err := handleStream(sseBody(body), func(string, error) bool {
		n++
		return n < 2 // 收到第 2 块就 break
	})
	if err != nil {
		t.Fatalf("break 不该产生错误: %v", err)
	}
	if n != 2 {
		t.Fatalf("yield 被调了 %d 次, want 2（第 3 块不该被读取）", n)
	}
}

// 边界值：行长恰好跨越 scanner 缓冲区上限。
func TestHandleStreamLineLengthBoundary(t *testing.T) {
	t.Run("100KB 行在上限内，应正常解析", func(t *testing.T) {
		big := strings.Repeat("a", 100*1024)
		got, err := drain(sseBody(chunkLine(big) + "data: [DONE]\n\n"))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got) != len(big) {
			t.Fatalf("got %d 字节, want %d", len(got), len(big))
		}
	})

	t.Run("2MB 行超过上限，应报 ErrTooLong 而非静默截断", func(t *testing.T) {
		big := strings.Repeat("a", 2*1024*1024)
		_, err := drain(sseBody(chunkLine(big) + "data: [DONE]\n\n"))
		if err == nil {
			t.Fatal("期望超长行报错，实际 err = nil")
		}
		if !strings.Contains(err.Error(), bufio.ErrTooLong.Error()) &&
			err != bufio.ErrTooLong {
			t.Fatalf("err = %v, 期望包含 %v", err, bufio.ErrTooLong)
		}
	})
}

// 流中途断开必须被报为错误，不能被当成"正常读完"。
func TestHandleStreamTruncatedStream(t *testing.T) {
	_, err := drain(&errReader{
		data: chunkLine("partial"),
		err:  fmt.Errorf("connection reset by peer"),
	})
	if err == nil {
		t.Fatal("底层读错误被吞掉了，scanner.Err() 没有向上冒")
	}
	if !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("err = %v, 期望保留底层错误信息", err)
	}
}

// errReader 先返回一段数据，之后的读取一律失败，用于模拟网络中断。
type errReader struct {
	data string
	err  error
	eof  bool
}

func (r *errReader) Read(p []byte) (int, error) {
	if !r.eof {
		r.eof = true
		return copy(p, r.data), nil
	}
	return 0, r.err
}

func (r *errReader) Close() error { return nil }

// ---------- 层 1：StreamChat ----------

// 端到端：请求头、请求体契约、流式响应聚合。
func TestStreamChatRequestContract(t *testing.T) {
	type captured struct {
		header http.Header
		req    ChatRequest
	}
	got := make(chan captured, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var cr ChatRequest
		if err := json.Unmarshal(b, &cr); err != nil {
			t.Errorf("请求体不是合法 JSON: %v (%q)", err, b)
		}
		got <- captured{header: r.Header.Clone(), req: cr}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"role":"assistant","content":""}}]}`+"\n\n")
		fmt.Fprint(w, chunkLine("pong"))
		fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	var sb strings.Builder
	for content, err := range StreamChat(context.Background(), srv.URL,
		[]Message{{Role: "user", Content: "ping"}}) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		sb.WriteString(content)
	}
	if sb.String() != "pong" {
		t.Fatalf("聚合结果 = %q, want %q", sb.String(), "pong")
	}

	c := <-got

	// 请求头
	if h := c.header.Get("Content-Type"); h != "application/json" {
		t.Errorf("Content-Type = %q", h)
	}
	if h := c.header.Get("Accept"); h != "text/event-stream" {
		t.Errorf("Accept = %q", h)
	}
	if want := "Bearer " + apiKey; c.header.Get("Authorization") != want {
		t.Errorf("Authorization = %q, want %q", c.header.Get("Authorization"), want)
	}

	// 请求体契约
	if c.req.Model != defaultModel {
		t.Errorf("Model = %q, want %q", c.req.Model, defaultModel)
	}
	if !c.req.Stream {
		t.Error("Stream = false, 流式请求必须为 true")
	}
	// V4.1 flash 思考模式默认开启，必须显式关闭，否则前半段全是 reasoning_content
	if c.req.Thinking == nil {
		t.Fatal("Thinking 字段缺失，思考模式不会被关闭")
	}
	if c.req.Thinking.Type != "disabled" {
		t.Errorf("Thinking.Type = %q, want disabled", c.req.Thinking.Type)
	}
	if len(c.req.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(c.req.Messages))
	}
	if c.req.Messages[0].Role != "user" || c.req.Messages[0].Content != "ping" {
		t.Errorf("Messages[0] = %+v", c.req.Messages[0])
	}
}

// 非 200 时错误必须带上状态码与响应体片段，否则 401 与 429 无法区分。
func TestStreamChatNon200(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"401 密钥错误", http.StatusUnauthorized, `{"error":{"message":"invalid api key"}}`},
		{"429 触发限流", http.StatusTooManyRequests, `{"error":{"message":"rate limited"}}`},
		{"500 服务端错误", http.StatusInternalServerError, `internal error`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer srv.Close()

			var yielded bool
			for content, err := range StreamChat(context.Background(), srv.URL,
				[]Message{{Role: "user", Content: "hi"}}) {
				yielded = true
				if err == nil {
					t.Fatalf("期望错误，却收到 content = %q", content)
				}
				if !strings.Contains(err.Error(), fmt.Sprint(tt.status)) {
					t.Errorf("错误 %q 不含状态码 %d", err, tt.status)
				}
				if !strings.Contains(err.Error(), tt.body) {
					t.Errorf("错误 %q 不含响应体片段 %q", err, tt.body)
				}
				if content != "" {
					t.Errorf("出错时 content 应为零值，实际 %q", content)
				}
			}
			if !yielded {
				t.Fatal("期望 yield 一次错误，实际迭代器没有产出任何值")
			}
		})
	}
}

// 惰性契约：只构造 seq 不发请求；range 时才发。
func TestStreamChatIsLazy(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	seq := StreamChat(context.Background(), srv.URL, []Message{{Role: "user", Content: "hi"}})
	time.Sleep(100 * time.Millisecond)
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("仅构造 seq 就发出了 %d 次请求，期望 0（应完全惰性）", n)
	}

	for _, err := range seq {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("range 后请求次数 = %d, want 1", n)
	}
}

// 初始化在闭包内，所以同一个 seq 可以重复消费，每次都是一轮完整请求。
func TestStreamChatSeqReusable(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, chunkLine("pong"))
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	seq := StreamChat(context.Background(), srv.URL, []Message{{Role: "user", Content: "ping"}})
	for i := 1; i <= 2; i++ {
		var sb strings.Builder
		for content, err := range seq {
			if err != nil {
				t.Fatalf("第 %d 次消费: %v", i, err)
			}
			sb.WriteString(content)
		}
		if sb.String() != "pong" {
			t.Fatalf("第 %d 次消费得到 %q, want pong", i, sb.String())
		}
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("服务器被调用 %d 次, want 2", n)
	}
}

// 消费者 break 后连接必须被关闭，否则泄漏。
func TestStreamChatBreakClosesConnection(t *testing.T) {
	disconnected := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter 不支持 http.Flusher")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for {
			select {
			case <-r.Context().Done(): // 客户端断开连接
				select {
				case disconnected <- struct{}{}:
				default:
				}
				return
			default:
			}
			fmt.Fprint(w, chunkLine("x"))
			flusher.Flush()
			time.Sleep(5 * time.Millisecond)
		}
	}))
	defer srv.Close()

	n := 0
	for content, err := range StreamChat(context.Background(), srv.URL,
		[]Message{{Role: "user", Content: "hi"}}) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if content != "x" {
			t.Fatalf("content = %q, want x", content)
		}
		n++
		if n == 3 {
			break
		}
	}
	if n != 3 {
		t.Fatalf("收到 %d 块, want 3", n)
	}

	select {
	case <-disconnected:
	case <-time.After(5 * time.Second):
		t.Fatal("break 后服务器未观察到连接关闭：defer resp.Body.Close() 可能没生效")
	}
}

// ctx 取消必须中断读流并通过 yield 报错，不能静默结束。
func TestStreamChatContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter 不支持 http.Flusher")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			fmt.Fprint(w, chunkLine("x"))
			flusher.Flush()
			time.Sleep(5 * time.Millisecond)
		}
	}))
	defer srv.Close()

	// 外层再套一个超时，保证 cancel 万一不生效时测试也不会挂死
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var gotErr error
	n := 0
	for _, err := range StreamChat(ctx, srv.URL, []Message{{Role: "user", Content: "hi"}}) {
		if err != nil {
			gotErr = err
			break
		}
		n++
		if n == 2 {
			cancel()
		}
	}

	if gotErr == nil {
		t.Fatal("ctx 取消后应通过 yield 收到错误，实际流被当成正常结束")
	}
	t.Logf("ctx 取消后收到错误（%d 块之后）: %v", n, gotErr)
}

// 端点不可达时的错误路径。
func TestStreamChatUnreachableEndpoint(t *testing.T) {
	var yielded bool
	for _, err := range StreamChat(context.Background(), "http://127.0.0.1:1/chat/completions",
		[]Message{{Role: "user", Content: "hi"}}) {
		yielded = true
		if err == nil {
			t.Fatal("期望错误")
		}
	}
	if !yielded {
		t.Fatal("期望 yield 一次错误")
	}
}

// 非法 URL 导致 NewRequestWithContext 失败，错误同样只能通过 yield 报出。
func TestStreamChatInvalidEndpoint(t *testing.T) {
	var gotErr error
	for _, err := range StreamChat(context.Background(), "://not-a-valid-url",
		[]Message{{Role: "user", Content: "hi"}}) {
		if err != nil {
			gotErr = err
			break
		}
	}
	if gotErr == nil {
		t.Fatal("非法 URL 应通过 yield 报出错误")
	}
	if !strings.Contains(gotErr.Error(), "://not-a-valid-url") {
		t.Logf("错误信息未包含原始 URL: %v", gotErr)
	}
}
