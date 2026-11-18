package apis

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestStreamChatToolCalls(t *testing.T) {
	// 编号二先到，编号零随后到达，参数分片交错返回
	first := `data: {"choices":[{"delta":{"tool_calls":[{"index":2,"id":"call_list","type":"function","function":{"name":"list_files","arguments":"{\"dir\":"}},{"index":0,"id":"call_read","type":"function","function":{"name":"read_test_file","arguments":"{\"path\":\""}}]}}]}` + "\n\n"
	second := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"测试 文件.txt\"}"}},{"index":2,"function":{"arguments":"\"docs\"}"}}]}}]}` + "\n\n"
	finish := sseLine("", "", "tool_calls")
	done := "data: [DONE]\n\n"

	wantFinish := StreamEvent{
		Kind:         EventFinish,
		FinishReason: "tool_calls",
		ToolCalls: []ToolCall{
			{Id: "call_read", Type: "function", Function: ToolCallFunction{Name: "read_test_file", Args: `{"path":"测试 文件.txt"}`}},
			{Id: "call_list", Type: "function", Function: ToolCallFunction{Name: "list_files", Args: `{"dir":"docs"}`}},
		},
	}

	tests := []struct {
		name string
		body string
		want []StreamEvent
	}{
		{"跳号调用按编号返回完整内容", first + second + finish + done, []StreamEvent{wantFinish, {Kind: EventDone}}},
		{"参数中途断流", first, nil},
		{"停止原因之后断流", first + second + finish, []StreamEvent{wantFinish}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()

			var got []StreamEvent
			for event, err := range StreamChat(t.Context(), []Message{{Role: "user", Content: "读取测试文件"}}, StreamOptions{}, testCfg(server.URL)) {
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, event)
			}

			if !slices.EqualFunc(got, tt.want, equalStreamEvent) {
				t.Fatalf("收到事件 %+v，预期 %+v", got, tt.want)
			}
		})
	}
}
